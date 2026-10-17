package rules

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// Kasus uji disalin dari app Android: FoodSafetyAndDeliveryTest, RescueTierTest, AppFeesTest.

func TestAssessMatchesKotlin(t *testing.T) {
	now := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	ago := func(h int) *time.Time { t := now.Add(-time.Duration(h) * time.Hour); return &t }
	all := AllChecks
	without := func(drop string) []string {
		var out []string
		for _, c := range all {
			if c != drop {
				out = append(out, c)
			}
		}
		return out
	}

	cases := []struct {
		name    string
		tier    string
		made    *time.Time
		storage string
		checks  []string
		level   string
	}{
		{"1 jam, suhu ruang", "human", ago(1), "room_temp", all, "safe"},
		{"3 jam = 75%", "human", ago(3), "room_temp", all, "caution"},
		{"5 jam lewat", "human", ago(5), "room_temp", all, "unsafe"},
		{"tepat 4 jam", "human", ago(4), "room_temp", all, "unsafe"},
		{"kulkas 30 jam", "human", ago(30), "chilled", all, "safe"},
		{"kulkas 49 jam", "human", ago(49), "chilled", all, "unsafe"},
		{"tanpa hygiene tetap lolos", "human", ago(0), "room_temp", without("hygiene"), "safe"},
		{"kompos", "compost", ago(100), "room_temp", nil, "not_for_humans"},
		{"tanpa made_at, checklist lengkap", "human", nil, "room_temp", all, "safe"},
	}
	for _, c := range cases {
		if got := Assess(c.tier, c.made, c.storage, c.checks, now).Level; got != c.level {
			t.Errorf("%s: %s, mau %s", c.name, got, c.level)
		}
	}

	a := Assess("human", ago(1), "room_temp", all, now)
	if a.SafeUntil == nil || !a.SafeUntil.Equal(now.Add(3*time.Hour)) || a.Score != 5 || a.Headline != "Lolos cek kelayakan" {
		t.Errorf("safe: %+v", a)
	}
	a = Assess("human", ago(0), "room_temp", without("untouched"), now)
	if !slices.Equal(a.MissingRequired, []string{"untouched"}) || a.Headline != "Checklist kelayakan belum lengkap" ||
		a.Advice != "Lengkapi 1 poin wajib sebelum makanan bisa dijual untuk dikonsumsi." || a.Score != 4 {
		t.Errorf("missing: %+v", a)
	}
	a = Assess("human", ago(5), "room_temp", all, now)
	if a.SuggestTier == nil || *a.SuggestTier != "animal_feed" || *a.HoursSinceMade != 5 ||
		!strings.Contains(a.Advice, "Sudah 5 jam sejak dimasak (suhu ruang (masakan) maks. 4 jam).") {
		t.Errorf("unsafe: %+v", a)
	}
	if a := Assess("compost", ago(100), "room_temp", nil, now); a.SafeUntil != nil || a.Headline != "Khusus bahan kompos" {
		t.Errorf("compost: %+v", a)
	}
	if a := Assess("animal_feed", nil, "room_temp", nil, now); a.Headline != "Khusus pakan ternak" {
		t.Errorf("feed: %+v", a)
	}
}

func TestDeliveryQuoteMatchesKotlin(t *testing.T) {
	p := QuoteDelivery("gosend", "priority", 3.0, 10_000, 10)
	s := QuoteDelivery("gosend", "saver", 3.0, 10_000, 10)
	if p.Fee != 18_000 || p.Fee <= s.Fee || p.ETAMin >= s.ETAMin || p.Fee%500 != 0 {
		t.Errorf("priority %+v vs saver %+v", p, s)
	}
	// perhitungan tangan dari laporan Kotlin
	if q := QuoteDelivery("gosend", "standard", 10.0, 5_000, 10); q.Fee != 31_000 || q.Discount != 0 {
		t.Errorf("gosend 10 km: %+v", q)
	}
	q := QuoteDelivery("gosend", "standard", 3.0, 10_000, 10)
	if q.ETAMin != 40 || q.ETAMax != 50 {
		t.Errorf("eta 3 km: %+v", q)
	}
	q = QuoteDelivery("gosend", "standard", 10.0, 25_000, 10)
	if q.Discount != min(q.Fee/2, 5_000) || q.Payable != q.Fee-q.Discount || q.PromoLabel == nil || *q.PromoLabel != "Diskon ongkir 50%" {
		t.Errorf("promo gosend: %+v", q)
	}
	if q := QuoteDelivery("grab", "saver", 2.0, 1_000, 10); q.Discount != 3_000 {
		t.Errorf("grab saver: %+v", q)
	}
	if q := QuoteDelivery("shopee", "standard", 2.0, 15_000, 10); q.Discount != 2_000 {
		t.Errorf("shopee: %+v", q)
	}
	if q := QuoteDelivery("lalamove", "standard", 0.1, 0, 5); q.Fee != 9_000 || q.ETAMin != 5+2+8+10 { // jarak minimum 0,5 km
		t.Errorf("jarak minimum: %+v", q)
	}
	// order dengan ongkir: total = items + payable (Kotlin: 10_000 + quote.payable, hemat 30_000)
	gq := QuoteDelivery("grab", "standard", 2.0, 10_000, 10)
	if total := 10_000 + gq.Payable; total != 10_000+gq.Fee { // grab standard tidak ada promo
		t.Errorf("total %d", total)
	}
	all := AllDeliveryQuotes("standard", 5.0, 30_000, 10)
	if len(all) != 4 || !slices.IsSortedFunc(all, func(a, b DeliveryQuote) int { return a.Payable - b.Payable }) {
		t.Errorf("urutan: %+v", all)
	}
}

func TestRescueTierMatchesKotlin(t *testing.T) {
	for portions, want := range map[int]string{0: "starter", 9: "starter", 10: "rescuer", 34: "hero", 60: "legend", 500: "legend"} {
		if got := TierOf(portions).Key; got != want {
			t.Errorf("%d porsi: %s, mau %s", portions, got, want)
		}
	}
	if CarbonKg(34) != 30 || CarbonKg(0) != 0 || CarbonKg(10) != 9 {
		t.Error("karbon = int(porsi*0.9)")
	}
}
