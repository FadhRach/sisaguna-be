package order

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
	"github.com/FadhRach/sisaguna-be/internal/platform/idempotency"
	pmedia "github.com/FadhRach/sisaguna-be/internal/platform/media"
	"github.com/FadhRach/sisaguna-be/internal/testutil"
)

type env struct {
	t    *testing.T
	svc  *Service
	fx   testutil.Fixtures
	pool *pgxpool.Pool
	ctx  context.Context
}

func setup(t *testing.T) env {
	pool := testutil.NewPool(t)
	return env{t: t, svc: NewService(NewRepository(pool), pmedia.URLs{Base: "http://api.test"}),
		fx: testutil.Fixtures{T: t, Pool: pool}, pool: pool, ctx: context.Background()}
}

type store struct{ owner, merchant uuid.UUID }

func (e env) store() store {
	owner := e.fx.User("Mitra")
	return store{owner: owner, merchant: e.fx.Merchant(owner, "approved", -6.2433, 106.7989)}
}

func (e env) place(buyer, merchant uuid.UUID, key, voucher string, lines ...LineInput) (uuid.UUID, error) {
	id, _, err := e.svc.Place(e.ctx, buyer, key, PlaceRequest{
		QuoteRequest: QuoteRequest{MerchantID: merchant, Lines: lines, VoucherCode: voucher}, PaymentKind: "cash"})
	return id, err
}

func line(l uuid.UUID, qty int) LineInput { return LineInput{ListingID: l, Quantity: qty} }

func (e env) sql(q string, args ...any) {
	e.t.Helper()
	if _, err := e.pool.Exec(e.ctx, q, args...); err != nil {
		e.t.Fatalf("%v\n%s", err, q)
	}
}

func (e env) order(id uuid.UUID) (code, status string) {
	e.t.Helper()
	if err := e.pool.QueryRow(e.ctx, `SELECT code,status::text FROM orders WHERE id=$1`, id).Scan(&code, &status); err != nil {
		e.t.Fatal(err)
	}
	return
}

func (e env) count(q string, args ...any) (n int) {
	e.t.Helper()
	if err := e.pool.QueryRow(e.ctx, q, args...).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return
}

func fields(err error) map[string]any {
	var f interface{ ErrorFields() map[string]any }
	if errors.As(err, &f) {
		return f.ErrorFields()
	}
	return nil
}

