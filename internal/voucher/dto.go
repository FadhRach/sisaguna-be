package voucher

import (
	"time"

	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
)

type VoucherDTO struct {
	Code          string    `json:"code"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	PercentOff    *int      `json:"percent_off"`
	FlatOff       *int      `json:"flat_off"`
	MaxDiscount   *int      `json:"max_discount"`
	MinSpend      int       `json:"min_spend"`
	MerchantID    *string   `json:"merchant_id"` // null = berlaku di semua toko
	MerchantName  *string   `json:"merchant_name"`
	AppliesToTier *string   `json:"applies_to_tier"`
	AppliesTo     string    `json:"applies_to"` // teks tampilan, mis. "Roti, kue & dessert"
	ImageURL      *string   `json:"image_url"`
	ExpiresAt     time.Time `json:"expires_at"`
	Terms         []string  `json:"terms"`
	AccentHex     string    `json:"accent_hex"`
	Status        string    `json:"status"` // available | claimed | used
}

// AdminCreateRequest: tepat satu dari percent_off / flat_off (aturan lain dicek di service).
type AdminCreateRequest struct {
	Code          string     `json:"code" binding:"required,alphanum,uppercase,min=4,max=20"`
	Title         string     `json:"title" binding:"required,notblank,max=80"`
	Description   string     `json:"description" binding:"max=300"`
	PercentOff    *int       `json:"percent_off"`
	FlatOff       *int       `json:"flat_off"`
	MaxDiscount   *int       `json:"max_discount"`
	MinSpend      int        `json:"min_spend" binding:"min=0,max=10000000"`
	MerchantID    *uuid.UUID `json:"merchant_id"`
	AppliesToTier string     `json:"applies_to_tier" binding:"omitempty,oneof=human animal_feed compost"`
	AppliesTo     string     `json:"applies_to" binding:"max=80"`
	ImageMediaID  *uuid.UUID `json:"image_media_id"`
	StartsAt      *time.Time `json:"starts_at"`
	ExpiresAt     time.Time  `json:"expires_at" binding:"required"`
	Quota         *int       `json:"quota"`
	Terms         []string   `json:"terms" binding:"omitempty,max=10,dive,notblank,max=120"`
	AccentHex     string     `json:"accent_hex" binding:"omitempty,hexcolor"`
}

type AdminUpdateRequest struct {
	Title        *string    `json:"title" binding:"omitempty,notblank,max=80"`
	Description  *string    `json:"description" binding:"omitempty,max=300"`
	MinSpend     *int       `json:"min_spend" binding:"omitempty,min=0,max=10000000"`
	ExpiresAt    *time.Time `json:"expires_at"`
	Quota        *int       `json:"quota"`
	IsActive     *bool      `json:"is_active"`
	Terms        []string   `json:"terms" binding:"omitempty,max=10,dive,notblank,max=120"`
	AccentHex    *string    `json:"accent_hex" binding:"omitempty,hexcolor"`
	AppliesTo    *string    `json:"applies_to" binding:"omitempty,max=80"`
	ImageMediaID *uuid.UUID `json:"image_media_id"`
}

type AdminListQuery struct{ httpx.PageQuery }

type AdminVoucherDTO struct {
	Code          string    `json:"code"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	PercentOff    *int      `json:"percent_off"`
	FlatOff       *int      `json:"flat_off"`
	MaxDiscount   *int      `json:"max_discount"`
	MinSpend      int       `json:"min_spend"`
	MerchantID    *string   `json:"merchant_id"`
	AppliesToTier *string   `json:"applies_to_tier"`
	AppliesTo     string    `json:"applies_to"`
	ImageMediaID  *string   `json:"image_media_id"`
	StartsAt      time.Time `json:"starts_at"`
	ExpiresAt     time.Time `json:"expires_at"`
	Quota         *int      `json:"quota"`
	UsedCount     int       `json:"used_count"`
	Terms         []string  `json:"terms"`
	AccentHex     string    `json:"accent_hex"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
}
