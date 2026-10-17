package notification

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/testutil"
)

func TestCreateRespectsPreferences(t *testing.T) {
	pool := testutil.NewPool(t)
	fx := testutil.Fixtures{T: t, Pool: pool}
	ctx := context.Background()
	q := sqlc.New(pool)
	svc := NewService(pool)

	count := func(user any) (n int) {
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id=$1`, user).Scan(&n)
		return
	}
	user := fx.User("U")
	n := New{UserID: user, Type: sqlc.NotificationTypeOrder, Title: "t", Body: "b"}
	if err := Create(ctx, q, n); err != nil || count(user) != 1 {
		t.Fatalf("default prefs should allow order notifications: %v", err)
	}
	n.Type = sqlc.NotificationTypePromo
	if err := Create(ctx, q, n); err != nil || count(user) != 1 {
		t.Errorf("promo is off by default, count=%d err=%v", count(user), err)
	}
	pool.Exec(ctx, `UPDATE notification_prefs SET "order" = false WHERE user_id=$1`, user)
	n.Type = sqlc.NotificationTypeOrder
	if err := Create(ctx, q, n); err != nil || count(user) != 1 {
		t.Errorf("order switched off, count=%d err=%v", count(user), err)
	}
	pool.Exec(ctx, `UPDATE notification_prefs SET "order" = true, enabled = false WHERE user_id=$1`, user)
	if err := Create(ctx, q, n); err != nil || count(user) != 1 {
		t.Errorf("master switch off, count=%d err=%v", count(user), err)
	}

	pool.Exec(ctx, `UPDATE notification_prefs SET enabled = true WHERE user_id=$1`, user)
	_ = Create(ctx, q, n)
	items, next, err := svc.List(ctx, user, "", nil, 1)
	if err != nil || len(items) != 1 || next == "" {
		t.Fatalf("page 1: %v %q %v", items, next, err)
	}
	if err := svc.MarkRead(ctx, user, mustParse(t, items[0].ID)); err != nil {
		t.Fatal(err)
	}
	if err := svc.MarkRead(ctx, fx.User("Other"), mustParse(t, items[0].ID)); err != ErrNotFound {
		t.Errorf("foreign mark read: %v", err)
	}
}

func mustParse(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestDeleteRestoreAndPrefs(t *testing.T) {
	pool := testutil.NewPool(t)
	fx := testutil.Fixtures{T: t, Pool: pool}
	ctx := context.Background()
	svc := NewService(pool)
	user, other := fx.User("U"), fx.User("Lain")

	// default: saklar promo mati, sisanya hidup
	p, err := svc.Prefs(ctx, user)
	if err != nil || !p.Enabled || p.Promo || !p.Order {
		t.Fatalf("default prefs: %+v %v", p, err)
	}
	on, off := true, false
	p, err = svc.SetPrefs(ctx, user, PrefsRequest{Enabled: &on, Pickup: &on, Order: &off, Merchant: &on, Promo: &on, Impact: &off})
	if err != nil || p.Order || !p.Promo || p.Impact {
		t.Fatalf("set prefs: %+v %v", p, err)
	}

	q := sqlc.New(pool)
	if err := Create(ctx, q, New{UserID: user, Type: sqlc.NotificationTypePromo, Title: "Diskon", Body: "b"}); err != nil {
		t.Fatal(err)
	}
	items, _, _ := svc.List(ctx, user, "", nil, 10)
	if len(items) != 1 {
		t.Fatalf("promo sekarang diizinkan: %+v", items)
	}
	id := mustParse(t, items[0].ID)
	if err := svc.Delete(ctx, other, id); err != ErrNotFound {
		t.Errorf("hapus milik orang lain: %v", err)
	}
	if err := svc.Delete(ctx, user, id); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, user, id); err != ErrNotFound {
		t.Errorf("hapus dua kali: %v", err)
	}
	if items, _, _ := svc.List(ctx, user, "", nil, 10); len(items) != 0 {
		t.Errorf("terhapus tidak boleh tampil: %+v", items)
	}
	if err := svc.Restore(ctx, user, id); err != nil {
		t.Fatal(err)
	}
	if err := svc.Restore(ctx, user, id); err != ErrNotFound {
		t.Errorf("pulihkan yang tidak terhapus: %v", err)
	}
	if items, _, _ := svc.List(ctx, user, "", nil, 10); len(items) != 1 {
		t.Errorf("setelah dipulihkan: %+v", items)
	}
}

func TestDeviceTokens(t *testing.T) {
	pool := testutil.NewPool(t)
	fx := testutil.Fixtures{T: t, Pool: pool}
	ctx := context.Background()
	svc := NewService(pool)
	a, b := fx.User("A"), fx.User("B")
	tok := "fcm-token-fcm-token-fcm-token-1234"

	owner := func() (id uuid.UUID) {
		_ = pool.QueryRow(ctx, `SELECT user_id FROM device_tokens WHERE token=$1`, tok).Scan(&id)
		return
	}
	for range 2 { // idempoten
		if err := svc.RegisterDevice(ctx, a, DeviceRequest{Token: tok, Platform: "android"}); err != nil {
			t.Fatal(err)
		}
	}
	if owner() != a {
		t.Fatal("token harus milik A")
	}
	_ = svc.RegisterDevice(ctx, b, DeviceRequest{Token: tok, Platform: "ios"}) // HP yang sama, login akun lain
	if owner() != b {
		t.Error("token harus pindah ke B")
	}
	_ = svc.RemoveDevice(ctx, a, tok) // bukan pemilik: tidak menghapus
	if owner() != b {
		t.Error("A tidak boleh menghapus token milik B")
	}
	_ = svc.RemoveDevice(ctx, b, tok)
	if owner() != uuid.Nil {
		t.Error("token harus terhapus")
	}
}
