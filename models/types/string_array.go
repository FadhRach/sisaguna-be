package types

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

// StringArray memetakan kolom Postgres text[] ke/dari []string. Parser di sini
// sengaja sederhana (tidak menangani koma di dalam elemen) karena dipakai
// untuk role enum yang tidak punya karakter spesial.
type StringArray []string

func (a *StringArray) Scan(value interface{}) error {
	if value == nil {
		*a = nil
		return nil
	}

	var raw string
	switch v := value.(type) {
	case string:
		raw = v
	case []byte:
		raw = string(v)
	default:
		return fmt.Errorf("string_array scan: tipe tidak didukung %T", value)
	}

	*a = parsePostgresArray(raw)
	return nil
}

func (a StringArray) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	escaped := make([]string, len(a))
	for i, s := range a {
		escaped[i] = `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return "{" + strings.Join(escaped, ",") + "}", nil
}

func (StringArray) GormDataType() string {
	return "text[]"
}

func parsePostgresArray(raw string) StringArray {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "{")
	raw = strings.TrimSuffix(raw, "}")
	if raw == "" {
		return StringArray{}
	}

	parts := strings.Split(raw, ",")
	result := make(StringArray, len(parts))
	for i, part := range parts {
		result[i] = strings.Trim(strings.TrimSpace(part), `"`)
	}
	return result
}
