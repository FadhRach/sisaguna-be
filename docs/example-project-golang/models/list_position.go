package models

import (
	"github.com/FadhRach/Learn-Golang-React/project-management/models/types"
)

type ListPosition struct {
	InternalID int64           `json:"internal_id" db:"internal_id" gorm:"primaryKey;autoIncrement"`
	PublicID   int64           `json:"public_id" db:"public_id" gorm:"public_id"`
	BoardID    int64           `json:"board_internal_id" db:"board_internal_id" gorm:"board_internal_id"`
	ListOrder  types.UUIDArray `json:"list_order" ` //array uuid {uuid1, uuid2}
}
