package order

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/internal/testutil"
)

// Pengiriman dan pembayaran non-tunai tiruan (PARTNERS_MODE=dummy).
func TestDummyPartners(t *testing.T) {
	e := setup(t)
	e.svc.WithPartners(true)
	delivery := func(l uuid.UUID, merchant uuid.UUID, qty int, courier, speed string) PlaceRequest {
		return PlaceRequest{QuoteRequest: QuoteRequest{MerchantID: merchant, Lines: []LineInput{line(l, qty)}, Fulfillment: "delivery", Courier: courier, Speed: speed}, PaymentKind: "cash"}
	}

	t.Run("quote menawarkan 4 kurir termurah dulu; total pickup tidak berubah", func(t *testing.T) {
		s := e.store()
		e.fx.Delivery(s.merchant, true)
		l := e.fx.Listing(s.merchant, testutil.ListingOpts{Price: 10000, Orig: 20000, Stock: 10})
		buyer := e.fx.User("Pembeli")
		e.fx.Address(buyer, -6.2433, 106.8300) // ~3 km dari toko (106.7989)

		q, err := e.svc.Quote(e.ctx, buyer, QuoteRequest{MerchantID: s.merchant, Lines: []LineInput{line(l, 2)}})
		if err != nil || q.Delivery == nil || !q.Delivery.Available || len(q.Delivery.Options) != 4 || q.Delivery.Selected != nil {
			t.Fatalf("quote: %+v %v", q.Delivery, err)
		}
		for i := 1; i < 4; i++ {
			if q.Delivery.Options[i-1].Payable > q.Delivery.Options[i].Payable {
				t.Errorf("urutan ongkir: %+v", q.Delivery.Options)
			}
		}
		if q.Total != 21000 || q.Fulfillment != "pickup" || q.PickupBy == nil {
			t.Errorf("pickup quote: total=%d %s", q.Total, q.Fulfillment)
		}
		if got := q.PaymentKinds; len(got) != 5 {
			t.Errorf("payment kinds: %v", got)
		}
	})

	t.Run("order pengiriman: ongkir masuk total, komisi hanya dari makanan, pickup_by kosong", func(t *testing.T) {
		s := e.store()
		e.fx.Delivery(s.merchant, true)
		l := e.fx.Listing(s.merchant, testutil.ListingOpts{Price: 10000, Orig: 20000, Stock: 10})
		buyer := e.fx.User("Pembeli")
		e.fx.Address(buyer, -6.2433, 106.8300)

		q, _ := e.svc.Quote(e.ctx, buyer, delivery(l, s.merchant, 2, "grab", "standard").QuoteRequest)
		if q.Delivery.Selected == nil || q.Delivery.Selected.Courier != "grab" || q.Fulfillment != "delivery" {
			t.Fatalf("selected: %+v", q.Delivery)
		}
		fee := q.Delivery.Selected.Payable
		req := delivery(l, s.merchant, 2, "grab", "standard")
		wantTotal := 20000 + 1000 + fee
		if q.Total != wantTotal {
			t.Fatalf("quote total %d, mau %d", q.Total, wantTotal)
		}
		bad := wantTotal - 1
		req.ExpectedTotal = &bad
		if _, _, err := e.svc.Place(e.ctx, buyer, "pk-stale", req); !errors.Is(err, ErrPriceChanged) || fields(err)["delivery_fee"] != fee {
			t.Errorf("price_changed membawa ongkir: %v %v", err, fields(err))
		}
		req.ExpectedTotal = &wantTotal
		id, _, err := e.svc.Place(e.ctx, buyer, "pk-1", req)
		if err != nil {
			t.Fatal(err)
		}
		d, _ := e.svc.Get(e.ctx, buyer, id)
		if d.Fulfillment != "delivery" || d.Total != wantTotal || d.DeliveryFee != fee || d.PickupBy != nil || d.Delivery == nil ||
			d.Delivery.Courier != "grab" || d.Delivery.Payable != fee || d.DeliveryAddress == nil || d.Delivery.ETAMax != d.Delivery.ETAMin+10 {
			t.Errorf("delivery order: %+v %+v", d, d.Delivery)
		}
		m, _ := e.svc.Get(e.ctx, s.owner, id)
		if *m.Commission != 2000 || *m.MerchantPayout != 18000 { // ongkir tidak dikomisi
			t.Errorf("commission=%d payout=%d", *m.Commission, *m.MerchantPayout)
		}
		if !d.CanCancel { // pengiriman tidak punya batas pickup_by
			t.Error("pembeli boleh membatalkan order pengiriman yang masih ready")
		}
	})

	t.Run("pengiriman ditolak: toko ambil-sendiri, terlalu jauh, tanpa alamat, dan mode off", func(t *testing.T) {
		s := e.store()
		l := e.fx.Listing(s.merchant, testutil.ListingOpts{Price: 10000, Orig: 20000, Stock: 10})
		buyer := e.fx.User("Pembeli")
		req := delivery(l, s.merchant, 1, "", "")
		e.fx.Delivery(s.merchant, true)
		if _, _, err := e.svc.Place(e.ctx, buyer, "pk-noaddr", req); !errors.Is(err, ErrDeliveryUnavailable) || fields(err)["reason"] != "address_required" {
			t.Errorf("tanpa alamat: %v %v", err, fields(err))
		}
		e.fx.Address(buyer, -6.2433, 106.9500) // ~17 km
		if _, _, err := e.svc.Place(e.ctx, buyer, "pk-far", req); !errors.Is(err, ErrDeliveryUnavailable) || fields(err)["reason"] != "too_far" {
			t.Errorf("terlalu jauh: %v %v", err, fields(err))
		}
		e.fx.Address(buyer, -6.2433, 106.8300)
		e.fx.Delivery(s.merchant, false)
		if _, _, err := e.svc.Place(e.ctx, buyer, "pk-pick", req); !errors.Is(err, ErrDeliveryUnavailable) || fields(err)["reason"] != "merchant_pickup_only" {
			t.Errorf("toko ambil sendiri: %v %v", err, fields(err))
		}
		if q, err := e.svc.Quote(e.ctx, buyer, req.QuoteRequest); err == nil || q != nil && q.Delivery != nil && q.Delivery.Available {
			t.Errorf("quote delivery untuk toko ambil-sendiri harus ditolak: %v", err)
		}
		e.fx.Delivery(s.merchant, true)
		e.svc.WithPartners(false)
		defer e.svc.WithPartners(true)
		if _, _, err := e.svc.Place(e.ctx, buyer, "pk-off", req); !errors.Is(err, ErrDeliveryUnavailable) {
			t.Errorf("mode off: %v", err)
		}
	})

	t.Run("pembayaran QRIS tiruan: langsung paid, batal -> refunded; e-wallet wajib ditautkan; gratis = tunai", func(t *testing.T) {
		s := e.store()
		l := e.fx.Listing(s.merchant, testutil.ListingOpts{Price: 10000, Orig: 20000, Stock: 10})
		free := e.fx.Listing(s.merchant, testutil.ListingOpts{Free: true, Stock: 10})
		buyer := e.fx.User("Pembeli")
		pay := func(key, kind string, lid uuid.UUID) (uuid.UUID, error) {
			id, _, err := e.svc.Place(e.ctx, buyer, key, PlaceRequest{QuoteRequest: QuoteRequest{MerchantID: s.merchant, Lines: []LineInput{line(lid, 1)}}, PaymentKind: kind})
			return id, err
		}
		id, err := pay("pay-qris", "qris", l)
		if err != nil {
			t.Fatal(err)
		}
		d, _ := e.svc.Get(e.ctx, buyer, id)
		if d.Payment.Kind != "qris" || d.Payment.Status != "paid" || d.Payment.Provider == nil || *d.Payment.Provider != "dummy" || d.Payment.PaidAt == nil {
			t.Errorf("payment: %+v", d.Payment)
		}
		if err := e.svc.CancelByBuyer(e.ctx, buyer, id, ""); err != nil {
			t.Fatal(err)
		}
		if d, _ := e.svc.Get(e.ctx, buyer, id); d.Payment.Status != "refunded" {
			t.Errorf("setelah batal: %+v", d.Payment)
		}
		if _, err := pay("pay-gopay", "gopay", l); !errors.Is(err, ErrPaymentNotLinked) {
			t.Errorf("gopay belum ditautkan: %v", err)
		}
		e.sql(`INSERT INTO payment_methods (user_id, kind, detail) VALUES ($1,'gopay','0812 •••• 7890')`, buyer)
		if _, err := pay("pay-gopay2", "gopay", l); err != nil {
			t.Errorf("gopay ditautkan: %v", err)
		}
		fid, err := pay("pay-free", "qris", free)
		if err != nil {
			t.Fatal(err)
		}
		if d, _ := e.svc.Get(e.ctx, buyer, fid); d.Payment.Kind != "cash" || d.Total != 0 {
			t.Errorf("gratis dipaksa tunai: %+v", d.Payment)
		}
		e.svc.WithPartners(false)
		defer e.svc.WithPartners(true)
		if _, err := pay("pay-off", "qris", l); !errors.Is(err, ErrPaymentUnavailable) {
			t.Errorf("mode off: %v", err)
		}
	})

	t.Run("pesanan pengiriman yang tak diselesaikan menjadi no_show 2 jam setelah jendela listing tutup", func(t *testing.T) {
		s := e.store()
		e.fx.Delivery(s.merchant, true)
		l := e.fx.Listing(s.merchant, testutil.ListingOpts{Price: 10000, Orig: 20000, Stock: 10})
		buyer := e.fx.User("Pembeli")
		e.fx.Address(buyer, -6.2433, 106.8300)
		id, _, err := e.svc.Place(e.ctx, buyer, "pk-stuck", delivery(l, s.merchant, 2, "gosend", "standard"))
		if err != nil {
			t.Fatal(err)
		}
		if err := e.svc.ExpireNoShows(e.ctx); err != nil {
			t.Fatal(err)
		}
		if _, st := e.order(id); st != "ready" {
			t.Fatalf("belum waktunya: %s", st)
		}
		e.sql(`UPDATE orders SET pickup_end = now() - interval '2 hours 20 minutes' WHERE id=$1`, id)
		if err := e.svc.ExpireNoShows(e.ctx); err != nil {
			t.Fatal(err)
		}
		if _, st := e.order(id); st != "no_show" {
			t.Errorf("status = %s, mau no_show", st)
		}
	})
}
