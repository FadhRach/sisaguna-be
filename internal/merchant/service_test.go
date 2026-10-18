package merchant

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/internal/listing"
	"github.com/FadhRach/sisaguna-be/internal/media"
	pmedia "github.com/FadhRach/sisaguna-be/internal/platform/media"
	"github.com/FadhRach/sisaguna-be/internal/testutil"
)

func fp(v float64) *float64 { return &v }
func sp(v string) *string   { return &v }

func TestMerchantLifecycle(t *testing.T) {
	pool := testutil.NewPool(t)
	fx := testutil.Fixtures{T: t, Pool: pool}
	ctx := context.Background()
	listings := listing.NewService(listing.NewRepository(pool), pmedia.URLs{Base: "http://api.test"})
	s := NewService(NewRepository(pool), listings, pmedia.URLs{Base: "http://api.test"})
	owner := fx.User("Mitra")

	m, err := s.Create(ctx, owner, CreateRequest{Name: " Warung A ", Address: "Jl. A", Lat: fp(-6.24), Lng: fp(106.80), Allergens: []string{"peanut", "egg"}})
	if err != nil {
		t.Fatal(err)
	}
	if m.Status != "pending" || m.Name != "Warung A" || m.Halal != "unverified" || m.PrepMinutes != 15 || len(m.Allergens) != 2 || m.Lat != -6.24 {
		t.Errorf("%+v", m)
	}
	if _, err := s.Create(ctx, owner, CreateRequest{Name: "Dua", Address: "x", Lat: fp(1), Lng: fp(1)}); !errors.Is(err, ErrExists) {
		t.Errorf("second store: %v", err)
	}
	if _, err := s.Public(ctx, mustID(t, m.ID), uuid.Nil, PublicQuery{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("pending store must not be public: %v", err)
	}

	t.Run("update is partial; offer can be set and cleared; lat/lng come in pairs", func(t *testing.T) {
		offer := "Nasi gratis"
		u, err := s.Update(ctx, owner, UpdateRequest{About: sp("Masakan rumahan"), TodaysOffer: &offer})
		if err != nil || u.About != "Masakan rumahan" || u.TodaysOffer == nil || *u.TodaysOffer != offer || u.Name != "Warung A" {
			t.Fatalf("%+v %v", u, err)
		}
		empty := ""
		if u, _ = s.Update(ctx, owner, UpdateRequest{TodaysOffer: &empty}); u.TodaysOffer != nil {
			t.Error("empty string must clear the offer")
		}
		if _, err := s.Update(ctx, owner, UpdateRequest{Lat: fp(1)}); !errors.Is(err, ErrPartialPoint) {
			t.Errorf("partial point: %v", err)
		}
		if _, err := s.Update(ctx, fx.User("Tanpa Toko"), UpdateRequest{About: sp("x")}); !errors.Is(err, ErrNotFound) {
			t.Errorf("no store: %v", err)
		}
	})

	t.Run("moving the store moves its listings (feed distance follows)", func(t *testing.T) {
		id := mustID(t, m.ID)
		pool.Exec(ctx, `UPDATE merchants SET status='approved' WHERE id=$1`, id)
		l := fx.Listing(id, testutil.ListingOpts{})
		if _, err := s.Update(ctx, owner, UpdateRequest{Lat: fp(-6.30), Lng: fp(106.90)}); err != nil {
			t.Fatal(err)
		}
		var lat, lng float64
		pool.QueryRow(ctx, `SELECT ST_Y(location::geometry), ST_X(location::geometry) FROM listings WHERE id=$1`, l).Scan(&lat, &lng)
		if lat != -6.30 || lng != 106.90 {
			t.Errorf("listing location = %v,%v", lat, lng)
		}
		pub, err := s.Public(ctx, id, uuid.Nil, PublicQuery{Lat: fp(-6.30), Lng: fp(106.90)})
		if err != nil || len(pub.Listings) != 1 || pub.DistanceKm == nil || *pub.DistanceKm > 0.1 {
			t.Errorf("public: %+v %v", pub, err)
		}
	})
}

func mustID(t *testing.T, s string) (id uuid.UUID) {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestMerchantMediaBreakdownAndList(t *testing.T) {
	pool := testutil.NewPool(t)
	fx := testutil.Fixtures{T: t, Pool: pool}
	ctx := context.Background()
	urls := pmedia.URLs{Base: "http://api.test"}
	lsvc := listing.NewService(listing.NewRepository(pool), urls)
	s := NewService(NewRepository(pool), lsvc, urls)

	owner := fx.User("Mitra")
	id := fx.Merchant(owner, "approved", -6.2433, 106.7989)
	banner, p1, p2 := fx.FreeMedia(owner), fx.FreeMedia(owner), fx.FreeMedia(owner)

	// banner dan galeri
	str := func(u uuid.UUID) *uuid.UUID { return &u }
	m, err := s.Update(ctx, owner, UpdateRequest{BannerMediaID: str(banner), PhotoMediaIDs: []uuid.UUID{p1, p2}})
	if err != nil || m.BannerURL == nil || len(m.Photos) != 2 {
		t.Fatalf("update media: %+v %v", m, err)
	}
	if _, err := s.Update(ctx, owner, UpdateRequest{BannerMediaID: str(fx.PrivateMedia(owner))}); !errors.Is(err, media.ErrInvalidMedia) {
		t.Errorf("banner privat harus ditolak: %v", err)
	}
	m, _ = s.Update(ctx, owner, UpdateRequest{PhotoMediaIDs: []uuid.UUID{}})
	if len(m.Photos) != 0 {
		t.Errorf("galeri dikosongkan: %+v", m.Photos)
	}

	// histogram bintang
	buyer := fx.User("Pembeli")
	for _, stars := range []int{5, 5, 4, 1} {
		o := fx.Order(buyer, id, uuid.Nil, "completed", time.Hour)
		pool.Exec(ctx, `INSERT INTO reviews (order_id, merchant_id, buyer_id, stars) VALUES ($1,$2,$3,$4)`, o, id, buyer, stars)
	}
	pub, err := s.Public(ctx, id, uuid.Nil, PublicQuery{})
	if err != nil || pub.RatingBreakdown["5"] != 2 || pub.RatingBreakdown["4"] != 1 || pub.RatingBreakdown["1"] != 1 || pub.RatingBreakdown["3"] != 0 {
		t.Errorf("breakdown: %+v %v", pub, err)
	}

	// daftar toko: jumlah listing yang tampil, toko sendiri dan toko pending tidak ikut
	other := fx.Merchant(fx.User("Mitra2"), "approved", -6.30, 106.90)
	fx.Listing(other, testutil.ListingOpts{})
	fx.Listing(other, testutil.ListingOpts{})
	fx.Merchant(fx.User("Mitra3"), "pending", -6.2, 106.8)
	cards, err := s.List(ctx, owner, ListQuery{Lat: fp(-6.2433), Lng: fp(106.7989)})
	if err != nil || len(cards) != 1 || cards[0].ID != other.String() || cards[0].AvailableCount != 2 || cards[0].NextPickupEnd == nil || cards[0].DistanceKm == nil {
		t.Errorf("list: %+v %v", cards, err)
	}
	all, _ := s.List(ctx, uuid.Nil, ListQuery{})
	onlyAvail, _ := s.List(ctx, uuid.Nil, ListQuery{Available: true})
	if len(all) != 2 || len(onlyAvail) != 1 {
		t.Errorf("anonim: semua=%d tersedia=%d", len(all), len(onlyAvail))
	}
}

func TestNewMerchantNotifiesAdminsOrAutoApproves(t *testing.T) {
	pool := testutil.NewPool(t)
	fx := testutil.Fixtures{T: t, Pool: pool}
	ctx := context.Background()
	listings := listing.NewService(listing.NewRepository(pool), pmedia.URLs{Base: "http://api.test"})
	s := NewService(NewRepository(pool), listings, pmedia.URLs{Base: "http://api.test"})
	admin := fx.User("Admin")
	if _, err := pool.Exec(ctx, `UPDATE users SET is_admin = true WHERE id = $1`, admin); err != nil {
		t.Fatal(err)
	}
	count := func() (n int) {
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'merchant'`, admin).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return
	}

	m, err := s.Create(ctx, fx.User("Mitra A"), CreateRequest{Name: "Warung A", Address: "Jl. A", Lat: fp(-6.24), Lng: fp(106.80)})
	if err != nil || m.Status != "pending" {
		t.Fatalf("create: %v %+v", err, m)
	}
	if count() != 1 {
		t.Errorf("admin harus dapat 1 notifikasi, dapat %d", count())
	}

	s.WithAutoApprove(func(ctx context.Context, id uuid.UUID) error {
		_, err := pool.Exec(ctx, `UPDATE merchants SET status = 'approved' WHERE id = $1`, id)
		return err
	})
	m, err = s.Create(ctx, fx.User("Mitra B"), CreateRequest{Name: "Warung B", Address: "Jl. B", Lat: fp(-6.25), Lng: fp(106.81)})
	if err != nil || m.Status != "approved" {
		t.Fatalf("auto approve: %v %+v", err, m)
	}
	if count() != 1 {
		t.Errorf("toko auto-approve tidak perlu memanggil admin, notifikasi = %d", count())
	}
}
