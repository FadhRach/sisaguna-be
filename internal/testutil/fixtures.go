package testutil

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Fixtures menyisipkan data langsung lewat SQL (tanpa service) supaya test satu domain
// tidak bergantung pada domain lain.
type Fixtures struct {
	T    testing.TB
	Pool *pgxpool.Pool
}

func (f Fixtures) exec(sql string, args ...any) {
	f.T.Helper()
	if _, err := f.Pool.Exec(context.Background(), sql, args...); err != nil {
		f.T.Fatalf("fixture: %v\n%s", err, sql)
	}
}

func (f Fixtures) User(name string) uuid.UUID {
	f.T.Helper()
	id := uuid.New()
	f.exec(`INSERT INTO users (id,name,email,phone,password_hash) VALUES ($1,$2,$3,'81234567890','x')`, id, name, id.String()+"@test.co")
	return id
}

// Merchant membuat toko berstatus status di sekitar Kebayoran Baru (lat/lng bisa digeser per test).
func (f Fixtures) Merchant(owner uuid.UUID, status string, lat, lng float64) uuid.UUID {
	f.T.Helper()
	id := uuid.New()
	f.exec(`INSERT INTO merchants (id,owner_id,name,status,address,location,location_label)
		VALUES ($1::uuid,$2,'Toko '||left($1::uuid::text,4),$3::merchant_status,'Jl. Test',ST_SetSRID(ST_MakePoint($5,$4),4326)::geography,'Test')`, id, owner, status, lat, lng)
	return id
}

type ListingOpts struct {
	Title    string
	Tier     string        // default human
	Free     bool          // gratis: harga diabaikan
	Price    int           // default 10000
	Orig     int           // default 20000
	Stock    int           // default 10
	StartAgo time.Duration // default 1 jam
	EndIn    time.Duration // default 2 jam
	Status   string        // default active
	Storage  string        // default room_temp
}

// Listing membuat listing (human: made_at 1 jam lalu, checklist lengkap) beserta satu foto cover.
func (f Fixtures) Listing(merchant uuid.UUID, o ListingOpts) uuid.UUID {
	f.T.Helper()
	if o.Title == "" {
		o.Title = "Nasi Test"
	}
	if o.Tier == "" {
		o.Tier = "human"
	}
	if o.Price == 0 {
		o.Price = 10000
	}
	if o.Orig == 0 {
		o.Orig = 20000
	}
	if o.Stock == 0 {
		o.Stock = 10
	}
	if o.StartAgo == 0 {
		o.StartAgo = time.Hour
	}
	if o.EndIn == 0 {
		o.EndIn = 2 * time.Hour
	}
	if o.Status == "" {
		o.Status = "active"
	}
	if o.Storage == "" {
		o.Storage = "room_temp"
	}
	now := time.Now()
	id := uuid.New()
	unit := "kg"
	var madeAt, safeUntil *time.Time
	checks := []string{}
	if o.Tier == "human" {
		unit = "portion"
		m := now.Add(-time.Hour)
		s := m.Add(72 * time.Hour)
		if o.Storage == "room_temp" {
			s = m.Add(4 * time.Hour)
		}
		madeAt, safeUntil = &m, &s
		checks = []string{"untouched", "sensory_ok", "stored_right", "packed_clean"}
	}
	var price, orig *int
	if !o.Free {
		price = &o.Price
		orig = &o.Orig
	}
	f.exec(`INSERT INTO listings (id,merchant_id,title,tier,unit,is_free,price_original,price,stock,made_at,storage,safety_checks,safe_until,
			pickup_start,pickup_end,location,status,idempotency_key)
		SELECT $1,m.id,$2,$3::listing_tier,$4::quantity_unit,$5,$6,$7,$8,$9,$10::storage_method,$11::text[]::safety_check[],$12,$13,$14,m.location,$15::listing_status,$16
		FROM merchants m WHERE m.id=$17`,
		id, o.Title, o.Tier, unit, o.Free, orig, price, o.Stock, madeAt, o.Storage, checks, safeUntil,
		now.Add(-o.StartAgo), now.Add(o.EndIn), o.Status, "fx-"+id.String(), merchant)

	var owner uuid.UUID
	if err := f.Pool.QueryRow(context.Background(), `SELECT owner_id FROM merchants WHERE id=$1`, merchant).Scan(&owner); err != nil {
		f.T.Fatal(err)
	}
	f.Photo(owner, id)
	return id
}

// Photo membuat media publik yang sudah terpasang sebagai cover listing.
func (f Fixtures) Photo(owner, listing uuid.UUID) uuid.UUID {
	f.T.Helper()
	mid := uuid.New()
	f.exec(`INSERT INTO media (id,owner_id,kind,visibility,mime,size_bytes,width,height,sha256,path,thumb_path,attached_at)
		VALUES ($1,$2,'image','public','image/jpeg',10,100,100,$3,'ab/'||$3||'.jpg','ab/'||$3||'_thumb.jpg',now())`, mid, owner, mid.String())
	f.exec(`INSERT INTO listing_media (listing_id,media_id,position) VALUES ($1,$2,0)`, listing, mid)
	return mid
}

