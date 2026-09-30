package types

import (
	"database/sql/driver"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
)

// GeoPoint memetakan kolom PostGIS geography(Point,4326).
type GeoPoint struct {
	Lat float64
	Lng float64
}

// Scan mem-parse WKB (well-known binary) hex yang dikembalikan Postgres untuk
// kolom geography kalau di-select tanpa ST_AsText(). Cuma menangani tipe Point
// (dengan atau tanpa flag SRID di header EWKB) karena itu satu-satunya tipe
// yang dipakai di skema ini.
func (p *GeoPoint) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	var hexStr string
	switch v := value.(type) {
	case string:
		hexStr = v
	case []byte:
		hexStr = string(v)
	default:
		return fmt.Errorf("geo_point scan: tipe tidak didukung %T", value)
	}

	raw, err := hex.DecodeString(hexStr)
	if err != nil {
		return fmt.Errorf("geo_point scan: decode hex: %w", err)
	}
	if len(raw) < 21 {
		return fmt.Errorf("geo_point scan: data WKB terlalu pendek")
	}

	order := binary.ByteOrder(binary.BigEndian)
	if raw[0] == 1 {
		order = binary.LittleEndian
	}

	geomType := order.Uint32(raw[1:5])
	offset := 5
	if geomType&0x20000000 != 0 { // EWKB: bit ini menandai ada SRID setelah geomType
		offset += 4
	}

	p.Lng = math.Float64frombits(order.Uint64(raw[offset : offset+8]))
	p.Lat = math.Float64frombits(order.Uint64(raw[offset+8 : offset+16]))
	return nil
}

// Value mengembalikan representasi EWKT ("SRID=4326;POINT(lng lat)"). Postgres
// otomatis cast text ini ke kolom geography saat INSERT/UPDATE.
func (p GeoPoint) Value() (driver.Value, error) {
	return fmt.Sprintf("SRID=4326;POINT(%f %f)", p.Lng, p.Lat), nil
}

func (GeoPoint) GormDataType() string {
	return "geography(Point,4326)"
}
