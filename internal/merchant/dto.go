package merchant

import (
	"time"

	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/internal/listing"
)

type CreateRequest struct {
	Name          string   `json:"name" binding:"required,notblank,min=2,max=60"`
	About         string   `json:"about" binding:"max=500"`
	Address       string   `json:"address" binding:"required,notblank,max=300"`
	Lat           *float64 `json:"lat" binding:"required,min=-90,max=90"`
	Lng           *float64 `json:"lng" binding:"required,min=-180,max=180"`
	LocationLabel string   `json:"location_label" binding:"max=60"`
	OpenHours     string   `json:"open_hours" binding:"max=60"`
	Halal         string   `json:"halal" binding:"omitempty,oneof=halal_certified non_halal unverified other"`
	Allergens     []string `json:"allergens" binding:"omitempty,max=8,dive,oneof=seafood peanut tree_nut milk egg gluten soy sesame"`
	PrepMinutes   *int     `json:"prep_minutes" binding:"omitempty,min=0,max=240"`
}

// UpdateRequest: field yang tidak dikirim tidak diubah. lat dan lng harus berpasangan.
// todays_offer berisi string kosong untuk menghapus penawaran.
type UpdateRequest struct {
	Name          *string  `json:"name" binding:"omitempty,notblank,min=2,max=60"`
	About         *string  `json:"about" binding:"omitempty,max=500"`
	Address       *string  `json:"address" binding:"omitempty,notblank,max=300"`
	Lat           *float64 `json:"lat" binding:"omitempty,min=-90,max=90"`
	Lng           *float64 `json:"lng" binding:"omitempty,min=-180,max=180"`
	LocationLabel *string  `json:"location_label" binding:"omitempty,max=60"`
	OpenHours     *string  `json:"open_hours" binding:"omitempty,max=60"`
	Halal         *string  `json:"halal" binding:"omitempty,oneof=halal_certified non_halal unverified other"`
	Allergens     []string `json:"allergens" binding:"omitempty,max=8,dive,oneof=seafood peanut tree_nut milk egg gluten soy sesame"`
	TodaysOffer   *string  `json:"todays_offer" binding:"omitempty,max=120"`
	PrepMinutes   *int     `json:"prep_minutes" binding:"omitempty,min=0,max=240"`
	// Banner dan galeri: id hasil upload purpose=merchant. photo_media_ids menggantikan seluruh galeri ([] = kosongkan).
	BannerMediaID *uuid.UUID  `json:"banner_media_id"`
	PhotoMediaIDs []uuid.UUID `json:"photo_media_ids" binding:"omitempty,max=10,unique"`
}

type PublicQuery struct {
	Lat *float64 `form:"lat" binding:"omitempty,min=-90,max=90"`
	Lng *float64 `form:"lng" binding:"omitempty,min=-180,max=180"`
}

type MerchantDTO struct {
	ID                string             `json:"id"`
	Name              string             `json:"name"`
	Status            string             `json:"status"`
	IsVerified        bool               `json:"is_verified"`
	About             string             `json:"about"`
	Address           string             `json:"address"`
	Lat               float64            `json:"lat"`
	Lng               float64            `json:"lng"`
	LocationLabel     string             `json:"location_label"`
	OpenHours         string             `json:"open_hours"`
	Halal             string             `json:"halal"`
	Allergens         []string           `json:"allergens"`
	TodaysOffer       *string            `json:"todays_offer"`
	DeliveryAvailable bool               `json:"delivery_available"`
	PrepMinutes       int                `json:"prep_minutes"`
	RatingAvg         *float64           `json:"rating_avg"`
	RatingCount       int                `json:"rating_count"`
	TotalRescued      int                `json:"total_rescued"`
	DistanceKm        *float64           `json:"distance_km,omitempty"`
	BannerURL         *string            `json:"banner_url"`
	Photos            []listing.PhotoDTO `json:"photos"`
}

type PublicDTO struct {
	MerchantDTO
	RatingBreakdown map[string]int    `json:"rating_breakdown"` // jumlah ulasan per bintang ("1".."5")
	Listings        []listing.CardDTO `json:"listings"`
	IsSaved         *bool             `json:"is_saved,omitempty"` // hanya untuk pengguna login
}

type ListQuery struct {
	Lat       *float64 `form:"lat" binding:"omitempty,min=-90,max=90"`
	Lng       *float64 `form:"lng" binding:"omitempty,min=-180,max=180"`
	Available bool     `form:"available"` // hanya toko yang punya listing tampil sekarang
	Limit     int      `form:"limit" binding:"omitempty,min=1,max=100"`
}

// CardDTO: kartu toko ringkas (rekomendasi penyedia, toko favorit).
type CardDTO struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	LocationLabel  string     `json:"location_label"`
	TodaysOffer    *string    `json:"todays_offer"`
	RatingAvg      *float64   `json:"rating_avg"`
	RatingCount    int        `json:"rating_count"`
	IsVerified     bool       `json:"is_verified"`
	BannerURL      *string    `json:"banner_url"`
	AvailableCount int        `json:"available_count"`
	NextPickupEnd  *time.Time `json:"next_pickup_end"`
	DistanceKm     *float64   `json:"distance_km"`
	IsSaved        bool       `json:"is_saved"`
}
