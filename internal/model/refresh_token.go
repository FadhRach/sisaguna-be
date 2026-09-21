package model

import (
	"time"

	"github.com/google/uuid"
)

// RefreshToken merepresentasikan satu sesi login (satu refresh token aktif).
// TokenHash menyimpan SHA-256 dari token asli, jangan pernah simpan token mentah.
type RefreshToken struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID    uuid.UUID  `gorm:"column:user_id;not null"                        json:"user_id"`
	TokenHash string     `gorm:"column:token_hash;not null;unique"              json:"-"`
	ExpiresAt time.Time  `gorm:"column:expires_at;not null"                     json:"expires_at"`
	RevokedAt *time.Time `gorm:"column:revoked_at"                              json:"revoked_at,omitempty"`
	CreatedAt time.Time  `gorm:"not null;default:now()"                         json:"created_at"`
}

func (RefreshToken) TableName() string {
	return "refresh_tokens"
}

// IsUsable false kalau token sudah kedaluwarsa atau sudah direvoke (logout).
func (t *RefreshToken) IsUsable() bool {
	return t.RevokedAt == nil && time.Now().Before(t.ExpiresAt)
}
