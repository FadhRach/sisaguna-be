package review

import (
	"time"

	"github.com/google/uuid"
)

type CreateRequest struct {
	Stars    int         `json:"stars" binding:"required,min=1,max=5"`
	Tags     []string    `json:"tags" binding:"omitempty,max=5,dive,notblank,max=30"`
	Comment  string      `json:"comment" binding:"max=500"`
	MediaIDs []uuid.UUID `json:"media_ids" binding:"omitempty,max=3"`
}

type ListQuery struct {
	Limit int `form:"limit" binding:"omitempty,min=1,max=50"`
}

type PhotoDTO struct {
	URL      string  `json:"url"`
	ThumbURL *string `json:"thumb_url"`
}

type ReviewDTO struct {
	ID        string     `json:"id"`
	Stars     int        `json:"stars"`
	Tags      []string   `json:"tags"`
	Comment   string     `json:"comment"`
	BuyerName string     `json:"buyer_name"`
	Photos    []PhotoDTO `json:"photos"`
	CreatedAt time.Time  `json:"created_at"`
}
