// Package rules berisi aturan bisnis murni (tanpa DB/HTTP), port dari Kotlin.
package rules

const commissionPercent = 10

// BuyerServiceFee: Rp0 kalau itemsTotal 0, 1.000 di bawah 25.000, 2.000 di bawah 75.000, selebihnya 3.000.
func BuyerServiceFee(itemsTotal int) int {
	switch {
	case itemsTotal <= 0:
		return 0
	case itemsTotal < 25_000:
		return 1_000
	case itemsTotal < 75_000:
		return 2_000
	default:
		return 3_000
	}
}

// Commission adalah 10% dari base, dibulatkan ke bawah.
func Commission(base int) int { return base * commissionPercent / 100 }

type Amounts struct {
	ItemsTotal     int
	ServiceFee     int
	Commission     int
	MerchantPayout int
	Total          int // sebelum ongkir
}

// Settle menghitung rincian order. Voucher toko (merchantFunded) ditanggung mitra:
// komisi dari itemsTotal. Voucher umum ditanggung SisaGuna: komisi dari subtotal.
func Settle(subtotal, voucherDiscount int, merchantFunded bool) Amounts {
	items := subtotal - voucherDiscount
	base := subtotal
	if merchantFunded {
		base = items
	}
	fee := BuyerServiceFee(items)
	c := Commission(base)
	return Amounts{ItemsTotal: items, ServiceFee: fee, Commission: c, MerchantPayout: base - c, Total: items + fee}
}
