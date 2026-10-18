package listing

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/media"
	"github.com/FadhRach/sisaguna-be/internal/notification"
	"github.com/FadhRach/sisaguna-be/internal/platform/apperr"
	"github.com/FadhRach/sisaguna-be/internal/platform/database"
	"github.com/FadhRach/sisaguna-be/internal/platform/events"
	"github.com/FadhRach/sisaguna-be/internal/platform/idempotency"
	pmedia "github.com/FadhRach/sisaguna-be/internal/platform/media"
	"github.com/FadhRach/sisaguna-be/internal/rules"
)

var (
	ErrNotFound            = errors.New("listing tidak ditemukan")
	ErrMerchantRequired    = errors.New("akun ini belum punya toko")
	ErrMerchantNotApproved = errors.New("toko belum disetujui admin")
	ErrInvalidPrice        = errors.New("harga tidak valid: gratis tanpa harga, berbayar harus > 0, harga coret >= harga")
	ErrMadeAtRequired      = errors.New("made_at wajib diisi dan tidak boleh di masa depan untuk makanan siap santap")
	ErrChecklistIncomplete = errors.New("checklist keamanan pangan belum lengkap")
	ErrPastSafeWindow      = errors.New("makanan sudah melewati batas aman konsumsi")
	ErrInvalidMedia        = media.ErrInvalidMedia
)

const feedDetailMore = 6

type Service struct {
	repo     *Repository
	urls     pmedia.URLs
	now      func() time.Time
	partners bool // kartu menampilkan ongkir kurir tiruan (PARTNERS_MODE=dummy)
}

// WithPartners mengaktifkan ongkir kurir tiruan di kartu listing.
func (s *Service) WithPartners(on bool) *Service {
	s.partners = on
	return s
}

func NewService(repo *Repository, urls pmedia.URLs) *Service {
	return &Service{repo: repo, urls: urls, now: time.Now}
}

// Create membuat listing. created=false berarti Idempotency-Key sudah pernah dipakai dan listing lama dikembalikan.
func (s *Service) Create(ctx context.Context, owner uuid.UUID, key string, in CreateRequest) (id uuid.UUID, created bool, err error) {
	m, err := s.repo.q.GetMerchantByOwner(ctx, owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, ErrMerchantRequired
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("listing service create: %w", err)
	}
	if m.Status != sqlc.MerchantStatusApproved {
		return uuid.Nil, false, ErrMerchantNotApproved
	}
	hash := idempotency.Hash(in)
	if existing, err := s.repo.q.GetListingByIdempotency(ctx, sqlc.GetListingByIdempotencyParams{MerchantID: m.ID, IdempotencyKey: key}); err == nil {
		if err := idempotency.Check(existing.RequestHash, hash); err != nil {
			return uuid.Nil, false, err
		}
		return existing.ID, false, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, fmt.Errorf("listing service create: %w", err)
	}

	p, err := s.buildInsert(m.ID, key, in)
	if err != nil {
		return uuid.Nil, false, err
	}
	err = s.repo.InTx(ctx, func(r *Repository) error {
		id, err = r.q.InsertListing(ctx, p)
		if err != nil {
			return err
		}
		err = media.Attach(ctx, r.q, owner, sqlc.MediaKindImage, sqlc.MediaVisibilityPublic, in.PhotoMediaIDs, func(mid uuid.UUID, pos int16) error {
			return r.q.AttachListingMedia(ctx, sqlc.AttachListingMediaParams{ListingID: id, MediaID: mid, Position: pos})
		})
		if err != nil {
			return err
		}
		if in.VideoMediaID != nil {
			err = media.Attach(ctx, r.q, owner, sqlc.MediaKindVideo, sqlc.MediaVisibilityPublic, []uuid.UUID{*in.VideoMediaID}, func(uuid.UUID, int16) error { return nil })
			if err != nil {
				return err
			}
		}
		return s.notifySavers(ctx, r.q, m.ID, m.Name, in)
	})
	if database.IsUniqueViolation(err) { // dua request dengan key sama berbarengan
		if existing, e := s.repo.q.GetListingByIdempotency(ctx, sqlc.GetListingByIdempotencyParams{MerchantID: m.ID, IdempotencyKey: key}); e == nil {
			if e := idempotency.Check(existing.RequestHash, hash); e != nil {
				return uuid.Nil, false, e
			}
			return existing.ID, false, nil
		}
	}
	if err != nil {
		if isDomainErr(err) {
			return uuid.Nil, false, err
		}
		return uuid.Nil, false, fmt.Errorf("listing service create: %w", err)
	}
	return id, true, nil
}

