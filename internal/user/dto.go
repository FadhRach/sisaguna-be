package user

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type RegisterRequest struct {
	Name        string `json:"name" binding:"required,notblank,max=80"`
	Email       string `json:"email" binding:"required,email,max=254"`
	Phone       string `json:"phone" binding:"required,numeric,excludesall=+-.,min=8,max=14"`
	Password    string `json:"password" binding:"required,min=8,max=72"`
	AccountType string `json:"account_type" binding:"required,oneof=buyer merchant"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,max=72"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type ChangePasswordRequest struct {
	Current string `json:"current" binding:"required,max=72"`
	New     string `json:"new" binding:"required,min=8,max=72,nefield=Current"`
}

// UpdateProfileRequest: field yang tidak dikirim tidak diubah.
// photo_media_id: dikirim null menghapus foto, dikirim UUID memasang foto.
type UpdateProfileRequest struct {
	Name         *string             `json:"name" binding:"omitempty,notblank,max=80"`
	Email        *string             `json:"email" binding:"omitempty,email,max=254"`
	Phone        *string             `json:"phone" binding:"omitempty,numeric,excludesall=+-.,min=8,max=14"`
	AvatarKey    *string             `json:"avatar_key" binding:"omitempty,min=1,max=30"`
	PhotoMediaID Nullable[uuid.UUID] `json:"photo_media_id"`
}

// Nullable membedakan "tidak dikirim" (Set=false), "null" (Set, !Valid), dan nilai (Set, Valid).
type Nullable[T any] struct {
	Value      T
	Set, Valid bool
}

func (n *Nullable[T]) UnmarshalJSON(b []byte) error {
	n.Set = true
	if string(b) == "null" {
		return nil
	}
	n.Valid = true
	return json.Unmarshal(b, &n.Value)
}

type ProfileDTO struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Email             string    `json:"email"`
	Phone             string    `json:"phone"`
	AvatarKey         *string   `json:"avatar_key"`
	PhotoURL          *string   `json:"photo_url"`
	LocationLabel     *string   `json:"location_label"`
	SelectedAddressID *string   `json:"selected_address_id"`
	IsAdmin           bool      `json:"is_admin"`
	CreatedAt         time.Time `json:"created_at"`
}

type SessionDTO struct {
	AccessToken  string     `json:"access_token"`
	RefreshToken string     `json:"refresh_token"`
	ExpiresAt    time.Time  `json:"expires_at"`
	User         ProfileDTO `json:"user"`
}

type MeDTO struct {
	ProfileDTO
	UnreadNotifications int64 `json:"unread_notifications"`
}

type ImpactDTO struct {
	CompletedOrders int64  `json:"completed_orders"`
	Portions        int64  `json:"portions"`
	FeedCompostKg   int64  `json:"feed_compost_kg"`
	SavedRupiah     int64  `json:"saved_rupiah"`
	CarbonKg        int64  `json:"carbon_kg"` // ESTIMASI 0,9 kg per porsi (rules.CarbonKg), sama dengan app Android
	Tier            string `json:"tier"`      // starter | rescuer | hero | legend
}
