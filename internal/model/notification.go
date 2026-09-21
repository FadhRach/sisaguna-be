package model

import (
	"time"

	"github.com/FadhRach/sisaguna-be/internal/model/types"
	"github.com/google/uuid"
)

// ValidNotificationTypes — enum tipe notifikasi.
var ValidNotificationTypes = []string{
	"listing_baru_dekat",
	"reminder_batas_ambil",
	"reservasi_dikonfirmasi",
	"review_baru",
}

// Notification merepresentasikan tabel notifications di CLAUDE.md.
type Notification struct {
	ID        uuid.UUID         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID    uuid.UUID         `gorm:"column:user_id;not null"                        json:"user_id"`
	Type      string            `gorm:"not null"                                       json:"type"`
	Payload   types.JSONPayload `gorm:"not null"                                       json:"payload"`
	ReadAt    *time.Time        `gorm:"column:read_at"                                 json:"read_at,omitempty"`
	CreatedAt time.Time         `gorm:"not null;default:now()"                         json:"created_at"`
}

func (Notification) TableName() string {
	return "notifications"
}
