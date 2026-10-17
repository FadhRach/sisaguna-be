package listing

import (
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
)

func tstz(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func floatArg(f *float64) pgtype.Float8 {
	if f == nil {
		return pgtype.Float8{}
	}
	return pgtype.Float8{Float64: *f, Valid: true}
}

func intPtr(i pgtype.Int4) *int {
	if !i.Valid {
		return nil
	}
	v := int(i.Int32)
	return &v
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func ratingPtr(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return nil
	}
	return &f.Float64
}

func ToAllergens(in []string) []sqlc.Allergen { return toAllergens(in) }

func AllergenStrings(in []sqlc.Allergen) []string { return fromAllergens(in) }

func toAllergens(in []string) []sqlc.Allergen {
	out := make([]sqlc.Allergen, 0, len(in))
	for _, a := range in {
		out = append(out, sqlc.Allergen(a))
	}
	return out
}

func fromAllergens(in []sqlc.Allergen) []string {
	out := make([]string, 0, len(in))
	for _, a := range in {
		out = append(out, string(a))
	}
	return out
}

func toChecks(in []string) []sqlc.SafetyCheck {
	out := make([]sqlc.SafetyCheck, 0, len(in))
	for _, c := range in {
		out = append(out, sqlc.SafetyCheck(c))
	}
	return out
}

func fromChecks(in []sqlc.SafetyCheck) []string {
	out := make([]string, 0, len(in))
	for _, c := range in {
		out = append(out, string(c))
	}
	return out
}

// strPtr: string kosong dari SQL (coalesce) berarti tidak ada.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// km mengubah meter dari SQL ke km 1 desimal; negatif (-1) berarti jarak tidak dihitung.
func km(m float64) *float64 {
	if m < 0 {
		return nil
	}
	v := math.Round(m/100) / 10
	return &v
}
