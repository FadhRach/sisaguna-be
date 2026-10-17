package rules

import (
	"math"
	"sort"
)

// MaxDeliveryKm: lebih jauh dari ini kualitas makanan turun, pengiriman tidak ditawarkan.
const MaxDeliveryKm = 15.0

type courierSpec struct {
	Label, App     string
	BaseFee, PerKm int
}

// Couriers memuat tarif dasar tiap kurir (port DeliveryPricing di app Android). Urutan tetap: dipakai sebagai
// pemecah seri saat diurutkan menurut ongkir.
var Couriers = []string{"gosend", "grab", "shopee", "lalamove"}

var courierSpecs = map[string]courierSpec{
	"gosend":   {"GoSend", "Gojek", 6_000, 2_500},
	"grab":     {"GrabExpress", "Grab", 6_500, 2_400},
	"shopee":   {"SPX Instant", "Shopee Express", 5_000, 2_600},
	"lalamove": {"Lalamove", "Lalamove", 8_000, 2_200},
}

type speedSpec struct {
	Label        string
	Factor       float64
	ExtraMinutes int
}

var speedSpecs = map[string]speedSpec{
	"priority": {"Prioritas", 1.35, 0},
	"standard": {"Standar", 1.0, 10},
	"saver":    {"Hemat", 0.75, 25},
}

// CourierLabel dan SpeedLabel dipakai untuk teks seperti "GoSend · Standar".
func CourierLabel(c string) string { return courierSpecs[c].Label }
func SpeedLabel(s string) string   { return speedSpecs[s].Label }

func ValidCourier(c string) bool { _, ok := courierSpecs[c]; return ok }
func ValidSpeed(s string) bool   { _, ok := speedSpecs[s]; return ok }

// DeliveryQuote adalah satu penawaran kurir.
type DeliveryQuote struct {
	Courier      string  `json:"courier"`
	CourierLabel string  `json:"courier_label"`
	Speed        string  `json:"speed"`
	SpeedLabel   string  `json:"speed_label"`
	Fee          int     `json:"fee"`
	Discount     int     `json:"discount"`
	Payable      int     `json:"payable"`
	ETAMin       int     `json:"eta_min"`
	ETAMax       int     `json:"eta_max"`
	PromoLabel   *string `json:"promo_label"`
	PromoLabelEN *string `json:"promo_label_en"`
}

// roundHalfUp meniru Kotlin roundToInt (math.Round membulatkan setengah menjauhi nol; hasilnya beda untuk angka negatif).
func roundHalfUp(x float64) int { return int(math.Floor(x + 0.5)) }

// QuoteDelivery adalah port DeliveryPricing.quote. shortcut: tabel promo di bawah adalah tiruan dari API promo
// kurir (sama seperti di app Android); ganti dengan pemanggilan API kurir setelah kemitraan terhubung.
func QuoteDelivery(courier, speed string, distanceKm float64, subtotal, prepMinutes int) DeliveryQuote {
	c, s := courierSpecs[courier], speedSpecs[speed]
	km := math.Max(distanceKm, 0.5)
	raw := (float64(c.BaseFee) + float64(c.PerKm)*km) * s.Factor
	fee := roundHalfUp(raw/500.0) * 500 // kurir menawar dalam kelipatan Rp500

	discount, promo, promoEN := 0, "", ""
	switch {
	case courier == "gosend" && subtotal >= 20_000:
		discount, promo, promoEN = min(fee/2, 5_000), "Diskon ongkir 50%", "50% off delivery"
	case courier == "grab" && speed == "saver":
		discount, promo, promoEN = min(3_000, fee), "Hemat Rp 3.000", "Save Rp 3,000"
	case courier == "shopee" && subtotal >= 15_000:
		discount, promo, promoEN = min(2_000, fee), "Potongan Rp 2.000", "Rp 2,000 off"
	}
	eta := prepMinutes + roundHalfUp(km*4) + 8 + s.ExtraMinutes
	q := DeliveryQuote{Courier: courier, CourierLabel: c.Label, Speed: speed, SpeedLabel: s.Label, Fee: fee, Discount: discount,
		Payable: max(fee-discount, 0), ETAMin: eta, ETAMax: eta + 10}
	if promo != "" {
		q.PromoLabel, q.PromoLabelEN = &promo, &promoEN
	}
	return q
}

// AllDeliveryQuotes: semua kurir untuk satu kecepatan, termurah dulu (seri mengikuti urutan Couriers).
func AllDeliveryQuotes(speed string, distanceKm float64, subtotal, prepMinutes int) []DeliveryQuote {
	out := make([]DeliveryQuote, 0, len(Couriers))
	for _, c := range Couriers {
		out = append(out, QuoteDelivery(c, speed, distanceKm, subtotal, prepMinutes))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Payable < out[j].Payable })
	return out
}
