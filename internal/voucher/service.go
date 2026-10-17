// Package voucher: daftar dan klaim voucher. Pemakaian voucher terjadi di checkout (package order).
package voucher

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/platform/dbconv"
	pmedia "github.com/FadhRach/sisaguna-be/internal/platform/media"
)

var (
	ErrUnavailable    = errors.New("voucher tidak tersedia")
	ErrAlreadyClaimed = errors.New("voucher sudah kamu klaim")
)

type Service struct {
	q    *sqlc.Queries
	urls pmedia.URLs
}

func NewService(pool *pgxpool.Pool, urls pmedia.URLs) *Service {
	return &Service{q: sqlc.New(pool), urls: urls}
}

func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]VoucherDTO, error) {
	rows, err := s.q.ListUserVouchers(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("voucher service list: %w", err)
	}
	out := make([]VoucherDTO, 0, len(rows))
	for _, r := range rows {
		d := VoucherDTO{Code: r.Code, Title: r.Title, Description: r.Description, PercentOff: int2Ptr(r.PercentOff),
			FlatOff: int4Ptr(r.FlatOff), MaxDiscount: int4Ptr(r.MaxDiscount), MinSpend: int(r.MinSpend),
			ExpiresAt: r.ExpiresAt.Time.UTC(), Terms: r.Terms, AccentHex: r.AccentHex, Status: "available",
			AppliesTo: r.AppliesToLabel, ImageURL: s.urls.PublicPtr(dbconv.Text(r.ImagePath)), MerchantName: dbconv.Text(r.MerchantName)}
		if r.MerchantID.Valid {
			m := r.MerchantID.UUID.String()
			d.MerchantID = &m
		}
		if r.AppliesToTier.Valid {
			t := string(r.AppliesToTier.ListingTier)
			d.AppliesToTier = &t
		}
		switch {
		case r.UsedAt.Valid:
			d.Status = "used"
		case r.Claimed:
			d.Status = "claimed"
		}
		out = append(out, d)
	}
	return out, nil
}

func (s *Service) Claim(ctx context.Context, userID uuid.UUID, code string) error {
	code = strings.ToUpper(strings.TrimSpace(code))
	v, err := s.q.GetVoucherState(ctx, code)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !v.Valid) {
		return ErrUnavailable
	}
	if err != nil {
		return fmt.Errorf("voucher service claim: %w", err)
	}
	n, err := s.q.ClaimVoucher(ctx, sqlc.ClaimVoucherParams{UserID: userID, VoucherCode: code})
	if err != nil {
		return fmt.Errorf("voucher service claim: %w", err)
	}
	if n == 0 {
		return ErrAlreadyClaimed
	}
	return nil
}

func int2Ptr(i pgtype.Int2) *int {
	if !i.Valid {
		return nil
	}
	v := int(i.Int16)
	return &v
}

func int4Ptr(i pgtype.Int4) *int {
	if !i.Valid {
		return nil
	}
	v := int(i.Int32)
	return &v
}
