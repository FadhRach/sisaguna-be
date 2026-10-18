package order

import (
	"time"

	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/internal/rules"
)

type LineInput struct {
	ListingID uuid.UUID `json:"listing_id" binding:"required"`
	Quantity  int       `json:"quantity" binding:"required,min=1,max=1000"`
}

// QuoteRequest dipakai POST /checkout/quote dan menjadi dasar POST /orders.
type QuoteRequest struct {
	MerchantID  uuid.UUID   `json:"merchant_id" binding:"required"`
	Lines       []LineInput `json:"lines" binding:"required,min=1,max=20,dive"`
	VoucherCode string      `json:"voucher_code" binding:"max=20"`
	Fulfillment string      `json:"fulfillment" binding:"omitempty,oneof=pickup delivery"`
	// Pengiriman (kurir tiruan): kosong = gosend/standard. address_id kosong = alamat terpilih (PUT /me/selected-address).
	Courier   string     `json:"courier" binding:"omitempty,oneof=gosend grab shopee lalamove"`
	Speed     string     `json:"speed" binding:"omitempty,oneof=priority standard saver"`
	AddressID *uuid.UUID `json:"address_id"`
}

// PlaceRequest untuk POST /orders. expected_total (wajib) adalah total yang tampil di HP: hanya dibandingkan,
// kalau beda server menjawab price_changed dengan angka baru. Wajib supaya klien tidak bisa melewati pengaman harga.
type PlaceRequest struct {
	QuoteRequest
	PaymentKind   string `json:"payment_kind" binding:"required,oneof=cash qris gopay ovo dana"`
	Note          string `json:"note" binding:"max=200"`
	ExpectedTotal *int   `json:"expected_total" binding:"required,min=0"`
}

type CancelRequest struct {
	Reason string `json:"reason" binding:"max=200"`
}

type MerchantCancelRequest struct {
	Reason string `json:"reason" binding:"required,notblank,max=200"`
}

type ConfirmRequest struct {
	Code string `json:"code" binding:"required,max=10"`
}

type BuyerListQuery struct {
	Tab    string `form:"tab" binding:"omitempty,oneof=ongoing history"`
	Filter string `form:"filter" binding:"omitempty,oneof=all completed cancelled"`
	Limit  int    `form:"limit" binding:"omitempty,min=1,max=50"`
}

type MerchantListQuery struct {
	Status string `form:"status" binding:"omitempty,oneof=ready done"`
	Limit  int    `form:"limit" binding:"omitempty,min=1,max=50"`
}

type QuoteLine struct {
	ListingID         string `json:"listing_id"`
	Title             string `json:"title"`
	Quantity          int    `json:"quantity"`
	UnitPrice         int    `json:"unit_price"`
	UnitOriginalPrice int    `json:"unit_original_price"`
	LineTotal         int    `json:"line_total"`
}

type QuoteDTO struct {
	Lines           []QuoteLine    `json:"lines"`
	Subtotal        int            `json:"subtotal"`
	OriginalTotal   int            `json:"original_total"`
	VoucherCode     *string        `json:"voucher_code"`
	VoucherDiscount int            `json:"voucher_discount"`
	ServiceFee      int            `json:"service_fee"`
	DeliveryFee     int            `json:"delivery_fee"`
	Total           int            `json:"total"`
	Fulfillment     string         `json:"fulfillment"`
	PickupBy        *time.Time     `json:"pickup_by"`
	PickupEnd       time.Time      `json:"pickup_end"`
	PaymentKinds    []string       `json:"payment_kinds"`
	Delivery        *DeliveryOffer `json:"delivery"` // null kalau pengiriman dimatikan (PARTNERS_MODE=off)
}

// DeliveryOffer: penawaran kurir untuk checkout. available=false menyertakan reason:
// merchant_pickup_only | too_far | address_required.
type DeliveryOffer struct {
	Available  bool                  `json:"available"`
	Reason     *string               `json:"reason"`
	DistanceKm *float64              `json:"distance_km"`
	Address    *AddressSnapshot      `json:"address"`
	Speed      string                `json:"speed"`
	Options    []rules.DeliveryQuote `json:"options"`  // semua kurir untuk speed ini, termurah dulu
	Selected   *rules.DeliveryQuote  `json:"selected"` // hanya kalau fulfillment=delivery
}

type AddressSnapshot struct {
	ID          string  `json:"id"`
	Label       string  `json:"label"`
	FullAddress string  `json:"full_address"`
	Note        string  `json:"note"`
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
}

