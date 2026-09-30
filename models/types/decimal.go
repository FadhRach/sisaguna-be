package types

import (
	"database/sql/driver"
	"fmt"
)

// Decimal menyimpan angka numeric Postgres sebagai string. Karena underlying
// type-nya string, encoding/json otomatis serialize ini sebagai JSON string
// (bukan number) sesuai Konvensi API di CLAUDE.md, tanpa perlu MarshalJSON manual.
type Decimal string

func (d *Decimal) Scan(value interface{}) error {
	if value == nil {
		*d = ""
		return nil
	}
	switch v := value.(type) {
	case string:
		*d = Decimal(v)
	case []byte:
		*d = Decimal(v)
	default:
		return fmt.Errorf("decimal scan: tipe tidak didukung %T", value)
	}
	return nil
}

func (d Decimal) Value() (driver.Value, error) {
	if d == "" {
		return nil, nil
	}
	return string(d), nil
}

func (Decimal) GormDataType() string {
	return "numeric"
}
