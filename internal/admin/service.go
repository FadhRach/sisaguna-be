// Package admin: tinjauan toko oleh admin. Komplain dan voucher admin menyusul.
package admin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/notification"
	"github.com/FadhRach/sisaguna-be/internal/platform/database"
	"github.com/FadhRach/sisaguna-be/internal/platform/events"
)

var ErrMerchantNotFound = errors.New("toko tidak ditemukan")

type MerchantRow struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Address    string    `json:"address"`
	OwnerName  string    `json:"owner_name"`
	OwnerEmail string    `json:"owner_email"`
	CreatedAt  time.Time `json:"created_at"`
}

type Service struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool, q: sqlc.New(pool)} }

func (s *Service) ListMerchants(ctx context.Context, status string, page, limit int) ([]MerchantRow, int64, error) {
	p := sqlc.ListMerchantsForAdminParams{Lim: int32(limit), Off: int32((page - 1) * limit)}
	if status != "" {
		p.Status = sqlc.NullMerchantStatus{MerchantStatus: sqlc.MerchantStatus(status), Valid: true}
	}
	rows, err := s.q.ListMerchantsForAdmin(ctx, p)
	if err != nil {
		return nil, 0, fmt.Errorf("admin service list merchants: %w", err)
	}
	var total int64
	out := make([]MerchantRow, 0, len(rows))
	for _, r := range rows {
		total = r.Total
		out = append(out, MerchantRow{ID: r.ID.String(), Name: r.Name, Status: string(r.Status), Address: r.Address,
			OwnerName: r.OwnerName, OwnerEmail: r.OwnerEmail, CreatedAt: r.CreatedAt.Time.UTC()})
	}
	return out, total, nil
}

// SetMerchantStatus menyetujui atau menolak toko; pemilik mendapat notifikasi dan event merchant.status_changed.
func (s *Service) SetMerchantStatus(ctx context.Context, id uuid.UUID, status sqlc.MerchantStatus) error {
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		m, err := q.SetMerchantStatus(ctx, sqlc.SetMerchantStatusParams{ID: id, Status: status})
		if err != nil {
			return err
		}
		title, body := "Toko disetujui", m.Name+" sudah bisa memposting makanan."
		if status == sqlc.MerchantStatusRejected {
			title, body = "Toko ditolak", "Pengajuan "+m.Name+" belum bisa disetujui."
		}
		if err := notification.Create(ctx, q, notification.New{UserID: m.OwnerID, Type: sqlc.NotificationTypeMerchant, Title: title, Body: body,
			Data: map[string]any{"merchant_id": id}}); err != nil {
			return err
		}
		return events.Emit(ctx, q, "merchant.status_changed", map[string]any{"merchant_id": id, "status": status}, []uuid.UUID{m.OwnerID}, "")
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMerchantNotFound
	}
	if err != nil {
		return fmt.Errorf("admin service set merchant status: %w", err)
	}
	return nil
}
