package user

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/internal/testutil"
)

func TestImpact(t *testing.T) {
	pool := testutil.NewPool(t)
	fx := testutil.Fixtures{T: t, Pool: pool}
	ctx := context.Background()
	svc := NewService(NewRepository(pool), nil, nil, 0)

	buyer := fx.User("Budi")
	if got, err := svc.Impact(ctx, buyer); err != nil || *got != (ImpactDTO{Tier: "starter"}) {
		t.Fatalf("tanpa order harus nol: %+v %v", got, err)
	}

	merchant := fx.Merchant(fx.User("M"), "approved", -6.24, 106.79)
	meal := fx.Listing(merchant, testutil.ListingOpts{})
	feed := fx.Listing(merchant, testutil.ListingOpts{Tier: "animal_feed"})
	fx.Order(buyer, merchant, meal, "completed", time.Hour)
	fx.Order(buyer, merchant, feed, "completed", time.Hour)
	fx.Order(buyer, merchant, meal, "cancelled", 0) // tidak dihitung
	fx.Order(buyer, merchant, uuid.Nil, "ready", 0)

	got, err := svc.Impact(ctx, buyer)
	// tiap order: 2 unit hemat (10.000-5.000) x 2 = 10.000
	want := ImpactDTO{CompletedOrders: 2, Portions: 2, FeedCompostKg: 2, SavedRupiah: 20000, CarbonKg: 1, Tier: "starter"}
	if err != nil || *got != want {
		t.Errorf("impact = %+v, mau %+v (%v)", got, want, err)
	}
}
