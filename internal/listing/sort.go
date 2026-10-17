package listing

import (
	"math"
	"sort"

	"github.com/FadhRach/sisaguna-be/internal/rules"
)

// Mode urutan yang diterima /home dan /listings (gabungan HomeSort dan CategorySort di app Android).
const sortModes = "relevant recommended rating delivery_fee delivery_time nearest cheapest price_low biggest_discount discount ending_soon popular"

// maxSortRows membatasi jumlah baris yang diurutkan di memori. shortcut: urutan dilakukan di Go agar sama persis dengan
// komparator Kotlin; naikkan batas atau pindahkan ke SQL kalau katalog aktif melewati beberapa ribu listing.
const maxSortRows = 500

const defaultKm = 5.0 // jarak bawaan app untuk listing tanpa jarak (CategoryViewModel)

func (c CardDTO) unitPrice() int {
	if c.Price == nil {
		return 0
	}
	return *c.Price
}

func (c CardDTO) distance() float64 {
	if c.distKm < 0 {
		return math.MaxFloat64
	}
	return c.distKm
}

// ratio: rasio hemat untuk HomeSort.BIGGEST_DISCOUNT (gratis = 1.0).
func (c CardDTO) ratio() float64 {
	if c.IsFree {
		return 1.0
	}
	orig := c.unitPrice()
	if c.PriceOriginal != nil {
		orig = *c.PriceOriginal
	}
	return 1.0 - float64(c.unitPrice())/float64(max(orig, 1))
}

func (c CardDTO) gosend() rules.DeliveryQuote {
	km := c.distKm
	if km < 0 {
		km = defaultKm
	}
	return rules.QuoteDelivery("gosend", "standard", km, c.unitPrice(), c.prep)
}

func (c CardDTO) recommendedScore() float64 {
	rating := 4.0
	if c.Merchant.RatingAvg != nil {
		rating = *c.Merchant.RatingAvg
	}
	km := c.distKm
	if km < 0 {
		km = defaultKm
	}
	return rating*20 + float64(c.DiscountPct)*0.5 + float64(c.SoldCount)*0.2 - km*4
}

// sortCards mengurutkan stabil (seri mempertahankan urutan sebelumnya, seperti sortedBy di Kotlin).
func sortCards(cards []CardDTO, mode string) {
	var less func(a, b CardDTO) bool
	switch mode {
	case "nearest":
		less = func(a, b CardDTO) bool { return a.distance() < b.distance() }
	case "cheapest", "price_low":
		less = func(a, b CardDTO) bool { return a.unitPrice() < b.unitPrice() }
	case "biggest_discount":
		less = func(a, b CardDTO) bool { return a.ratio() > b.ratio() }
	case "discount":
		less = func(a, b CardDTO) bool { return a.DiscountPct > b.DiscountPct }
	case "ending_soon":
		less = func(a, b CardDTO) bool { return a.PickupEnd.Before(b.PickupEnd) }
	case "popular":
		less = func(a, b CardDTO) bool { return a.SoldCount > b.SoldCount }
	case "rating":
		less = func(a, b CardDTO) bool { return ratingOrZero(a) > ratingOrZero(b) }
	case "recommended":
		less = func(a, b CardDTO) bool { return a.recommendedScore() > b.recommendedScore() }
	case "delivery_fee":
		less = func(a, b CardDTO) bool { return a.gosend().Payable < b.gosend().Payable }
	case "delivery_time":
		less = func(a, b CardDTO) bool { return a.gosend().ETAMin < b.gosend().ETAMin }
	default: // relevant: urutan dasar
		return
	}
	sort.SliceStable(cards, func(i, j int) bool { return less(cards[i], cards[j]) })
}

func ratingOrZero(c CardDTO) float64 {
	if c.Merchant.RatingAvg == nil {
		return 0
	}
	return *c.Merchant.RatingAvg
}
