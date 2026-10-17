// Package events mengirim event realtime lewat pg_notify('sg_events', ...) di dalam transaksi
// yang sama dengan perubahan datanya (NOTIFY baru terkirim saat commit). Hub WebSocket yang
// mendengarkan channel ini dibuat di M4; payload hanya tipe, penerima, dan data ringkas.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const maxPayloadBytes = 8000 // batas pg_notify

type Event struct {
	Type    string      `json:"type"`
	UserIDs []uuid.UUID `json:"user_ids,omitempty"`
	Topic   string      `json:"topic,omitempty"` // mis. listing:<id>
	At      time.Time   `json:"at"`
	Data    any         `json:"data"`
}

// Notifier adalah bagian sqlc.Queries (terikat transaksi) yang dipakai untuk mengirim.
type Notifier interface {
	Notify(ctx context.Context, payload string) error
}

func Emit(ctx context.Context, n Notifier, typ string, data any, userIDs []uuid.UUID, topic string) error {
	b, err := json.Marshal(Event{Type: typ, UserIDs: userIDs, Topic: topic, At: time.Now().UTC(), Data: data})
	if err != nil {
		return fmt.Errorf("events marshal: %w", err)
	}
	if len(b) > maxPayloadBytes {
		return fmt.Errorf("events: payload %d byte melebihi batas %d", len(b), maxPayloadBytes)
	}
	return n.Notify(ctx, string(b))
}