func TestCheckout(t *testing.T) {
	e := setup(t)

	t.Run("happy path: server computes money, stock drops, notifications and views", func(t *testing.T) {
		s := e.store()
		l := e.fx.Listing(s.merchant, testutil.ListingOpts{Price: 10000, Orig: 20000, Stock: 10})
		buyer := e.fx.User("Pembeli")

		id, err := e.place(buyer, s.merchant, "k1", "", line(l, 2))
		if err != nil {
			t.Fatal(err)
		}
		if e.fx.Stock(l) != 8 {
			t.Errorf("stock = %d, want 8", e.fx.Stock(l))
		}
		d, err := e.svc.Get(e.ctx, buyer, id)
		if err != nil {
			t.Fatal(err)
		}
		// 2 x 10.000 = 20.000; fee 1.000 (< 25.000); komisi 10% = 2.000; payout 18.000
		if d.Subtotal != 20000 || d.OriginalTotal != 40000 || d.ServiceFee != 1000 || d.Total != 21000 || d.Status != "ready" {
			t.Errorf("buyer view: %+v", d)
		}
		if d.Code == nil || !regexp.MustCompile(`^SG-\d{4}$`).MatchString(*d.Code) || d.Commission != nil {
			t.Errorf("buyer must see code and not commission: %+v", d)
		}
		if d.PickupBy == nil || time.Until(*d.PickupBy) > 46*time.Minute || time.Until(*d.PickupBy) < 40*time.Minute {
			t.Errorf("pickup_by = %v", d.PickupBy)
		}
		if len(d.Items) != 1 || d.Items[0].Quantity != 2 || d.Items[0].ImageURL == nil {
			t.Errorf("items: %+v", d.Items)
		}
		m, err := e.svc.Get(e.ctx, s.owner, id)
		if err != nil {
			t.Fatal(err)
		}
		if m.Code != nil || m.Commission == nil || *m.Commission != 2000 || *m.MerchantPayout != 18000 || m.BuyerName == nil {
			t.Errorf("merchant view must hide code and show payout: %+v", m)
		}
		if _, err := e.svc.Get(e.ctx, e.fx.User("Orang Lain"), id); !errors.Is(err, ErrNotFound) {
			t.Errorf("stranger: %v", err)
		}
		if n := e.count(`SELECT count(*) FROM notifications WHERE user_id = ANY($1)`, []uuid.UUID{buyer, s.owner}); n != 2 {
			t.Errorf("notifications = %d, want 2", n)
		}
	})

	t.Run("idempotency-key returns the same order and does not take stock twice", func(t *testing.T) {
		s := e.store()
		l := e.fx.Listing(s.merchant, testutil.ListingOpts{Stock: 10})
		buyer := e.fx.User("P")
		req := PlaceRequest{QuoteRequest: QuoteRequest{MerchantID: s.merchant, Lines: []LineInput{line(l, 2)}}, PaymentKind: "cash"}
		id1, created1, err1 := e.svc.Place(e.ctx, buyer, "same", req)
		id2, created2, err2 := e.svc.Place(e.ctx, buyer, "same", req)
		if err1 != nil || err2 != nil || id1 != id2 || !created1 || created2 {
			t.Fatalf("%v %v %v %v %v %v", id1, id2, created1, created2, err1, err2)
		}
		if e.fx.Stock(l) != 8 {
			t.Errorf("stock = %d, want 8", e.fx.Stock(l))
		}
	})

	t.Run("rejections", func(t *testing.T) {
		s := e.store()
		other := e.store()
		buyer := e.fx.User("P")
		l := e.fx.Listing(s.merchant, testutil.ListingOpts{Stock: 3})
		foreign := e.fx.Listing(other.merchant, testutil.ListingOpts{})
		expired := e.fx.Listing(s.merchant, testutil.ListingOpts{StartAgo: 5 * time.Hour, EndIn: -time.Hour})
		paused := e.fx.Listing(s.merchant, testutil.ListingOpts{Status: "paused"})
		pending := e.store()
		e.sql(`UPDATE merchants SET status='pending' WHERE id=$1`, pending.merchant)
		pl := e.fx.Listing(pending.merchant, testutil.ListingOpts{})

		cases := []struct {
			name string
			do   func() error
			want error
		}{
			{"more than stock", func() error { _, err := e.place(buyer, s.merchant, "a", "", line(l, 4)); return err }, ErrOutOfStock},
			{"mixed merchants", func() error { _, err := e.place(buyer, s.merchant, "b", "", line(l, 1), line(foreign, 1)); return err }, ErrInvalidCart},
			{"duplicate lines", func() error { _, err := e.place(buyer, s.merchant, "c", "", line(l, 1), line(l, 1)); return err }, ErrInvalidCart},
			{"unknown listing", func() error { _, err := e.place(buyer, s.merchant, "d", "", line(uuid.New(), 1)); return err }, ErrInvalidCart},
			{"expired window", func() error { _, err := e.place(buyer, s.merchant, "e", "", line(expired, 1)); return err }, ErrListingUnavailable},
			{"paused", func() error { _, err := e.place(buyer, s.merchant, "f", "", line(paused, 1)); return err }, ErrListingUnavailable},
			{"pending merchant", func() error { _, err := e.place(buyer, pending.merchant, "g", "", line(pl, 1)); return err }, ErrMerchantUnavailable},
			{"own store", func() error { _, err := e.place(s.owner, s.merchant, "h", "", line(l, 1)); return err }, ErrOwnMerchant},
			{"qris", func() error {
				_, _, err := e.svc.Place(e.ctx, buyer, "i", PlaceRequest{QuoteRequest: QuoteRequest{MerchantID: s.merchant, Lines: []LineInput{line(l, 1)}}, PaymentKind: "qris"})
				return err
			}, ErrPaymentUnavailable},
			{"delivery", func() error {
				_, _, err := e.svc.Place(e.ctx, buyer, "j", PlaceRequest{QuoteRequest: QuoteRequest{MerchantID: s.merchant, Lines: []LineInput{line(l, 1)}, Fulfillment: "delivery"}, PaymentKind: "cash"})
				return err
			}, ErrDeliveryUnavailable},
		}
		for _, c := range cases {
			if err := c.do(); !errors.Is(err, c.want) {
				t.Errorf("%s: got %v, want %v", c.name, err, c.want)
			}
		}
		if e.fx.Stock(l) != 3 {
			t.Errorf("failed checkouts must not change stock, got %d", e.fx.Stock(l))
		}
	})

	t.Run("price_changed carries the new numbers", func(t *testing.T) {
		s := e.store()
		l := e.fx.Listing(s.merchant, testutil.ListingOpts{Price: 10000})
		wrong := 5000
		_, _, err := e.svc.Place(e.ctx, e.fx.User("P"), "pc", PlaceRequest{
			QuoteRequest: QuoteRequest{MerchantID: s.merchant, Lines: []LineInput{line(l, 1)}}, PaymentKind: "cash", ExpectedTotal: &wrong})
		if !errors.Is(err, ErrPriceChanged) || fields(err)["total"] != 11000 {
			t.Errorf("err=%v fields=%v", err, fields(err))
		}
		if e.fx.Stock(l) != 10 {
			t.Error("stock must be untouched")
		}
		right := 11000
		if _, _, err := e.svc.Place(e.ctx, e.fx.User("P"), "pc2", PlaceRequest{
			QuoteRequest: QuoteRequest{MerchantID: s.merchant, Lines: []LineInput{line(l, 1)}}, PaymentKind: "cash", ExpectedTotal: &right}); err != nil {
			t.Errorf("matching expected_total: %v", err)
		}
	})

	t.Run("quote matches checkout and changes nothing", func(t *testing.T) {
		s := e.store()
		l := e.fx.Listing(s.merchant, testutil.ListingOpts{Price: 12000, Stock: 5})
		buyer := e.fx.User("P")
		q, err := e.svc.Quote(e.ctx, buyer, QuoteRequest{MerchantID: s.merchant, Lines: []LineInput{line(l, 2)}})
		if err != nil {
			t.Fatal(err)
		}
		// 2 x 12.000 = 24.000 (< 25.000) -> fee 1.000, total 25.000
		if q.Subtotal != 24000 || q.ServiceFee != 1000 || q.Total != 25000 || len(q.Lines) != 1 || q.Fulfillment != "pickup" {
			t.Errorf("quote = %+v", q)
		}
		if e.fx.Stock(l) != 5 {
			t.Error("quote must not change stock")
		}
	})

	t.Run("only the stock that exists is sold under concurrency", func(t *testing.T) {
		s := e.store()
		l := e.fx.Listing(s.merchant, testutil.ListingOpts{Stock: 3})
		const buyers = 10
		var wg sync.WaitGroup
		results := make(chan error, buyers)
		for i := 0; i < buyers; i++ {
			buyer := e.fx.User("P")
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := e.place(buyer, s.merchant, uuid.NewString(), "", line(l, 1))
				results <- err
			}()
		}
		wg.Wait()
		close(results)
		ok, soldOut := 0, 0
		for err := range results {
			switch {
			case err == nil:
				ok++
			case errors.Is(err, ErrOutOfStock):
				soldOut++
			default:
				t.Errorf("unexpected: %v", err)
			}
		}
		if ok != 3 || soldOut != 7 || e.fx.Stock(l) != 0 {
			t.Errorf("ok=%d soldOut=%d stock=%d", ok, soldOut, e.fx.Stock(l))
		}
	})
}

