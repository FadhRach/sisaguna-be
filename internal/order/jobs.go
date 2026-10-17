package order

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/notification"
)

// ExpireNoShows: order ready yang melewati pickup_by + 15 menit menjadi no_show; stok dikembalikan
// (voucher tidak). Dijalankan scheduler tiap menit di bawah advisory lock.
func (s *Service) ExpireNoShows(ctx context.Context) error {
	ids, err := s.repo.q.ListNoShowCandidates(ctx)
	if err != nil {
		return fmt.Errorf("order jobs no-show list: %w", err)
	}
	for _, id := range ids {
		if err := s.noShow(ctx, id); err != nil {
			slog.Error("no-show gagal", "order_id", id, "err", err) // satu order gagal tidak menghentikan yang lain
		}
	}
	return nil
}

func (s *Service) noShow(ctx context.Context, id uuid.UUID) error {
	return s.repo.InTx(ctx, func(r *Repository) error {
		o, err := r.q.LockOrder(ctx, id)
		if err != nil {
			return err
		}
		due := o.PickupBy.Time
		if !o.PickupBy.Valid {
			due = o.PickupEnd.Time.Add(2 * time.Hour) // pengiriman, sama dengan ListNoShowCandidates
		}
		if o.Status != sqlc.OrderStatusReady || s.now().Before(due.Add(noShowGrace)) {
			return nil // sudah berubah sejak daftar kandidat dibuat
		}
		m, err := r.q.GetMerchantBrief(ctx, o.MerchantID)
		if err != nil {
			return err
		}
		if err := s.release(ctx, r.q, o, false); err != nil {
			return err
		}
		if err := r.q.SetOrderCancelled(ctx, sqlc.SetOrderCancelledParams{ID: o.ID, Status: sqlc.OrderStatusNoShow}); err != nil {
			return err
		}
		if err := notification.Create(ctx, r.q, notification.New{UserID: o.BuyerID, Type: sqlc.NotificationTypeOrder, Title: "Pesanan tidak diambil",
			Body: "Pesanan " + o.Code + " di " + m.Name + " dibatalkan karena tidak diambil.", Data: map[string]any{"order_id": o.ID}}); err != nil {
			return err
		}
		return emitUpdated(ctx, r.q, o, m.OwnerID, sqlc.OrderStatusNoShow)
	})
}

// SendReminders: order ready yang batas ambilnya kurang dari 30 menit lagi mendapat satu notifikasi pickup.
func (s *Service) SendReminders(ctx context.Context) error {
	ids, err := s.repo.q.ListReminderCandidates(ctx)
	if err != nil {
		return fmt.Errorf("order jobs reminder list: %w", err)
	}
	for _, id := range ids {
		if err := s.remind(ctx, id); err != nil {
			slog.Error("reminder gagal", "order_id", id, "err", err)
		}
	}
	return nil
}

func (s *Service) remind(ctx context.Context, id uuid.UUID) error {
	return s.repo.InTx(ctx, func(r *Repository) error {
		o, err := r.q.LockOrder(ctx, id)
		if err != nil {
			return err
		}
		if o.Status != sqlc.OrderStatusReady || o.RemindedAt.Valid {
			return nil
		}
		m, err := r.q.GetMerchantBrief(ctx, o.MerchantID)
		if err != nil {
			return err
		}
		left := max(int(time.Until(o.PickupBy.Time).Minutes()), 1)
		if err := notification.Create(ctx, r.q, notification.New{UserID: o.BuyerID, Type: sqlc.NotificationTypePickup, Title: "Jangan lupa ambil pesananmu",
			Body: fmt.Sprintf("Pesanan %s di %s harus diambil dalam %d menit.", o.Code, m.Name, left), Data: map[string]any{"order_id": o.ID}}); err != nil {
			return err
		}
		return r.q.MarkReminded(ctx, o.ID) // ditandai juga kalau preferensi mematikan notifikasi, supaya tidak dicoba terus
	})
}
