// Package notification: pembuatan notifikasi (dipakai domain lain di dalam transaksi mereka)
// dan endpoint daftar notifikasi pengguna.
package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/platform/events"
)

// New adalah satu notifikasi yang akan dibuat.
type New struct {
	UserID uuid.UUID
	Type   sqlc.NotificationType
	Title  string
	Body   string
	Data   map[string]any // mis. {"order_id": "..."} untuk deep link
}

// Create menyimpan notifikasi (kalau preferensi pengguna mengizinkan) dan mengirim event
// notification.created. Harus dipanggil dengan Queries yang terikat transaksi pemanggil.
func Create(ctx context.Context, q *sqlc.Queries, n New) error {
	if err := q.EnsureNotificationPrefs(ctx, n.UserID); err != nil {
		return fmt.Errorf("notification prefs: %w", err)
	}
	data, err := json.Marshal(n.Data)
	if err != nil || n.Data == nil {
		data = []byte("{}")
	}
	row, err := q.CreateNotification(ctx, sqlc.CreateNotificationParams{UserID: n.UserID, Type: n.Type, Title: n.Title, Body: n.Body, Data: data})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // dimatikan di preferensi
	}
	if err != nil {
		return fmt.Errorf("notification create: %w", err)
	}
	return events.Emit(ctx, q, "notification.created",
		map[string]any{"notification_id": row.ID, "type": row.Type, "title": row.Title}, []uuid.UUID{n.UserID}, "")
}
