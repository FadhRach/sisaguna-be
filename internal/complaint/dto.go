package complaint

import (
	"time"

	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
)

type CreateRequest struct {
	Reason   string      `json:"reason" binding:"required,oneof=not_fresh wrong_item missing packaging hygiene merchant other"`
	Detail   string      `json:"detail" binding:"required,notblank,min=10,max=1000"`
	MediaIDs []uuid.UUID `json:"media_ids" binding:"omitempty,max=3"`
}

type AdminListQuery struct {
	httpx.PageQuery
	Status string `form:"status" binding:"omitempty,oneof=submitted reviewing resolved rejected"`
}

// UpdateRequest dipakai admin. Status akhir (resolved/rejected) wajib disertai catatan.
type UpdateRequest struct {
	Status         string `json:"status" binding:"required,oneof=reviewing resolved rejected"`
	ResolutionNote string `json:"resolution_note" binding:"max=500"`
}

type ComplaintDTO struct {
	ID             string     `json:"id"`
	OrderID        string     `json:"order_id"`
	Reason         string     `json:"reason"`
	Detail         string     `json:"detail"`
	Status         string     `json:"status"`
	ResolutionNote *string    `json:"resolution_note"`
	PhotoURLs      []string   `json:"photo_urls"` // privat: hanya bisa dibuka pemilik, pemilik toko, dan admin
	CreatedAt      time.Time  `json:"created_at"`
	ResolvedAt     *time.Time `json:"resolved_at"`
}

type AdminComplaintDTO struct {
	ComplaintDTO
	OrderCode    string `json:"order_code"`
	MerchantName string `json:"merchant_name"`
	BuyerName    string `json:"buyer_name"`
}