// buildInsert menerapkan aturan kelayakan pangan dan harga (CLAUDE.md aturan 5) dan menghitung pickup_end.
func (s *Service) buildInsert(merchantID uuid.UUID, key string, in CreateRequest) (sqlc.InsertListingParams, error) {
	now := s.now()
	if err := validatePrice(in.IsFree, in.Price, in.PriceOriginal); err != nil {
		return sqlc.InsertListingParams{}, err
	}
	storage := in.Storage
	if storage == "" {
		storage = "room_temp"
	}
	p := sqlc.InsertListingParams{
		MerchantID: merchantID, Title: strings.TrimSpace(in.Title), Description: in.Description, Tier: sqlc.ListingTier(in.Tier),
		Unit: sqlc.QuantityUnitKg, IsFree: in.IsFree, Stock: int32(in.Stock), Allergens: toAllergens(in.Allergens),
		Storage: sqlc.StorageMethod(storage), SafetyChecks: []sqlc.SafetyCheck{}, PickupStart: tstz(now), IdempotencyKey: key, RequestHash: pgtype.Text{String: idempotency.Hash(in), Valid: true},
	}
	if !in.IsFree {
		p.Price = pgtype.Int4{Int32: int32(*in.Price), Valid: true}
	}
	if in.PriceOriginal != nil {
		p.PriceOriginal = pgtype.Int4{Int32: int32(*in.PriceOriginal), Valid: true}
	}
	if in.VideoMediaID != nil {
		p.VideoMediaID = uuid.NullUUID{UUID: *in.VideoMediaID, Valid: true}
	}
	if in.Halal != "" {
		p.Halal = sqlc.NullHalalStatus{HalalStatus: sqlc.HalalStatus(in.Halal), Valid: true}
	}
	end := now.Add(time.Duration(in.PickupHours) * time.Hour)

	if in.Tier == "human" {
		p.Unit = sqlc.QuantityUnitPortion
		if in.MadeAt == nil || in.MadeAt.After(now.Add(5*time.Minute)) {
			return p, ErrMadeAtRequired
		}
		if missing := rules.MissingChecks(in.SafetyChecks); len(missing) > 0 {
			return p, apperr.WithFields(ErrChecklistIncomplete, map[string]any{"missing": missing})
		}
		safeUntil, _ := rules.SafeUntil(*in.MadeAt, storage) // storage sudah divalidasi binding
		if !now.Before(safeUntil) {
			return p, apperr.WithFields(ErrPastSafeWindow, map[string]any{"suggested_tier": "animal_feed"})
		}
		if safeUntil.Before(end) {
			end = safeUntil
		}
		p.MadeAt = tstz(*in.MadeAt)
		p.SafeUntil = tstz(safeUntil)
		p.SafetyChecks = toChecks(in.SafetyChecks)
	}
	p.PickupEnd = tstz(end)
	return p, nil
}

func validatePrice(isFree bool, price, original *int) error {
	if isFree {
		if price != nil || (original != nil && *original <= 0) { // gratis boleh membawa harga normal (tampil sebagai harga coret)
			return ErrInvalidPrice
		}
		return nil
	}
	if price == nil || (original != nil && *original < *price) {
		return ErrInvalidPrice
	}
	return nil
}