func TestVouchers(t *testing.T) {
	e := setup(t)
	s := e.store()
	l := e.fx.Listing(s.merchant, testutil.ListingOpts{Price: 10000, Stock: 50})
	feed := e.fx.Listing(s.merchant, testutil.ListingOpts{Tier: "animal_feed", Price: 2000, Orig: 5000, Stock: 50})

	t.Run("merchant voucher is funded by merchant: commission from items_total", func(t *testing.T) {
		code := "M" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:7]
		e.fx.Voucher(code, 20, 0, 8000, 15000, &s.merchant, "")
		buyer := e.fx.User("P")
		e.fx.Claim(buyer, code)
		id, err := e.place(buyer, s.merchant, "v1", code, line(l, 3)) // 30.000, diskon 20% = 6.000
		if err != nil {
			t.Fatal(err)
		}
		d, _ := e.svc.Get(e.ctx, s.owner, id)
		// items 24.000 -> fee 1.000, komisi 2.400, payout 21.600, total 25.000
		if d.VoucherDiscount != 6000 || d.Total != 25000 || *d.Commission != 2400 || *d.MerchantPayout != 21600 {
			t.Errorf("%+v commission=%v payout=%v", d, *d.Commission, *d.MerchantPayout)
		}
		var funded string
		_ = e.pool.QueryRow(e.ctx, `SELECT voucher_funded_by FROM orders WHERE id=$1`, id).Scan(&funded)
		if funded != "merchant" {
			t.Errorf("funded_by = %s", funded)
		}
		if _, err := e.place(buyer, s.merchant, "v1b", code, line(l, 3)); !errors.Is(err, ErrVoucherUsed) {
			t.Errorf("reuse: %v", err)
		}
	})

	t.Run("platform voucher is funded by SisaGuna: commission from subtotal", func(t *testing.T) {
		code := "P" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:7]
		e.fx.Voucher(code, 0, 5000, 0, 0, nil, "")
		buyer := e.fx.User("P")
		e.fx.Claim(buyer, code)
		id, err := e.place(buyer, s.merchant, "v2", code, line(l, 2)) // 20.000 - 5.000
		if err != nil {
			t.Fatal(err)
		}
		d, _ := e.svc.Get(e.ctx, s.owner, id)
		// items 15.000 -> fee 1.000, total 16.000; komisi dari subtotal 20.000 = 2.000, payout 18.000
		if d.Total != 16000 || *d.Commission != 2000 || *d.MerchantPayout != 18000 {
			t.Errorf("%+v", d)
		}
	})

	t.Run("rejections", func(t *testing.T) {
		buyer := e.fx.User("P")
		other := e.store()
		min, tier, otherStore := "MIN"+strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:5], "TIER"+strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:4], "OTH"+strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:5]
		e.fx.Voucher(min, 0, 1000, 0, 50000, nil, "")
		e.fx.Voucher(tier, 50, 0, 10000, 0, nil, "animal_feed")
		e.fx.Voucher(otherStore, 10, 0, 0, 0, &other.merchant, "")
		for _, c := range []string{min, tier, otherStore} {
			e.fx.Claim(buyer, c)
		}
		unclaimed := "UNC" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:5]
		e.fx.Voucher(unclaimed, 0, 1000, 0, 0, nil, "")

		if _, err := e.place(buyer, s.merchant, "r1", unclaimed, line(l, 1)); !errors.Is(err, ErrVoucherNotClaimed) {
			t.Errorf("unclaimed: %v", err)
		}
		if _, err := e.place(buyer, s.merchant, "r2", min, line(l, 1)); !errors.Is(err, ErrVoucherMinSpend) || fields(err)["min_spend"] != int32(50000) {
			t.Errorf("min spend: %v %v", err, fields(err))
		}
		if _, err := e.place(buyer, s.merchant, "r3", tier, line(l, 1)); !errors.Is(err, ErrVoucherInvalid) {
			t.Errorf("tier mismatch: %v", err)
		}
		if _, err := e.place(buyer, s.merchant, "r4", otherStore, line(l, 1)); !errors.Is(err, ErrVoucherInvalid) {
			t.Errorf("other store: %v", err)
		}
		if _, err := e.place(buyer, s.merchant, "r5", tier, line(feed, 2)); err != nil { // tier cocok -> boleh
			t.Errorf("tier match: %v", err)
		}
	})
}

