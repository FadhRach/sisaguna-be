package rules

// RescueTier: tingkat penyelamat menurut jumlah porsi (port RescueTier di app Android).
type RescueTier struct {
	Key         string
	MinPortions int
}

var rescueTiers = []RescueTier{{"starter", 0}, {"rescuer", 10}, {"hero", 30}, {"legend", 60}}

func TierOf(portions int) RescueTier {
	t := rescueTiers[0]
	for _, c := range rescueTiers {
		if portions >= c.MinPortions {
			t = c
		}
	}
	return t
}

// CarbonKg adalah ESTIMASI kasar 0,9 kg CO2e per porsi, sama dengan app Android. shortcut: faktor emisi belum
// diverifikasi (sumber resmi); ganti angka ini saat sudah ada.
func CarbonKg(portions int) int { return int(float64(portions) * 0.9) }
