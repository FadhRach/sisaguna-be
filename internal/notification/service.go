package notification

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
)

var ErrNotFound = errors.New("notifikasi tidak ditemukan")

type Service struct{ q *sqlc.Queries }

func NewService(pool *pgxpool.Pool) *Service { return &Service{q: sqlc.New(pool)} }

// List mengembalikan satu halaman (terbaru dulu) dan cursor halaman berikutnya ("" kalau habis).
func (s *Service) List(ctx context.Context, userID uuid.UUID, typ string, cur *httpx.Cursor, limit int) ([]NotificationDTO, string, error) {
	p := sqlc.ListNotificationsParams{UserID: userID, Lim: int32(limit + 1)}
	if typ != "" {
		p.Type = sqlc.NullNotificationType{NotificationType: sqlc.NotificationType(typ), Valid: true}
	}
	if cur != nil {
		p.CurAt = pgtype.Timestamptz{Time: cur.At, Valid: true}
		p.CurID = uuid.NullUUID{UUID: cur.ID, Valid: true}
	}
	rows, err := s.q.ListNotifications(ctx, p)
	if err != nil {
		return nil, "", fmt.Errorf("notification service list: %w", err)
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		next = httpx.EncodeCursor(last.CreatedAt.Time, last.ID)
	}
	out := make([]NotificationDTO, 0, len(rows))
	for _, r := range rows {
		d := NotificationDTO{ID: r.ID.String(), Type: string(r.Type), Title: r.Title, Body: r.Body, Data: r.Data, CreatedAt: r.CreatedAt.Time.UTC()}
		if r.ReadAt.Valid {
			t := r.ReadAt.Time.UTC()
			d.ReadAt = &t
		}
		out = append(out, d)
	}
	return out, next, nil
}

func (s *Service) MarkRead(ctx context.Context, userID, id uuid.UUID) error {
	n, err := s.q.MarkNotificationRead(ctx, sqlc.MarkNotificationReadParams{ID: id, UserID: userID})
	if err != nil {
		return fmt.Errorf("notification service mark read: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	if err := s.q.MarkAllNotificationsRead(ctx, userID); err != nil {
		return fmt.Errorf("notification service mark all read: %w", err)
	}
	return nil
}

// Delete adalah soft delete; Restore membatalkannya (tombol Batalkan di snackbar).
func (s *Service) Delete(ctx context.Context, userID, id uuid.UUID) error {
	n, err := s.q.DeleteNotification(ctx, sqlc.DeleteNotificationParams{ID: id, UserID: userID})
	if err != nil {
		return fmt.Errorf("notification service delete: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) Restore(ctx context.Context, userID, id uuid.UUID) error {
	n, err := s.q.RestoreNotification(ctx, sqlc.RestoreNotificationParams{ID: id, UserID: userID})
	if err != nil {
		return fmt.Errorf("notification service restore: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Prefs membuat baris default dulu kalau pengguna belum pernah punya.
func (s *Service) Prefs(ctx context.Context, userID uuid.UUID) (*PrefsDTO, error) {
	if err := s.q.EnsureNotificationPrefs(ctx, userID); err != nil {
		return nil, fmt.Errorf("notification service prefs: %w", err)
	}
	p, err := s.q.GetNotificationPrefs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("notification service prefs: %w", err)
	}
	return &PrefsDTO{Enabled: p.Enabled, Pickup: p.Pickup, Order: p.Order, Merchant: p.Merchant, Promo: p.Promo, Impact: p.Impact}, nil
}

func (s *Service) SetPrefs(ctx context.Context, userID uuid.UUID, in PrefsRequest) (*PrefsDTO, error) {
	if err := s.q.EnsureNotificationPrefs(ctx, userID); err != nil {
		return nil, fmt.Errorf("notification service set prefs: %w", err)
	}
	err := s.q.SetNotificationPrefs(ctx, sqlc.SetNotificationPrefsParams{UserID: userID, Enabled: *in.Enabled, Pickup: *in.Pickup,
		OrderOn: *in.Order, Merchant: *in.Merchant, Promo: *in.Promo, Impact: *in.Impact})
	if err != nil {
		return nil, fmt.Errorf("notification service set prefs: %w", err)
	}
	return s.Prefs(ctx, userID)
}

func (s *Service) RegisterDevice(ctx context.Context, userID uuid.UUID, in DeviceRequest) error {
	if err := s.q.UpsertDeviceToken(ctx, sqlc.UpsertDeviceTokenParams{Token: in.Token, UserID: userID, Platform: in.Platform}); err != nil {
		return fmt.Errorf("notification service register device: %w", err)
	}
	return nil
}

func (s *Service) RemoveDevice(ctx context.Context, userID uuid.UUID, token string) error {
	if err := s.q.DeleteDeviceToken(ctx, sqlc.DeleteDeviceTokenParams{Token: token, UserID: userID}); err != nil {
		return fmt.Errorf("notification service remove device: %w", err)
	}
	return nil
}
