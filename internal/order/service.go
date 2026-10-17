// Package order: quote, checkout (tunai), pembatalan, konfirmasi pickup, riwayat, dashboard mitra,
// dan job terjadwal (no-show, reminder). Semua angka dihitung di server.
package order

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/notification"
	"github.com/FadhRach/sisaguna-be/internal/payment"
	"github.com/FadhRach/sisaguna-be/internal/platform/apperr"
	"github.com/FadhRach/sisaguna-be/internal/platform/database"
	"github.com/FadhRach/sisaguna-be/internal/platform/events"
	pmedia "github.com/FadhRach/sisaguna-be/internal/platform/media"
)

var (
	ErrDeliveryUnavailable = errors.New("pengantaran tidak tersedia untuk pesanan ini, gunakan ambil di toko")
	ErrPaymentUnavailable  = errors.New("metode pembayaran belum tersedia, gunakan tunai")
	ErrPaymentNotLinked    = errors.New("e-wallet belum ditautkan, tautkan dulu di metode pembayaran")
	ErrAddressNotFound     = errors.New("alamat tidak ditemukan")
	ErrMerchantUnavailable = errors.New("toko tidak tersedia")
	ErrMerchantRequired    = errors.New("akun ini belum punya toko")
	ErrOwnMerchant         = errors.New("tidak bisa memesan dari toko sendiri")
	ErrInvalidCart         = errors.New("keranjang tidak valid: semua item harus dari satu toko, tanpa duplikat")
	ErrListingUnavailable  = errors.New("listing sudah tidak tersedia")
	ErrOutOfStock          = errors.New("stok tidak cukup")
	ErrVoucherNotClaimed   = errors.New("voucher belum kamu klaim")
	ErrVoucherUsed         = errors.New("voucher sudah terpakai")
	ErrVoucherInvalid      = errors.New("voucher tidak berlaku untuk pesanan ini")
	ErrVoucherMinSpend     = errors.New("belanja belum mencapai minimum voucher")
	ErrPriceChanged        = errors.New("harga berubah, periksa ringkasan terbaru")
	ErrNotFound            = errors.New("pesanan tidak ditemukan")
	ErrInvalidState        = errors.New("status pesanan tidak mengizinkan aksi ini")
	ErrCancelWindowClosed  = errors.New("pesanan tidak bisa dibatalkan setelah batas pengambilan")
	ErrWrongCode           = errors.New("kode pickup salah")
	ErrTooManyAttempts     = errors.New("terlalu banyak kode salah, coba lagi 10 menit lagi")
)

const maxCodeAttempts = 5

var wib = time.FixedZone("WIB", 7*60*60)

type Service struct {
	repo     *Repository
	urls     pmedia.URLs
	now      func() time.Time
	partners bool // kurir + pembayaran non-tunai tiruan aktif (PARTNERS_MODE=dummy)
}

// WithPartners mengaktifkan pengiriman dan pembayaran non-tunai TIRUAN (tanpa kemitraan nyata).
func (s *Service) WithPartners(on bool) *Service {
	s.partners = on
	return s
}

func NewService(repo *Repository, urls pmedia.URLs) *Service {
	return &Service{repo: repo, urls: urls, now: time.Now}
}

// Quote menghitung rincian harga tanpa mengubah data apa pun.
func (s *Service) Quote(ctx context.Context, buyer uuid.UUID, req QuoteRequest) (*QuoteDTO, error) {
	p, err := s.price(ctx, s.repo.q, buyer, req, false)
	if err != nil {
		return nil, wrapUnknown("order service quote", err)
	}
	return s.quoteDTO(p), nil
}

// Place membuat order dalam satu transaksi (CLAUDE.md aturan 2). created=false berarti Idempotency-Key
// sudah pernah dipakai dan order lama dikembalikan.
func (s *Service) Place(ctx context.Context, buyer uuid.UUID, key string, req PlaceRequest) (id uuid.UUID, created bool, err error) {
	if existing, err := s.repo.q.GetOrderByIdempotency(ctx, sqlc.GetOrderByIdempotencyParams{BuyerID: buyer, IdempotencyKey: key}); err == nil {
		return existing, false, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, fmt.Errorf("order service place: %w", err)
	}

	err = s.repo.InTx(ctx, func(r *Repository) error {
		p, err := s.price(ctx, r.q, buyer, req.QuoteRequest, true)
		if err != nil {
			return err
		}
		if req.ExpectedTotal != nil && *req.ExpectedTotal != p.total {
			fields := map[string]any{"subtotal": p.subtotal, "voucher_discount": p.discount, "service_fee": p.amounts.ServiceFee,
				"delivery_fee": 0, "total": p.total}
			if p.delivery != nil {
				fields["delivery_fee"] = p.delivery.quote.Payable
			}
			return apperr.WithFields(ErrPriceChanged, fields)
		}
		kind, err := s.paymentKind(ctx, r.q, buyer, req.PaymentKind, p.total)
		if err != nil {
			return err
		}
		id, err = s.persist(ctx, r, buyer, key, req.Note, kind, p)
		return err
	})
	if database.IsUniqueViolation(err) { // dua request dengan key sama berbarengan
		if existing, e := s.repo.q.GetOrderByIdempotency(ctx, sqlc.GetOrderByIdempotencyParams{BuyerID: buyer, IdempotencyKey: key}); e == nil {
			return existing, false, nil
		}
	}
	if err != nil {
		return uuid.Nil, false, wrapUnknown("order service place", err)
	}
	return id, true, nil
}