func TestCancelAndRestore(t *testing.T) {
	e := setup(t)

	newOrder := func(voucher string) (s store, buyer, listing, id uuid.UUID) {
		s = e.store()
		listing = e.fx.Listing(s.merchant, testutil.ListingOpts{Price: 10000, Stock: 10})
		buyer = e.fx.User("P")
		if voucher != "" {
			e.fx.Voucher(voucher, 0, 3000, 0, 0, nil, "")
			e.fx.Claim(buyer, voucher)
		}
		id, err := e.place(buyer, s.merchant, "k", voucher, line(listing, 2))
		if err != nil {
			t.Fatal(err)
		}
		return
	}

	t.Run("buyer cancel restores stock and voucher, second cancel is rejected", func(t *testing.T) {
		code := "C" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:7]
		s, buyer, l, id := newOrder(code)
		_ = s
		if e.fx.Stock(l) != 8 {
			t.Fatalf("stock = %d", e.fx.Stock(l))
		}
		if err := e.svc.CancelByBuyer(e.ctx, buyer, id, "berubah pikiran"); err != nil {
			t.Fatal(err)
		}
		if _, st := e.order(id); st != "cancelled" || e.fx.Stock(l) != 10 {
			t.Errorf("status=%s stock=%d", st, e.fx.Stock(l))
		}
		if e.count(`SELECT count(*) FROM user_vouchers WHERE voucher_code=$1 AND used_at IS NULL`, code) != 1 ||
			e.count(`SELECT used_count FROM vouchers WHERE code=$1`, code) != 0 {
			t.Error("voucher must be released")
		}
		if err := e.svc.CancelByBuyer(e.ctx, buyer, id, ""); !errors.Is(err, ErrInvalidState) {
			t.Errorf("second cancel: %v", err)
		}
	})

	t.Run("buyer cancel is closed after pickup_by; merchant can still cancel", func(t *testing.T) {
		s, buyer, l, id := newOrder("")
		e.sql(`UPDATE orders SET pickup_by = now() - interval '1 minute' WHERE id=$1`, id)
		if err := e.svc.CancelByBuyer(e.ctx, buyer, id, ""); !errors.Is(err, ErrCancelWindowClosed) {
			t.Errorf("buyer: %v", err)
		}
		if err := e.svc.CancelByMerchant(e.ctx, s.owner, id, "kehabisan bahan"); err != nil {
			t.Fatal(err)
		}
		if e.fx.Stock(l) != 10 {
			t.Errorf("stock = %d", e.fx.Stock(l))
		}
		d, _ := e.svc.Get(e.ctx, buyer, id)
		if d.CancelledBy == nil || *d.CancelledBy != "merchant" || d.CancelReason == nil || *d.CancelReason != "kehabisan bahan" {
			t.Errorf("%+v", d)
		}
	})

	t.Run("strangers cannot cancel; stock is not restored once the listing window has passed", func(t *testing.T) {
		s, buyer, l, id := newOrder("")
		if err := e.svc.CancelByBuyer(e.ctx, e.fx.User("X"), id, ""); !errors.Is(err, ErrNotFound) {
			t.Errorf("stranger buyer: %v", err)
		}
		if err := e.svc.CancelByMerchant(e.ctx, e.fx.User("X"), id, "x"); !errors.Is(err, ErrNotFound) {
			t.Errorf("stranger merchant: %v", err)
		}
		e.sql(`UPDATE listings SET pickup_end = now() - interval '1 minute' WHERE id=$1`, l)
		if err := e.svc.CancelByBuyer(e.ctx, buyer, id, ""); err != nil {
			t.Fatal(err)
		}
		if e.fx.Stock(l) != 8 {
			t.Errorf("stock must stay 8 (window passed), got %d", e.fx.Stock(l))
		}
		_ = s
	})
}