// requireApproved: toko yang ditangguhkan/ditolak tidak boleh lagi mengubah atau menghapus listing.
func (s *Service) requireApproved(ctx context.Context, owner uuid.UUID) error {
	m, err := s.repo.q.GetMerchantByOwner(ctx, owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMerchantRequired
	}
	if err != nil {
		return err
	}
	if m.Status != sqlc.MerchantStatusApproved {
		return ErrMerchantNotApproved
	}
	return nil
}

func (s *Service) Update(ctx context.Context, owner, id uuid.UUID, in UpdateRequest) error {
	if err := s.requireApproved(ctx, owner); err != nil {
		if errors.Is(err, ErrMerchantRequired) || errors.Is(err, ErrMerchantNotApproved) {
			return err
		}
		return fmt.Errorf("listing service update: %w", err)
	}
	cur, err := s.repo.q.GetOwnedListing(ctx, sqlc.GetOwnedListingParams{ID: id, OwnerID: owner})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("listing service update: %w", err)
	}
	if cur.IsFree && (in.Price != nil || in.PriceOriginal != nil) {
		return ErrInvalidPrice
	}
	p := sqlc.UpdateListingParams{ID: id}
	if in.Stock != nil {
		p.Stock = pgtype.Int4{Int32: int32(*in.Stock), Valid: true}
	}
	if in.Price != nil || in.PriceOriginal != nil {
		price := cur.Price
		if in.Price != nil {
			price = pgtype.Int4{Int32: int32(*in.Price), Valid: true}
			p.Price = price
		}
		orig := cur.PriceOriginal
		if in.PriceOriginal != nil {
			p.SetOriginal = true
			orig = pgtype.Int4{}
			if *in.PriceOriginal > 0 {
				orig = pgtype.Int4{Int32: int32(*in.PriceOriginal), Valid: true}
				p.PriceOriginal = orig
			}
		}
		if orig.Valid && orig.Int32 < price.Int32 {
			return ErrInvalidPrice
		}
	}
	if in.PickupHours != nil {
		now := s.now()
		end := now.Add(time.Duration(*in.PickupHours) * time.Hour)
		if cur.SafeUntil.Valid {
			if !now.Before(cur.SafeUntil.Time) {
				return apperr.WithFields(ErrPastSafeWindow, map[string]any{"suggested_tier": "animal_feed"})
			}
			if cur.SafeUntil.Time.Before(end) {
				end = cur.SafeUntil.Time
			}
		}
		p.PickupEnd = tstz(end)
	}
	if in.Status != nil {
		p.Status = sqlc.NullListingStatus{ListingStatus: sqlc.ListingStatus(*in.Status), Valid: true}
	}
	err = s.repo.InTx(ctx, func(r *Repository) error {
		if err := r.q.UpdateListing(ctx, p); err != nil {
			return err
		}
		if in.Stock != nil {
			return events.Emit(ctx, r.q, "listing.stock_changed", map[string]any{"listing_id": id, "stock": *in.Stock}, nil, "listing:"+id.String())
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("listing service update: %w", err)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, owner, id uuid.UUID) error {
	if err := s.requireApproved(ctx, owner); err != nil {
		if errors.Is(err, ErrMerchantRequired) || errors.Is(err, ErrMerchantNotApproved) {
			return err
		}
		return fmt.Errorf("listing service delete: %w", err)
	}
	if _, err := s.repo.q.GetOwnedListing(ctx, sqlc.GetOwnedListingParams{ID: id, OwnerID: owner}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("listing service delete: %w", err)
	}
	if _, err := s.repo.q.SoftDeleteListing(ctx, id); err != nil {
		return fmt.Errorf("listing service delete: %w", err)
	}
	return nil
}

// Mine adalah Katalog Saya: semua listing toko kecuali yang dihapus, dengan status turunan.
func (s *Service) Mine(ctx context.Context, owner uuid.UUID, page, limit int) ([]MineDTO, int64, error) {
	m, err := s.repo.q.GetMerchantByOwner(ctx, owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, ErrMerchantRequired
	}
	if err != nil {
		return nil, 0, fmt.Errorf("listing service mine: %w", err)
	}
	rows, err := s.repo.q.ListMerchantListings(ctx, sqlc.ListMerchantListingsParams{MerchantID: m.ID, Lim: int32(limit), Off: int32((page - 1) * limit)})
	if err != nil {
		return nil, 0, fmt.Errorf("listing service mine: %w", err)
	}
	now := s.now()
	var total int64
	out := make([]MineDTO, 0, len(rows))
	for _, r := range rows {
		total = r.Total
		d := MineDTO{
			ID: r.ID.String(), Title: r.Title, Tier: string(r.Tier), Unit: string(r.Unit), IsFree: r.IsFree,
			Price: intPtr(r.Price), PriceOriginal: intPtr(r.PriceOriginal), Stock: int(r.Stock), SoldCount: int(r.SoldCount),
			Status:      derivedStatus(r.Status, int(r.Stock), r.PickupEnd.Time, now),
			PickupStart: r.PickupStart.Time.UTC(), PickupEnd: r.PickupEnd.Time.UTC(),
			CoverURL: s.urls.PublicPtr(strPtr(r.CoverPath)), ThumbURL: s.urls.PublicPtr(strPtr(r.ThumbPath)),
		}
		if r.SafeUntil.Valid {
			t := r.SafeUntil.Time.UTC()
			d.SafeUntil = &t
		}
		out = append(out, d)
	}
	return out, total, nil
}

// derivedStatus: sold_out dan expired tidak disimpan, diturunkan saat membaca (CLAUDE.md aturan 4).
func derivedStatus(stored sqlc.ListingStatus, stock int, pickupEnd, now time.Time) string {
	switch {
	case stored == sqlc.ListingStatusPaused:
		return "paused"
	case stock == 0:
		return "sold_out"
	case pickupEnd.Before(now):
		return "expired"
	}
	return "active"
}

// Get adalah detail listing publik. Listing yang sudah habis/kedaluwarsa tetap bisa dibuka (status turunan).
func (s *Service) Get(ctx context.Context, id uuid.UUID, q PointQuery) (*DetailDTO, error) {
	l, err := s.repo.q.GetListingDetail(ctx, sqlc.GetListingDetailParams{ID: id, Lat: floatArg(q.Lat), Lng: floatArg(q.Lng)})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("listing service get: %w", err)
	}
	photos, err := s.repo.q.ListingPhotos(ctx, []uuid.UUID{id})
	if err != nil {
		return nil, fmt.Errorf("listing service get: %w", err)
	}
	more, _, err := s.feed(ctx, feedFilter{merchantID: l.MerchantID, excludeID: id, lat: q.Lat, lng: q.Lng, sort: "popular", limit: feedDetailMore})
	if err != nil {
		return nil, fmt.Errorf("listing service get: %w", err)
	}

	now := s.now()
	var madeAt *time.Time
	if l.MadeAt.Valid {
		t := l.MadeAt.Time
		madeAt = &t
	}
	d := &DetailDTO{
		ID: l.ID.String(), Title: l.Title, Description: l.Description, Tier: string(l.Tier), Unit: string(l.Unit),
		Status: derivedStatus(l.Status, int(l.Stock), l.PickupEnd.Time, now), IsFree: l.IsFree, Price: intPtr(l.Price), PriceOriginal: intPtr(l.PriceOriginal),
		Stock: int(l.Stock), SoldCount: int(l.SoldCount), Halal: string(l.Halal), Allergens: fromAllergens(l.Allergens),
		Storage: string(l.Storage), SafetyChecks: fromChecks(l.SafetyChecks), PickupStart: l.PickupStart.Time.UTC(), PickupEnd: l.PickupEnd.Time.UTC(),
		DistanceKm: km(l.DistanceM), Photos: []PhotoDTO{}, Safety: rules.Assess(string(l.Tier), madeAt, string(l.Storage), fromChecks(l.SafetyChecks), now), MoreFromMerchant: more,
		Merchant: MerchantDetail{
			MerchantRef: MerchantRef{ID: l.MerchantID.String(), Name: l.MerchantName, LocationLabel: l.LocationLabel,
				RatingAvg: ratingPtr(l.RatingAvg), RatingCount: int(l.RatingCount), IsVerified: l.IsVerified},
			About: l.MerchantAbout, Address: l.MerchantAddress, Lat: l.MerchantLat, Lng: l.MerchantLng, OpenHours: l.OpenHours,
			Halal: string(l.MerchantHalal), Allergens: fromAllergens(l.MerchantAllergens), DeliveryAvailable: l.DeliveryAvailable, PrepMinutes: int(l.PrepMinutes),
		},
	}
	if l.MadeAt.Valid {
		t := l.MadeAt.Time.UTC()
		d.MadeAt = &t
	}
	for _, ph := range photos {
		d.Photos = append(d.Photos, PhotoDTO{URL: s.urls.Public(ph.Path), ThumbURL: s.urls.PublicPtr(textPtr(ph.ThumbPath))})
	}
	d.VideoURL = s.urls.PublicPtr(textPtr(l.VideoPath))
	return d, nil
}

// List adalah GET /listings.
func (s *Service) List(ctx context.Context, q ListQuery) ([]CardDTO, int64, error) {
	page, limit := q.Normalize()
	f := feedFilter{lat: q.Lat, lng: q.Lng, sort: q.Sort, q: q.Q, onlyFree: q.Free, maxKm: q.MaxKm, limit: limit, offset: (page - 1) * limit}
	if q.Tier != "" {
		f.tiers = []sqlc.ListingTier{sqlc.ListingTier(q.Tier)}
	}
	return s.feed(ctx, f)
}

// OfMerchant: listing yang masih tampil di feed untuk halaman toko.
func (s *Service) OfMerchant(ctx context.Context, merchantID uuid.UUID, lat, lng *float64) ([]CardDTO, error) {
	cards, _, err := s.feed(ctx, feedFilter{merchantID: merchantID, lat: lat, lng: lng, sort: "relevant", limit: 50})
	return cards, err
}

var (
	humanTiers = []sqlc.ListingTier{sqlc.ListingTierHuman}
	farmTiers  = []sqlc.ListingTier{sqlc.ListingTierAnimalFeed, sqlc.ListingTierCompost}
)

// Home menyusun rail beranda. Tab human: nearby/deals/popular untuk makanan; tab farm: untuk pakan & kompos
// plus rail animal_feed dan compost.
// Home mengembalikan semua rail sekaligus, aturan sama dengan FakeListingRepository.getHomeFeed + HomeViewModel di app Android:
// nearby (terdekat), deals (rasio hemat terbesar), popular (terlaris), animal_feed, compost; maks 12 per rail. Filter q/free/max_km
// berlaku untuk semua rail, dan sort (kalau bukan relevant) mengurutkan ulang semua rail kecuali popular.
func (s *Service) Home(ctx context.Context, q HomeQuery, viewer uuid.UUID) (*HomeDTO, error) {
	base := feedFilter{lat: q.Lat, lng: q.Lng, q: q.Q, onlyFree: q.Free, maxKm: q.MaxKm, limit: maxSortRows}
	humanF, farmF := base, base
	humanF.tiers, farmF.tiers = humanTiers, farmTiers
	human, _, err := s.query(ctx, humanF)
	if err != nil {
		return nil, fmt.Errorf("listing service home: %w", err)
	}
	farm, _, err := s.query(ctx, farmF)
	if err != nil {
		return nil, fmt.Errorf("listing service home: %w", err)
	}

	rail := func(src []CardDTO, native string, user bool) []CardDTO {
		cards := slices.Clone(src)
		sortCards(cards, native)
		if user {
			sortCards(cards, q.Sort)
		}
		return cards[:min(len(cards), homeRailSize)]
	}
	h := &HomeDTO{
		Nearby: rail(human, "nearest", true), Deals: rail(human, "biggest_discount", true), Popular: rail(human, "popular", false),
		AnimalFeed: rail(onlyTier(farm, "animal_feed"), "relevant", true), Compost: rail(onlyTier(farm, "compost"), "relevant", true),
	}
	h.HumanCount = distinct(h.Nearby, h.Deals)
	h.FarmCount = distinct(h.AnimalFeed, h.Compost)

	filtering := q.Q != "" || q.Free || q.MaxKm != nil || (q.Sort != "" && q.Sort != "relevant")
	active, other := h.HumanCount, h.FarmCount
	if q.Tab == "farm" {
		active, other = other, active
	}
	if filtering && active == 0 {
		h.OtherTabMatchCount = other
	}
	if h.OfferMerchants, err = s.offerMerchants(ctx, q, viewer); err != nil {
		return nil, fmt.Errorf("listing service home: %w", err)
	}
	return h, nil
}

const homeRailSize = 12

func onlyTier(cards []CardDTO, tier string) []CardDTO {
	out := make([]CardDTO, 0, len(cards))
	for _, c := range cards {
		if c.Tier == tier {
			out = append(out, c)
		}
	}
	return out
}

// distinct menghitung listing unik di gabungan beberapa rail.
func distinct(rails ...[]CardDTO) int {
	seen := map[string]bool{}
	for _, r := range rails {
		for _, c := range r {
			seen[c.ID] = true
		}
	}
	return len(seen)
}

func (s *Service) offerMerchants(ctx context.Context, q HomeQuery, viewer uuid.UUID) ([]OfferMerchant, error) {
	p := sqlc.ListOfferMerchantsParams{Lat: floatArg(q.Lat), Lng: floatArg(q.Lng), MaxKm: floatArg(q.MaxKm), Lim: 10}
	if viewer != uuid.Nil { // toko sendiri tidak ikut direkomendasikan
		p.ExcludeOwner = uuid.NullUUID{UUID: viewer, Valid: true}
	}
	rows, err := s.repo.q.ListOfferMerchants(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]OfferMerchant, 0, len(rows))
	for _, r := range rows {
		out = append(out, OfferMerchant{
			MerchantRef: MerchantRef{ID: r.ID.String(), Name: r.Name, LocationLabel: r.LocationLabel, RatingAvg: ratingPtr(r.RatingAvg), RatingCount: int(r.RatingCount), IsVerified: r.IsVerified},
			TodaysOffer: r.TodaysOffer.String, DistanceKm: km(r.DistanceM),
		})
	}
	return out, nil
}

