package rules

import "testing"

func TestBuyerServiceFee(t *testing.T) {
	cases := map[int]int{0: 0, 1: 1_000, 24_999: 1_000, 25_000: 2_000, 74_999: 2_000, 75_000: 3_000, 500_000: 3_000}
	for in, want := range cases {
		if got := BuyerServiceFee(in); got != want {
			t.Errorf("BuyerServiceFee(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestCommissionRoundsDown(t *testing.T) {
	if got := Commission(99); got != 9 {
		t.Errorf("Commission(99) = %d, want 9", got)
	}
}

func TestSettleVoucherFunding(t *testing.T) {
	// voucher toko: komisi dari items_total
	m := Settle(20_000, 4_000, true)
	if m.ItemsTotal != 16_000 || m.Commission != 1_600 || m.MerchantPayout != 14_400 || m.ServiceFee != 1_000 || m.Total != 17_000 {
		t.Errorf("merchant funded: %+v", m)
	}
	// voucher umum: komisi dari subtotal
	p := Settle(20_000, 4_000, false)
	if p.Commission != 2_000 || p.MerchantPayout != 18_000 {
		t.Errorf("platform funded: %+v", p)
	}
}