func TestConfirmPickup(t *testing.T) {
	e := setup(t)
	fresh := func() (s store, buyer, id uuid.UUID, code string) {
		s = e.store()
		l := e.fx.Listing(s.merchant, testutil.ListingOpts{Price: 10000, Stock: 10})
		buyer = e.fx.User("P")
		id, err := e.place(buyer, s.merchant, "k", "", line(l, 2))
		if err != nil {
			t.Fatal(err)
		}
		code, _ = e.order(id)
		return
	}

	t.Run("accepts 4 digits, sg-prefix and lowercase; completes, pays, counts rescued", func(t *testing.T) {
		for _, form := range []func(string) string{
			func(c string) string { return c },
			func(c string) string { return c[3:] },
			func(c string) string { return " sg-" + c[3:] + " " },
		} {
			s, buyer, id, code := fresh()
			if err := e.svc.Confirm(e.ctx, s.owner, id, form(code)); err != nil {
				t.Fatalf("form %q: %v", form(code), err)
			}
			d, _ := e.svc.Get(e.ctx, buyer, id)
			if d.Status != "completed" || d.PaymentStatus != "paid" || d.CompletedAt == nil {
				t.Errorf("%+v", d)
			}
			if e.count(`SELECT total_rescued FROM merchants WHERE id=$1`, s.merchant) != 2 {
				t.Error("total_rescued should be 2")
			}
			if err := e.svc.Confirm(e.ctx, s.owner, id, code); !errors.Is(err, ErrInvalidState) {
				t.Errorf("confirm twice: %v", err)
			}
		}
	})

	t.Run("wrong code counts attempts and locks after 5 within 10 minutes", func(t *testing.T) {
		s, _, id, code := fresh()
		wrong := "SG-0000"
		if code == wrong {
			wrong = "SG-0001"
		}
		for i := 1; i <= 5; i++ {
			err := e.svc.Confirm(e.ctx, s.owner, id, wrong)
			if !errors.Is(err, ErrWrongCode) || fields(err)["attempts_left"] != 5-i {
				t.Fatalf("attempt %d: %v %v", i, err, fields(err))
			}
		}
		if err := e.svc.Confirm(e.ctx, s.owner, id, code); !errors.Is(err, ErrTooManyAttempts) {
			t.Fatalf("locked: %v", err)
		}
		e.sql(`UPDATE order_code_attempts SET attempted_at = now() - interval '11 minutes' WHERE order_id=$1`, id)
		if err := e.svc.Confirm(e.ctx, s.owner, id, code); err != nil {
			t.Errorf("after lock expiry: %v", err)
		}
	})

	t.Run("only the store owner can confirm", func(t *testing.T) {
		_, buyer, id, code := fresh()
		for _, who := range []uuid.UUID{buyer, e.fx.User("X")} {
			if err := e.svc.Confirm(e.ctx, who, id, code); !errors.Is(err, ErrNotFound) {
				t.Errorf("%v", err)
			}
		}
	})
}