// persist: kurangi stok, pakai voucher, simpan order + item, kirim notifikasi dan event. Dipanggil dalam tx.
func (s *Service) persist(ctx context.Context, r *Repository, buyer uuid.UUID, key, note, kind string, p *priced) (uuid.UUID, error) {
	for _, l := range p.lines {
		stock, err := r.q.DecrementStock(ctx, sqlc.DecrementStockParams{ID: l.listing.ID, Qty: int32(l.qty)})
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, apperr.WithFields(ErrOutOfStock, map[string]any{"listing_id": l.listing.ID})
		}
		if err != nil {
			return uuid.Nil, err
		}
		if err := events.Emit(ctx, r.q, "listing.stock_changed", map[string]any{"listing_id": l.listing.ID, "stock": stock}, nil, "listing:"+l.listing.ID.String()); err != nil {
			return uuid.Nil, err
		}
	}
	if p.voucher != "" {
		if n, err := r.q.ConsumeVoucherQuota(ctx, p.voucher); err != nil || n == 0 {
			if err != nil {
				return uuid.Nil, err
			}
			return uuid.Nil, ErrVoucherInvalid
		}
	}

	if err := payment.Charge(kind, p.total); err != nil { // tiruan: selalu berhasil
		return uuid.Nil, err
	}
	status := sqlc.PaymentStatusUnpaid
	if kind != "cash" {
		status = sqlc.PaymentStatusPaid
	}
	ins := sqlc.InsertOrderParams{
		BuyerID: buyer, MerchantID: p.merchant.ID, Note: note, Subtotal: int32(p.subtotal), OriginalTotal: int32(p.original),
		VoucherDiscount: int32(p.discount), ServiceFee: int32(p.amounts.ServiceFee), Total: int32(p.total),
		Commission: int32(p.amounts.Commission), MerchantPayout: int32(p.amounts.MerchantPayout), // ongkir tidak dikenai komisi
		Fulfillment: sqlc.FulfillmentPickup, PaymentKind: sqlc.PaymentKind(kind), PaymentStatus: status,
		PickupEnd: pgtype.Timestamptz{Time: p.pickupEnd, Valid: true}, IdempotencyKey: key,
	}
	if p.delivery == nil {
		ins.PickupBy = pgtype.Timestamptz{Time: p.pickupBy, Valid: true}
	} else {
		d, t := p.delivery.quote, p.delivery.target
		addr, _ := json.Marshal(AddressSnapshot{ID: t.ID.String(), Label: t.Label, FullAddress: t.FullAddress, Note: t.Note, Lat: t.Lat, Lng: t.Lng})
		ins.Fulfillment, ins.DeliveryFee, ins.DeliveryDiscount = sqlc.FulfillmentDelivery, int32(d.Payable), int32(d.Discount)
		ins.DeliveryCourier = sqlc.NullCourier{Courier: sqlc.Courier(d.Courier), Valid: true}
		ins.DeliverySpeed = sqlc.NullDeliverySpeed{DeliverySpeed: sqlc.DeliverySpeed(d.Speed), Valid: true}
		ins.DeliveryEtaMin = pgtype.Int2{Int16: int16(d.ETAMin), Valid: true}
		ins.DeliveryEtaMax = pgtype.Int2{Int16: int16(d.ETAMax), Valid: true}
		ins.DeliveryAddress = addr
	}
	if p.voucher != "" {
		ins.VoucherCode = pgtype.Text{String: p.voucher, Valid: true}
		ins.VoucherFundedBy = pgtype.Text{String: p.fundedBy, Valid: true}
	}
	id, code, err := s.insertWithUniqueCode(ctx, r, ins)
	if err != nil {
		return uuid.Nil, err
	}
	for _, l := range p.lines {
		orig := l.listing.PriceOriginal.Int32
		if !l.listing.PriceOriginal.Valid {
			orig = l.listing.Price.Int32
		}
		cover, err := firstMedia(ctx, r.q, l.listing.ID)
		if err != nil {
			return uuid.Nil, err
		}
		if err := r.q.InsertOrderItem(ctx, sqlc.InsertOrderItemParams{OrderID: id, ListingID: l.listing.ID, Title: l.listing.Title,
			Tier: l.listing.Tier, Unit: l.listing.Unit, ImageMediaID: cover, Quantity: int32(l.qty), UnitPrice: l.listing.Price.Int32, UnitOriginalPrice: orig}); err != nil {
			return uuid.Nil, err
		}
	}
	if p.voucher != "" {
		if n, err := r.q.MarkVoucherUsed(ctx, sqlc.MarkVoucherUsedParams{UserID: buyer, VoucherCode: p.voucher, OrderID: uuid.NullUUID{UUID: id, Valid: true}}); err != nil || n == 0 {
			if err != nil {
				return uuid.Nil, err
			}
			return uuid.Nil, ErrVoucherUsed
		}
	}
	return id, s.notifyCreated(ctx, r.q, id, code, buyer, p)
}

