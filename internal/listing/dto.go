package listing

import (
	"time"

	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
	"github.com/FadhRach/sisaguna-be/internal/rules"
)

// CreateRequest untuk POST /listings. Aturan lintas-field (harga, checklist, batas aman) divalidasi di service.
type CreateRequest struct {
	Title         string      `json:"title" binding:"required,notblank,min=3,max=80"`
	Description   string      `json:"description" binding:"max=500"`
	Tier          string      `json:"tier" binding:"required,oneof=human animal_feed compost"`
	IsFree        bool        `json:"is_free"`
	Price         *int        `json:"price" binding:"omitempty,min=1,max=10000000"`
	PriceOriginal *int        `json:"price_original" binding:"omitempty,min=1,max=10000000"`
	Stock         int         `json:"stock" binding:"required,min=1,max=10000"`
	Halal         string      `json:"halal" binding:"omitempty,oneof=halal_certified non_halal unverified other"`
	Allergens     []string    `json:"allergens" binding:"omitempty,max=8,dive,oneof=seafood peanut tree_nut milk egg gluten soy sesame"`
	MadeAt        *time.Time  `json:"made_at"`
	Storage       string      `json:"storage" binding:"omitempty,oneof=room_temp shelf chilled frozen"`
	SafetyChecks  []string    `json:"safety_checks" binding:"omitempty,max=5,dive,oneof=untouched sensory_ok stored_right packed_clean hygiene"`
	PickupHours   int         `json:"pickup_hours" binding:"required,oneof=1 2 4 8"`
	PhotoMediaIDs []uuid.UUID `json:"photo_media_ids" binding:"required,min=1,max=5,unique"`
	VideoMediaID  *uuid.UUID  `json:"video_media_id"` // opsional: hasil upload video MP4 (maks 30 detik)
}

// UpdateRequest untuk PATCH /listings/:id. price_original 0 berarti hapus harga coret.
type UpdateRequest struct {
	Stock         *int    `json:"stock" binding:"omitempty,min=0,max=10000"`
	Price         *int    `json:"price" binding:"omitempty,min=1,max=10000000"`
	PriceOriginal *int    `json:"price_original" binding:"omitempty,min=0,max=10000000"`
	PickupHours   *int    `json:"pickup_hours" binding:"omitempty,oneof=1 2 4 8"`
	Status        *string `json:"status" binding:"omitempty,oneof=active paused"`
}

type ListQuery struct {
	httpx.PageQuery
	Tier  string   `form:"tier" binding:"omitempty,oneof=human animal_feed compost"`
	Lat   *float64 `form:"lat" binding:"omitempty,min=-90,max=90"`
	Lng   *float64 `form:"lng" binding:"omitempty,min=-180,max=180"`
	Sort  string   `form:"sort" binding:"omitempty,oneof=relevant recommended rating delivery_fee delivery_time nearest cheapest price_low biggest_discount discount ending_soon popular"`
	Q     string   `form:"q" binding:"max=60"`
	Free  bool     `form:"free"`
	MaxKm *float64 `form:"max_km" binding:"omitempty,gt=0,max=100"`
}

type HomeQuery struct {
	Tab   string   `form:"tab" binding:"omitempty,oneof=human farm"` // hanya menentukan other_tab_match_count; semua rail selalu dikirim
	Sort  string   `form:"sort" binding:"omitempty,oneof=relevant nearest cheapest biggest_discount ending_soon"`
	Lat   *float64 `form:"lat" binding:"omitempty,min=-90,max=90"`
	Lng   *float64 `form:"lng" binding:"omitempty,min=-180,max=180"`
	Q     string   `form:"q" binding:"max=60"`
	Free  bool     `form:"free"`
	MaxKm *float64 `form:"max_km" binding:"omitempty,gt=0,max=100"`
}

type PointQuery struct {
	Lat *float64 `form:"lat" binding:"omitempty,min=-90,max=90"`
	Lng *float64 `form:"lng" binding:"omitempty,min=-180,max=180"`
}

type MerchantRef struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	LocationLabel string   `json:"location_label"`
	RatingAvg     *float64 `json:"rating_avg"`
	RatingCount   int      `json:"rating_count"`
	IsVerified    bool     `json:"is_verified"`
}

