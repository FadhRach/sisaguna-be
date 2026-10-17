package saved

import (
	"context"
	"errors"
	"testing"

	pmedia "github.com/FadhRach/sisaguna-be/internal/platform/media"
	"github.com/FadhRach/sisaguna-be/internal/testutil"
)

func TestSavedMerchants(t *testing.T) {
	pool := testutil.NewPool(t)
	fx := testutil.Fixtures{T: t, Pool: pool}
	ctx := context.Background()
	svc := NewService(pool, pmedia.URLs{Base: "http://api.test"})

	buyer := fx.User("Budi")
	a := fx.Merchant(fx.User("A"), "approved", -6.24, 106.79)
	pending := fx.Merchant(fx.User("P"), "pending", -6.24, 106.79)

	if err := svc.Save(ctx, buyer, pending); !errors.Is(err, ErrMerchantNotFound) {
		t.Errorf("toko pending: %v", err)
	}
	for range 2 { // PUT idempoten
		if err := svc.Save(ctx, buyer, a); err != nil {
			t.Fatal(err)
		}
	}
	lat, lng := -6.30, 106.90
	rows, err := svc.List(ctx, buyer, ListQuery{Lat: &lat, Lng: &lng})
	if err != nil || len(rows) != 1 || rows[0].DistanceKm == nil || *rows[0].DistanceKm <= 0 {
		t.Fatalf("list: %+v %v", rows, err)
	}
	if rows, _ := svc.List(ctx, buyer, ListQuery{}); rows[0].DistanceKm != nil {
		t.Error("tanpa lat/lng jarak harus null")
	}
	// toko yang kemudian ditangguhkan hilang dari daftar
	pool.Exec(ctx, `UPDATE merchants SET status='suspended' WHERE id=$1`, a)
	if rows, _ := svc.List(ctx, buyer, ListQuery{}); len(rows) != 0 {
		t.Errorf("toko non-approved harus disembunyikan: %+v", rows)
	}
	pool.Exec(ctx, `UPDATE merchants SET status='approved' WHERE id=$1`, a)
	for range 2 { // DELETE idempoten
		if err := svc.Remove(ctx, buyer, a); err != nil {
			t.Fatal(err)
		}
	}
	if rows, _ := svc.List(ctx, buyer, ListQuery{}); len(rows) != 0 {
		t.Errorf("setelah dihapus: %+v", rows)
	}
}
