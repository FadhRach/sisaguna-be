package model

import (
	"time"

	"github.com/FadhRach/sisaguna-be/internal/model/types"
	"github.com/google/uuid"
)

// ValidListingTiers — enum tier listing, tetap konstanta Go sampai spec OpenAPI ada.
var ValidListingTiers = []string{"konsumsi_manusia", "pakan_ternak", "kompos"}

// ValidListingStatuses — enum status listing.
var ValidListingStatuses = []string{"available", "reserved", "completed", "expired", "cancelled"}

// Listing merepresentasikan tabel listings di CLAUDE.md.
type Listing struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	MitraID        uuid.UUID      `gorm:"column:mitra_id;not null"                        json:"mitra_id"`
	Title          string         `gorm:"not null"                                        json:"title"`
	Description    *string        `json:"description,omitempty"`
	Tier           string         `gorm:"not null"                                        json:"tier"`
	Quantity       types.Decimal  `gorm:"type:numeric;not null"                           json:"quantity"`
	Unit           string         `gorm:"not null"                                        json:"unit"`
	Price          types.Decimal  `gorm:"type:numeric;not null;default:0"                 json:"price"`
	PhotoURL       *string        `gorm:"column:photo_url"                                json:"photo_url,omitempty"`
	PickupLocation types.GeoPoint `gorm:"column:pickup_location;type:geography(Point,4326);not null" json:"pickup_location"`
	PickupAddress  string         `gorm:"column:pickup_address;not null"                  json:"pickup_address"`
	ReadyAt        time.Time      `gorm:"column:ready_at;not null"                        json:"ready_at"`
	PickupDeadline time.Time      `gorm:"column:pickup_deadline;not null"                 json:"pickup_deadline"`
	Status         string         `gorm:"not null;default:available"                      json:"status"`
	CreatedAt      time.Time      `gorm:"not null;default:now()"                          json:"created_at"`
}

func (Listing) TableName() string {
	return "listings"
}