func (s *Service) notifyCreated(ctx context.Context, q *sqlc.Queries, id uuid.UUID, code string, buyer uuid.UUID, p *priced) error {
	items := 0
	for _, l := range p.lines {
		items += l.qty
	}
	// teks sama dengan notifikasi lokal di app Android
	if err := notification.Create(ctx, q, notification.New{UserID: buyer, Type: sqlc.NotificationTypeOrder, Title: fmt.Sprintf("Pesanan %s siap diambil", code),
		Body: fmt.Sprintf("%s sudah menyiapkan %d item. Tunjukkan kode %s saat ambil.", p.merchant.Name, items, code), Data: map[string]any{"order_id": id}}); err != nil {
		return err
	}
	if err := notification.Create(ctx, q, notification.New{UserID: p.merchant.OwnerID, Type: sqlc.NotificationTypeOrder, Title: "Pesanan baru",
		Body: fmt.Sprintf("%d item senilai Rp%d menunggu diambil.", items, p.total), Data: map[string]any{"order_id": id}}); err != nil {
		return err
	}
	return events.Emit(ctx, q, "order.created", map[string]any{"order_id": id, "item_count": items, "total": p.total}, // tanpa code: mitra harus mendapat kode dari pembeli
		[]uuid.UUID{p.merchant.OwnerID}, "")
}

// paymentKind memutuskan metode yang dipakai. Total 0 selalu tunai (makanan gratis tidak perlu dibayar).
// Non-tunai hanya saat mitra tiruan aktif; e-wallet harus sudah ditautkan.
func (s *Service) paymentKind(ctx context.Context, q *sqlc.Queries, buyer uuid.UUID, kind string, total int) (string, error) {
	if total == 0 || kind == "cash" {
		return "cash", nil
	}
	if !s.partners {
		return "", ErrPaymentUnavailable
	}
	if payment.Linkable(kind) {
		ok, err := q.HasPaymentMethod(ctx, sqlc.HasPaymentMethodParams{UserID: buyer, Kind: sqlc.PaymentKind(kind)})
		if err != nil {
			return "", err
		}
		if !ok {
			return "", ErrPaymentNotLinked
		}
	}
	return kind, nil
}

// insertWithUniqueCode: kode SG-0000..9999 acak dari crypto/rand, unik per toko per tanggal WIB. Tabrakan memicu
// ulang (maks 5 kali); setiap percobaan di savepoint supaya transaksi tidak ikut batal.
func (s *Service) insertWithUniqueCode(ctx context.Context, r *Repository, p sqlc.InsertOrderParams) (uuid.UUID, string, error) {
	for i := 0; i < 5; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10000))
		if err != nil {
			return uuid.Nil, "", err
		}
		p.Code = fmt.Sprintf("SG-%04d", n.Int64())
		sp, err := r.tx.Begin(ctx)
		if err != nil {
			return uuid.Nil, "", err
		}
		id, err := r.q.WithTx(sp).InsertOrder(ctx, p)
		if err == nil {
			return id, p.Code, sp.Commit(ctx)
		}
		_ = sp.Rollback(ctx)
		if database.ConstraintName(err) != "orders_merchant_id_code_date_code_key" {
			return uuid.Nil, "", err
		}
	}
	return uuid.Nil, "", errNoCode
}

// firstMedia: foto cover listing disalin ke order_items saat checkout (kosong kalau listing tanpa foto).
func firstMedia(ctx context.Context, q *sqlc.Queries, listing uuid.UUID) (uuid.NullUUID, error) {
	id, err := q.GetMediaIDByListingCover(ctx, listing)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.NullUUID{}, nil
	}
	return uuid.NullUUID{UUID: id, Valid: err == nil}, err
}

// wrapUnknown meneruskan error yang dikenal client apa adanya dan membungkus sisanya dengan konteks.
func wrapUnknown(ctx string, err error) error {
	if err == nil {
		return nil
	}
	for _, known := range []error{ErrDeliveryUnavailable, ErrPaymentUnavailable, ErrPaymentNotLinked, ErrAddressNotFound, ErrMerchantUnavailable, ErrMerchantRequired, ErrOwnMerchant,
		ErrInvalidCart, ErrListingUnavailable, ErrOutOfStock, ErrVoucherNotClaimed, ErrVoucherUsed, ErrVoucherInvalid, ErrVoucherMinSpend,
		ErrPriceChanged, ErrNotFound, ErrInvalidState, ErrCancelWindowClosed, ErrWrongCode, ErrTooManyAttempts} {
		if errors.Is(err, known) {
			return err
		}
	}
	return fmt.Errorf("%s: %w", ctx, err)
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	u := t.Time.UTC()
	return &u
}
