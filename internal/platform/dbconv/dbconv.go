// Package dbconv mengubah tipe pgtype hasil sqlc ke nilai JSON-friendly (nil = NULL).
package dbconv

import (
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func Text(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func Time(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	u := t.Time.UTC()
	return &u
}

func Rating(n pgtype.Numeric) *float64 {
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return nil
	}
	return &f.Float64
}

// Km mengubah meter dari SQL ke km 1 desimal; negatif (-1) berarti jarak tidak dihitung.
func Km(m float64) *float64 {
	if m < 0 {
		return nil
	}
	v := math.Round(m/100) / 10
	return &v
}
