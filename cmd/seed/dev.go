package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/FadhRach/sisaguna-be/internal/config"
	"github.com/FadhRach/sisaguna-be/internal/platform/auth"
	pmedia "github.com/FadhRach/sisaguna-be/internal/platform/media"
	"github.com/FadhRach/sisaguna-be/internal/rules"
)

const devPassword = "Password123!"

var seedNS = uuid.MustParse("5a5a5a5a-1111-4222-8333-444455556666")

// sid membuat UUID deterministik dari nama, supaya seed idempoten.
func sid(name string) uuid.UUID { return uuid.NewSHA1(seedNS, []byte(name)) }

// merchantCoords: koordinat toko per daerah (nyata, bukan jarak tetap dari dummy Android). Rumah Budi = alamat a1 di Tangerang.
// Pengiriman dibatasi 15 km, jadi dari "Rumah" hanya toko Tangerang/Kebon Jeruk yang bisa dikirim; coba alamat "Kantor" untuk sisanya.
var merchantCoords = map[string][2]float64{
	"m0": {-6.2275, 106.6544}, "m1": {-6.2615, 106.8140}, "m2": {-6.2441, 106.7994}, "m3": {-6.2780, 106.7960},
	"m4": {-6.4000, 106.7700}, "m5": {-6.2950, 106.8000}, "m6": {-6.2250, 106.6500}, "m7": {-6.1900, 106.7700},
	"m8": {-6.2260, 106.8540}, "m9": {-6.2370, 106.8090}, "m10": {-6.1950, 106.8820}, "m11": {-6.2840, 106.8440},
	"m12": {-6.2250, 106.8330}, "m13": {-6.2900, 106.8000},
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	return strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

// seedDev menyalin data dummy app Android ke database (lihat scripts/extract_android_seed.py). Aman dijalankan ulang:
// waktu relatif (jendela pickup, umur order, notifikasi, chat) dan stok dikembalikan ke keadaan awal.
func seedDev(ctx context.Context, tx pgx.Tx, pw *auth.Passwords, cfg *config.Config) error {
	d, err := loadAndroidData()
	if err != nil {
		return err
	}
	hash, err := pw.Hash(devPassword)
	if err != nil {
		return err
	}
	s := &seeder{ctx: ctx, tx: tx, now: time.Now().UTC(), hash: hash, d: d, mediaRoot: cfg.MediaRoot, userIDs: map[string]uuid.UUID{}, cover: map[string]uuid.UUID{}, listing: map[string]aListing{}}
	for _, l := range d.Listings {
		s.listing[l.ID] = l
	}
	steps := []func() error{s.users, s.media, s.merchants, s.listings, s.addresses, s.paymentMethods, s.vouchers, s.orders, s.incoming, s.reviews,
		s.chat, s.notifications, s.saved, s.impact}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return s.vet(cfg)
}

type seeder struct {
	ctx       context.Context
	tx        pgx.Tx
	now       time.Time
	hash      string
	d         *androidData
	userIDs   map[string]uuid.UUID // kunci pengguna -> id
	cover     map[string]uuid.UUID // id listing -> id media cover
	listing   map[string]aListing
	adminID   uuid.UUID
	phones    int
	mediaRoot string
}

func (s *seeder) exec(what, sql string, args ...any) error {
	if _, err := s.tx.Exec(s.ctx, sql, args...); err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	return nil
}

// user memastikan pengguna ada (satu per nama tampilan) dan mengembalikan id-nya.
func (s *seeder) user(key, name, email string) (uuid.UUID, error) {
	if id, ok := s.userIDs[key]; ok {
		return id, nil
	}
	id := sid("user:" + key)
	s.phones++
	phone := fmt.Sprintf("8123450%04d", s.phones)
	if err := s.exec("user "+key, `INSERT INTO users (id, name, email, phone, password_hash) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (email) DO NOTHING`,
		id, name, email, phone, s.hash); err != nil {
		return uuid.Nil, err
	}
	if err := s.exec("prefs "+key, `INSERT INTO notification_prefs (user_id) VALUES ($1) ON CONFLICT DO NOTHING`, id); err != nil {
		return uuid.Nil, err
	}
	s.userIDs[key] = id
	return id, nil
}

// buyer: pengguna untuk nama tampilan di dummy ("Clara T."); "me" = pemilik akun app (Budi).
func (s *seeder) buyer(name string) (uuid.UUID, error) {
	if name == "me" {
		return s.userIDs["budi"], nil
	}
	return s.user("buyer:"+slug(name), name, slug(name)+"@example.com")
}

func (s *seeder) users() error {
	if err := s.tx.QueryRow(s.ctx, `SELECT id FROM users WHERE is_admin ORDER BY created_at LIMIT 1`).Scan(&s.adminID); err != nil {
		return fmt.Errorf("admin belum ada: %w", err)
	}
	p := s.d.Profile
	if _, err := s.user("budi", p.Name, p.Email); err != nil {
		return err
	}
	if err := s.exec("profil budi", `UPDATE users SET phone=$2, location_label=$3, created_at=($4::text || '-01')::date WHERE id=$1`,
		s.userIDs["budi"], p.Phone, p.Location, p.MemberSince); err != nil {
		return err
	}
	for _, m := range s.d.Merchants {
		if m.ID == "m0" {
			s.userIDs["owner:m0"] = s.userIDs["budi"] // Dapur Budi milik akun yang sama dengan pembeli (satu akun, dua mode)
			continue
		}
		if _, err := s.user("owner:"+m.ID, "Pemilik "+m.Name, "owner-"+m.ID+"@example.com"); err != nil {
			return err
		}
	}
	// toko ekstra khusus dev untuk menguji persetujuan admin; tidak ada di dummy Android
	_, err := s.user("owner:m13", "Wati Hartono", "wati@example.com")
	return err
}

func (s *seeder) media() error {
	disk := pmedia.NewDisk(s.mediaRoot)
	for _, name := range s.d.Images {
		raw, err := imgFS.ReadFile("img/" + name + ".jpg")
		if err != nil {
			return fmt.Errorf("gambar %s: %w", name, err)
		}
		img, err := pmedia.ProcessImage(raw)
		if err != nil {
			return fmt.Errorf("gambar %s: %w", name, err)
		}
		sha, rel, thumb := pmedia.Paths(pmedia.Public, img.Main)
		if err := disk.Write(pmedia.Public, rel, img.Main); err != nil {
			return err
		}
		if err := disk.Write(pmedia.Public, thumb, img.Thumb); err != nil {
			return err
		}
		if err := s.exec("media "+name, `INSERT INTO media (id, owner_id, kind, visibility, mime, size_bytes, width, height, sha256, path, thumb_path, attached_at)
			VALUES ($1,$2,'image','public','image/jpeg',$3,$4,$5,$6,$7,$8,now()) ON CONFLICT (id) DO NOTHING`,
			sid("media:"+name), s.adminID, len(img.Main), img.Width, img.Height, sha, rel, thumb); err != nil {
			return err
		}
	}
	return nil
}

func img(name string) uuid.UUID { return sid("media:" + name) }

func (s *seeder) merchants() error {
	all := append([]aMerchant{}, s.d.Merchants...)
	about := "Katering rumahan, menunggu persetujuan admin."
	all = append(all, aMerchant{ID: "m13", Name: "Dapur Nusantara", Status: "pending", Location: "Cilandak", About: about, Address: "Jl. Cilandak KKO No. 8, Cilandak, Jakarta Selatan",
		OpenHours: "08.00-18.00", Halal: "unverified", Allergens: []string{}, PrepMinutes: 15})
	for _, m := range all {
		c := merchantCoords[m.ID]
		var banner *uuid.UUID
		if m.Banner != "" {
			b := img(m.Banner)
			banner = &b
		}
		allergens := m.Allergens
		if allergens == nil {
			allergens = []string{}
		}
		var rating *float64
		if m.Rating != nil {
			rating = m.Rating
		}
		if err := s.exec("merchant "+m.ID, `INSERT INTO merchants (id, owner_id, name, status, is_verified, about, address, location, location_label, open_hours, halal,
				allergens, todays_offer, delivery_available, prep_minutes, banner_media_id, rating_avg, rating_count, total_rescued)
			VALUES ($1,$2,$3,$4::merchant_status,$5,$6,$7,ST_SetSRID(ST_MakePoint($9,$8),4326)::geography,$10,$11,$12::halal_status,$13::text[]::allergen[],$14,$15,$16,$17,$18,$19,$20)
			ON CONFLICT (id) DO UPDATE SET todays_offer=EXCLUDED.todays_offer, status=EXCLUDED.status`,
			sid("merchant:"+m.ID), s.userIDs["owner:"+m.ID], m.Name, m.Status, m.Verified, m.About, m.Address, c[0], c[1], m.Location, m.OpenHours, m.Halal,
			allergens, m.TodaysOffer, m.DeliveryAvailable || m.ID == "m13", m.PrepMinutes, banner, rating, m.RatingCount, m.TotalRescued); err != nil {
			return err
		}
		if err := s.exec("merchant photos", `DELETE FROM merchant_photos WHERE merchant_id=$1`, sid("merchant:"+m.ID)); err != nil {
			return err
		}
		for i, p := range m.Photos {
			if err := s.exec("merchant photo", `INSERT INTO merchant_photos (merchant_id, media_id, position) VALUES ($1,$2,$3)`, sid("merchant:"+m.ID), img(p), i); err != nil {
				return err
			}
		}
	}
	return nil
}

var safeHours = map[string]time.Duration{"room_temp": 4 * time.Hour, "shelf": 72 * time.Hour, "chilled": 48 * time.Hour, "frozen": 168 * time.Hour}

func hours(h float64) time.Duration { return time.Duration(h * float64(time.Hour)) }

func (s *seeder) listings() error {
	for i, l := range s.d.Listings {
		unit := "portion"
		if l.Tier != "human" {
			unit = "kg"
		}
		var madeAt, safeUntil *time.Time
		if l.Tier == "human" {
			m := s.now.Add(-hours(l.MadeHoursAgo))
			u := m.Add(safeHours[l.Storage])
			madeAt, safeUntil = &m, &u
		} else {
			m := s.now.Add(-hours(l.MadeHoursAgo)) // dummy Android mengisi made_at 5 jam lalu juga untuk pakan/kompos
			madeAt = &m
		}
		allergens, checks := l.Allergens, l.SafetyChecks
		if allergens == nil {
			allergens = []string{}
		}
		if err := s.exec("listing "+l.ID, `INSERT INTO listings (id, merchant_id, title, description, tier, unit, is_free, price_original, price, stock, sold_count,
				halal, allergens, made_at, storage, safety_checks, safe_until, pickup_start, pickup_end, location, status, idempotency_key, created_at, video_media_id)
			SELECT $1,$2,$3,$4,$5::listing_tier,$6::quantity_unit,$7,$8,$9,$10,$11,$12::halal_status,$13::text[]::allergen[],$14,$15::storage_method,
			       $16::text[]::safety_check[],$17,$18,$19,m.location,'active',$20,$21,NULL
			FROM merchants m WHERE m.id = $2
			ON CONFLICT (id) DO UPDATE SET stock=EXCLUDED.stock, sold_count=EXCLUDED.sold_count, made_at=EXCLUDED.made_at, safe_until=EXCLUDED.safe_until,
			    pickup_start=EXCLUDED.pickup_start, pickup_end=EXCLUDED.pickup_end, created_at=EXCLUDED.created_at, status='active'`,
			sid("listing:"+l.ID), sid("merchant:"+l.MerchantID), l.Title, l.Description, l.Tier, unit, l.IsFree, l.Original, l.Price, l.Stock, l.Sold,
			l.Halal, allergens, madeAt, l.Storage, checks, safeUntil, s.now.Add(hours(l.PickupStartIn)), s.now.Add(hours(l.PickupEndInH)), "seed-"+l.ID,
			s.now.Add(-time.Duration(i)*time.Second)); err != nil { // urutan daftar Kotlin = terbaru dulu
			return err
		}
		if err := s.exec("listing media", `DELETE FROM listing_media WHERE listing_id=$1`, sid("listing:"+l.ID)); err != nil {
			return err
		}
		for pos, name := range append([]string{l.Image}, l.Gallery...) {
			if err := s.exec("listing media", `INSERT INTO listing_media (listing_id, media_id, position) VALUES ($1,$2,$3)`, sid("listing:"+l.ID), img(name), pos); err != nil {
				return err
			}
		}
		s.cover[l.ID] = img(l.Image)
	}
	return nil
}

func (s *seeder) addresses() error {
	for _, a := range s.d.Addresses {
		if err := s.exec("address "+a.ID, `INSERT INTO addresses (id, user_id, label, full_address, note, location)
			VALUES ($1,$2,$3,$4,$5,ST_SetSRID(ST_MakePoint($7,$6),4326)::geography) ON CONFLICT (id) DO NOTHING`,
			sid("address:"+a.ID), s.userIDs["budi"], a.Label, a.FullAddr, a.Note, a.Lat, a.Lng); err != nil {
			return err
		}
	}
	return s.exec("alamat terpilih", `UPDATE users SET selected_address_id=$1 WHERE id=$2 AND selected_address_id IS NULL`, sid("address:"+s.d.Addresses[0].ID), s.userIDs["budi"])
}

func (s *seeder) paymentMethods() error {
	for _, p := range s.d.Payments {
		if err := s.exec("payment "+p.Kind, `INSERT INTO payment_methods (user_id, kind, detail, is_default) VALUES ($1,$2::payment_kind,$3,$2='qris') ON CONFLICT DO NOTHING`,
			s.userIDs["budi"], p.Kind, p.Detail); err != nil {
			return err
		}
	}
	return nil
}

func (s *seeder) vouchers() error {
	for _, v := range s.d.Vouchers {
		accent := fmt.Sprintf("#%06X", v.Accent&0xFFFFFF)
		if err := s.exec("voucher "+v.Code, `INSERT INTO vouchers (code, title, description, percent_off, flat_off, max_discount, min_spend, applies_to_label, image_media_id,
				starts_at, expires_at, terms, accent_hex)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
			ON CONFLICT (code) DO UPDATE SET expires_at=EXCLUDED.expires_at, title=EXCLUDED.title, description=EXCLUDED.description, terms=EXCLUDED.terms,
			    percent_off=EXCLUDED.percent_off, flat_off=EXCLUDED.flat_off, max_discount=EXCLUDED.max_discount, min_spend=EXCLUDED.min_spend,
			    applies_to_label=EXCLUDED.applies_to_label, image_media_id=EXCLUDED.image_media_id, accent_hex=EXCLUDED.accent_hex, is_active=true`,
			v.Code, v.Title, v.Description, v.PercentOff, v.FlatOff, v.MaxDiscount, v.MinSpend, v.AppliesTo, img(v.Image),
			s.now.Add(-24*time.Hour), v.Expires.at(s.now), v.Terms, accent); err != nil {
			return err
		}
	}
	for _, code := range s.d.Claimed {
		if err := s.exec("klaim voucher", `INSERT INTO user_vouchers (user_id, voucher_code) VALUES ($1,$2) ON CONFLICT DO NOTHING`, s.userIDs["budi"], code); err != nil {
			return err
		}
	}
	return nil
}

type orderRow struct {
	id, buyer, merchant          uuid.UUID
	code, status, payment, note  string
	created, pickupBy, pickupEnd time.Time
	completed                    *time.Time
	lines                        []aOrderLine
	fulfillment, courier         string
	cancelledBy, cancelReason    *string
}

// insertOrder menulis satu order beserta item. Angka mengikuti rules.Settle; biaya layanan 0 seperti order dummy di app.
func (s *seeder) insertOrder(o orderRow) error {
	subtotal, original := 0, 0
	for _, l := range o.lines {
		subtotal += l.UnitPrice * l.Quantity
		original += max(l.UnitOriginalPrice, l.UnitPrice) * l.Quantity
	}
	amt := rules.Settle(subtotal, 0, false)
	fee, discount, total := 0, 0, subtotal
	var courier, speed *string
	var etaMin, etaMax *int
	var addr []byte
	if o.fulfillment == "delivery" {
		q := rules.QuoteDelivery(o.courier, "standard", 3.0, subtotal, 10)
		fee, discount, total = q.Payable, q.Discount, subtotal+q.Payable
		courier, speed, etaMin, etaMax = &o.courier, ptr("standard"), &q.ETAMin, &q.ETAMax
		addr, _ = json.Marshal(map[string]any{"label": "Rumah", "full_address": "Jl. Sutera Onyx XII, Tangerang", "note": "", "lat": -6.2275, "lng": 106.6544})
	}
	paymentStatus := "unpaid"
	var paidAt *time.Time
	if o.payment != "cash" || o.status == "completed" {
		paymentStatus = "paid"
		paidAt = &o.created
		if o.status == "cancelled" && o.payment != "cash" {
			paymentStatus = "refunded"
		}
	}
	var cancelledAt *time.Time
	if o.status == "cancelled" || o.status == "no_show" {
		c := o.created.Add(30 * time.Minute)
		cancelledAt = &c
	}
	var pickupBy *time.Time
	if o.fulfillment == "pickup" {
		pickupBy = &o.pickupBy
	}
	if err := s.exec("order "+o.code, `INSERT INTO orders (id, code, code_date, buyer_id, merchant_id, status, fulfillment, payment_kind, payment_status, note, subtotal, original_total,
			service_fee, delivery_fee, delivery_discount, total, commission, merchant_payout, delivery_courier, delivery_speed, delivery_eta_min, delivery_eta_max, delivery_address,
			pickup_by, pickup_end, idempotency_key, created_at, paid_at, completed_at, cancelled_at, cancelled_by, cancel_reason)
		VALUES ($1,$2,($3::timestamptz AT TIME ZONE 'Asia/Jakarta')::date,$4,$5,$6::order_status,$7::fulfillment,$8::payment_kind,$9::payment_status,$10,$11,$12,0,$13,$14,$15,$16,$17,
		        $18::courier,$19::delivery_speed,$20,$21,$22::jsonb,$23,$24,$25,$3,$26,$27,$28,$29::chat_side,$30)
		ON CONFLICT (id) DO UPDATE SET status=EXCLUDED.status, payment_status=EXCLUDED.payment_status, created_at=EXCLUDED.created_at, code_date=EXCLUDED.code_date,
		    pickup_by=EXCLUDED.pickup_by, pickup_end=EXCLUDED.pickup_end, paid_at=EXCLUDED.paid_at, completed_at=EXCLUDED.completed_at,
		    cancelled_at=EXCLUDED.cancelled_at, cancelled_by=EXCLUDED.cancelled_by, cancel_reason=EXCLUDED.cancel_reason, reminded_at=NULL`,
		o.id, o.code, o.created, o.buyer, o.merchant, o.status, o.fulfillment, o.payment, paymentStatus, o.note, subtotal, original,
		fee, discount, total, amt.Commission, amt.MerchantPayout, courier, speed, etaMin, etaMax, nullJSON(addr),
		pickupBy, o.pickupEnd, "seed-"+o.id.String(), paidAt, o.completed, cancelledAt, o.cancelledBy, o.cancelReason); err != nil {
		return err
	}
	for _, l := range o.lines {
		unit := "portion"
		if l.Tier != "human" {
			unit = "kg"
		}
		if err := s.exec("order item", `INSERT INTO order_items (order_id, listing_id, title, tier, unit, image_media_id, quantity, unit_price, unit_original_price)
			VALUES ($1,$2,$3,$4::listing_tier,$5::quantity_unit,$6,$7,$8,$9) ON CONFLICT DO NOTHING`,
			o.id, sid("listing:"+l.ListingID), l.Title, l.Tier, unit, s.cover[l.ListingID], l.Quantity, l.UnitPrice, l.UnitOriginalPrice); err != nil {
			return err
		}
	}
	return nil
}

func ptr[T any](v T) *T { return &v }

func nullJSON(b []byte) any {
	if b == nil {
		return nil
	}
	return string(b)
}

// orders: riwayat pesanan Budi (OrderRepository.orders). Pickup_by dibuat = pickup_end supaya order READY tidak langsung
// dianggap tidak diambil oleh job terjadwal (di dummy Android pickupBy kosong).
func (s *seeder) orders() error {
	for _, o := range s.d.Orders {
		row := orderRow{id: sid("order:" + o.ID), buyer: s.userIDs["budi"], merchant: sid("merchant:" + o.MerchantID), code: o.Code, status: o.Status, payment: o.Payment,
			note: o.Note, created: o.Created.at(s.now), pickupEnd: o.PickupEnd.at(s.now), lines: o.Lines, fulfillment: "pickup"}
		row.pickupBy = row.pickupEnd
		if o.Completed != nil {
			c := o.Completed.at(s.now)
			row.completed = &c
		}
		if o.Status == "cancelled" {
			row.cancelledBy, row.cancelReason = ptr("buyer"), ptr("Berubah pikiran")
		}
		if err := s.insertOrder(row); err != nil {
			return err
		}
		if o.Rating != nil { // rating pada order dummy = ulasan toko dari order itu
			tags := o.Rating.Tags
			if err := s.exec("rating order", `INSERT INTO reviews (id, order_id, merchant_id, buyer_id, stars, tags, comment, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
				ON CONFLICT (id) DO UPDATE SET created_at=EXCLUDED.created_at`,
				sid("review:"+o.ID), row.id, row.merchant, row.buyer, o.Rating.Stars, tags, o.Rating.Comment, row.completed); err != nil {
				return err
			}
		}
	}
	return nil
}

// incoming: pesanan masuk ke toko Dapur Budi (IncomingOrderRepository).
func (s *seeder) incoming() error {
	for _, in := range s.d.Incoming {
		buyer, err := s.buyer(in.Buyer)
		if err != nil {
			return err
		}
		lid := "l10"
		if strings.Contains(in.ItemTitle, "Kulit") {
			lid = "l11"
		}
		l := s.listing[lid]
		status := map[string]string{"waiting": "ready", "handed_over": "completed", "no_show": "no_show"}[in.Status]
		unitPrice := 0
		if in.Quantity > 0 {
			unitPrice = in.Total / in.Quantity
		}
		orig := unitPrice
		if l.Original != nil {
			orig = *l.Original
		}
		row := orderRow{id: sid("order:" + in.ID), buyer: buyer, merchant: sid("merchant:m0"), code: in.Code, status: status, payment: "cash",
			created: in.Created.at(s.now), pickupBy: in.PickupBy.at(s.now), fulfillment: strings.ToLower(in.Fulfillment),
			lines: []aOrderLine{{ListingID: lid, Title: in.ItemTitle, Tier: l.Tier, Quantity: in.Quantity, UnitPrice: unitPrice, UnitOriginalPrice: max(orig, unitPrice)}}}
		row.pickupEnd = row.pickupBy
		if row.fulfillment == "delivery" {
			row.courier = "gosend"
			row.pickupEnd = row.created.Add(2 * time.Hour)
		}
		if status == "completed" {
			c := row.pickupBy.Add(-20 * time.Minute)
			row.completed = &c
		}
		if err := s.insertOrder(row); err != nil {
			return err
		}
	}
	return nil
}

// reviews: ulasan dummy. Backend hanya menerima ulasan dari order selesai, jadi tiap ulasan dibuatkan satu order selesai
// (1 item, listing yang judulnya cocok dengan item_title) atas nama penulisnya.
func (s *seeder) reviews() error {
	byTitle := map[string]aListing{}
	for _, l := range s.d.Listings {
		byTitle[l.Title] = l
	}
	for i, r := range s.d.Reviews {
		buyer, err := s.buyer(r.Author)
		if err != nil {
			return err
		}
		l, ok := byTitle[r.ItemTitle]
		if !ok {
			return fmt.Errorf("ulasan %s: listing %q tidak ada", r.ID, r.ItemTitle)
		}
		price := 0
		if l.Price != nil {
			price = *l.Price
		}
		orig := price
		if l.Original != nil {
			orig = *l.Original
		}
		at := r.At.at(s.now)
		done := at.Add(-30 * time.Minute)
		row := orderRow{id: sid("order:rv:" + r.ID), buyer: buyer, merchant: sid("merchant:" + r.MerchantID), code: fmt.Sprintf("SG-%04d", 9000+i),
			status: "completed", payment: "cash", created: done.Add(-time.Hour), pickupBy: done, pickupEnd: done, completed: &done, fulfillment: "pickup",
			lines: []aOrderLine{{ListingID: l.ID, Title: l.Title, Tier: l.Tier, Quantity: 1, UnitPrice: price, UnitOriginalPrice: max(orig, price)}}}
		if err := s.insertOrder(row); err != nil {
			return err
		}
		tags := r.Tags
		if tags == nil {
			tags = []string{}
		}
		if err := s.exec("review "+r.ID, `INSERT INTO reviews (id, order_id, merchant_id, buyer_id, stars, tags, comment, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (id) DO UPDATE SET created_at=EXCLUDED.created_at`, sid("review:"+r.ID), row.id, row.merchant, buyer, r.Stars, tags, r.Comment, at); err != nil {
			return err
		}
		if err := s.exec("review media", `DELETE FROM review_media WHERE review_id=$1`, sid("review:"+r.ID)); err != nil {
			return err
		}
		for pos, p := range r.Photos {
			if err := s.exec("review media", `INSERT INTO review_media (review_id, media_id, position) VALUES ($1,$2,$3)`, sid("review:"+r.ID), img(p), pos); err != nil {
				return err
			}
		}
	}
	return nil
}

// chat: percakapan Budi dengan toko lain, dan pembeli dengan Dapur Budi. Tanda baca diturunkan dari isRead tiap pesan.
func (s *seeder) chat() error {
	type convKey struct{ buyer, merchant string }
	ordered := append([]aChat{}, s.d.Chat...)
	// urut waktu naik supaya id pesan (identity) searah dengan waktu
	for i := 1; i < len(ordered); i++ {
		for j := i; j > 0 && ordered[j].At.Min < ordered[j-1].At.Min; j-- {
			ordered[j], ordered[j-1] = ordered[j-1], ordered[j]
		}
	}
	convs := map[convKey]uuid.UUID{}
	for _, m := range ordered {
		buyer, err := s.buyer(m.Buyer)
		if err != nil {
			return err
		}
		k := convKey{m.Buyer, m.MerchantID}
		cid := sid("conv:" + m.Buyer + ":" + m.MerchantID)
		if _, ok := convs[k]; !ok {
			if err := s.exec("conversation", `INSERT INTO conversations (id, buyer_id, merchant_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, cid, buyer, sid("merchant:"+m.MerchantID)); err != nil {
				return err
			}
			convs[k] = cid
		}
		side, sender := "merchant", s.userIDs["owner:"+m.MerchantID]
		if m.FromBuyer {
			side, sender = "buyer", buyer
		}
		var listing *uuid.UUID
		if m.ListingID != nil {
			l := sid("listing:" + *m.ListingID)
			listing = &l
		}
		if err := s.exec("message "+m.ID, `INSERT INTO messages (conversation_id, sender_side, sender_id, body, listing_id, client_id, created_at)
			VALUES ($1,$2::chat_side,$3,$4,$5,$6,$7) ON CONFLICT (conversation_id, client_id) DO UPDATE SET created_at=EXCLUDED.created_at`,
			cid, side, sender, m.Text, listing, sid("msg:"+m.ID), m.At.at(s.now)); err != nil {
			return err
		}
	}
	// last_message + tanda baca: semua dibaca, kecuali pesan lawan yang isRead=false (penanda berhenti tepat sebelum pesan itu)
	for _, cid := range convs {
		if err := s.exec("last message", `UPDATE conversations c SET last_message_id = l.id, last_message_at = l.created_at, buyer_last_read_id = l.id, merchant_last_read_id = l.id
			FROM (SELECT id, created_at FROM messages WHERE conversation_id=$1 ORDER BY id DESC LIMIT 1) l WHERE c.id=$1`, cid); err != nil {
			return err
		}
	}
	for _, m := range ordered {
		if m.IsRead {
			continue
		}
		col := "buyer_last_read_id" // pesan toko yang belum dibaca pembeli
		if m.FromBuyer {
			col = "merchant_last_read_id"
		}
		if err := s.exec("pesan belum dibaca", fmt.Sprintf(`UPDATE conversations SET %[1]s = LEAST(%[1]s, (SELECT id FROM messages WHERE conversation_id=$1 AND client_id=$2) - 1) WHERE id=$1`, col),
			convs[convKey{m.Buyer, m.MerchantID}], sid("msg:"+m.ID)); err != nil {
			return err
		}
	}
	return nil
}

func (s *seeder) notifications() error {
	budi := s.userIDs["budi"]
	for _, n := range s.d.Notifications {
		at := n.At.at(s.now)
		var readAt *time.Time
		if n.IsRead {
			readAt = &at
		}
		data := map[string]any{}
		switch {
		case strings.Contains(n.Body, "SG-4821"), strings.Contains(n.Title, "Pesanan siap"):
			data["order_id"] = sid("order:o4")
		case n.Title == "Pesanan selesai":
			data["order_id"] = sid("order:o3")
		case n.Type == "merchant" && strings.HasPrefix(n.Title, "Warung Bu Sari"):
			data["merchant_id"] = sid("merchant:m1")
		case n.Type == "merchant":
			data["merchant_id"] = sid("merchant:m4")
		}
		b, _ := json.Marshal(data)
		if err := s.exec("notification "+n.ID, `INSERT INTO notifications (id, user_id, type, title, body, data, created_at, read_at, deleted_at)
			VALUES ($1,$2,$3::notification_type,$4,$5,$6::jsonb,$7,$8,NULL)
			ON CONFLICT (id) DO UPDATE SET created_at=EXCLUDED.created_at, read_at=EXCLUDED.read_at, deleted_at=NULL`,
			sid("notif:"+n.ID), budi, n.Type, n.Title, n.Body, string(b), at, readAt); err != nil {
			return err
		}
	}
	return nil
}

func (s *seeder) saved() error {
	if err := s.exec("reset favorit", `DELETE FROM saved_merchants WHERE user_id=$1`, s.userIDs["budi"]); err != nil {
		return err
	}
	for i, id := range s.d.Saved {
		if err := s.exec("favorit", `INSERT INTO saved_merchants (user_id, merchant_id, created_at) VALUES ($1,$2,$3)`,
			s.userIDs["budi"], sid("merchant:"+id), s.now.Add(-time.Duration(len(s.d.Saved)-i)*time.Hour)); err != nil {
			return err
		}
	}
	return nil
}

// impact: ImpactStats di dummy Android konstan (34 porsi, ...) padahal ordernya hanya sedikit. Selisihnya disimpan sebagai
// baseline pengguna (riwayat sebelum memakai backend) supaya /me/impact sama dengan app.
func (s *seeder) impact() error {
	t := s.d.Profile.Impact
	return s.exec("impact", `UPDATE users u SET
			impact_base_portions     = $2 - (v.portions - u.impact_base_portions),
			impact_base_feed_kg      = $3 - (v.feed_compost_kg - u.impact_base_feed_kg),
			impact_base_saved_rupiah = $4 - (v.saved_rupiah - u.impact_base_saved_rupiah)
		FROM user_impact v WHERE v.user_id = u.id AND u.id = $1`, s.userIDs["budi"], t.Portions, t.CompostKg, t.Saved)
}

// vet memastikan hasil seed konsisten dengan dummy: jumlah baris utama sesuai.
func (s *seeder) vet(cfg *config.Config) error {
	var merchants, listings, reviews int
	if err := s.tx.QueryRow(s.ctx, `SELECT (SELECT count(*) FROM merchants WHERE status='approved'), (SELECT count(*) FROM listings), (SELECT count(*) FROM reviews)`).
		Scan(&merchants, &listings, &reviews); err != nil {
		return err
	}
	if merchants < len(s.d.Merchants) || listings < len(s.d.Listings) || reviews < len(s.d.Reviews) {
		return fmt.Errorf("seed tidak lengkap: toko=%d listing=%d ulasan=%d", merchants, listings, reviews)
	}
	return nil
}