type ItemDTO struct {
	ListingID         string  `json:"listing_id"`
	Title             string  `json:"title"`
	Tier              string  `json:"tier"`
	Unit              string  `json:"unit"`
	Quantity          int     `json:"quantity"`
	UnitPrice         int     `json:"unit_price"`
	UnitOriginalPrice int     `json:"unit_original_price"`
	ImageURL          *string `json:"image_url"`
	ThumbURL          *string `json:"thumb_url"`
}

type PartyDTO struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	LocationLabel *string  `json:"location_label,omitempty"`
	Lat           *float64 `json:"lat,omitempty"`
	Lng           *float64 `json:"lng,omitempty"`
}

// OrderDelivery: fee = tarif kurir, discount = potongan, payable = yang dibayar pembeli (sudah termasuk di total).
type OrderDelivery struct {
	Courier      string `json:"courier"`
	CourierLabel string `json:"courier_label"`
	Speed        string `json:"speed"`
	SpeedLabel   string `json:"speed_label"`
	Fee          int    `json:"fee"`
	Discount     int    `json:"discount"`
	Payable      int    `json:"payable"`
	ETAMin       int    `json:"eta_min"`
	ETAMax       int    `json:"eta_max"`
}

type PaymentDTO struct {
	Kind     string     `json:"kind"`
	Status   string     `json:"status"`
	Provider *string    `json:"provider"` // "dummy" untuk non-tunai selama gateway belum terhubung
	PaidAt   *time.Time `json:"paid_at"`
}

type RatingDTO struct {
	Stars     int       `json:"stars"`
	Tags      []string  `json:"tags"`
	Comment   string    `json:"comment"`
	CreatedAt time.Time `json:"created_at"`
}

type ComplaintInfo struct {
	Reason         string    `json:"reason"`
	Detail         string    `json:"detail"`
	Status         string    `json:"status"`
	ResolutionNote *string   `json:"resolution_note"`
	PhotoURLs      []string  `json:"photo_urls"`
	CreatedAt      time.Time `json:"created_at"`
}

// OrderDTO dipakai pembeli dan mitra. Kode pickup hanya terlihat oleh pembeli (mitra harus mengetiknya
// dari pembeli); komisi dan payout hanya terlihat oleh mitra; nama pembeli hanya terlihat oleh mitra.
type OrderDTO struct {
	ID              string           `json:"id"`
	Code            *string          `json:"code,omitempty"`
	Status          string           `json:"status"`
	Fulfillment     string           `json:"fulfillment"`
	PaymentKind     string           `json:"payment_kind"`
	PaymentStatus   string           `json:"payment_status"`
	Note            string           `json:"note"`
	Merchant        PartyDTO         `json:"merchant"`
	BuyerName       *string          `json:"buyer_name,omitempty"`
	Items           []ItemDTO        `json:"items"`
	Subtotal        int              `json:"subtotal"`
	OriginalTotal   int              `json:"original_total"`
	VoucherCode     *string          `json:"voucher_code"`
	VoucherDiscount int              `json:"voucher_discount"`
	ServiceFee      int              `json:"service_fee"`
	DeliveryFee     int              `json:"delivery_fee"`
	Total           int              `json:"total"`
	Commission      *int             `json:"commission,omitempty"`
	MerchantPayout  *int             `json:"merchant_payout,omitempty"`
	PickupBy        *time.Time       `json:"pickup_by"`
	PickupEnd       time.Time        `json:"pickup_end"`
	CreatedAt       time.Time        `json:"created_at"`
	CompletedAt     *time.Time       `json:"completed_at"`
	CancelledAt     *time.Time       `json:"cancelled_at"`
	CancelledBy     *string          `json:"cancelled_by"`
	CancelReason    *string          `json:"cancel_reason"`
	Reviewed        bool             `json:"reviewed"`
	ComplaintStatus *string          `json:"complaint_status"`
	Rating          *RatingDTO       `json:"rating"`
	Complaint       *ComplaintInfo   `json:"complaint"`
	Delivery        *OrderDelivery   `json:"delivery"`
	DeliveryAddress *AddressSnapshot `json:"delivery_address"`
	Payment         PaymentDTO       `json:"payment"`
	CanCancel       bool             `json:"can_cancel"`
	CanRate         bool             `json:"can_rate"`
	CanComplain     bool             `json:"can_complain"`
}

type DashboardDTO struct {
	SalesToday      int64 `json:"sales_today"`
	CommissionToday int64 `json:"commission_today"`
	NetToday        int64 `json:"net_today"`
	CompletedToday  int64 `json:"completed_today"`
	PendingOrders   int64 `json:"pending_orders"`
	ActiveListings  int64 `json:"active_listings"`
}
