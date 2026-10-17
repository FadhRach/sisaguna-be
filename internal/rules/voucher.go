package rules

// Voucher memuat bagian voucher yang menentukan diskon. Tepat satu dari PercentOff/FlatOff terisi.
type Voucher struct {
	PercentOff  *int
	FlatOff     *int
	MaxDiscount *int
}

// DiscountFor menghitung diskon: persen dibulatkan ke bawah dan dibatasi MaxDiscount,
// atau potongan tetap; tidak pernah melebihi subtotal.
func (v Voucher) DiscountFor(subtotal int) int {
	d := 0
	switch {
	case v.PercentOff != nil:
		d = subtotal * *v.PercentOff / 100
		if v.MaxDiscount != nil && d > *v.MaxDiscount {
			d = *v.MaxDiscount
		}
	case v.FlatOff != nil:
		d = *v.FlatOff
	}
	return min(max(d, 0), subtotal)
}
