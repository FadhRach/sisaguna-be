package types

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// JSONPayload memetakan kolom Postgres jsonb ke/dari JSON mentah. Sengaja
// tidak pakai package eksternal (mis. gorm.io/datatypes) karena package itu
// ikut menarik driver MySQL/SQL Server yang tidak relevan buat proyek ini —
// tipe kecil ini cukup untuk kebutuhan satu kolom payload notifikasi.
type JSONPayload json.RawMessage

func (p *JSONPayload) Scan(value interface{}) error {
	if value == nil {
		*p = nil
		return nil
	}
	switch v := value.(type) {
	case string:
		*p = JSONPayload(v)
	case []byte:
		*p = JSONPayload(append([]byte(nil), v...))
	default:
		return fmt.Errorf("json_payload scan: tipe tidak didukung %T", value)
	}
	return nil
}

func (p JSONPayload) Value() (driver.Value, error) {
	if len(p) == 0 {
		return "{}", nil
	}
	return string(p), nil
}

func (JSONPayload) GormDataType() string {
	return "jsonb"
}

func (p JSONPayload) MarshalJSON() ([]byte, error) {
	if len(p) == 0 {
		return []byte("{}"), nil
	}
	return p, nil
}

func (p *JSONPayload) UnmarshalJSON(data []byte) error {
	*p = append((*p)[0:0], data...)
	return nil
}
