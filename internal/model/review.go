package model

import (
	"time"

	"github.com/google/uuid"
)

// ValidReviewDirections — enum arah review.
var ValidReviewDirections = []string{"mitra_ke_penerima", "penerima_ke_mitra"}

// Review merepresentasikan tabel reviews di CLAUDE.md.
type Review struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ReservationID uuid.UUID `gorm:"column:reservation_id;not null"                  json:"reservation_id"`
	ReviewerID    uuid.UUID `gorm:"column:reviewer_id;not null"                     json:"reviewer_id"`
	RevieweeID    uuid.UUID `gorm:"column:reviewee_id;not null"                     json:"reviewee_id"`
	Direction     string    `gorm:"not null"                                        json:"direction"`
	Rating        int16     `gorm:"not null"                                        json:"rating"`
	Comment       *string   `json:"comment,omitempty"`
	CreatedAt     time.Time `gorm:"not null;default:now()"                          json:"created_at"`
}

func (Review) TableName() string {
	return "reviews"
}
