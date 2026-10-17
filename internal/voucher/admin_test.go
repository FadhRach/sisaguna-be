package voucher

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	pmedia "github.com/FadhRach/sisaguna-be/internal/platform/media"
	"github.com/FadhRach/sisaguna-be/internal/testutil"
)

func TestAdminVouchers(t *testing.T) {
	pool := testutil.NewPool(t)
	fx := testutil.Fixtures{T: t, Pool: pool}
	ctx := context.Background()
	svc := NewService(pool, pmedia.URLs{Base: "http://api.test"})
	ip := func(v int) *int { return &v }
	soon := time.Now().Add(24 * time.Hour)

	base := func() AdminCreateRequest {
		return AdminCreateRequest{Code: "HEMAT15", Title: "Hemat 15%", PercentOff: ip(15), MaxDiscount: ip(10000), MinSpend: 20000, ExpiresAt: soon, Quota: ip(2)}
	}
	if err := svc.Create(ctx, base()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Create(ctx, base()); !errors.Is(err, ErrCodeTaken) {
		t.Errorf("kode ganda: %v", err)
	}

	bad := map[string]func(r *AdminCreateRequest){
		"persen dan flat":      func(r *AdminCreateRequest) { r.FlatOff = ip(5000) },
		"tanpa diskon":         func(r *AdminCreateRequest) { r.PercentOff = nil },
		"max_discount di flat": func(r *AdminCreateRequest) { r.PercentOff, r.FlatOff = nil, ip(5000) },
		"persen 101":           func(r *AdminCreateRequest) { r.PercentOff = ip(101) },
		"kadaluwarsa lampau":   func(r *AdminCreateRequest) { r.ExpiresAt = time.Now().Add(-time.Hour) },
		"kuota 0":              func(r *AdminCreateRequest) { r.Quota = ip(0) },
	}
	for name, mutate := range bad {
		r := base()
		r.Code = "BAD" + name[:2]
		mutate(&r)
		if err := svc.Create(ctx, r); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	ghost := uuid.New()
	r := base()
	r.Code, r.MerchantID = "GHOST1", &ghost
	if err := svc.Create(ctx, r); !errors.Is(err, ErrMerchantNotFound) {
		t.Errorf("toko tidak ada: %v", err)
	}
	merchant := fx.Merchant(fx.User("M"), "approved", -6.2, 106.8)
	r = base()
	r.Code, r.MerchantID, r.AppliesToTier, r.PercentOff, r.MaxDiscount, r.FlatOff = "TOKO5K", &merchant, "human", nil, nil, ip(5000)
	if err := svc.Create(ctx, r); err != nil {
		t.Fatalf("voucher toko: %v", err)
	}

	// pembeli bisa mengklaim; kuota 2 -> pemakaian dihitung di checkout, di sini cukup klaim
	buyer := fx.User("Budi")
	if err := svc.Claim(ctx, buyer, "HEMAT15"); err != nil {
		t.Fatal(err)
	}

	pool.Exec(ctx, `UPDATE vouchers SET used_count = 2 WHERE code='HEMAT15'`)
	if err := svc.Update(ctx, "hemat15", AdminUpdateRequest{Quota: ip(1)}); !errors.Is(err, ErrInvalid) {
		t.Errorf("kuota di bawah terpakai: %v", err)
	}
	off := false
	title := "Hemat Besar"
	if err := svc.Update(ctx, "hemat15", AdminUpdateRequest{IsActive: &off, Title: &title, Terms: []string{"Tunai saja"}}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(ctx, "HEMAT15")
	if err != nil || got.IsActive || got.Title != "Hemat Besar" || len(got.Terms) != 1 || got.PercentOff == nil || *got.PercentOff != 15 {
		t.Fatalf("setelah update: %+v %v", got, err)
	}
	if err := svc.Claim(ctx, fx.User("Lain"), "HEMAT15"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("voucher nonaktif tidak bisa diklaim: %v", err)
	}
	if err := svc.Update(ctx, "NOPE", AdminUpdateRequest{IsActive: &off}); !errors.Is(err, ErrNotFound) {
		t.Errorf("kode tidak ada: %v", err)
	}

	rows, total, err := svc.AdminList(ctx, 1, 1)
	if err != nil || total != 2 || len(rows) != 1 {
		t.Errorf("list: %d baris dari %d (%v)", len(rows), total, err)
	}
}
