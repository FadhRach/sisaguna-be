package merchant

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/listing"
	"github.com/FadhRach/sisaguna-be/internal/media"
	"github.com/FadhRach/sisaguna-be/internal/platform/database"
	pmedia "github.com/FadhRach/sisaguna-be/internal/platform/media"
)

var (
	ErrExists       = errors.New("akun ini sudah punya toko")
	ErrNotFound     = errors.New("toko tidak ditemukan")
	ErrPartialPoint = errors.New("lat dan lng harus dikirim berpasangan")
)

type Service struct {
	repo     *Repository
	listings *listing.Service
	urls     pmedia.URLs
}

func NewService(repo *Repository, listings *listing.Service, urls pmedia.URLs) *Service {
	return &Service{repo: repo, listings: listings, urls: urls}
}

// Create membuat toko berstatus pending; toko baru bisa menyusun profil tapi belum bisa posting.
func (s *Service) Create(ctx context.Context, owner uuid.UUID, in CreateRequest) (*MerchantDTO, error) {
	halal := in.Halal
	if halal == "" {
		halal = "unverified"
	}
	prep := int16(15)
	if in.PrepMinutes != nil {
		prep = int16(*in.PrepMinutes)
	}
	_, err := s.repo.Create(ctx, sqlc.CreateMerchantParams{
		OwnerID: owner, Name: strings.TrimSpace(in.Name), About: in.About, Address: strings.TrimSpace(in.Address),
		Lat: *in.Lat, Lng: *in.Lng, LocationLabel: in.LocationLabel, OpenHours: in.OpenHours,
		Halal: sqlc.HalalStatus(halal), Allergens: listing.ToAllergens(in.Allergens), PrepMinutes: prep,
	})
	if err != nil {
		if database.IsUniqueViolation(err) {
			return nil, ErrExists
		}
		return nil, fmt.Errorf("merchant service create: %w", err)
	}
	return s.Mine(ctx, owner)
}

func (s *Service) Mine(ctx context.Context, owner uuid.UUID) (*MerchantDTO, error) {
	m, err := s.repo.ByOwner(ctx, owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("merchant service mine: %w", err)
	}
	dto := &MerchantDTO{
		ID: m.ID.String(), Name: m.Name, Status: string(m.Status), IsVerified: m.IsVerified, About: m.About, Address: m.Address,
		Lat: m.Lat, Lng: m.Lng, LocationLabel: m.LocationLabel, OpenHours: m.OpenHours, Halal: string(m.Halal),
		Allergens: listing.AllergenStrings(m.Allergens), TodaysOffer: textPtr(m.TodaysOffer), DeliveryAvailable: m.DeliveryAvailable,
		PrepMinutes: int(m.PrepMinutes), RatingAvg: ratingPtr(m.RatingAvg), RatingCount: int(m.RatingCount), TotalRescued: int(m.TotalRescued),
	}
	if err := s.addMedia(ctx, dto, m.ID, m.BannerPath); err != nil {
		return nil, fmt.Errorf("merchant service mine: %w", err)
	}
	return dto, nil
}

func (s *Service) Update(ctx context.Context, owner uuid.UUID, in UpdateRequest) (*MerchantDTO, error) {
	if (in.Lat == nil) != (in.Lng == nil) {
		return nil, ErrPartialPoint
	}
	cur, err := s.repo.ByOwner(ctx, owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("merchant service update: %w", err)
	}
	p := sqlc.UpdateMerchantParams{
		ID: cur.ID, Name: text(in.Name, true), About: text(in.About, false), Address: text(in.Address, true),
		LocationLabel: text(in.LocationLabel, false), OpenHours: text(in.OpenHours, false),
		Lat: float(in.Lat), Lng: float(in.Lng),
	}
	if in.Halal != nil {
		p.Halal = sqlc.NullHalalStatus{HalalStatus: sqlc.HalalStatus(*in.Halal), Valid: true}
	}
	if in.Allergens != nil {
		p.Allergens = listing.ToAllergens(in.Allergens)
	}
	if in.TodaysOffer != nil {
		p.SetOffer = true
		if t := strings.TrimSpace(*in.TodaysOffer); t != "" {
			p.TodaysOffer = pgtype.Text{String: t, Valid: true}
		}
	}
	if in.PrepMinutes != nil {
		p.PrepMinutes = pgtype.Int2{Int16: int16(*in.PrepMinutes), Valid: true}
	}
	if in.BannerMediaID != nil {
		p.SetBanner = true
		p.BannerMediaID = uuid.NullUUID{UUID: *in.BannerMediaID, Valid: true}
	}
	err = s.repo.InTx(ctx, func(r *Repository) error {
		if in.BannerMediaID != nil {
			if err := r.replaceBanner(ctx, owner, cur.ID, *in.BannerMediaID); err != nil {
				return err
			}
		}
		if in.PhotoMediaIDs != nil {
			if err := r.replacePhotos(ctx, owner, cur.ID, in.PhotoMediaIDs); err != nil {
				return err
			}
		}
		if err := r.Update(ctx, p); err != nil {
			return err
		}
		if in.Lat != nil {
			return r.SyncListingsLocation(ctx, cur.ID)
		}
		return nil
	})
	if errors.Is(err, media.ErrInvalidMedia) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("merchant service update: %w", err)
	}
	return s.Mine(ctx, owner)
}

