package order

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/notification"
	"github.com/FadhRach/sisaguna-be/internal/platform/apperr"
	"github.com/FadhRach/sisaguna-be/internal/platform/events"
)

// CancelByBuyer: hanya saat ready dan sebelum pickup_by. Stok dan voucher dikembalikan.
func (s *Service) CancelByBuyer(ctx context.Context, buyer, orderID uuid.UUID, reason string) error {
	return s.cancel(ctx, orderID, sqlc.ChatSideBuyer, buyer, reason)
}

// CancelByMerchant: hanya saat ready, alasan wajib (divalidasi handler).
func (s *Service) CancelByMerchant(ctx context.Context, owner, orderID uuid.UUID, reason string) error {
	return s.cancel(ctx, orderID, sqlc.ChatSideMerchant, owner, reason)
}

func (s *Service) cancel(ctx context.Context, orderID uuid.UUID, by sqlc.ChatSide, actor uuid.UUID, reason string) error {
	err := s.repo.InTx(ctx, func(r *Repository) error {
		o, err := r.q.LockOrder(ctx, orderID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		m, err := r.q.GetMerchantBrief(ctx, o.MerchantID)
		if err != nil {
			return err
		}
		// bukan pihak order -> pura-pura tidak ada
		if (by == sqlc.ChatSideBuyer && o.BuyerID != actor) || (by == sqlc.ChatSideMerchant && m.OwnerID != actor) {
			return ErrNotFound
		}
		if o.Status != sqlc.OrderStatusReady {
			return ErrInvalidState
		}
		if by == sqlc.ChatSideBuyer && o.PickupBy.Valid && !s.now().Before(o.PickupBy.Time) {
			return ErrCancelWindowClosed
		}
		if err := s.release(ctx, r.q, o, true); err != nil {
			return err
		}
		if err := r.q.SetOrderCancelled(ctx, sqlc.SetOrderCancelledParams{
			ID: o.ID, Status: sqlc.OrderStatusCancelled, CancelledBy: sqlc.NullChatSide{ChatSide: by, Valid: true},
			Reason: pgtype.Text{String: strings.TrimSpace(reason), Valid: strings.TrimSpace(reason) != ""}}); err != nil {
			return err
		}
		// pihak lawan diberi tahu
		other, title, body := m.OwnerID, "Pesanan dibatalkan pembeli", "Pembeli membatalkan pesanan."
		if by == sqlc.ChatSideMerchant {
			other, title, body = o.BuyerID, "Pesanan dibatalkan toko", m.Name+" membatalkan pesananmu."
			if reason != "" {
				body += " Alasan: " + strings.TrimSpace(reason)
			}
		}
		if err := notification.Create(ctx, r.q, notification.New{UserID: other, Type: sqlc.NotificationTypeOrder, Title: title, Body: body,
			Data: map[string]any{"order_id": o.ID}}); err != nil {
			return err
		}
		return emitUpdated(ctx, r.q, o, m.OwnerID, sqlc.OrderStatusCancelled)
	})
	return wrapUnknown("order service cancel", err)
}

// release mengembalikan stok (kalau listing masih aktif dan jendela belum lewat) dan, kalau restoreVoucher,
// voucher yang dipakai. No-show tidak mengembalikan voucher (pembeli yang tidak datang).
func (s *Service) release(ctx context.Context, q *sqlc.Queries, o sqlc.Order, restoreVoucher bool) error {
	items, err := q.ListOrderItems(ctx, []uuid.UUID{o.ID})
	if err != nil {
		return err
	}
	for _, it := range items {
		stock, err := q.RestoreStock(ctx, sqlc.RestoreStockParams{ID: it.ListingID, Qty: it.Quantity})
		if errors.Is(err, pgx.ErrNoRows) {
			continue // listing sudah tidak aktif atau jendela lewat
		}
		if err != nil {
			return err
		}
		if err := events.Emit(ctx, q, "listing.stock_changed", map[string]any{"listing_id": it.ListingID, "stock": stock}, nil, "listing:"+it.ListingID.String()); err != nil {
			return err
		}
	}
	if !restoreVoucher {
		return nil
	}
	code, err := q.ReleaseVoucherOfOrder(ctx, uuid.NullUUID{UUID: o.ID, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return q.DecrementVoucherUsage(ctx, code)
}

var pickupDigits = regexp.MustCompile(`^(?:SG-?)?(\d{4})$`)

// normalizeCode: "4821", "sg-4821", " SG4821 " -> "SG-4821". Bentuk lain dikembalikan apa adanya (pasti salah).
func normalizeCode(raw string) string {
	up := strings.ToUpper(strings.TrimSpace(raw))
	if m := pickupDigits.FindStringSubmatch(up); m != nil {
		return "SG-" + m[1]
	}
	return up
}

// Confirm dipakai mitra untuk menyerahkan pesanan: kode harus cocok. 5 kode salah dalam 10 menit
// mengunci konfirmasi order itu sampai percobaan tertua berumur lebih dari 10 menit.
func (s *Service) Confirm(ctx context.Context, owner, orderID uuid.UUID, rawCode string) error {
	wrong, left := false, 0
	err := s.repo.InTx(ctx, func(r *Repository) error {
		o, err := r.q.LockOrder(ctx, orderID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		m, err := r.q.GetMerchantBrief(ctx, o.MerchantID)
		if err != nil {
			return err
		}
		if m.OwnerID != owner {
			return ErrNotFound
		}
		if o.Status != sqlc.OrderStatusReady {
			return ErrInvalidState
		}
		attempts, err := r.q.CountRecentWrongCodes(ctx, o.ID)
		if err != nil {
			return err
		}
		if attempts >= maxCodeAttempts {
			return ErrTooManyAttempts
		}
		if normalizeCode(rawCode) != o.Code {
			wrong, left = true, int(maxCodeAttempts-attempts-1)
			return r.q.AddWrongCodeAttempt(ctx, o.ID) // di-commit; error dikembalikan setelah tx
		}
		if err := r.q.SetOrderCompleted(ctx, o.ID); err != nil {
			return err
		}
		items, err := r.q.ListOrderItems(ctx, []uuid.UUID{o.ID})
		if err != nil {
			return err
		}
		qty := int32(0)
		for _, it := range items {
			qty += it.Quantity
		}
		if err := r.q.IncMerchantRescued(ctx, sqlc.IncMerchantRescuedParams{ID: m.ID, TotalRescued: qty}); err != nil {
			return err
		}
		first := ""
		if len(items) > 0 {
			first = items[0].Title
		}
		if err := notification.Create(ctx, r.q, notification.New{UserID: o.BuyerID, Type: sqlc.NotificationTypeOrder, Title: "Pesanan selesai",
			Body: fmt.Sprintf("Terima kasih sudah menyelamatkan %s. Beri rating untuk %s?", first, m.Name), Data: map[string]any{"order_id": o.ID}}); err != nil {
			return err
		}
		return emitUpdated(ctx, r.q, o, m.OwnerID, sqlc.OrderStatusCompleted)
	})
	if err == nil && wrong {
		return apperr.WithFields(ErrWrongCode, map[string]any{"attempts_left": left})
	}
	return wrapUnknown("order service confirm", err)
}

func emitUpdated(ctx context.Context, q *sqlc.Queries, o sqlc.Order, ownerID uuid.UUID, status sqlc.OrderStatus) error {
	return events.Emit(ctx, q, "order.updated", map[string]any{"order_id": o.ID, "status": status}, []uuid.UUID{o.BuyerID, ownerID}, "")
}