// FreeMedia membuat foto publik milik owner yang belum dipakai (untuk POST /listings).
func (f Fixtures) FreeMedia(owner uuid.UUID) uuid.UUID {
	f.T.Helper()
	mid := uuid.New()
	f.exec(`INSERT INTO media (id,owner_id,kind,visibility,mime,size_bytes,width,height,sha256,path,thumb_path)
		VALUES ($1,$2,'image','public','image/jpeg',10,100,100,$3,'cd/'||$3||'.jpg','cd/'||$3||'_thumb.jpg')`, mid, owner, mid.String())
	return mid
}

// FreeVideo membuat video publik milik owner yang belum dipakai.
func (f Fixtures) FreeVideo(owner uuid.UUID) uuid.UUID {
	f.T.Helper()
	mid := uuid.New()
	f.exec(`INSERT INTO media (id,owner_id,kind,visibility,mime,size_bytes,duration_ms,sha256,path)
		VALUES ($1::uuid,$2,'video','public','video/mp4',10,5000,($1::uuid)::text,'cd/'||($1::uuid)::text||'.mp4')`, mid, owner)
	return mid
}

// Voucher membuat voucher aktif satu tahun. merchant/tier boleh kosong.
func (f Fixtures) Voucher(code string, percent, flat, maxDiscount, minSpend int, merchant *uuid.UUID, tier string) {
	f.T.Helper()
	var p, fl, mx *int
	if percent > 0 {
		p = &percent
	}
	if flat > 0 {
		fl = &flat
	}
	if maxDiscount > 0 {
		mx = &maxDiscount
	}
	var t *string
	if tier != "" {
		t = &tier
	}
	f.exec(`INSERT INTO vouchers (code,title,percent_off,flat_off,max_discount,min_spend,merchant_id,applies_to_tier,expires_at)
		VALUES ($1,'Voucher '||$1,$2,$3,$4,$5,$6,$7::listing_tier,now()+interval '1 year')`, code, p, fl, mx, minSpend, merchant, t)
}

func (f Fixtures) Claim(user uuid.UUID, code string) {
	f.T.Helper()
	f.exec(`INSERT INTO user_vouchers (user_id,voucher_code) VALUES ($1,$2)`, user, code)
}

func (f Fixtures) Stock(listing uuid.UUID) (stock int) {
	f.T.Helper()
	if err := f.Pool.QueryRow(context.Background(), `SELECT stock FROM listings WHERE id=$1`, listing).Scan(&stock); err != nil {
		f.T.Fatal(err)
	}
	return stock
}

var orderSeq atomic.Int32

// Order membuat order tunai Rp11.000 (subtotal 10.000 + biaya layanan 1.000) tanpa lewat checkout.
// Status completed diisi completed_at = sekarang - completedAgo. Kalau listing diberikan, ditambah satu
// item 2 unit @5.000 (harga asli 10.000) sebagai bahan statistik dampak.
func (f Fixtures) Order(buyer, merchant, listing uuid.UUID, status string, completedAgo time.Duration) uuid.UUID {
	f.T.Helper()
	id := uuid.New()
	code := fmt.Sprintf("SG-%04d", orderSeq.Add(1)%10000)
	f.exec(`INSERT INTO orders (id,code,buyer_id,merchant_id,status,payment_kind,payment_status,subtotal,original_total,service_fee,total,commission,merchant_payout,
			pickup_by,pickup_end,idempotency_key,completed_at)
		VALUES ($1::uuid,$2,$3,$4,($5::text)::order_status,'cash',CASE WHEN $5::text='completed' THEN 'paid'::payment_status ELSE 'unpaid'::payment_status END,
			10000,20000,1000,11000,1000,9000,now()+interval '1 hour',now()+interval '2 hours',($1::uuid)::text,
			CASE WHEN $5::text='completed' THEN now() - make_interval(secs => $6::float8) END)`,
		id, code, buyer, merchant, status, completedAgo.Seconds())
	if listing != uuid.Nil {
		f.exec(`INSERT INTO order_items (order_id,listing_id,title,tier,unit,quantity,unit_price,unit_original_price)
			SELECT $1,id,title,tier,unit,2,5000,10000 FROM listings WHERE id=$2`, id, listing)
	}
	return id
}

// PrivateMedia membuat foto privat (komplain) milik owner yang belum dipakai.
func (f Fixtures) PrivateMedia(owner uuid.UUID) uuid.UUID {
	f.T.Helper()
	mid := uuid.New()
	f.exec(`INSERT INTO media (id,owner_id,kind,visibility,mime,size_bytes,width,height,sha256,path)
		VALUES ($1::uuid,$2,'image','private','image/jpeg',10,100,100,'private:'||($1::uuid)::text,'ef/'||($1::uuid)::text||'.jpg')`, mid, owner)
	return mid
}

// Address membuat alamat pembeli di titik (lat, lng) dan menjadikannya alamat terpilih.
func (f Fixtures) Address(user uuid.UUID, lat, lng float64) uuid.UUID {
	f.T.Helper()
	id := uuid.New()
	f.exec(`INSERT INTO addresses (id,user_id,label,full_address,location) VALUES ($1,$2,'Rumah','Jl. Uji No. 1',ST_SetSRID(ST_MakePoint($4,$3),4326)::geography)`, id, user, lat, lng)
	f.exec(`UPDATE users SET selected_address_id=$1 WHERE id=$2`, id, user)
	return id
}

// Delivery menyalakan pengiriman untuk toko.
func (f Fixtures) Delivery(merchant uuid.UUID, on bool) {
	f.T.Helper()
	f.exec(`UPDATE merchants SET delivery_available=$2 WHERE id=$1`, merchant, on)
}
