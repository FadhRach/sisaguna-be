package payment

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/platform/database"
)

var (
	ErrInvalidPhone = errors.New("nomor e-wallet tidak valid (minimal 8 digit)")
	ErrFixed        = errors.New("QRIS dan tunai selalu tersedia dan tidak bisa dilepas")
	ErrNotFound     = errors.New("metode pembayaran tidak ditemukan")
)

var labels = map[string]string{"qris": "QRIS", "gopay": "GoPay", "ovo": "OVO", "dana": "DANA", "cash": "Bayar saat ambil"}

type MethodDTO struct {
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	Detail    string `json:"detail"`
	IsDefault bool   `json:"is_default"`
}

type LinkRequest struct {
	Kind  string `json:"kind" binding:"required,oneof=gopay ovo dana"`
	Phone string `json:"phone" binding:"required,min=8,max=20"`
}

type DefaultRequest struct {
	Kind string `json:"kind" binding:"required,oneof=qris gopay ovo dana cash"`
}

type Service struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool, q: sqlc.New(pool)} }

// List: QRIS, e-wallet yang ditautkan, tunai. Baris QRIS dan tunai dibuat saat pertama kali dibuka.
func (s *Service) List(ctx context.Context, user uuid.UUID) ([]MethodDTO, error) {
	if err := s.q.EnsurePaymentMethods(ctx, user); err != nil {
		return nil, fmt.Errorf("payment service list: %w", err)
	}
	rows, err := s.q.ListPaymentMethods(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("payment service list: %w", err)
	}
	out := make([]MethodDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, MethodDTO{Kind: string(r.Kind), Label: labels[string(r.Kind)], Detail: r.Detail, IsDefault: r.IsDefault})
	}
	return out, nil
}

// Link menautkan e-wallet (tiruan: tidak ada verifikasi ke penyedia). Menautkan ulang mengganti nomornya.
func (s *Service) Link(ctx context.Context, user uuid.UUID, in LinkRequest) ([]MethodDTO, error) {
	masked := MaskPhone(in.Phone)
	if len(masked) < 8 {
		return nil, ErrInvalidPhone
	}
	if err := s.q.EnsurePaymentMethods(ctx, user); err != nil {
		return nil, fmt.Errorf("payment service link: %w", err)
	}
	if err := s.q.LinkPaymentMethod(ctx, sqlc.LinkPaymentMethodParams{UserID: user, Kind: sqlc.PaymentKind(in.Kind), Detail: masked}); err != nil {
		return nil, fmt.Errorf("payment service link: %w", err)
	}
	return s.List(ctx, user)
}

// Unlink melepas e-wallet; kalau itu default, QRIS menjadi default lagi.
func (s *Service) Unlink(ctx context.Context, user uuid.UUID, kind string) ([]MethodDTO, error) {
	if kind == "qris" || kind == "cash" {
		return nil, ErrFixed
	}
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		wasDefault, err := q.UnlinkPaymentMethod(ctx, sqlc.UnlinkPaymentMethodParams{UserID: user, Kind: sqlc.PaymentKind(kind)})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // sudah tidak ada: idempoten
		}
		if err != nil {
			return err
		}
		if wasDefault {
			_, err = q.SetDefaultPayment(ctx, sqlc.SetDefaultPaymentParams{UserID: user, Kind: sqlc.PaymentKindQris})
		}
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("payment service unlink: %w", err)
	}
	return s.List(ctx, user)
}

func (s *Service) SetDefault(ctx context.Context, user uuid.UUID, kind string) ([]MethodDTO, error) {
	if err := s.q.EnsurePaymentMethods(ctx, user); err != nil {
		return nil, fmt.Errorf("payment service default: %w", err)
	}
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		ok, err := q.HasPaymentMethod(ctx, sqlc.HasPaymentMethodParams{UserID: user, Kind: sqlc.PaymentKind(kind)})
		if err != nil {
			return err
		}
		if !ok {
			return ErrNotFound
		}
		if err := q.ClearDefaultPayment(ctx, user); err != nil {
			return err
		}
		_, err = q.SetDefaultPayment(ctx, sqlc.SetDefaultPaymentParams{UserID: user, Kind: sqlc.PaymentKind(kind)})
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("payment service default: %w", err)
	}
	return s.List(ctx, user)
}
