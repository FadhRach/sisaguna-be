package listing

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
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
	return env{t: t, svc: NewService(NewRepository(pool), pmedia.URLs{Base: "http://api.test"}), fx: testutil.Fixtures{T: t, Pool: pool}, pool: pool, ctx: context.Background()}
}

func fp(v float64) *float64 { return &v }
func ip(v int) *int         { return &v }

func humanReq(media uuid.UUID) CreateRequest {
	made := time.Now().Add(-time.Hour)
	return CreateRequest{
		Title: "Nasi Ayam", Tier: "human", Price: ip(12000), PriceOriginal: ip(25000), Stock: 5, PickupHours: 2,
		MadeAt: &made, Storage: "room_temp", SafetyChecks: []string{"untouched", "sensory_ok", "stored_right", "packed_clean"},
		PhotoMediaIDs: []uuid.UUID{media},
	}
}

func TestCreateListingFoodSafety(t *testing.T) {
	e := setup(t)
	owner := e.fx.User("Mitra")
	e.fx.Merchant(owner, "approved", -6.24, 106.80)

	t.Run("human: safe_until from storage, pickup_end is the earlier of requested and safe_until", func(t *testing.T) {
		req := humanReq(e.fx.FreeMedia(owner))
		made := time.Now().Add(-3*time.Hour - 30*time.Minute) // room_temp 4 jam -> sisa 30 menit
		req.MadeAt = &made
		req.PickupHours = 8
		id, created, err := e.svc.Create(e.ctx, owner, "k1", req)
		if err != nil || !created {
			t.Fatal(err)
		}
		d, err := e.svc.Get(e.ctx, id, PointQuery{})
		if err != nil {
			t.Fatal(err)
		}
		left := time.Until(d.PickupEnd)
		if left < 25*time.Minute || left > 31*time.Minute {
			t.Errorf("pickup_end in %v, want ~30m (clamped to safe_until)", left)
		}
		if d.Unit != "portion" || d.Safety.Level != "caution" || len(d.Photos) != 1 || d.Status != "active" {
			t.Errorf("%+v", d)
		}
	})

	t.Run("past safe window suggests animal_feed", func(t *testing.T) {
		req := humanReq(e.fx.FreeMedia(owner))
		made := time.Now().Add(-5 * time.Hour)
		req.MadeAt = &made
		_, _, err := e.svc.Create(e.ctx, owner, "k2", req)
		var f interface{ ErrorFields() map[string]any }
		if !errors.Is(err, ErrPastSafeWindow) || !errors.As(err, &f) || f.ErrorFields()["suggested_tier"] != "animal_feed" {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("human needs made_at and all four checks", func(t *testing.T) {
		req := humanReq(e.fx.FreeMedia(owner))
		req.MadeAt = nil
		if _, _, err := e.svc.Create(e.ctx, owner, "k3", req); !errors.Is(err, ErrMadeAtRequired) {
			t.Errorf("no made_at: %v", err)
		}
		req = humanReq(e.fx.FreeMedia(owner))
		future := time.Now().Add(time.Hour)
		req.MadeAt = &future
		if _, _, err := e.svc.Create(e.ctx, owner, "k3b", req); !errors.Is(err, ErrMadeAtRequired) {
			t.Errorf("future made_at: %v", err)
		}
		req = humanReq(e.fx.FreeMedia(owner))
		req.SafetyChecks = []string{"untouched", "hygiene"}
		_, _, err := e.svc.Create(e.ctx, owner, "k3c", req)
		var f interface{ ErrorFields() map[string]any }
		if !errors.Is(err, ErrChecklistIncomplete) || !errors.As(err, &f) || len(f.ErrorFields()["missing"].([]string)) != 3 {
			t.Errorf("checks: %v", err)
		}
	})

	t.Run("feed and compost need no made_at/checks and are priced per kg", func(t *testing.T) {
		req := CreateRequest{Title: "Roti keras", Tier: "animal_feed", Price: ip(2000), Stock: 20, PickupHours: 8, PhotoMediaIDs: []uuid.UUID{e.fx.FreeMedia(owner)}}
		id, _, err := e.svc.Create(e.ctx, owner, "k4", req)
		if err != nil {
			t.Fatal(err)
		}
		d, _ := e.svc.Get(e.ctx, id, PointQuery{})
		if d.Unit != "kg" || d.Safety.Level != "not_for_humans" || d.MadeAt != nil {
			t.Errorf("%+v", d)
		}
	})

	t.Run("price rules", func(t *testing.T) {
		cases := []struct {
			name string
			mod  func(*CreateRequest)
		}{
			{"free with price", func(r *CreateRequest) { r.IsFree = true }},
			{"paid without price", func(r *CreateRequest) { r.Price = nil }},
			{"original below price", func(r *CreateRequest) { r.PriceOriginal = ip(5000) }},
		}
		for i, c := range cases {
			req := humanReq(e.fx.FreeMedia(owner))
			c.mod(&req)
			if _, _, err := e.svc.Create(e.ctx, owner, "kp"+string(rune('a'+i)), req); !errors.Is(err, ErrInvalidPrice) {
				t.Errorf("%s: %v", c.name, err)
			}
		}
		free := humanReq(e.fx.FreeMedia(owner))
		free.IsFree, free.Price, free.PriceOriginal = true, nil, nil
		id, _, err := e.svc.Create(e.ctx, owner, "kfree", free)
		if err != nil {
			t.Fatal(err)
		}
		if d, _ := e.svc.Get(e.ctx, id, PointQuery{}); !d.IsFree || d.Price != nil {
			t.Errorf("%+v", d)
		}
	})

	t.Run("idempotency-key returns the same listing", func(t *testing.T) {
		req := humanReq(e.fx.FreeMedia(owner))
		id1, c1, err1 := e.svc.Create(e.ctx, owner, "same", req)
		id2, c2, err2 := e.svc.Create(e.ctx, owner, "same", req)
		if err1 != nil || err2 != nil || id1 != id2 || !c1 || c2 {
			t.Errorf("%v %v %v %v %v %v", id1, id2, c1, c2, err1, err2)
		}
	})

	t.Run("photos must be yours, public, and unused; they become attached", func(t *testing.T) {
		stranger := e.fx.User("X")
		req := humanReq(e.fx.FreeMedia(stranger))
		if _, _, err := e.svc.Create(e.ctx, owner, "m1", req); !errors.Is(err, ErrInvalidMedia) {
			t.Errorf("foreign media: %v", err)
		}
		mine := e.fx.FreeMedia(owner)
		req = humanReq(mine)
		if _, _, err := e.svc.Create(e.ctx, owner, "m2", req); err != nil {
			t.Fatal(err)
		}
		if _, _, err := e.svc.Create(e.ctx, owner, "m3", req); !errors.Is(err, ErrInvalidMedia) {
			t.Errorf("reused media: %v", err)
		}
		req.PhotoMediaIDs = []uuid.UUID{uuid.New()}
		if _, _, err := e.svc.Create(e.ctx, owner, "m4", req); !errors.Is(err, ErrInvalidMedia) {
			t.Errorf("unknown media: %v", err)
		}
	})

	t.Run("merchant must exist and be approved", func(t *testing.T) {
		noStore := e.fx.User("N")
		if _, _, err := e.svc.Create(e.ctx, noStore, "n1", humanReq(e.fx.FreeMedia(noStore))); !errors.Is(err, ErrMerchantRequired) {
			t.Errorf("no store: %v", err)
		}
		pend := e.fx.User("P")
		e.fx.Merchant(pend, "pending", -6.2, 106.8)
		if _, _, err := e.svc.Create(e.ctx, pend, "n2", humanReq(e.fx.FreeMedia(pend))); !errors.Is(err, ErrMerchantNotApproved) {
			t.Errorf("pending: %v", err)
		}
	})
}

func TestUpdateAndDelete(t *testing.T) {
	e := setup(t)
	owner := e.fx.User("Mitra")
	m := e.fx.Merchant(owner, "approved", -6.24, 106.80)
	l := e.fx.Listing(m, testutil.ListingOpts{Price: 10000, Orig: 20000, Stock: 5})

	if err := e.svc.Update(e.ctx, owner, l, UpdateRequest{Stock: ip(0)}); err != nil {
		t.Fatal(err)
	}
	if d, _ := e.svc.Get(e.ctx, l, PointQuery{}); d.Stock != 0 || d.Status != "sold_out" {
		t.Errorf("sold out: %+v", d)
	}
	if err := e.svc.Update(e.ctx, owner, l, UpdateRequest{Price: ip(15000)}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Update(e.ctx, owner, l, UpdateRequest{PriceOriginal: ip(12000)}); !errors.Is(err, ErrInvalidPrice) {
		t.Errorf("original below price: %v", err)
	}
	if err := e.svc.Update(e.ctx, owner, l, UpdateRequest{PriceOriginal: ip(0)}); err != nil {
		t.Errorf("clear original: %v", err)
	}
	paused := "paused"
	if err := e.svc.Update(e.ctx, owner, l, UpdateRequest{Status: &paused}); err != nil {
		t.Fatal(err)
	}
	if d, _ := e.svc.Get(e.ctx, l, PointQuery{}); d.Status != "paused" {
		t.Errorf("status = %s", d.Status)
	}
	other := e.fx.User("Lain")
	e.fx.Merchant(other, "approved", -6.3, 106.8)
	if err := e.svc.Update(e.ctx, other, l, UpdateRequest{Stock: ip(1)}); !errors.Is(err, ErrNotFound) {
		t.Errorf("stranger update: %v", err)
	}
	if err := e.svc.Update(e.ctx, e.fx.User("X"), l, UpdateRequest{Stock: ip(1)}); !errors.Is(err, ErrMerchantRequired) {
		t.Errorf("no store update: %v", err)
	}
	if err := e.svc.Update(e.ctx, owner, l, UpdateRequest{PickupHours: ip(1)}); err != nil {
		t.Errorf("extend: %v", err)
	}

	free := e.fx.Listing(m, testutil.ListingOpts{Free: true})
	if err := e.svc.Update(e.ctx, owner, free, UpdateRequest{Price: ip(1000)}); !errors.Is(err, ErrInvalidPrice) {
		t.Errorf("price on free listing: %v", err)
	}

	if err := e.svc.Delete(e.ctx, other, l); !errors.Is(err, ErrNotFound) {
		t.Errorf("stranger delete: %v", err)
	}
	if _, err := e.pool.Exec(e.ctx, `UPDATE merchants SET status = 'suspended' WHERE id = $1`, m); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Update(e.ctx, owner, l, UpdateRequest{Stock: ip(1)}); !errors.Is(err, ErrMerchantNotApproved) {
		t.Errorf("suspended update: %v", err)
	}
	if err := e.svc.Delete(e.ctx, owner, l); !errors.Is(err, ErrMerchantNotApproved) {
		t.Errorf("suspended delete: %v", err)
	}
	if _, err := e.pool.Exec(e.ctx, `UPDATE merchants SET status = 'approved' WHERE id = $1`, m); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Delete(e.ctx, owner, l); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Get(e.ctx, l, PointQuery{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted listing visible: %v", err)
	}
	if err := e.svc.Delete(e.ctx, owner, l); !errors.Is(err, ErrNotFound) {
		t.Errorf("double delete: %v", err)
	}
}

func TestFeed(t *testing.T) {
	e := setup(t)
	ownerA, ownerB, ownerP := e.fx.User("A"), e.fx.User("B"), e.fx.User("P")
	near := e.fx.Merchant(ownerA, "approved", -6.2433, 106.7989) // Kebayoran
	far := e.fx.Merchant(ownerB, "approved", -6.2260, 106.8530)  // Tebet (~6 km)
	pend := e.fx.Merchant(ownerP, "pending", -6.2433, 106.7989)

	nasi := e.fx.Listing(near, testutil.ListingOpts{Title: "Nasi Ayam Bakar", Price: 12000, Orig: 25000})
	free := e.fx.Listing(near, testutil.ListingOpts{Title: "Nasi Kuning", Free: true})
	roti := e.fx.Listing(far, testutil.ListingOpts{Title: "Roti Tawar 50%_off", Price: 9000, Orig: 10000, Storage: "shelf"})
	pakan := e.fx.Listing(far, testutil.ListingOpts{Title: "Roti Keras", Tier: "animal_feed", Price: 2000, Orig: 5000})
	kompos := e.fx.Listing(near, testutil.ListingOpts{Title: "Ampas Kopi", Tier: "compost", Free: true})
	soldOut := e.fx.Listing(near, testutil.ListingOpts{Title: "Habis"})
	e.pool.Exec(e.ctx, `UPDATE listings SET stock=0 WHERE id=$1`, soldOut)
	e.fx.Listing(near, testutil.ListingOpts{Title: "Kedaluwarsa", StartAgo: 5 * time.Hour, EndIn: -time.Hour})
	e.fx.Listing(near, testutil.ListingOpts{Title: "Belum Mulai", StartAgo: -time.Hour, EndIn: 2 * time.Hour})
	e.fx.Listing(near, testutil.ListingOpts{Title: "Dijeda", Status: "paused"})
	e.fx.Listing(pend, testutil.ListingOpts{Title: "Toko Pending"})

	ids := func(cards []CardDTO) []string {
		out := make([]string, len(cards))
		for i, c := range cards {
			out[i] = c.ID
		}
		return out
	}
	has := func(cards []CardDTO, id uuid.UUID) bool {
		for _, c := range cards {
			if c.ID == id.String() {
				return true
			}
		}
		return false
	}

	t.Run("only active, in-stock, in-window listings from approved merchants", func(t *testing.T) {
		cards, total, err := e.svc.List(e.ctx, ListQuery{})
		if err != nil || total != 5 || len(cards) != 5 {
			t.Fatalf("total=%d len=%d err=%v titles=%v", total, len(cards), err, ids(cards))
		}
		for _, id := range []uuid.UUID{nasi, free, roti, pakan, kompos} {
			if !has(cards, id) {
				t.Errorf("missing %s", id)
			}
		}
	})

	t.Run("tier, free and search filters (wildcards are literal)", func(t *testing.T) {
		if c, _, _ := e.svc.List(e.ctx, ListQuery{Tier: "animal_feed"}); len(c) != 1 || c[0].ID != pakan.String() {
			t.Errorf("tier: %v", ids(c))
		}
		if c, _, _ := e.svc.List(e.ctx, ListQuery{Free: true}); len(c) != 2 {
			t.Errorf("free: %d", len(c))
		}
		if c, _, _ := e.svc.List(e.ctx, ListQuery{Q: "ayam"}); len(c) != 1 || c[0].ID != nasi.String() {
			t.Errorf("q=ayam: %v", ids(c))
		}
		if c, _, _ := e.svc.List(e.ctx, ListQuery{Q: "%"}); len(c) != 1 || c[0].ID != roti.String() { // hanya judul yang memuat "%" literal
			t.Errorf("q=%% must not act as a wildcard, got %v", ids(c))
		}
		if c, _, _ := e.svc.List(e.ctx, ListQuery{Q: "50%_off"}); len(c) != 1 || c[0].ID != roti.String() {
			t.Errorf("literal %% and _: %v", ids(c))
		}
	})

	t.Run("distance, max_km and nearest sort", func(t *testing.T) {
		q := ListQuery{Lat: fp(-6.2433), Lng: fp(106.7989), Sort: "nearest"}
		cards, _, _ := e.svc.List(e.ctx, q)
		if cards[0].DistanceKm == nil || *cards[0].DistanceKm > 0.1 {
			t.Fatalf("nearest first with ~0 km: %+v", cards[0].DistanceKm)
		}
		last := cards[len(cards)-1]
		if last.DistanceKm == nil || *last.DistanceKm < 5 || *last.DistanceKm > 7 {
			t.Errorf("far item distance = %v", last.DistanceKm)
		}
		q.MaxKm = fp(2)
		cards, _, _ = e.svc.List(e.ctx, q)
		if len(cards) != 3 || has(cards, roti) || has(cards, pakan) {
			t.Errorf("max_km=2: %v", ids(cards))
		}
		noPoint, _, _ := e.svc.List(e.ctx, ListQuery{})
		if noPoint[0].DistanceKm != nil {
			t.Error("no lat/lng -> distance_km must be null")
		}
	})

	t.Run("cheapest and biggest_discount sorts", func(t *testing.T) {
		c, _, _ := e.svc.List(e.ctx, ListQuery{Tier: "human", Sort: "cheapest"})
		if c[0].ID != free.String() || c[1].ID != roti.String() {
			t.Errorf("cheapest: %v", ids(c))
		}
		c, _, _ = e.svc.List(e.ctx, ListQuery{Tier: "human", Sort: "biggest_discount"})
		if c[0].ID != free.String() || c[1].ID != nasi.String() { // gratis 100%, nasi 52%, roti 10%
			t.Errorf("biggest_discount: %v", ids(c))
		}
	})

	t.Run("pagination", func(t *testing.T) {
		p1, total, _ := e.svc.List(e.ctx, ListQuery{PageQuery: pq(1, 2)})
		p3, _, _ := e.svc.List(e.ctx, ListQuery{PageQuery: pq(3, 2)})
		if total != 5 || len(p1) != 2 || len(p3) != 1 {
			t.Errorf("total=%d p1=%d p3=%d", total, len(p1), len(p3))
		}
	})

	t.Run("home: semua rail, sort, dan other_tab_match_count (aturan HomeViewModel)", func(t *testing.T) {
		h, err := e.svc.Home(e.ctx, HomeQuery{Lat: fp(-6.2433), Lng: fp(106.7989)}, uuid.Nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(h.Nearby) != 3 || len(h.AnimalFeed) != 1 || len(h.Compost) != 1 || h.Nearby[0].Tier != "human" || h.HumanCount != 3 || h.FarmCount != 2 {
			t.Errorf("rails: nearby=%d feed=%d compost=%d human=%d farm=%d", len(h.Nearby), len(h.AnimalFeed), len(h.Compost), h.HumanCount, h.FarmCount)
		}
		if h.Nearby[0].DistanceKm == nil || *h.Nearby[0].DistanceKm > *h.Nearby[2].DistanceKm {
			t.Error("nearby harus terdekat dulu")
		}
		if h.Deals[0].ID != free.String() { // gratis = rasio 1.0
			t.Errorf("deals: gratis dulu, dapat %s", h.Deals[0].Title)
		}
		cheap, _ := e.svc.Home(e.ctx, HomeQuery{Sort: "cheapest"}, uuid.Nil)
		if cheap.Nearby[0].ID != free.String() || cheap.Popular[0].SoldCount < cheap.Popular[len(cheap.Popular)-1].SoldCount {
			t.Errorf("sort=cheapest mengurutkan rail kecuali popular: %v", ids(cheap.Nearby))
		}
		// tab farm, pencarian tanpa hasil di farm -> hitung hasil di tab human
		o, _ := e.svc.Home(e.ctx, HomeQuery{Tab: "farm", Q: "nasi"}, uuid.Nil)
		if o.FarmCount != 0 || o.OtherTabMatchCount != o.HumanCount || o.OtherTabMatchCount == 0 {
			t.Errorf("other tab matches = %d (human=%d farm=%d)", o.OtherTabMatchCount, o.HumanCount, o.FarmCount)
		}
		// ada hasil di tab aktif -> tidak perlu menunjuk tab lain
		if r, _ := e.svc.Home(e.ctx, HomeQuery{Tab: "farm", Q: "roti"}, uuid.Nil); r.OtherTabMatchCount != 0 {
			t.Errorf("tab aktif punya hasil, other harus 0, dapat %d", r.OtherTabMatchCount)
		}
	})

	t.Run("category sorts: recommended, rating, delivery", func(t *testing.T) {
		for _, mode := range []string{"recommended", "rating", "delivery_fee", "delivery_time", "price_low", "discount", "ending_soon"} {
			c, total, err := e.svc.List(e.ctx, ListQuery{Tier: "human", Sort: mode, Lat: fp(-6.2433), Lng: fp(106.7989)})
			if err != nil || len(c) != 3 || total != 3 {
				t.Errorf("%s: %v %d %v", mode, ids(c), total, err)
			}
		}
	})

	t.Run("detail: derived status, safety, more_from_merchant, no leak of pending stores", func(t *testing.T) {
		d, err := e.svc.Get(e.ctx, nasi, PointQuery{Lat: fp(-6.2433), Lng: fp(106.7989)})
		if err != nil {
			t.Fatal(err)
		}
		if d.Status != "active" || d.Merchant.Name == "" || len(d.MoreFromMerchant) == 0 || d.DistanceKm == nil {
			t.Errorf("%+v", d)
		}
		for _, c := range d.MoreFromMerchant {
			if c.ID == d.ID {
				t.Error("more_from_merchant must exclude the listing itself")
			}
		}
		if so, err := e.svc.Get(e.ctx, soldOut, PointQuery{}); err != nil || so.Status != "sold_out" {
			t.Errorf("sold out detail: %v %v", so, err)
		}
		var pendingListing uuid.UUID
		_ = e.pool.QueryRow(e.ctx, `SELECT id FROM listings WHERE title='Toko Pending'`).Scan(&pendingListing)
		if _, err := e.svc.Get(e.ctx, pendingListing, PointQuery{}); !errors.Is(err, ErrNotFound) {
			t.Errorf("pending store listing: %v", err)
		}
	})

	t.Run("merchant catalog shows derived statuses", func(t *testing.T) {
		rows, total, err := e.svc.Mine(e.ctx, ownerA, 1, 20)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]int{}
		for _, r := range rows {
			got[r.Status]++
		}
		if total != int64(len(rows)) || got["active"] != 4 || // termasuk "Belum Mulai" (dijadwalkan, belum tampil di feed)
			got["sold_out"] != 1 || got["paused"] != 1 || got["expired"] != 1 {
			t.Errorf("total=%d statuses=%v", total, got)
		}
		if _, _, err := e.svc.Mine(e.ctx, e.fx.User("N"), 1, 20); !errors.Is(err, ErrMerchantRequired) {
			t.Errorf("non merchant: %v", err)
		}
	})
}

func pq(page, limit int) httpx.PageQuery { return httpx.PageQuery{Page: page, Limit: limit} }

func TestListingVideo(t *testing.T) {
	e := setup(t)
	owner := e.fx.User("Mitra")
	e.fx.Merchant(owner, "approved", -6.24, 106.80)

	video := e.fx.FreeVideo(owner)
	req := humanReq(e.fx.FreeMedia(owner))
	req.VideoMediaID = &video
	id, _, err := e.svc.Create(e.ctx, owner, "v1", req)
	if err != nil {
		t.Fatal(err)
	}
	d, err := e.svc.Get(e.ctx, id, PointQuery{})
	if err != nil || d.VideoURL == nil || !strings.HasSuffix(*d.VideoURL, ".mp4") {
		t.Fatalf("video_url: %+v %v", d, err)
	}

	// video yang sudah dipakai, foto yang dikirim sebagai video, dan video orang lain ditolak
	for name, vid := range map[string]uuid.UUID{"dipakai": video, "foto": e.fx.FreeMedia(owner), "milik orang lain": e.fx.FreeVideo(e.fx.User("Lain"))} {
		r := humanReq(e.fx.FreeMedia(owner))
		r.VideoMediaID = &vid
		if _, _, err := e.svc.Create(e.ctx, owner, "bad-"+name, r); !errors.Is(err, ErrInvalidMedia) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// listing tanpa video: video_url null
	id2, _, _ := e.svc.Create(e.ctx, owner, "v2", humanReq(e.fx.FreeMedia(owner)))
	if d, _ := e.svc.Get(e.ctx, id2, PointQuery{}); d.VideoURL != nil {
		t.Errorf("video_url harus null: %v", *d.VideoURL)
	}
}

func TestNewListingNotifiesSavers(t *testing.T) {
	e := setup(t)
	owner := e.fx.User("Mitra")
	m := e.fx.Merchant(owner, "approved", -6.24, 106.80)
	saver, silent, stranger := e.fx.User("Penyimpan"), e.fx.User("Diam"), e.fx.User("Lain")
	if _, err := e.pool.Exec(e.ctx, `INSERT INTO saved_merchants (user_id, merchant_id) VALUES ($1,$3),($2,$3)`, saver, silent, m); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(e.ctx, `INSERT INTO notification_prefs (user_id, merchant) VALUES ($1, false)`, silent); err != nil {
		t.Fatal(err)
	}

	req := humanReq(e.fx.FreeMedia(owner))
	req.Title = "Sayur Sop Sisa Hari Ini"
	req.Price, req.PriceOriginal = ip(4000), ip(15000)
	if _, _, err := e.svc.Create(e.ctx, owner, "n1", req); err != nil {
		t.Fatal(err)
	}
	var title, body string
	if err := e.pool.QueryRow(e.ctx, `SELECT title, body FROM notifications WHERE user_id=$1 AND type='merchant'`, saver).Scan(&title, &body); err != nil {
		t.Fatalf("penyimpan harus dinotifikasi: %v", err)
	}
	if title != "Toko "+m.String()[:4]+" upload menu baru" || body != "Sayur Sop Sisa Hari Ini, Rp 4.000 dari Rp 15.000. Penyedia yang kamu simpan baru saja menambah makanan." {
		t.Errorf("teks: %q / %q", title, body)
	}
	for name, u := range map[string]uuid.UUID{"preferensi mati": silent, "bukan penyimpan": stranger} {
		var n int
		_ = e.pool.QueryRow(e.ctx, `SELECT count(*) FROM notifications WHERE user_id=$1`, u).Scan(&n)
		if n != 0 {
			t.Errorf("%s: %d notifikasi", name, n)
		}
	}
}

// Listing gratis boleh membawa harga normal (tampil sebagai harga coret), seperti dummy "Nasi Box Rapat Berlebih".
func TestFreeListingKeepsOriginalPrice(t *testing.T) {
	e := setup(t)
	owner := e.fx.User("Mitra")
	e.fx.Merchant(owner, "approved", -6.24, 106.80)
	req := humanReq(e.fx.FreeMedia(owner))
	req.IsFree, req.Price, req.PriceOriginal = true, nil, ip(20000)
	id, _, err := e.svc.Create(e.ctx, owner, "free-orig", req)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := e.svc.Get(e.ctx, id, PointQuery{})
	if !d.IsFree || d.Price != nil || d.PriceOriginal == nil || *d.PriceOriginal != 20000 {
		t.Errorf("%+v", d)
	}
	cards, _, _ := e.svc.List(e.ctx, ListQuery{Free: true})
	if len(cards) != 1 || cards[0].DiscountPct != 100 {
		t.Errorf("diskon gratis harus 100: %+v", cards)
	}
	req2 := humanReq(e.fx.FreeMedia(owner))
	req2.IsFree, req2.Price, req2.PriceOriginal = true, ip(1000), nil
	if _, _, err := e.svc.Create(e.ctx, owner, "free-price", req2); !errors.Is(err, ErrInvalidPrice) {
		t.Errorf("gratis dengan harga jual: %v", err)
	}
}
