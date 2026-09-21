package model

import (
	"time"

	"github.com/FadhRach/sisaguna-be/internal/model/types"
	"github.com/google/uuid"
)

// ImpactRecord merepresentasikan tabel impact_records di CLAUDE.md.
// CO2AvoidedKg sengaja nullable dan tidak diisi otomatis di Fase 1 — lihat
// batasan kritis #3 soal konstanta konversi CO2 yang belum diverifikasi.
type ImpactRecord struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ReservationID uuid.UUID      `gorm:"column:reservation_id;not null;unique"           json:"reservation_id"`
	WeightKg      types.Decimal  `gorm:"column:weight_kg;type:numeric;not null"          json:"weight_kg"`
	CO2AvoidedKg  *types.Decimal `gorm:"column:co2_avoided_kg;type:numeric"              json:"co2_avoided_kg"`
	VerifiedAt    *time.Time     `gorm:"column:verified_at"                              json:"verified_at,omitempty"`
}

func (ImpactRecord) TableName() string {
	return "impact_records"
}
