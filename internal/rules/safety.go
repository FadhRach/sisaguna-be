package rules

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Jam aman per cara simpan untuk makanan siap santap (tier human).
var safeHours = map[string]time.Duration{
	"room_temp": 4 * time.Hour,
	"shelf":     72 * time.Hour,
	"chilled":   48 * time.Hour,
	"frozen":    168 * time.Hour,
}

// RequiredChecks wajib dicentang untuk tier human.
var RequiredChecks = []string{"untouched", "sensory_ok", "stored_right", "packed_clean"}

// SafeUntil = madeAt + jam aman storage. ok=false kalau storage tidak dikenal.
func SafeUntil(madeAt time.Time, storage string) (time.Time, bool) {
	h, ok := safeHours[storage]
	return madeAt.Add(h), ok
}

// MissingChecks mengembalikan checklist wajib yang belum dicentang.
func MissingChecks(checks []string) []string {
	have := make(map[string]bool, len(checks))
	for _, c := range checks {
		have[c] = true
	}
	var missing []string
	for _, c := range RequiredChecks {
		if !have[c] {
			missing = append(missing, c)
		}
	}
	return missing
}

var storageLabel = map[string]string{
	"room_temp": "Suhu ruang (masakan)",
	"shelf":     "Suhu ruang, kering (roti/kue)",
	"chilled":   "Kulkas (1-5°C)",
	"frozen":    "Freezer",
}

// AllChecks adalah semua butir checklist (yang terakhir, hygiene, tidak wajib).
var AllChecks = []string{"untouched", "sensory_ok", "stored_right", "packed_clean", "hygiene"}

// Assessment adalah port FoodSafety.assess di app Android (teks sama persis).
// Level: safe | caution | unsafe | not_for_humans.
type Assessment struct {
	Level           string     `json:"level"`
	HoursSinceMade  *int64     `json:"hours_since_made"`
	SafeUntil       *time.Time `json:"safe_until"`
	PassedChecks    []string   `json:"passed_checks"`
	MissingRequired []string   `json:"missing_required"`
	Headline        string     `json:"headline"`
	Advice          string     `json:"advice"`
	SuggestTier     *string    `json:"suggest_tier"` // terisi kalau sudah lewat batas aman
	Score           int        `json:"score"`        // butir yang lolos dari 5
}

// Assess menilai kelayakan. Urutan: bukan human -> checklist wajib kurang -> jendela aman terpakai >= 100% -> >= 75% -> aman.
func Assess(tier string, madeAt *time.Time, storage string, checks []string, now time.Time) Assessment {
	passed := make([]string, 0, len(checks))
	score := 0
	for _, c := range AllChecks {
		if slices.Contains(checks, c) {
			passed = append(passed, c)
			score++
		}
	}
	var hours *int64
	if madeAt != nil {
		h := int64(now.Sub(*madeAt) / time.Hour)
		hours = &h
	}
	a := Assessment{HoursSinceMade: hours, PassedChecks: passed, MissingRequired: []string{}, Score: score}

	if tier != "human" {
		a.Level, a.Advice = "not_for_humans", "Bukan untuk dikonsumsi manusia. Pastikan bebas plastik dan benda tajam."
		a.Headline = "Khusus bahan kompos"
		if tier == "animal_feed" {
			a.Headline = "Khusus pakan ternak"
		}
		return a
	}
	if missing := MissingChecks(checks); len(missing) > 0 {
		a.Level, a.MissingRequired = "unsafe", missing
		a.Headline = "Checklist kelayakan belum lengkap"
		a.Advice = fmt.Sprintf("Lengkapi %d poin wajib sebelum makanan bisa dijual untuk dikonsumsi.", len(missing))
		if madeAt != nil {
			a.SafeUntil = safeUntilPtr(*madeAt, storage)
		}
		return a
	}
	if madeAt != nil {
		a.SafeUntil = safeUntilPtr(*madeAt, storage)
	}
	var used float64 = -1
	if a.SafeUntil != nil {
		window := max(int64(a.SafeUntil.Sub(*madeAt)/time.Minute), 1)
		used = float64(int64(now.Sub(*madeAt)/time.Minute)) / float64(window)
	}
	switch {
	case used >= 1.0:
		feed := "animal_feed"
		a.Level, a.SuggestTier = "unsafe", &feed
		a.Headline = "Lewat batas aman konsumsi"
		h, _ := safeHours[storage]
		a.Advice = fmt.Sprintf("Sudah %d jam sejak dimasak (%s maks. %d jam). Alihkan jadi pakan ternak atau kompos.",
			*hours, strings.ToLower(storageLabel[storage]), int64(h/time.Hour))
	case used >= 0.75:
		a.Level, a.Headline = "caution", "Layak, segera dikonsumsi"
		a.Advice = "Mendekati batas aman. Sarankan pembeli langsung makan atau simpan di kulkas."
	default:
		a.Level, a.Headline = "safe", "Lolos cek kelayakan"
		a.Advice = "Checklist wajib lengkap dan masih dalam batas waktu aman."
	}
	return a
}

func safeUntilPtr(madeAt time.Time, storage string) *time.Time {
	t, ok := SafeUntil(madeAt, storage)
	if !ok {
		return nil
	}
	t = t.UTC() // JSON selalu UTC
	return &t
}
