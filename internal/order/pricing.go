package order

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/platform/apperr"
	"github.com/FadhRach/sisaguna-be/internal/rules"
)

const (
	pickupWindow = 45 * time.Minute // pickup_by = least(now + 45 menit, pickup_end paling awal)
	noShowGrace  = 15 * time.Minute
)

type pricedLine struct {
	listing sqlc.Listing
	qty     int
}

// priced adalah hasil perhitungan server untuk satu keranjang (dipakai quote dan checkout).
type priced struct {
	merchant  sqlc.GetMerchantBriefRow
	lines     []pricedLine
	subtotal  int
	original  int
	discount  int
	amounts   rules.Amounts
	voucher   string    // kode voucher yang dipakai, "" kalau tidak ada
	fundedBy  string    // merchant | platform, "" kalau tanpa voucher
	pickupBy  time.Time // nol untuk pengiriman (tidak ada batas datang)
	pickupEnd time.Time
	total     int // yang dibayar pembeli: items + biaya layanan + ongkir
	offer     *DeliveryOffer
	delivery  *deliveryChoice // terisi hanya kalau fulfillment=delivery
}

type deliveryChoice struct {
	quote  rules.DeliveryQuote
	target sqlc.GetDeliveryTargetRow
}

// price menghitung ulang semuanya dari data server (CLAUDE.md aturan 1). lock=true (checkout) mengunci baris
// listing dengan FOR UPDATE; lock=false (quote) hanya membaca.
func (s *Service) price(ctx context.Context, q *sqlc.Queries, buyer uuid.UUID, req QuoteRequest, lock bool) (*priced, error) {
	if req.Fulfillment == "delivery" && !s.partners {
		return nil, apperr.WithFields(ErrDeliveryUnavailable, map[string]any{"reason": "disabled"})
	}
	m, err := q.GetMerchantBrief(ctx, req.MerchantID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && m.Status != sqlc.MerchantStatusApproved) {
		return nil, ErrMerchantUnavailable
	}
	if err != nil {
		return nil, err
	}
	if m.OwnerID == buyer {
		return nil, ErrOwnMerchant
	}

	ids := make([]uuid.UUID, 0, len(req.Lines))
	qty := make(map[uuid.UUID]int, len(req.Lines))
	for _, l := range req.Lines {
		if _, dup := qty[l.ListingID]; dup {
			return nil, ErrInvalidCart
		}
		qty[l.ListingID] = l.Quantity
		ids = append(ids, l.ListingID)
	}
	var listings []sqlc.Listing
	if lock {
		listings, err = q.LockListingsForCheckout(ctx, ids)
	} else {
		listings, err = q.GetListingsByIDs(ctx, ids)
	}
	if err != nil {
		return nil, err
	}
	if len(listings) != len(ids) {
		return nil, ErrInvalidCart
	}

	now := s.now()
	p := &priced{merchant: m, pickupBy: now.Add(pickupWindow)}
	for i, l := range listings {
		n := qty[l.ID]
		switch {
		case l.MerchantID != m.ID:
			return nil, ErrInvalidCart
		case l.Status != sqlc.ListingStatusActive || now.Before(l.PickupStart.Time) || !now.Before(l.PickupEnd.Time) ||
			(l.Tier == sqlc.ListingTierHuman && l.SafeUntil.Valid && !now.Before(l.SafeUntil.Time)):
			return nil, apperr.WithFields(ErrListingUnavailable, map[string]any{"listing_id": l.ID})
		case int(l.Stock) < n:
			return nil, apperr.WithFields(ErrOutOfStock, map[string]any{"listing_id": l.ID, "available": l.Stock})
		}
		unit := int(l.Price.Int32) // 0 kalau gratis
		orig := unit
		if l.PriceOriginal.Valid {
			orig = int(l.PriceOriginal.Int32)
		}
		p.subtotal += unit * n
		p.original += orig * n
		p.lines = append(p.lines, pricedLine{listing: l, qty: n})
		if end := l.PickupEnd.Time; i == 0 || end.Before(p.pickupEnd) {
			p.pickupEnd = end
		}
	}
	if p.pickupEnd.Before(p.pickupBy) {
		p.pickupBy = p.pickupEnd
	}

	merchantFunded := false
	if code := strings.ToUpper(strings.TrimSpace(req.VoucherCode)); code != "" {
		d, funded, err := s.applyVoucher(ctx, q, buyer, code, m.ID, p)
		if err != nil {
			return nil, err
		}
		p.discount, p.voucher, merchantFunded = d, code, funded
		p.fundedBy = "platform"
		if funded {
			p.fundedBy = "merchant"
		}
	}
	p.amounts = rules.Settle(p.subtotal, p.discount, merchantFunded)
	p.total = p.amounts.Total
	if s.partners && (!lock || req.Fulfillment == "delivery") { // checkout pickup tidak butuh penawaran kurir
		if err := s.priceDelivery(ctx, q, buyer, req, p); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// priceDelivery menyusun penawaran kurir (tiruan, rumus app Android) dan, kalau pembeli memilih pengiriman,
// menambahkan ongkir ke total. Jarak = garis lurus alamat pembeli ke toko (PostGIS).
func (s *Service) priceDelivery(ctx context.Context, q *sqlc.Queries, buyer uuid.UUID, req QuoteRequest, p *priced) error {
	want := req.Fulfillment == "delivery"
	speed, courier := req.Speed, req.Courier
	if speed == "" {
		speed = "standard"
	}
	if courier == "" {
		courier = "gosend"
	}
	offer := &DeliveryOffer{Speed: speed, Options: []rules.DeliveryQuote{}}
	p.offer = offer
	unavailable := func(reason string) error {
		offer.Reason = &reason
		if want {
			return apperr.WithFields(ErrDeliveryUnavailable, map[string]any{"reason": reason})
		}
		return nil
	}

	addrID := uuid.NullUUID{}
	if req.AddressID != nil {
		addrID = uuid.NullUUID{UUID: *req.AddressID, Valid: true}
	} else if sel, err := q.GetSelectedAddressID(ctx, buyer); err != nil {
		return err
	} else {
		addrID = sel
	}
	if !addrID.Valid {
		return unavailable("address_required")
	}
	t, err := q.GetDeliveryTarget(ctx, sqlc.GetDeliveryTargetParams{AddressID: addrID.UUID, UserID: buyer, MerchantID: p.merchant.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAddressNotFound
	}
	if err != nil {
		return err
	}
	km := math.Round(t.DistanceKm*10) / 10
	offer.DistanceKm = &km
	offer.Address = &AddressSnapshot{ID: t.ID.String(), Label: t.Label, FullAddress: t.FullAddress, Note: t.Note, Lat: t.Lat, Lng: t.Lng}
	switch {
	case !t.DeliveryAvailable:
		return unavailable("merchant_pickup_only")
	case t.DistanceKm > rules.MaxDeliveryKm:
		return unavailable("too_far")
	}
	offer.Available = true
	offer.Options = rules.AllDeliveryQuotes(speed, t.DistanceKm, p.subtotal, int(t.PrepMinutes))
	if want {
		sel := rules.QuoteDelivery(courier, speed, t.DistanceKm, p.subtotal, int(t.PrepMinutes))
		offer.Selected = &sel
		p.delivery = &deliveryChoice{quote: sel, target: t}
		p.total += sel.Payable
		p.pickupBy = time.Time{}
	}
	return nil
}

// applyVoucher memeriksa voucher milik pembeli dan mengembalikan diskon serta apakah ditanggung mitra.
func (s *Service) applyVoucher(ctx context.Context, q *sqlc.Queries, buyer uuid.UUID, code string, merchantID uuid.UUID, p *priced) (discount int, merchantFunded bool, err error) {
	uv, err := q.GetUserVoucherForUpdate(ctx, sqlc.GetUserVoucherForUpdateParams{UserID: buyer, VoucherCode: code})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, ErrVoucherNotClaimed
	}
	if err != nil {
		return 0, false, err
	}
	if uv.UsedAt.Valid {
		return 0, false, ErrVoucherUsed
	}
	state, err := q.GetVoucherState(ctx, code)
	if err != nil {
		return 0, false, err
	}
	v := state.Voucher
	if !state.Valid {
		return 0, false, ErrVoucherInvalid
	}
	if v.MerchantID.Valid && v.MerchantID.UUID != merchantID {
		return 0, false, ErrVoucherInvalid
	}
	if v.AppliesToTier.Valid {
		for _, l := range p.lines {
			if l.listing.Tier != v.AppliesToTier.ListingTier {
				return 0, false, ErrVoucherInvalid
			}
		}
	}
	if p.subtotal < int(v.MinSpend) {
		return 0, false, apperr.WithFields(ErrVoucherMinSpend, map[string]any{"min_spend": v.MinSpend})
	}
	rv := rules.Voucher{}
	if v.PercentOff.Valid {
		pct := int(v.PercentOff.Int16)
		rv.PercentOff = &pct
	}
	if v.FlatOff.Valid {
		f := int(v.FlatOff.Int32)
		rv.FlatOff = &f
	}
	if v.MaxDiscount.Valid {
		m := int(v.MaxDiscount.Int32)
		rv.MaxDiscount = &m
	}
	return rv.DiscountFor(p.subtotal), v.MerchantID.Valid, nil
}

func (s *Service) quoteDTO(p *priced) *QuoteDTO {
	d := &QuoteDTO{
		Lines: make([]QuoteLine, 0, len(p.lines)), Subtotal: p.subtotal, OriginalTotal: p.original, VoucherDiscount: p.discount,
		ServiceFee: p.amounts.ServiceFee, Total: p.total, Fulfillment: "pickup",
		PickupEnd: p.pickupEnd.UTC(), PaymentKinds: s.paymentKinds(), Delivery: p.offer,
	}
	if p.delivery != nil {
		d.Fulfillment, d.DeliveryFee = "delivery", p.delivery.quote.Payable
	} else {
		pb := p.pickupBy.UTC()
		d.PickupBy = &pb
	}
	if p.voucher != "" {
		d.VoucherCode = &p.voucher
	}
	for _, l := range p.lines {
		unit := int(l.listing.Price.Int32)
		orig := unit
		if l.listing.PriceOriginal.Valid {
			orig = int(l.listing.PriceOriginal.Int32)
		}
		d.Lines = append(d.Lines, QuoteLine{ListingID: l.listing.ID.String(), Title: l.listing.Title, Quantity: l.qty,
			UnitPrice: unit, UnitOriginalPrice: orig, LineTotal: unit * l.qty})
	}
	return d
}

var errNoCode = fmt.Errorf("order: gagal membuat kode pickup unik")

// paymentKinds: tunai selalu ada; sisanya hanya saat mitra tiruan aktif.
func (s *Service) paymentKinds() []string {
	if s.partners {
		return []string{"qris", "gopay", "ovo", "dana", "cash"}
	}
	return []string{"cash"}
}
