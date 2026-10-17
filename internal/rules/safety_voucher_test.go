package rules

import (
	"testing"
	"time"
)

func TestSafeUntil(t *testing.T) {
	made := time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)
	for storage, hours := range map[string]int{"room_temp": 4, "shelf": 72, "chilled": 48, "frozen": 168} {
		got, ok := SafeUntil(made, storage)
		if !ok || !got.Equal(made.Add(time.Duration(hours)*time.Hour)) {
			t.Errorf("%s: %v %v", storage, got, ok)
		}
	}
	if _, ok := SafeUntil(made, "lemari"); ok {
		t.Error("unknown storage must not be ok")
	}
}

func TestMissingChecks(t *testing.T) {
	if m := MissingChecks(RequiredChecks); len(m) != 0 {
		t.Errorf("all present, missing %v", m)
	}
	m := MissingChecks([]string{"untouched", "hygiene"})
	if len(m) != 3 || m[0] != "sensory_ok" {
		t.Errorf("missing = %v", m)
	}
}

func TestVoucherDiscountFor(t *testing.T) {
	p10, p50, cap8k, flat5k := 10, 50, 8000, 5000
	cases := []struct {
		name     string
		v        Voucher
		subtotal int
		want     int
	}{
		{"percent rounds down", Voucher{PercentOff: &p10}, 12_345, 1_234},
		{"percent capped", Voucher{PercentOff: &p50, MaxDiscount: &cap8k}, 40_000, 8_000},
		{"flat", Voucher{FlatOff: &flat5k}, 30_000, 5_000},
		{"never above subtotal", Voucher{FlatOff: &flat5k}, 3_000, 3_000},
		{"free order", Voucher{PercentOff: &p10}, 0, 0},
	}
	for _, c := range cases {
		if got := c.v.DiscountFor(c.subtotal); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
}
