// Package model berisi struct Go yang merepresentasikan tabel di database
// (lihat "Skema database" di CLAUDE.md) — satu file per tabel.
package model

import (
	"time"

	"github.com/FadhRach/sisaguna-be/internal/model/types"
	"github.com/google/uuid"
)

// ValidRoles adalah daftar role yang diizinkan untuk User.Roles. Didefinisikan
// sebagai konstanta Go dulu, belum lewat spec OpenAPI (lihat Konvensi API di CLAUDE.md).
var ValidRoles = []string{
	"mitra_usaha",
	"rumah_tangga",
	"penerima",
	"peternak",
	"pengelola_kompos",
	"admin",
}

// User merepresentasikan tabel users di CLAUDE.md.
type User struct {
	ID           uuid.UUID         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name         string            `gorm:"not null"                                        json:"name"`
	Email        string            `gorm:"not null;unique"                                 json:"email"`
	Phone        string            `gorm:"not null"                                        json:"phone"`
	PasswordHash string            `gorm:"column:password_hash;not null"                   json:"-"`
	Roles        types.StringArray `gorm:"type:text[];not null"                            json:"roles"`
	BusinessName *string           `gorm:"column:business_name"                            json:"business_name,omitempty"`
	AvgRating    *types.Decimal    `gorm:"column:avg_rating;type:numeric(2,1)"             json:"avg_rating"`
	CreatedAt    time.Time         `gorm:"not null;default:now()"                          json:"created_at"`
}

func (User) TableName() string {
	return "users"
}

// PublicUser adalah profil publik user (dipakai GET /users/:id). Sengaja tidak
// menyertakan email/phone supaya tidak bocor ke pengguna lain.
type PublicUser struct {
	ID           uuid.UUID         `json:"id"`
	Name         string            `json:"name"`
	Roles        types.StringArray `json:"roles"`
	BusinessName *string           `json:"business_name,omitempty"`
	AvgRating    *types.Decimal    `json:"avg_rating"`
	CreatedAt    time.Time         `json:"created_at"`
}

func (u *User) ToPublicUser() *PublicUser {
	return &PublicUser{
		ID:           u.ID,
		Name:         u.Name,
		Roles:        u.Roles,
		BusinessName: u.BusinessName,
		AvgRating:    u.AvgRating,
		CreatedAt:    u.CreatedAt,
	}
}
