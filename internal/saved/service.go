// Package saved: toko favorit pembeli.
package saved

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/platform/dbconv"
	pmedia "github.com/FadhRach/sisaguna-be/internal/platform/media"
)

var ErrMerchantNotFound = errors.New("toko tidak ditemukan")

type Service struct {
	q    *sqlc.Queries
	urls pmedia.URLs
}

func NewService(pool *pgxpool.Pool, urls pmedia.URLs) *Service {
	return &Service{q: sqlc.New(pool), urls: urls}
}

// Save idempoten (dipakai juga untuk tombol Batalkan setelah unsave). Hanya toko approved.
func (s *Service) Save(ctx context.Context, user, merchantID uuid.UUID) error {
	m, err := s.q.GetMerchantBrief(ctx, merchantID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && m.Status != sqlc.MerchantStatusApproved) {
		return ErrMerchantNotFound
	}
	if err != nil {
		return fmt.Errorf("saved service save: %w", err)
	}
	if err := s.q.SaveMerchant(ctx, sqlc.SaveMerchantParams{UserID: user, MerchantID: merchantID}); err != nil {
		return fmt.Errorf("saved service save: %w", err)
	}
	return nil
}

// Remove idempoten: menghapus toko yang tidak ada di daftar tetap berhasil.
func (s *Service) Remove(ctx context.Context, user, merchantID uuid.UUID) error {
	if err := s.q.UnsaveMerchant(ctx, sqlc.UnsaveMerchantParams{UserID: user, MerchantID: merchantID}); err != nil {
		return fmt.Errorf("saved service remove: %w", err)
	}
	return nil
}

func (s *Service) List(ctx context.Context, user uuid.UUID, q ListQuery) ([]MerchantCardDTO, error) {
	p := sqlc.ListSavedMerchantsParams{UserID: user}
	if q.Lat != nil && q.Lng != nil {
		p.Lat = pgtype.Float8{Float64: *q.Lat, Valid: true}
		p.Lng = pgtype.Float8{Float64: *q.Lng, Valid: true}
	}
	rows, err := s.q.ListSavedMerchants(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("saved service list: %w", err)
	}
	out := make([]MerchantCardDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, MerchantCardDTO{ID: r.ID.String(), Name: r.Name, LocationLabel: r.LocationLabel, TodaysOffer: dbconv.Text(r.TodaysOffer),
			RatingAvg: dbconv.Rating(r.RatingAvg), RatingCount: int(r.RatingCount), IsVerified: r.IsVerified,
			DistanceKm: dbconv.Km(r.DistanceM), SavedAt: r.SavedAt.Time.UTC(), BannerURL: s.urls.PublicPtr(dbconv.Text(r.BannerPath)),
			AvailableCount: int(r.AvailableCount), NextPickupEnd: dbconv.Time(r.NextPickupEnd)})
	}
	return out, nil
}
