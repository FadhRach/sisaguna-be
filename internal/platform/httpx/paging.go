package httpx

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PageQuery adalah ?page=&limit= untuk daftar terbatas.
type PageQuery struct {
	Page  int `form:"page" binding:"omitempty,min=1"`
	Limit int `form:"limit" binding:"omitempty,min=1,max=50"`
}

func (p PageQuery) Normalize() (page, limit int) {
	page, limit = p.Page, p.Limit
	if page == 0 {
		page = 1
	}
	if limit == 0 {
		limit = 20
	}
	return page, limit
}

// PageMeta menyusun meta untuk daftar ber-halaman.
func PageMeta(page, limit int, total int64) Meta {
	return Meta{"page": page, "limit": limit, "total": total, "total_pages": (total + int64(limit) - 1) / int64(limit)}
}

// Cursor menunjuk posisi (created_at, id) untuk daftar yang terus bertambah.
type Cursor struct {
	At time.Time `json:"at"`
	ID uuid.UUID `json:"id"`
}

func EncodeCursor(at time.Time, id uuid.UUID) string {
	b, _ := json.Marshal(Cursor{At: at.UTC(), ID: id})
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor adalah kebalikan EncodeCursor.
func DecodeCursor(raw string) (Cursor, bool) {
	var out Cursor
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || json.Unmarshal(b, &out) != nil || out.ID == uuid.Nil {
		return Cursor{}, false
	}
	return out, true
}

// ParseCursor membaca ?cursor=. Kosong -> nil. Rusak -> menulis 400 dan ok=false.
func ParseCursor(c *gin.Context) (cur *Cursor, ok bool) {
	raw := c.Query("cursor")
	if raw == "" {
		return nil, true
	}
	out, ok := DecodeCursor(raw)
	if !ok {
		Error(c, http.StatusBadRequest, "invalid_cursor", "cursor tidak valid")
		return nil, false
	}
	return &out, true
}

// IdempotencyKey membaca header wajib Idempotency-Key. Kosong atau terlalu panjang -> 400.
func IdempotencyKey(c *gin.Context) (string, bool) {
	k := c.GetHeader("Idempotency-Key")
	if k == "" || len(k) > 64 {
		Error(c, http.StatusBadRequest, "idempotency_key_required", "header Idempotency-Key wajib diisi (maks 64 karakter)")
		return "", false
	}
	return k, true
}
