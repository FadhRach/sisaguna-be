package review

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/internal/media"
	pmedia "github.com/FadhRach/sisaguna-be/internal/platform/media"
	"github.com/FadhRach/sisaguna-be/internal/testutil"
)

func TestReview(t *testing.T) {
	pool := testutil.NewPool(t)
	fx := testutil.Fixtures{T: t, Pool: pool}
	ctx := context.Background()
	svc := NewService(pool, pmedia.URLs{Base: "http://api.test"})

	owner := fx.User("Mitra")
	merchant := fx.Merchant(owner, "approved", -6.24, 106.79)
	buyer, other := fx.User("Budi Santoso"), fx.User("Ani")

	ready := fx.Order(buyer, merchant, uuid.Nil, "ready", 0)
	if _, err := svc.Create(ctx, buyer, ready, CreateRequest{Stars: 5}); !errors.Is(err, ErrNotCompleted) {
		t.Errorf("order belum selesai: %v", err)
	}
	done := fx.Order(buyer, merchant, uuid.Nil, "completed", time.Hour)
	if _, err := svc.Create(ctx, other, done, CreateRequest{Stars: 5}); !errors.Is(err, ErrOrderNotFound) {
		t.Errorf("bukan pembeli order: %v", err)
	}
	old := fx.Order(buyer, merchant, uuid.Nil, "completed", 8*24*time.Hour)
	if _, err := svc.Create(ctx, buyer, old, CreateRequest{Stars: 5}); !errors.Is(err, ErrWindowClosed) {
		t.Errorf("lewat 7 hari: %v", err)
	}
	if _, err := svc.Create(ctx, buyer, done, CreateRequest{Stars: 5, MediaIDs: []uuid.UUID{fx.PrivateMedia(buyer)}}); !errors.Is(err, media.ErrInvalidMedia) {
		t.Errorf("foto privat tidak boleh di ulasan: %v", err)
	}

	photo := fx.FreeMedia(buyer)
	r, err := svc.Create(ctx, buyer, done, CreateRequest{Stars: 5, Tags: []string{" Porsi pas "}, Comment: "Enak!", MediaIDs: []uuid.UUID{photo}})
	if err != nil || r.Tags[0] != "Porsi pas" {
		t.Fatalf("create: %+v %v", r, err)
	}
	if _, err := svc.Create(ctx, buyer, done, CreateRequest{Stars: 1}); !errors.Is(err, ErrAlreadyReviewed) {
		t.Errorf("ulasan kedua: %v", err)
	}
	// gagal di tengah (foto salah) tidak boleh meninggalkan ulasan setengah jadi
	var n int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM reviews`).Scan(&n)
	if n != 1 {
		t.Errorf("jumlah ulasan = %d, mau 1", n)
	}

	second := fx.Order(other, merchant, uuid.Nil, "completed", time.Hour)
	if _, err := svc.Create(ctx, other, second, CreateRequest{Stars: 4}); err != nil {
		t.Fatal(err)
	}
	var avg float64
	var count int
	_ = pool.QueryRow(ctx, `SELECT rating_avg, rating_count FROM merchants WHERE id=$1`, merchant).Scan(&avg, &count)
	if avg != 4.5 || count != 2 {
		t.Errorf("rating toko = %v dari %d, mau 4.5 dari 2", avg, count)
	}
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id=$1 AND title='Ulasan baru'`, owner).Scan(&n)
	if n != 2 {
		t.Errorf("notifikasi ke pemilik toko = %d, mau 2", n)
	}

	items, next, err := svc.ListByMerchant(ctx, merchant, nil, 1)
	if err != nil || len(items) != 1 || next == "" {
		t.Fatalf("halaman 1: %v %q %v", items, next, err)
	}
	if items[0].BuyerName != "Ani" {
		t.Errorf("terbaru dulu, got %q", items[0].BuyerName)
	}
	_ = next
	all, _, _ := svc.ListByMerchant(ctx, merchant, nil, 10)
	if len(all) != 2 || all[1].BuyerName != "Budi S." || len(all[1].Photos) != 1 || all[1].Photos[0].ThumbURL == nil {
		t.Errorf("nama depan + foto: %+v", all)
	}

	pending := fx.Merchant(fx.User("P"), "pending", -6.2, 106.8)
	if _, _, err := svc.ListByMerchant(ctx, pending, nil, 10); !errors.Is(err, ErrMerchantMissing) {
		t.Errorf("toko pending: %v", err)
	}
}

func TestDisplayName(t *testing.T) {
	for in, want := range map[string]string{"Budi Santoso": "Budi S.", "Dinda P.": "Dinda P.", "Pak Darto": "Pak D.", "Komunitas Kebun Kita": "Komunitas Kebun K.",
		"Ani": "Ani", "  ": "Pembeli", "Kebun Warga RW 04": "Kebun Warga RW 04"} {
		if got := displayName(in); got != want {
			t.Errorf("%q -> %q, mau %q", in, got, want)
		}
	}
}