type feedFilter struct {
	tiers           []sqlc.ListingTier
	q               string
	onlyFree        bool
	merchantID      uuid.UUID
	excludeID       uuid.UUID
	lat, lng, maxKm *float64
	sort            string
	limit, offset   int
}

// feed memberi urutan sesuai f.sort. "relevant" (urutan dasar) dipaging di SQL; sort lain mengurutkan hingga maxSortRows
// baris di memori lalu memotong halaman.
func (s *Service) feed(ctx context.Context, f feedFilter) ([]CardDTO, int64, error) {
	if f.sort == "" || f.sort == "relevant" {
		return s.query(ctx, f)
	}
	limit, offset := f.limit, f.offset
	f.limit, f.offset = maxSortRows, 0
	cards, _, err := s.query(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	sortCards(cards, f.sort)
	total := int64(len(cards))
	return cards[min(offset, len(cards)):min(offset+limit, len(cards))], total, nil
}

func (s *Service) query(ctx context.Context, f feedFilter) ([]CardDTO, int64, error) {
	p := sqlc.ListFeedParams{Lim: int32(f.limit), Off: int32(f.offset), Lat: floatArg(f.lat), Lng: floatArg(f.lng), MaxKm: floatArg(f.maxKm), Tiers: f.tiers}
	if f.q != "" {
		p.Q = pgtype.Text{String: escapeLike(strings.TrimSpace(f.q)), Valid: true}
	}
	if f.onlyFree {
		p.OnlyFree = pgtype.Bool{Bool: true, Valid: true}
	}
	if f.merchantID != uuid.Nil {
		p.MerchantID = uuid.NullUUID{UUID: f.merchantID, Valid: true}
	}
	if f.excludeID != uuid.Nil {
		p.ExcludeID = uuid.NullUUID{UUID: f.excludeID, Valid: true}
	}
	rows, err := s.repo.q.ListFeed(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	out := make([]CardDTO, 0, len(rows))
	for _, r := range rows {
		total = r.Total
		c := CardDTO{
			ID: r.ID.String(), Title: r.Title, Tier: string(r.Tier), Unit: string(r.Unit), IsFree: r.IsFree,
			Price: intPtr(r.Price), PriceOriginal: intPtr(r.PriceOriginal), DiscountPct: int(r.DiscountPct), Stock: int(r.Stock),
			Halal: string(r.Halal), Allergens: fromAllergens(r.Allergens), PickupStart: r.PickupStart.Time.UTC(), PickupEnd: r.PickupEnd.Time.UTC(),
			DistanceKm: km(r.DistanceM), CoverURL: s.urls.PublicPtr(strPtr(r.CoverPath)), ThumbURL: s.urls.PublicPtr(strPtr(r.ThumbPath)),
			Merchant: MerchantRef{ID: r.MerchantID.String(), Name: r.MerchantName, LocationLabel: r.LocationLabel,
				RatingAvg: ratingPtr(r.RatingAvg), RatingCount: int(r.RatingCount), IsVerified: r.IsVerified},
			SoldCount: int(r.SoldCount), distKm: -1, prep: int(r.PrepMinutes),
		}
		if r.DistanceM >= 0 {
			c.distKm = r.DistanceM / 1000
		}
		if s.partners && r.DeliveryAvailable {
			q := c.gosend()
			c.Delivery = &CardDelivery{Fee: q.Payable, ETAMin: q.ETAMin, ETAMax: q.ETAMax}
		}
		out = append(out, c)
	}
	return out, total, nil
}

// escapeLike mencegah % dan _ dari input pengguna dianggap wildcard.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func isDomainErr(err error) bool {
	for _, e := range []error{ErrInvalidMedia, ErrInvalidPrice, ErrMadeAtRequired, ErrChecklistIncomplete, ErrPastSafeWindow, idempotency.ErrKeyReused} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// notifySavers memberi tahu pengguna yang menyimpan toko bahwa ada menu baru (teks sama dengan notifikasi di app Android).
// Preferensi pengguna dihormati oleh notification.Create.
func (s *Service) notifySavers(ctx context.Context, q *sqlc.Queries, merchantID uuid.UUID, merchantName string, in CreateRequest) error {
	users, err := q.ListMerchantSavers(ctx, merchantID)
	if err != nil {
		return err
	}
	body := strings.TrimSpace(in.Title) + ", gratis."
	if !in.IsFree && in.Price != nil {
		body = fmt.Sprintf("%s, %s", strings.TrimSpace(in.Title), rupiah(*in.Price))
		if in.PriceOriginal != nil {
			body += " dari " + rupiah(*in.PriceOriginal)
		}
		body += "."
	}
	for _, u := range users {
		err := notification.Create(ctx, q, notification.New{UserID: u, Type: sqlc.NotificationTypeMerchant, Title: merchantName + " upload menu baru",
			Body: body + " Penyedia yang kamu simpan baru saja menambah makanan.", Data: map[string]any{"merchant_id": merchantID}})
		if err != nil {
			return err
		}
	}
	return nil
}

// rupiah memformat "Rp 4.000" (titik pemisah ribuan, seperti formatRupiah di app).
func rupiah(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "." + s[i:]
	}
	return "Rp " + s
}
