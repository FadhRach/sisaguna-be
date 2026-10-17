package complaint

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

func TestComplaintFlow(t *testing.T) {
	pool := testutil.NewPool(t)
	fx := testutil.Fixtures{T: t, Pool: pool}
	ctx := context.Background()
	urls := pmedia.URLs{Base: "http://api.test"}
	svc := NewService(pool, urls)

	owner := fx.User("Mitra")
	merchant := fx.Merchant(owner, "approved", -6.24, 106.79)
	buyer, stranger := fx.User("Budi"), fx.User("Orang Lain")
	detail := CreateRequest{Reason: "not_fresh", Detail: "Nasinya sudah berbau dan basi."}

	if _, err := svc.Create(ctx, buyer, fx.Order(buyer, merchant, uuid.Nil, "ready", 0), detail); !errors.Is(err, ErrNotCompleted) {
		t.Errorf("belum selesai: %v", err)
	}
	if _, err := svc.Create(ctx, buyer, fx.Order(buyer, merchant, uuid.Nil, "completed", 25*time.Hour), detail); !errors.Is(err, ErrWindowClosed) {
		t.Errorf("lewat 24 jam: %v", err)
	}
	done := fx.Order(buyer, merchant, uuid.Nil, "completed", time.Hour)
	if _, err := svc.Create(ctx, stranger, done, detail); !errors.Is(err, ErrOrderNotFound) {
		t.Errorf("bukan pembeli: %v", err)
	}
	bad := detail
	bad.MediaIDs = []uuid.UUID{fx.FreeMedia(buyer)} // foto publik tidak boleh dipakai komplain
	if _, err := svc.Create(ctx, buyer, done, bad); !errors.Is(err, media.ErrInvalidMedia) {
		t.Errorf("foto publik: %v", err)
	}

	photo := fx.PrivateMedia(buyer)
	detail.MediaIDs = []uuid.UUID{photo}
	c, err := svc.Create(ctx, buyer, done, detail)
	if err != nil || c.Status != "submitted" || len(c.PhotoURLs) != 1 {
		t.Fatalf("create: %+v %v", c, err)
	}
	if _, err := svc.Create(ctx, buyer, done, detail); !errors.Is(err, ErrAlreadyComplaint) {
		t.Errorf("komplain kedua: %v", err)
	}

	// foto privat: pemilik, pemilik toko yang dikomplain, dan admin boleh; orang lain tidak
	ms := media.NewService(media.NewRepository(pool), pmedia.NewDisk(t.TempDir()), urls)
	for name, tc := range map[string]struct {
		user  uuid.UUID
		admin bool
		ok    bool
	}{"pemilik": {buyer, false, true}, "toko": {owner, false, true}, "admin": {stranger, true, true}, "orang lain": {stranger, false, false}} {
		_, err := ms.PrivateFile(ctx, tc.user, tc.admin, photo)
		if (err == nil) != tc.ok {
			t.Errorf("%s: err=%v, ok seharusnya %v", name, err, tc.ok)
		}
	}

	// admin
	list, total, err := svc.AdminList(ctx, "submitted", 1, 10)
	if err != nil || total != 1 || len(list) != 1 || list[0].MerchantName == "" || len(list[0].PhotoURLs) != 1 {
		t.Fatalf("admin list: %+v total=%d err=%v", list, total, err)
	}
	id := uuid.MustParse(c.ID)
	if err := svc.Update(ctx, id, UpdateRequest{Status: "resolved"}); !errors.Is(err, ErrNoteRequired) {
		t.Errorf("status akhir tanpa catatan: %v", err)
	}
	if err := svc.Update(ctx, id, UpdateRequest{Status: "reviewing"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Update(ctx, id, UpdateRequest{Status: "resolved", ResolutionNote: "Toko sudah dihubungi."}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Update(ctx, id, UpdateRequest{Status: "reviewing"}); !errors.Is(err, ErrClosed) {
		t.Errorf("komplain tertutup: %v", err)
	}
	if err := svc.Update(ctx, uuid.New(), UpdateRequest{Status: "reviewing"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("id acak: %v", err)
	}
	var resolvedAt *time.Time
	var notifs int
	_ = pool.QueryRow(ctx, `SELECT resolved_at FROM complaints WHERE id=$1`, id).Scan(&resolvedAt)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id=$1 AND title='Update komplain'`, buyer).Scan(&notifs)
	if resolvedAt == nil || notifs != 2 {
		t.Errorf("resolved_at=%v notifikasi pembeli=%d (mau 2)", resolvedAt, notifs)
	}
}