func TestScheduledJobs(t *testing.T) {
	e := setup(t)
	s := e.store()
	code := "J" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:7]
	e.fx.Voucher(code, 0, 2000, 0, 0, nil, "")
	l := e.fx.Listing(s.merchant, testutil.ListingOpts{Price: 10000, Stock: 10})

	t.Run("no-show frees stock but keeps the voucher spent", func(t *testing.T) {
		buyer := e.fx.User("P")
		e.fx.Claim(buyer, code)
		id, err := e.place(buyer, s.merchant, "ns", code, line(l, 2))
		if err != nil {
			t.Fatal(err)
		}
		before := e.fx.Stock(l)
		e.sql(`UPDATE orders SET pickup_by = now() - interval '10 minutes' WHERE id=$1`, id) // belum lewat masa tenggang 15 menit
		if err := e.svc.ExpireNoShows(e.ctx); err != nil {
			t.Fatal(err)
		}
		if _, st := e.order(id); st != "ready" {
			t.Fatalf("within grace period must stay ready, got %s", st)
		}
		e.sql(`UPDATE orders SET pickup_by = now() - interval '16 minutes' WHERE id=$1`, id)
		if err := e.svc.ExpireNoShows(e.ctx); err != nil {
			t.Fatal(err)
		}
		if _, st := e.order(id); st != "no_show" || e.fx.Stock(l) != before+2 {
			t.Errorf("status=%s stock=%d want %d", st, e.fx.Stock(l), before+2)
		}
		if e.count(`SELECT count(*) FROM user_vouchers WHERE voucher_code=$1 AND used_at IS NOT NULL`, code) != 1 {
			t.Error("voucher must stay used after no-show")
		}
		if err := e.svc.ExpireNoShows(e.ctx); err != nil { // idempoten
			t.Fatal(err)
		}
		if e.fx.Stock(l) != before+2 {
			t.Error("second run must not restore stock again")
		}
	})

	t.Run("reminder is sent once, 30 minutes before pickup_by", func(t *testing.T) {
		buyer := e.fx.User("P")
		id, err := e.place(buyer, s.merchant, "rm", "", line(l, 1))
		if err != nil {
			t.Fatal(err)
		}
		if err := e.svc.SendReminders(e.ctx); err != nil { // pickup_by masih ~45 menit lagi
			t.Fatal(err)
		}
		if e.count(`SELECT count(*) FROM notifications WHERE user_id=$1 AND type='pickup'`, buyer) != 0 {
			t.Fatal("too early for a reminder")
		}
		e.sql(`UPDATE orders SET pickup_by = now() + interval '10 minutes' WHERE id=$1`, id)
		for i := 0; i < 2; i++ {
			if err := e.svc.SendReminders(e.ctx); err != nil {
				t.Fatal(err)
			}
		}
		if e.count(`SELECT count(*) FROM notifications WHERE user_id=$1 AND type='pickup'`, buyer) != 1 {
			t.Error("exactly one reminder expected")
		}
	})

	t.Run("reminder respects notification preferences", func(t *testing.T) {
		buyer := e.fx.User("P")
		e.sql(`INSERT INTO notification_prefs (user_id, pickup) VALUES ($1, false)`, buyer)
		id, _ := e.place(buyer, s.merchant, "rm2", "", line(l, 1))
		e.sql(`UPDATE orders SET pickup_by = now() + interval '10 minutes' WHERE id=$1`, id)
		_ = e.svc.SendReminders(e.ctx)
		if e.count(`SELECT count(*) FROM notifications WHERE user_id=$1 AND type='pickup'`, buyer) != 0 {
			t.Error("pickup notifications are disabled")
		}
		if e.count(`SELECT count(*) FROM orders WHERE id=$1 AND reminded_at IS NOT NULL`, id) != 1 {
			t.Error("order must still be marked reminded")
		}
	})
}

