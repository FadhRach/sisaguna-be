package chat

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/internal/testutil"
)

func TestChat(t *testing.T) {
	pool := testutil.NewPool(t)
	fx := testutil.Fixtures{T: t, Pool: pool}
	ctx := context.Background()
	svc := NewService(pool)

	owner := fx.User("Mitra")
	merchant := fx.Merchant(owner, "approved", -6.24, 106.79)
	buyer, stranger := fx.User("Budi"), fx.User("Orang Lain")
	listing := fx.Listing(merchant, testutil.ListingOpts{})
	otherListing := fx.Listing(fx.Merchant(fx.User("M2"), "approved", -6.25, 106.8), testutil.ListingOpts{})

	if _, err := svc.Start(ctx, owner, merchant); !errors.Is(err, ErrOwnMerchant) {
		t.Errorf("chat dengan toko sendiri: %v", err)
	}
	if _, err := svc.Start(ctx, buyer, fx.Merchant(fx.User("P"), "pending", -6.2, 106.8)); !errors.Is(err, ErrMerchantNotFound) {
		t.Errorf("toko pending: %v", err)
	}
	conv, err := svc.Start(ctx, buyer, merchant)
	again, _ := svc.Start(ctx, buyer, merchant)
	if err != nil || conv.ID != again.ID {
		t.Fatalf("start harus idempoten: %v %v", conv, err)
	}
	cid := uuid.MustParse(conv.ID)

	// percakapan kosong tidak muncul di inbox
	if rows, _, _ := svc.Inbox(ctx, owner, "merchant", nil, 10); len(rows) != 0 {
		t.Errorf("inbox mitra harus kosong sebelum ada pesan: %v", rows)
	}

	send := func(user uuid.UUID, body string, clientID uuid.UUID) (*MessageDTO, bool) {
		t.Helper()
		m, created, err := svc.Send(ctx, user, cid, SendRequest{ClientID: clientID, Body: body})
		if err != nil {
			t.Fatalf("send %q: %v", body, err)
		}
		return m, created
	}
	k1 := uuid.New()
	m1, created := send(buyer, "Masih ada?", k1)
	if !created || m1.SenderSide != "buyer" {
		t.Fatalf("pesan pertama: %+v created=%v", m1, created)
	}
	if dup, created := send(buyer, "Masih ada?", k1); created || dup.ID != m1.ID {
		t.Errorf("client_id sama harus mengembalikan pesan lama: %+v created=%v", dup, created)
	}
	send(owner, "Ada kak", uuid.New())
	send(owner, "Silakan datang", uuid.New())

	if _, _, err := svc.Send(ctx, stranger, cid, SendRequest{ClientID: uuid.New(), Body: "hai"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("orang luar mengirim: %v", err)
	}
	if _, _, err := svc.Send(ctx, buyer, cid, SendRequest{ClientID: uuid.New(), Body: "x", ListingID: &otherListing}); !errors.Is(err, ErrInvalidListing) {
		t.Errorf("produk toko lain: %v", err)
	}
	if m, _, err := svc.Send(ctx, buyer, cid, SendRequest{ClientID: uuid.New(), Body: "yang ini", ListingID: &listing}); err != nil || m.ListingID == nil {
		t.Errorf("produk toko ini: %+v %v", m, err)
	}

	// inbox: pembeli belum membaca 2 balasan, mitra belum membaca 2 pesan pembeli
	buyerBox, _, _ := svc.Inbox(ctx, buyer, "buyer", nil, 10)
	merchantBox, _, _ := svc.Inbox(ctx, owner, "merchant", nil, 10)
	if len(buyerBox) != 1 || buyerBox[0].Unread != 2 || buyerBox[0].Counterpart.Name == "" {
		t.Errorf("inbox pembeli: %+v", buyerBox)
	}
	if len(merchantBox) != 1 || merchantBox[0].Unread != 2 || merchantBox[0].Counterpart.Name != "Budi" || merchantBox[0].LastMessage.Body != "yang ini" {
		t.Errorf("inbox mitra: %+v", merchantBox)
	}
	if rows, _, _ := svc.Inbox(ctx, stranger, "buyer", nil, 10); len(rows) != 0 {
		t.Errorf("inbox orang luar: %v", rows)
	}

	// riwayat: terbaru dulu dipotong, hasil urut naik; before_id / after_id
	last, _, err := svc.Messages(ctx, buyer, cid, MessagesQuery{Limit: 2})
	if err != nil || len(last) != 2 || last[0].ID >= last[1].ID || last[1].Body != "yang ini" {
		t.Fatalf("pesan terbaru: %+v %v", last, err)
	}
	older, _, _ := svc.Messages(ctx, buyer, cid, MessagesQuery{BeforeID: &last[0].ID, Limit: 10})
	if len(older) != 2 || older[1].ID >= last[0].ID {
		t.Errorf("before_id: %+v", older)
	}
	newer, _, _ := svc.Messages(ctx, buyer, cid, MessagesQuery{AfterID: &m1.ID})
	if len(newer) != 3 || newer[0].ID <= m1.ID {
		t.Errorf("after_id: %+v", newer)
	}
	if _, _, err := svc.Messages(ctx, stranger, cid, MessagesQuery{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("orang luar membaca: %v", err)
	}

	// tanda baca: maju saja, tidak melewati pesan terakhir
	lastID := last[1].ID
	got, err := svc.MarkRead(ctx, buyer, cid, lastID+100)
	if err != nil || got != lastID {
		t.Errorf("dibatasi sampai pesan terakhir: %d %v", got, err)
	}
	if got, _ := svc.MarkRead(ctx, buyer, cid, 1); got != lastID {
		t.Errorf("tidak boleh mundur: %d", got)
	}
	if rows, _, _ := svc.Inbox(ctx, buyer, "buyer", nil, 10); rows[0].Unread != 0 {
		t.Errorf("unread pembeli harus 0: %+v", rows[0])
	}
	if _, peerRead, _ := svc.Messages(ctx, owner, cid, MessagesQuery{}); peerRead != lastID {
		t.Errorf("mitra melihat tanda baca pembeli: %d", peerRead)
	}
	if peer, ok := svc.Peer(ctx, buyer, cid); !ok || peer != owner {
		t.Errorf("peer pembeli = %v %v", peer, ok)
	}
	if _, ok := svc.Peer(ctx, stranger, cid); ok {
		t.Error("orang luar bukan peserta")
	}
}
