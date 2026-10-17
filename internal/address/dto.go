package address

import "github.com/google/uuid"

type CreateRequest struct {
	Label       string   `json:"label" binding:"required,notblank,max=30"`
	FullAddress string   `json:"full_address" binding:"required,notblank,max=300"`
	Note        string   `json:"note" binding:"max=200"`
	Lat         *float64 `json:"lat" binding:"required,min=-90,max=90"`
	Lng         *float64 `json:"lng" binding:"required,min=-180,max=180"`
}

// UpdateRequest: field yang tidak dikirim tidak diubah. lat dan lng harus dikirim berpasangan.
type UpdateRequest struct {
	Label       *string  `json:"label" binding:"omitempty,notblank,max=30"`
	FullAddress *string  `json:"full_address" binding:"omitempty,notblank,max=300"`
	Note        *string  `json:"note" binding:"omitempty,max=200"`
	Lat         *float64 `json:"lat" binding:"omitempty,min=-90,max=90"`
	Lng         *float64 `json:"lng" binding:"omitempty,min=-180,max=180"`
}

// SelectRequest: address_id null berarti memakai lokasi GPS.
type SelectRequest struct {
	AddressID *uuid.UUID `json:"address_id"`
}

type AddressDTO struct {
	ID          string  `json:"id"`
	Label       string  `json:"label"`
	FullAddress string  `json:"full_address"`
	Note        string  `json:"note"`
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
}