func TestListsAndDashboard(t *testing.T) {
	e := setup(t)
	s := e.store()
	l := e.fx.Listing(s.merchant, testutil.ListingOpts{Price: 10000, Stock: 50})
	buyer := e.fx.User("P")
	var ids []uuid.UUID
	for i := 0; i < 3; i++ {
		id, err := e.place(buyer, s.merchant, uuid.NewString(), "", line(l, 1))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
		time.Sleep(5 * time.Millisecond)
	}
	if err := e.svc.CancelByBuyer(e.ctx, buyer, ids[0], ""); err != nil {
		t.Fatal(err)
	}
	code, _ := e.order(ids[1])
	if err := e.svc.Confirm(e.ctx, s.owner, ids[1], code); err != nil {
		t.Fatal(err)
	}

	t.Run("buyer tabs and filters", func(t *testing.T) {
		ongoing, _, err := e.svc.ListBuyer(e.ctx, buyer, BuyerListQuery{Tab: "ongoing"}, nil)
		if err != nil || len(ongoing) != 1 || ongoing[0].ID != ids[2].String() {
			t.Errorf("ongoing: %v %v", ongoing, err)
		}
		all, _, _ := e.svc.ListBuyer(e.ctx, buyer, BuyerListQuery{Tab: "history", Filter: "all"}, nil)
		done, _, _ := e.svc.ListBuyer(e.ctx, buyer, BuyerListQuery{Tab: "history", Filter: "completed"}, nil)
		cancelled, _, _ := e.svc.ListBuyer(e.ctx, buyer, BuyerListQuery{Tab: "history", Filter: "cancelled"}, nil)
		if len(all) != 2 || len(done) != 1 || len(cancelled) != 1 {
			t.Errorf("all=%d completed=%d cancelled=%d", len(all), len(done), len(cancelled))
		}
	})

	t.Run("cursor pages newest first without gaps or repeats", func(t *testing.T) {
		var seen []string
		var cur *httpx.Cursor
		for page := 0; page < 5; page++ {
			items, next, err := e.svc.ListBuyer(e.ctx, buyer, BuyerListQuery{Tab: "history", Filter: "all", Limit: 1}, cur)
			if err != nil {
				t.Fatal(err)
			}
			for _, it := range items {
				seen = append(seen, it.ID)
			}
			if next == "" {
				break
			}
			c := mustCursor(t, next)
			cur = &c
		}
		if len(seen) != 2 || seen[0] != ids[1].String() || seen[1] != ids[0].String() {
			t.Errorf("seen = %v", seen)
		}
	})

	t.Run("merchant lists and dashboard", func(t *testing.T) {
		ready, _, err := e.svc.ListMerchant(e.ctx, s.owner, MerchantListQuery{Status: "ready"}, nil)
		if err != nil || len(ready) != 1 || ready[0].Code != nil {
			t.Errorf("ready: %+v %v", ready, err)
		}
		done, _, _ := e.svc.ListMerchant(e.ctx, s.owner, MerchantListQuery{Status: "done"}, nil)
		if len(done) != 2 {
			t.Errorf("done = %d", len(done))
		}
		d, err := e.svc.Dashboard(e.ctx, s.owner)
		// 1 order selesai hari ini: 10.000 penjualan, komisi 1.000, bersih 9.000; 1 menunggu; 1 listing aktif
		if err != nil || d.SalesToday != 10000 || d.CommissionToday != 1000 || d.NetToday != 9000 || d.CompletedToday != 1 || d.PendingOrders != 1 || d.ActiveListings != 1 {
			t.Errorf("%+v %v", d, err)
		}
		if _, err := e.svc.Dashboard(e.ctx, buyer); !errors.Is(err, ErrMerchantRequired) {
			t.Errorf("non-merchant: %v", err)
		}
	})
}

