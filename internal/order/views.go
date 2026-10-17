package order

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/payment"
	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
	"github.com/FadhRach/sisaguna-be/internal/rules"
)

type view struct {
	o            sqlc.Order
	merchantName string
	ownerID      uuid.UUID
	buyerName    string
	reviewed     bool
	complaint    string // status komplain, kosong kalau tidak ada
	location     string
	lat, lng     float64
}

// Get: pembeli atau pemilik toko order. Pihak lain dijawab "tidak ditemukan".
func (s *Service) Get(ctx context.Context, user, id uuid.UUID) (*OrderDTO, error) {
	r, err := s.repo.q.GetOrderView(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("order service get: %w", err)
	}
	v := view{o: r.Order, merchantName: r.MerchantName, ownerID: r.MerchantOwnerID, buyerName: r.BuyerName, reviewed: r.HasReview, complaint: r.ComplaintStatus,
		location: r.MerchantLocation, lat: r.MerchantLat, lng: r.MerchantLng}
	asBuyer := r.Order.BuyerID == user
	if !asBuyer && r.MerchantOwnerID != user {
		return nil, ErrNotFound
	}
	out, err := s.build(ctx, []view{v}, asBuyer)
	if err != nil {
		return nil, fmt.Errorf("order service get: %w", err)
	}
	return &out[0], nil
}

// ListBuyer: riwayat dan pesanan berjalan milik pembeli, cursor terbaru dulu.
func (s *Service) ListBuyer(ctx context.Context, buyer uuid.UUID, q BuyerListQuery, cur *httpx.Cursor) ([]OrderDTO, string, error) {
	statuses := []sqlc.OrderStatus{sqlc.OrderStatusPendingPayment, sqlc.OrderStatusReady}
	if q.Tab == "history" {
		switch q.Filter {
		case "completed":
			statuses = []sqlc.OrderStatus{sqlc.OrderStatusCompleted}
		case "cancelled":
			statuses = []sqlc.OrderStatus{sqlc.OrderStatusCancelled, sqlc.OrderStatusNoShow, sqlc.OrderStatusExpired}
		default:
			statuses = []sqlc.OrderStatus{sqlc.OrderStatusCompleted, sqlc.OrderStatusCancelled, sqlc.OrderStatusNoShow, sqlc.OrderStatusExpired}
		}
	}
	limit := limitOrDefault(q.Limit)
	p := sqlc.ListBuyerOrdersParams{BuyerID: buyer, Statuses: statuses, Lim: int32(limit + 1)}
	p.CurAt, p.CurID = cursorArgs(cur)
	rows, err := s.repo.q.ListBuyerOrders(ctx, p)
	if err != nil {
		return nil, "", fmt.Errorf("order service list buyer: %w", err)
	}
	views := make([]view, 0, len(rows))
	for _, r := range rows {
		views = append(views, view{o: r.Order, merchantName: r.MerchantName, ownerID: r.MerchantOwnerID, buyerName: r.BuyerName, reviewed: r.HasReview, complaint: r.ComplaintStatus,
			location: r.MerchantLocation, lat: r.MerchantLat, lng: r.MerchantLng})
	}
	return s.page(ctx, views, limit, true)
}

// ListMerchant: pesanan masuk toko. status=ready (menunggu) atau done (selesai/batal/tidak diambil).
func (s *Service) ListMerchant(ctx context.Context, owner uuid.UUID, q MerchantListQuery, cur *httpx.Cursor) ([]OrderDTO, string, error) {
	m, err := s.repo.q.GetMerchantByOwner(ctx, owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrMerchantRequired
	}
	if err != nil {
		return nil, "", fmt.Errorf("order service list merchant: %w", err)
	}
	statuses := []sqlc.OrderStatus{sqlc.OrderStatusReady}
	if q.Status == "done" {
		statuses = []sqlc.OrderStatus{sqlc.OrderStatusCompleted, sqlc.OrderStatusCancelled, sqlc.OrderStatusNoShow}
	}
	limit := limitOrDefault(q.Limit)
	p := sqlc.ListMerchantOrdersParams{MerchantID: m.ID, Statuses: statuses, Lim: int32(limit + 1)}
	p.CurAt, p.CurID = cursorArgs(cur)
	rows, err := s.repo.q.ListMerchantOrders(ctx, p)
	if err != nil {
		return nil, "", fmt.Errorf("order service list merchant: %w", err)
	}
	views := make([]view, 0, len(rows))
	for _, r := range rows {
		views = append(views, view{o: r.Order, merchantName: r.MerchantName, ownerID: r.MerchantOwnerID, buyerName: r.BuyerName, reviewed: r.HasReview, complaint: r.ComplaintStatus,
			location: r.MerchantLocation, lat: r.MerchantLat, lng: r.MerchantLng})
	}
	return s.page(ctx, views, limit, false)
}

