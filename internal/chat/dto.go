package chat

import (
	"time"

	"github.com/google/uuid"
)

type StartRequest struct {
	MerchantID uuid.UUID `json:"merchant_id" binding:"required"`
}

type SendRequest struct {
	ClientID  uuid.UUID  `json:"client_id" binding:"required"`
	Body      string     `json:"body" binding:"required,notblank,max=1000"`
	ListingID *uuid.UUID `json:"listing_id"`
}

type ReadRequest struct {
	LastReadID *int64 `json:"last_read_id" binding:"required,min=0"`
}

type InboxQuery struct {
	As    string `form:"as" binding:"required,oneof=buyer merchant"`
	Limit int    `form:"limit" binding:"omitempty,min=1,max=50"`
}

// MessagesQuery: tanpa parameter = pesan terbaru; before_id = scroll ke atas; after_id = sinkron setelah reconnect.
type MessagesQuery struct {
	BeforeID *int64 `form:"before_id" binding:"omitempty,min=1,excluded_with=AfterID"`
	AfterID  *int64 `form:"after_id" binding:"omitempty,min=0"`
	Limit    int    `form:"limit" binding:"omitempty,min=1,max=100"`
}

type PartyDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type LastMessageDTO struct {
	Body string    `json:"body"`
	Side string    `json:"side"`
	At   time.Time `json:"at"`
}

// ConversationDTO: counterpart adalah lawan bicara (toko untuk pembeli, pembeli untuk mitra).
type ConversationDTO struct {
	ID          string          `json:"id"`
	Counterpart PartyDTO        `json:"counterpart"`
	LastMessage *LastMessageDTO `json:"last_message"`
	Unread      int64           `json:"unread"`
}

// MessageDTO juga dipakai sebagai data event message.created.
type MessageDTO struct {
	ID             int64     `json:"id"`
	ConversationID string    `json:"conversation_id"`
	SenderSide     string    `json:"sender_side"`
	Body           string    `json:"body"`
	ListingID      *string   `json:"listing_id"`
	ClientID       string    `json:"client_id"`
	CreatedAt      time.Time `json:"created_at"`
}