func mustCursor(t *testing.T, s string) httpx.Cursor {
	t.Helper()
	c, ok := httpx.DecodeCursor(s)
	if !ok {
		t.Fatalf("bad cursor %q", s)
	}
	return c
}

// Event order.created dikirim ke pemilik toko; kode pickup tidak boleh ikut, karena mitra harus mendapat kode dari pembeli.
func TestOrderCreatedEventHidesPickupCode(t *testing.T) {
	e := setup(t)
	s := e.store()
	buyer := e.fx.User("Budi")
	listing := e.fx.Listing(s.merchant, testutil.ListingOpts{})

	conn, err := pgx.ConnectConfig(e.ctx, e.pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(e.ctx)
	if _, err := conn.Exec(e.ctx, "LISTEN sg_events"); err != nil {
		t.Fatal(err)
	}
	id, err := e.place(buyer, s.merchant, "k-evt", "", line(listing, 1))
	if err != nil {
		t.Fatal(err)
	}
	code, _ := e.order(id)

	ctx, cancel := context.WithTimeout(e.ctx, 5*time.Second)
	defer cancel()
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			t.Fatal("event order.created tidak diterima:", err)
		}
		if strings.Contains(n.Payload, `"order.created"`) {
			if strings.Contains(n.Payload, code) || strings.Contains(n.Payload, `"code"`) {
				t.Errorf("payload membocorkan kode pickup: %s", n.Payload)
			}
			return
		}
	}
}

func TestOrderHardening(t *testing.T) {
	e := setup(t)
	s := e.store()

	t.Run("idempotency-key dengan isi berbeda ditolak", func(t *testing.T) {
		l := e.fx.Listing(s.merchant, testutil.ListingOpts{Stock: 10})
		buyer := e.fx.User("Pembeli K")
		if _, err := e.place(buyer, s.merchant, "same-key", "", line(l, 1)); err != nil {
			t.Fatal(err)
		}
		if _, err := e.place(buyer, s.merchant, "same-key", "", line(l, 2)); !errors.Is(err, idempotency.ErrKeyReused) {
			t.Errorf("want ErrKeyReused, got %v", err)
		}
		if e.fx.Stock(l) != 9 {
			t.Errorf("stock = %d, want 9", e.fx.Stock(l))
		}
	})

	t.Run("maks pesanan aktif per pembeli", func(t *testing.T) {
		l := e.fx.Listing(s.merchant, testutil.ListingOpts{Stock: 50})
		buyer := e.fx.User("Penimbun")
		for i := 0; i < maxActiveOrders; i++ {
			if _, err := e.place(buyer, s.merchant, uuid.NewString(), "", line(l, 1)); err != nil {
				t.Fatalf("order %d: %v", i, err)
			}
		}
		if _, err := e.place(buyer, s.merchant, uuid.NewString(), "", line(l, 1)); !errors.Is(err, ErrTooManyActive) {
			t.Errorf("want ErrTooManyActive, got %v", err)
		}
		if e.fx.Stock(l) != 50-maxActiveOrders {
			t.Errorf("stock = %d", e.fx.Stock(l))
		}
	})

	t.Run("nilai pesanan terlalu besar", func(t *testing.T) {
		l := e.fx.Listing(s.merchant, testutil.ListingOpts{Price: 50_000_000, Orig: 60_000_000, Stock: 10})
		buyer := e.fx.User("Borong")
		if _, err := e.place(buyer, s.merchant, "big", "", line(l, 3)); !errors.Is(err, ErrOrderTooLarge) {
			t.Errorf("want ErrOrderTooLarge, got %v", err)
		}
		if e.fx.Stock(l) != 10 {
			t.Errorf("stock = %d, want 10", e.fx.Stock(l))
		}
	})
}
