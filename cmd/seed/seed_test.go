package main

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/FadhRach/sisaguna-be/internal/config"
	"github.com/FadhRach/sisaguna-be/internal/listing"
	"github.com/FadhRach/sisaguna-be/internal/merchant"
	"github.com/FadhRach/sisaguna-be/internal/platform/auth"
	"github.com/FadhRach/sisaguna-be/internal/platform/database"
	pmedia "github.com/FadhRach/sisaguna-be/internal/platform/media"
	"github.com/FadhRach/sisaguna-be/internal/review"
	"github.com/FadhRach/sisaguna-be/internal/testutil"
)

// Seed dev harus sama dengan dummy Android: jumlah, harga, stok, impact, urutan rail, tanda baca chat.
func TestSeedMatchesAndroidDummy(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := context.Background()
	pw, err := auth.NewPasswords()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{AdminEmail: "admin@test.co", AdminPassword: "Admin12345!", AdminName: "Admin", MediaRoot: t.TempDir()}
	run := func() {
		t.Helper()
		err := database.InTx(ctx, pool, func(tx pgx.Tx) error {
			if err := seedProd(ctx, tx, pw, cfg, false); err != nil {
				return err
			}
			return seedDev(ctx, tx, pw, cfg)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	run()
	run() // menjalankan ulang harus aman (idempoten)

	d, err := loadAndroidData()
	if err != nil {
		t.Fatal(err)
	}
	count := func(q string, args ...any) (n int) {
		t.Helper()
		if err := pool.QueryRow(ctx, q, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return
	}
	if n := count(`SELECT count(*) FROM merchants WHERE status='approved'`); n != 13 {
		t.Errorf("toko approved = %d, mau 13", n)
	}
	if n := count(`SELECT count(*) FROM merchants WHERE status='pending'`); n != 1 {
		t.Errorf("toko pending (khusus dev) = %d", n)
	}
	if n := count(`SELECT count(*) FROM listings`); n != len(d.Listings) || n != 39 {
		t.Errorf("listing = %d, mau %d", n, len(d.Listings))
	}
	if n := count(`SELECT count(*) FROM vouchers`); n != 4 {
		t.Errorf("voucher = %d, mau 4 (tanpa voucher prod di dev)", n)
	}
	if n := count(`SELECT count(*) FROM reviews`); n != 60 { // 59 ulasan + rating pada order o2
		t.Errorf("ulasan = %d, mau 60", n)
	}
	if n := count(`SELECT count(*) FROM saved_merchants`); n != 4 {
		t.Errorf("favorit = %d", n)
	}

	// baris persis dari Kotlin: l1 Nasi Kuning 25.000 -> 8.000, stok 6, terjual 48
	var price, orig, stock, sold int
	if err := pool.QueryRow(ctx, `SELECT price, price_original, stock, sold_count FROM listings WHERE id=$1`, sid("listing:l1")).Scan(&price, &orig, &stock, &sold); err != nil ||
		price != 8000 || orig != 25000 || stock != 6 || sold != 48 {
		t.Errorf("l1: %d %d %d %d (%v)", price, orig, stock, sold, err)
	}
	var origFree *int
	if err := pool.QueryRow(ctx, `SELECT price_original FROM listings WHERE id=$1 AND is_free AND price IS NULL`, sid("listing:l3")).Scan(&origFree); err != nil || origFree == nil || *origFree != 20000 {
		t.Errorf("l3 gratis tetap membawa harga normal: %v %v", origFree, err)
	}

	// impact Budi persis konstanta dummy, dengan karbon estimasi
	var portions, feed, saved, orders int
	if err := pool.QueryRow(ctx, `SELECT portions, feed_compost_kg, saved_rupiah, completed_orders FROM user_impact WHERE user_id=$1`, sid("user:budi")).Scan(&portions, &feed, &saved, &orders); err != nil ||
		portions != 34 || feed != 3 || saved != 245000 || orders != 2 {
		t.Errorf("impact: %d %d %d orders=%d (%v)", portions, feed, saved, orders, err)
	}

	// order Budi: 4 riwayat, kode & status seperti dummy
	rows, err := pool.Query(ctx, `SELECT code, status::text, payment_kind::text, total FROM orders WHERE buyer_id=$1 ORDER BY created_at DESC`, sid("user:budi"))
	if err != nil {
		t.Fatal(err)
	}
	type o struct {
		code, status, payment string
		total                 int
	}
	var got []o
	for rows.Next() {
		var x o
		_ = rows.Scan(&x.code, &x.status, &x.payment, &x.total)
		got = append(got, x)
	}
	want := []o{{"SG-4821", "ready", "qris", 10000}, {"SG-3307", "completed", "gopay", 3000}, {"SG-2190", "completed", "cash", 20000}, {"SG-1044", "cancelled", "cash", 0}}
	if len(got) != 4 {
		t.Fatalf("order Budi = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("order %d = %+v, mau %+v", i, got[i], want[i])
		}
	}

	// pesanan masuk Dapur Budi: 3 menunggu + 1 diserahkan, satu lewat kurir
	if n := count(`SELECT count(*) FROM orders WHERE merchant_id=$1 AND status='ready'`, sid("merchant:m0")); n != 3 {
		t.Errorf("pesanan menunggu = %d", n)
	}
	if n := count(`SELECT count(*) FROM orders WHERE merchant_id=$1 AND fulfillment='delivery' AND delivery_courier='gosend'`, sid("merchant:m0")); n != 1 {
		t.Errorf("order kurir = %d", n)
	}

	// chat: Budi belum membaca balasan Roti Segar; Dapur Budi punya 2 pesan Clara belum dibaca
	var unreadBuyer, unreadMerchant int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM messages m JOIN conversations c ON c.id=m.conversation_id
		WHERE c.buyer_id=$1 AND m.sender_side='merchant' AND m.id > c.buyer_last_read_id`, sid("user:budi")).Scan(&unreadBuyer)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM messages m JOIN conversations c ON c.id=m.conversation_id
		WHERE c.merchant_id=$1 AND m.sender_side='buyer' AND m.id > c.merchant_last_read_id`, sid("merchant:m0")).Scan(&unreadMerchant)
	if unreadBuyer != 1 || unreadMerchant != 2 {
		t.Errorf("belum dibaca: pembeli=%d mitra=%d, mau 1 dan 2", unreadBuyer, unreadMerchant)
	}
	if n := count(`SELECT count(*) FROM notifications WHERE user_id=$1 AND read_at IS NULL`, sid("user:budi")); n != 3 {
		t.Errorf("notifikasi belum dibaca = %d, mau 3", n)
	}

	// lewat service nyata: urutan rail Home sama dengan Kotlin (nearby = terdekat, popular = terlaris, maks 12)
	urls := pmedia.URLs{Base: "http://api.test"}
	lsvc := listing.NewService(listing.NewRepository(pool), urls)
	lat, lng := -6.2275, 106.6544 // alamat "Rumah"
	h, err := lsvc.Home(ctx, listing.HomeQuery{Lat: &lat, Lng: &lng}, uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Nearby) != 12 || len(h.Popular) != 12 || len(h.AnimalFeed) != 5 || len(h.Compost) != 7 || h.HumanCount < 12 {
		t.Errorf("rail: nearby=%d popular=%d feed=%d compost=%d", len(h.Nearby), len(h.Popular), len(h.AnimalFeed), len(h.Compost))
	}
	if h.Nearby[0].Merchant.Name != "Dapur Budi" && h.Nearby[0].Merchant.Name != "Dessert Rara" {
		t.Errorf("terdekat dari Rumah: %s", h.Nearby[0].Merchant.Name)
	}
	for i := 1; i < len(h.Popular); i++ {
		if h.Popular[i-1].SoldCount < h.Popular[i].SoldCount {
			t.Errorf("popular harus menurun menurut sold_count: %v", h.Popular[i].SoldCount)
		}
	}
	if len(h.OfferMerchants) == 0 {
		t.Error("toko dengan penawaran hari ini harus tampil")
	}
	// feed & compost mengikuti urutan daftar Kotlin: feed = l36, l6, l7, l19, l20
	if h.AnimalFeed[0].Title != "Ampas Kelapa Parut" || h.AnimalFeed[4].Title != "Dedak Padi Sisa Penggilingan" {
		t.Errorf("urutan feed: %s ... %s", h.AnimalFeed[0].Title, h.AnimalFeed[4].Title)
	}

	// halaman toko dan ulasan
	msvc := merchant.NewService(merchant.NewRepository(pool), lsvc, urls)
	pub, err := msvc.Public(ctx, sid("merchant:m1"), sid("user:budi"), merchant.PublicQuery{Lat: &lat, Lng: &lng})
	if err != nil || pub.Name != "Warung Bu Sari" || pub.RatingCount != 212 || *pub.RatingAvg != 4.9 || pub.BannerURL == nil || len(pub.Photos) != 5 ||
		pub.IsSaved == nil || !*pub.IsSaved || len(pub.Listings) != 4 {
		t.Errorf("m1: %+v %v", pub, err)
	}
	rsvc := review.NewService(pool, urls)
	revs, _, err := rsvc.ListByMerchant(ctx, sid("merchant:m1"), nil, 50)
	if err != nil || len(revs) != 8 { // r1-r3, r18-r21 dari dummy + rating order o2 milik Budi
		t.Errorf("ulasan m1: %d %v", len(revs), err)
	}
	for _, r := range revs {
		if r.BuyerName == "Pembeli" {
			t.Error("nama penulis kosong")
		}
	}
}
