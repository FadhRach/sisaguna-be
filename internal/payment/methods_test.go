package payment

import (
	"context"
	"errors"
	"testing"

	"github.com/FadhRach/sisaguna-be/internal/testutil"
)

func TestMaskPhone(t *testing.T) {
	for in, want := range map[string]string{"081298767890": "0812 •••• 7890", "0812-9876-7890": "0812 •••• 7890", "1234567": "1234567", "": ""} {
		if got := MaskPhone(in); got != want {
			t.Errorf("%q -> %q, mau %q", in, got, want)
		}
	}
}

// Perilaku sama dengan FakePaymentMethodRepository di app Android.
func TestPaymentMethods(t *testing.T) {
	pool := testutil.NewPool(t)
	fx := testutil.Fixtures{T: t, Pool: pool}
	ctx := context.Background()
	svc := NewService(pool)
	user := fx.User("Budi")

	kinds := func(ms []MethodDTO) (out []string) {
		for _, m := range ms {
			out = append(out, m.Kind)
		}
		return
	}
	ms, err := svc.List(ctx, user)
	if err != nil || len(ms) != 2 || ms[0].Kind != "qris" || !ms[0].IsDefault || ms[1].Kind != "cash" {
		t.Fatalf("awal: %+v %v", ms, err)
	}
	ms, err = svc.Link(ctx, user, LinkRequest{Kind: "ovo", Phone: "081298765432"})
	if err != nil || len(ms) != 3 || kinds(ms)[1] != "ovo" || ms[1].Detail != "0812 •••• 5432" || ms[2].Kind != "cash" { // tunai selalu terakhir
		t.Fatalf("link ovo: %+v %v", ms, err)
	}
	ms, _ = svc.Link(ctx, user, LinkRequest{Kind: "gopay", Phone: "081200007890"})
	if got := kinds(ms); got[0] != "qris" || got[1] != "ovo" || got[2] != "gopay" || got[3] != "cash" {
		t.Errorf("urutan tautan: %v", got)
	}
	if _, err := svc.Link(ctx, user, LinkRequest{Kind: "dana", Phone: "1234"}); !errors.Is(err, ErrInvalidPhone) {
		t.Errorf("nomor pendek: %v", err)
	}
	if ms, err = svc.SetDefault(ctx, user, "ovo"); err != nil || !ms[1].IsDefault || ms[0].IsDefault {
		t.Errorf("default ovo: %+v %v", ms, err)
	}
	if _, err := svc.SetDefault(ctx, user, "dana"); !errors.Is(err, ErrNotFound) {
		t.Errorf("default metode yang belum ada: %v", err)
	}
	if _, err := svc.Unlink(ctx, user, "qris"); !errors.Is(err, ErrFixed) {
		t.Errorf("lepas QRIS: %v", err)
	}
	if _, err := svc.Unlink(ctx, user, "cash"); !errors.Is(err, ErrFixed) {
		t.Errorf("lepas tunai: %v", err)
	}
	ms, err = svc.Unlink(ctx, user, "ovo") // yang default dilepas -> QRIS kembali default
	if err != nil || len(ms) != 3 || !ms[0].IsDefault || ms[0].Kind != "qris" {
		t.Errorf("lepas default: %+v %v", ms, err)
	}
	if _, err := svc.Unlink(ctx, user, "ovo"); err != nil {
		t.Errorf("lepas dua kali harus idempoten: %v", err)
	}
}