// Public menampilkan toko approved beserta listing yang masih tampil di feed. viewer = uuid.Nil untuk anonim;
// pengguna login juga mendapat is_saved.
func (s *Service) Public(ctx context.Context, id, viewer uuid.UUID, q PublicQuery) (*PublicDTO, error) {
	m, err := s.repo.Approved(ctx, sqlc.GetApprovedMerchantParams{ID: id, UserLat: float(q.Lat), UserLng: float(q.Lng)})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("merchant service public: %w", err)
	}
	cards, err := s.listings.OfMerchant(ctx, id, q.Lat, q.Lng)
	if err != nil {
		return nil, fmt.Errorf("merchant service public: %w", err)
	}
	dto := MerchantDTO{
		ID: m.ID.String(), Name: m.Name, Status: string(m.Status), IsVerified: m.IsVerified, About: m.About, Address: m.Address,
		Lat: m.Lat, Lng: m.Lng, LocationLabel: m.LocationLabel, OpenHours: m.OpenHours, Halal: string(m.Halal),
		Allergens: listing.AllergenStrings(m.Allergens), TodaysOffer: textPtr(m.TodaysOffer), DeliveryAvailable: m.DeliveryAvailable,
		PrepMinutes: int(m.PrepMinutes), RatingAvg: ratingPtr(m.RatingAvg), RatingCount: int(m.RatingCount), TotalRescued: int(m.TotalRescued),
	}
	if m.DistanceM >= 0 { // -1 = jarak tidak dihitung
		km := math.Round(m.DistanceM/100) / 10
		dto.DistanceKm = &km
	}
	if err := s.addMedia(ctx, &dto, id, m.BannerPath); err != nil {
		return nil, fmt.Errorf("merchant service public: %w", err)
	}
	breakdown, err := s.breakdown(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("merchant service public: %w", err)
	}
	out := &PublicDTO{MerchantDTO: dto, RatingBreakdown: breakdown, Listings: cards}
	if viewer != uuid.Nil {
		saved, err := s.repo.IsSaved(ctx, viewer, id)
		if err != nil {
			return nil, fmt.Errorf("merchant service public: %w", err)
		}
		out.IsSaved = &saved
	}
	return out, nil
}

func text(s *string, trim bool) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	v := *s
	if trim {
		v = strings.TrimSpace(v)
	}
	return pgtype.Text{String: v, Valid: true}
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func float(f *float64) pgtype.Float8 {
	if f == nil {
		return pgtype.Float8{}
	}
	return pgtype.Float8{Float64: *f, Valid: true}
}

func ratingPtr(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return nil
	}
	return &f.Float64
}

// addMedia mengisi banner_url dan galeri foto toko.
func (s *Service) addMedia(ctx context.Context, dto *MerchantDTO, id uuid.UUID, banner pgtype.Text) error {
	dto.BannerURL = s.urls.PublicPtr(textPtr(banner))
	rows, err := s.repo.q.ListMerchantPhotos(ctx, []uuid.UUID{id})
	if err != nil {
		return err
	}
	dto.Photos = make([]listing.PhotoDTO, 0, len(rows))
	for _, r := range rows {
		dto.Photos = append(dto.Photos, listing.PhotoDTO{URL: s.urls.Public(r.Path), ThumbURL: s.urls.PublicPtr(textPtr(r.ThumbPath))})
	}
	return nil
}

func (s *Service) breakdown(ctx context.Context, id uuid.UUID) (map[string]int, error) {
	rows, err := s.repo.q.MerchantRatingBreakdown(ctx, id)
	if err != nil {
		return nil, err
	}
	out := map[string]int{"1": 0, "2": 0, "3": 0, "4": 0, "5": 0}
	for _, r := range rows {
		out[strconv.Itoa(int(r.Stars))] = int(r.N)
	}
	return out, nil
}

// List: toko approved untuk rekomendasi penyedia (viewer sendiri tidak ikut), terdekat dulu.
func (s *Service) List(ctx context.Context, viewer uuid.UUID, q ListQuery) ([]CardDTO, error) {
	limit := q.Limit
	if limit == 0 {
		limit = 50
	}
	p := sqlc.ListApprovedMerchantsParams{Lat: float(q.Lat), Lng: float(q.Lng), AvailableOnly: q.Available, Lim: int32(limit)}
	if viewer != uuid.Nil {
		p.Viewer = uuid.NullUUID{UUID: viewer, Valid: true}
	}
	rows, err := s.repo.q.ListApprovedMerchants(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("merchant service list: %w", err)
	}
	out := make([]CardDTO, 0, len(rows))
	for _, r := range rows {
		c := CardDTO{ID: r.ID.String(), Name: r.Name, LocationLabel: r.LocationLabel, TodaysOffer: textPtr(r.TodaysOffer), RatingAvg: ratingPtr(r.RatingAvg),
			RatingCount: int(r.RatingCount), IsVerified: r.IsVerified, BannerURL: s.urls.PublicPtr(textPtr(r.BannerPath)),
			AvailableCount: int(r.AvailableCount), IsSaved: r.IsSaved}
		if r.NextPickupEnd.Valid {
			t := r.NextPickupEnd.Time.UTC()
			c.NextPickupEnd = &t
		}
		if r.DistanceM >= 0 {
			km := math.Round(r.DistanceM/100) / 10
			c.DistanceKm = &km
		}
		out = append(out, c)
	}
	return out, nil
}