type CardDTO struct {
	ID            string        `json:"id"`
	Title         string        `json:"title"`
	Tier          string        `json:"tier"`
	Unit          string        `json:"unit"`
	IsFree        bool          `json:"is_free"`
	Price         *int          `json:"price"`
	PriceOriginal *int          `json:"price_original"`
	DiscountPct   int           `json:"discount_pct"`
	Stock         int           `json:"stock"`
	Halal         string        `json:"halal"`
	Allergens     []string      `json:"allergens"`
	PickupStart   time.Time     `json:"pickup_start"`
	PickupEnd     time.Time     `json:"pickup_end"`
	DistanceKm    *float64      `json:"distance_km"`
	CoverURL      *string       `json:"cover_url"`
	ThumbURL      *string       `json:"thumb_url"`
	Merchant      MerchantRef   `json:"merchant"`
	SoldCount     int           `json:"sold_count"`
	Delivery      *CardDelivery `json:"delivery"` // GoSend/Standar; null = toko hanya melayani ambil sendiri

	distKm float64 // km mentah, -1 kalau tanpa lat/lng; dipakai pengurutan
	prep   int     // menit persiapan toko
}

// CardDelivery: ongkir dan perkiraan tiba untuk tampilan kartu kategori (rumus tiruan, lihat rules.QuoteDelivery).
type CardDelivery struct {
	Fee    int `json:"fee"`
	ETAMin int `json:"eta_min"`
	ETAMax int `json:"eta_max"`
}

type PhotoDTO struct {
	URL      string  `json:"url"`
	ThumbURL *string `json:"thumb_url"`
}

type MerchantDetail struct {
	MerchantRef
	About             string   `json:"about"`
	Address           string   `json:"address"`
	Lat               float64  `json:"lat"`
	Lng               float64  `json:"lng"`
	OpenHours         string   `json:"open_hours"`
	Halal             string   `json:"halal"`
	Allergens         []string `json:"allergens"`
	DeliveryAvailable bool     `json:"delivery_available"`
	PrepMinutes       int      `json:"prep_minutes"`
}

type DetailDTO struct {
	ID               string           `json:"id"`
	Title            string           `json:"title"`
	Description      string           `json:"description"`
	Tier             string           `json:"tier"`
	Unit             string           `json:"unit"`
	Status           string           `json:"status"` // active | sold_out | expired | paused
	IsFree           bool             `json:"is_free"`
	Price            *int             `json:"price"`
	PriceOriginal    *int             `json:"price_original"`
	Stock            int              `json:"stock"`
	SoldCount        int              `json:"sold_count"`
	Halal            string           `json:"halal"`
	Allergens        []string         `json:"allergens"`
	MadeAt           *time.Time       `json:"made_at"`
	Storage          string           `json:"storage"`
	SafetyChecks     []string         `json:"safety_checks"`
	PickupStart      time.Time        `json:"pickup_start"`
	PickupEnd        time.Time        `json:"pickup_end"`
	DistanceKm       *float64         `json:"distance_km"`
	Photos           []PhotoDTO       `json:"photos"`
	VideoURL         *string          `json:"video_url"`
	Safety           rules.Assessment `json:"safety"`
	Merchant         MerchantDetail   `json:"merchant"`
	MoreFromMerchant []CardDTO        `json:"more_from_merchant"`
}

// MineDTO adalah satu baris Katalog Saya (mitra).
type MineDTO struct {
	ID            string     `json:"id"`
	Title         string     `json:"title"`
	Tier          string     `json:"tier"`
	Unit          string     `json:"unit"`
	Status        string     `json:"status"` // active | sold_out | expired | paused
	IsFree        bool       `json:"is_free"`
	Price         *int       `json:"price"`
	PriceOriginal *int       `json:"price_original"`
	Stock         int        `json:"stock"`
	SoldCount     int        `json:"sold_count"`
	PickupStart   time.Time  `json:"pickup_start"`
	PickupEnd     time.Time  `json:"pickup_end"`
	SafeUntil     *time.Time `json:"safe_until"`
	CoverURL      *string    `json:"cover_url"`
	ThumbURL      *string    `json:"thumb_url"`
}

type HomeDTO struct {
	Nearby             []CardDTO       `json:"nearby"`
	Deals              []CardDTO       `json:"deals"`
	Popular            []CardDTO       `json:"popular"`
	AnimalFeed         []CardDTO       `json:"animal_feed"`
	Compost            []CardDTO       `json:"compost"`
	OfferMerchants     []OfferMerchant `json:"offer_merchants"`
	HumanCount         int             `json:"human_count"`
	FarmCount          int             `json:"farm_count"`
	OtherTabMatchCount int             `json:"other_tab_match_count"`
}

type OfferMerchant struct {
	MerchantRef
	TodaysOffer string   `json:"todays_offer"`
	DistanceKm  *float64 `json:"distance_km"`
}