func (s *Service) Dashboard(ctx context.Context, owner uuid.UUID) (*DashboardDTO, error) {
	m, err := s.repo.q.GetMerchantByOwner(ctx, owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrMerchantRequired
	}
	if err != nil {
		return nil, fmt.Errorf("order service dashboard: %w", err)
	}
	d, err := s.repo.q.MerchantDashboard(ctx, m.ID)
	if err != nil {
		return nil, fmt.Errorf("order service dashboard: %w", err)
	}
	active, err := s.repo.q.CountActiveListings(ctx, m.ID)
	if err != nil {
		return nil, fmt.Errorf("order service dashboard: %w", err)
	}
	return &DashboardDTO{SalesToday: d.SalesToday, CommissionToday: d.CommissionToday, NetToday: d.NetToday,
		CompletedToday: d.CompletedToday, PendingOrders: d.PendingOrders, ActiveListings: active}, nil
}

// page memotong kelebihan satu baris (penanda masih ada halaman berikutnya) dan membuat cursor.
func (s *Service) page(ctx context.Context, views []view, limit int, asBuyer bool) ([]OrderDTO, string, error) {
	next := ""
	if len(views) > limit {
		views = views[:limit]
		last := views[len(views)-1].o
		next = httpx.EncodeCursor(last.CreatedAt.Time, last.ID)
	}
	out, err := s.build(ctx, views, asBuyer)
	return out, next, err
}

// build mengambil item semua order dalam satu query lalu menyusun DTO sesuai sudut pandang (pembeli/mitra).
func (s *Service) build(ctx context.Context, views []view, asBuyer bool) ([]OrderDTO, error) {
	ids := make([]uuid.UUID, 0, len(views))
	for _, v := range views {
		ids = append(ids, v.o.ID)
	}
	items, err := s.repo.q.ListOrderItems(ctx, ids)
	if err != nil {
		return nil, err
	}
	byOrder := make(map[uuid.UUID][]ItemDTO, len(views))
	for _, it := range items {
		d := ItemDTO{ListingID: it.ListingID.String(), Title: it.Title, Tier: string(it.Tier), Unit: string(it.Unit), Quantity: int(it.Quantity),
			UnitPrice: int(it.UnitPrice), UnitOriginalPrice: int(it.UnitOriginalPrice)}
		if it.ImagePath.Valid {
			d.ImageURL = s.urls.PublicPtr(&it.ImagePath.String)
			d.ThumbURL = s.urls.PublicPtr(textPtr(it.ThumbPath))
		}
		byOrder[it.OrderID] = append(byOrder[it.OrderID], d)
	}

	ratings, complaints, err := s.extras(ctx, ids)
	if err != nil {
		return nil, err
	}

	now := s.now()
	out := make([]OrderDTO, 0, len(views))
	for _, v := range views {
		o := v.o
		loc, lat, lng := v.location, v.lat, v.lng
		d := OrderDTO{
			ID: o.ID.String(), Status: string(o.Status), Fulfillment: string(o.Fulfillment), PaymentKind: string(o.PaymentKind),
			PaymentStatus: string(o.PaymentStatus), Note: o.Note, Merchant: PartyDTO{ID: o.MerchantID.String(), Name: v.merchantName, LocationLabel: &loc, Lat: &lat, Lng: &lng},
			Items: byOrder[o.ID], Subtotal: int(o.Subtotal), OriginalTotal: int(o.OriginalTotal), VoucherCode: textPtr(o.VoucherCode),
			VoucherDiscount: int(o.VoucherDiscount), ServiceFee: int(o.ServiceFee), DeliveryFee: int(o.DeliveryFee), Total: int(o.Total),
			PickupBy: timePtr(o.PickupBy), PickupEnd: o.PickupEnd.Time.UTC(), CreatedAt: o.CreatedAt.Time.UTC(),
			CompletedAt: timePtr(o.CompletedAt), CancelledAt: timePtr(o.CancelledAt), CancelReason: textPtr(o.CancelReason),
			Reviewed: v.reviewed, Rating: ratings[o.ID], Complaint: complaints[o.ID],
			Payment: PaymentDTO{Kind: string(o.PaymentKind), Status: string(o.PaymentStatus), PaidAt: timePtr(o.PaidAt)},
		}
		if o.PaymentKind != sqlc.PaymentKindCash {
			prov := payment.Provider
			d.Payment.Provider = &prov
		}
		if v.complaint != "" {
			d.ComplaintStatus = &v.complaint
		}
		if o.Fulfillment == sqlc.FulfillmentDelivery && o.DeliveryCourier.Valid {
			c, sp := string(o.DeliveryCourier.Courier), string(o.DeliverySpeed.DeliverySpeed)
			d.Delivery = &OrderDelivery{Courier: c, CourierLabel: rules.CourierLabel(c), Speed: sp, SpeedLabel: rules.SpeedLabel(sp),
				Fee: int(o.DeliveryFee + o.DeliveryDiscount), Discount: int(o.DeliveryDiscount), Payable: int(o.DeliveryFee),
				ETAMin: int(o.DeliveryEtaMin.Int16), ETAMax: int(o.DeliveryEtaMax.Int16)}
			var snap AddressSnapshot
			if json.Unmarshal(o.DeliveryAddress, &snap) == nil {
				d.DeliveryAddress = &snap
			}
		}
		ready := o.Status == sqlc.OrderStatusReady
		d.CanCancel = ready && (!asBuyer || !o.PickupBy.Valid || now.Before(o.PickupBy.Time))
		if asBuyer && o.Status == sqlc.OrderStatusCompleted && o.CompletedAt.Valid {
			since := now.Sub(o.CompletedAt.Time)
			d.CanRate = !v.reviewed && since <= 7*24*time.Hour
			d.CanComplain = v.complaint == "" && since <= 24*time.Hour
		}
		if o.CancelledBy.Valid {
			by := string(o.CancelledBy.ChatSide)
			d.CancelledBy = &by
		}
		if d.Items == nil {
			d.Items = []ItemDTO{}
		}
		if asBuyer {
			d.Code = &o.Code
		} else {
			c, p := int(o.Commission), int(o.MerchantPayout)
			d.Commission, d.MerchantPayout, d.BuyerName = &c, &p, &v.buyerName
		}
		out = append(out, d)
	}
	return out, nil
}

