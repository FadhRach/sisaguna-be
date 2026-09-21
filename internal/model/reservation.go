package model

import (
	"time"

	"github.com/google/uuid"
)

// ValidReservationStatuses — enum status reservasi.
var ValidReservationStatuses = []string{"pending", "confirmed", "completed", "no_show", "cancelled"}

// Reservation merepresentasikan tabel reservations di CLAUDE.md.
type Reservation struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ListingID       uuid.UUID `gorm:"column:listing_id;not null"                      json:"listing_id"`
	PenerimaID      uuid.UUID `gorm:"column:penerima_id;not null"                     json:"penerima_id"`
	PickupSlotStart time.Time `gorm:"column:pickup_slot_start;not null"               json:"pickup_slot_start"`
	PickupSlotEnd   time.Time `gorm:"column:pickup_slot_end;not null"                 json:"pickup_slot_end"`
	Status          string    `gorm:"not null;default:pending"                        json:"status"`
	CreatedAt       time.Time `gorm:"not null;default:now()"                          json:"created_at"`
}

func (Reservation) TableName() string {
	return "reservations"
}
