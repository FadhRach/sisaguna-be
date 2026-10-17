package saved

import "time"

type ListQuery struct {
	Lat *float64 `form:"lat" binding:"omitempty,min=-90,max=90"`
	Lng *float64 `form:"lng" binding:"omitempty,min=-180,max=180"`
}

type MerchantCardDTO struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	LocationLabel  string     `json:"location_label"`
	TodaysOffer    *string    `json:"todays_offer"`
	RatingAvg      *float64   `json:"rating_avg"`
	RatingCount    int        `json:"rating_count"`
	IsVerified     bool       `json:"is_verified"`
	DistanceKm     *float64   `json:"distance_km"`
	SavedAt        time.Time  `json:"saved_at"`
	BannerURL      *string    `json:"banner_url"`
	AvailableCount int        `json:"available_count"` // listing yang tampil sekarang
	NextPickupEnd  *time.Time `json:"next_pickup_end"` // batas pickup paling awal di antaranya
}