func limitOrDefault(l int) int {
	if l == 0 {
		return 20
	}
	return l
}

func cursorArgs(cur *httpx.Cursor) (pgtype.Timestamptz, uuid.NullUUID) {
	if cur == nil {
		return pgtype.Timestamptz{}, uuid.NullUUID{}
	}
	return pgtype.Timestamptz{Time: cur.At, Valid: true}, uuid.NullUUID{UUID: cur.ID, Valid: true}
}

// extras mengambil ulasan dan komplain semua order sekaligus (tanpa query per order).
func (s *Service) extras(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]*RatingDTO, map[uuid.UUID]*ComplaintInfo, error) {
	rs, err := s.repo.q.ListOrderReviews(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	ratings := make(map[uuid.UUID]*RatingDTO, len(rs))
	for _, r := range rs {
		tags := r.Tags
		if tags == nil {
			tags = []string{}
		}
		ratings[r.OrderID] = &RatingDTO{Stars: int(r.Stars), Tags: tags, Comment: r.Comment, CreatedAt: r.CreatedAt.Time.UTC()}
	}
	cs, err := s.repo.q.ListOrderComplaints(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	cids := make([]uuid.UUID, 0, len(cs))
	for _, c := range cs {
		cids = append(cids, c.ID)
	}
	media, err := s.repo.q.ListComplaintMedia(ctx, cids)
	if err != nil {
		return nil, nil, err
	}
	photos := map[uuid.UUID][]string{}
	for _, m := range media {
		photos[m.ComplaintID] = append(photos[m.ComplaintID], s.urls.Private(m.MediaID.String()))
	}
	complaints := make(map[uuid.UUID]*ComplaintInfo, len(cs))
	for _, c := range cs {
		urls := photos[c.ID]
		if urls == nil {
			urls = []string{}
		}
		complaints[c.OrderID] = &ComplaintInfo{Reason: string(c.Reason), Detail: c.Detail, Status: string(c.Status), ResolutionNote: textPtr(c.ResolutionNote),
			PhotoURLs: urls, CreatedAt: c.CreatedAt.Time.UTC()}
	}
	return ratings, complaints, nil
}
