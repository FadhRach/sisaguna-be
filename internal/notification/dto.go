package notification

import (
	"encoding/json"
	"time"
)

type ListQuery struct {
	Type  string `form:"type" binding:"omitempty,oneof=pickup order merchant promo impact"`
	Limit int    `form:"limit" binding:"omitempty,min=1,max=50"`
}

type MarkReadRequest struct {
	Read *bool `json:"read" binding:"required,eq=true"`
}

type NotificationDTO struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Title     string          `json:"title"`
	Body      string          `json:"body"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
	ReadAt    *time.Time      `json:"read_at"`
}

// PrefsRequest menggantikan seluruh preferensi (PUT), jadi semua saklar wajib dikirim.
type PrefsRequest struct {
	Enabled  *bool `json:"enabled" binding:"required"`
	Pickup   *bool `json:"pickup" binding:"required"`
	Order    *bool `json:"order" binding:"required"`
	Merchant *bool `json:"merchant" binding:"required"`
	Promo    *bool `json:"promo" binding:"required"`
	Impact   *bool `json:"impact" binding:"required"`
}

type PrefsDTO struct {
	Enabled  bool `json:"enabled"`
	Pickup   bool `json:"pickup"`
	Order    bool `json:"order"`
	Merchant bool `json:"merchant"`
	Promo    bool `json:"promo"`
	Impact   bool `json:"impact"`
}

// DeviceRequest mendaftarkan token push (FCM) perangkat. Pengiriman push sungguhan menunggu Fase 2;
// token disimpan sekarang supaya aplikasi mobile bisa langsung memanggilnya.
type DeviceRequest struct {
	Token    string `json:"token" binding:"required,min=20,max=4096"`
	Platform string `json:"platform" binding:"required,oneof=android ios"`
}
