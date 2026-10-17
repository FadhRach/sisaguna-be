// Package httpx berisi amplop respons, pemetaan error ke HTTP, dan middleware Gin.
package httpx

import (
	"errors"
	"log/slog"
	"net/http"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

const (
	ctxUserID  = "user_id"
	ctxIsAdmin = "is_admin"
)

type Meta map[string]any

func OK(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"data": data})
}

func OKMeta(c *gin.Context, data any, meta Meta) {
	c.JSON(http.StatusOK, gin.H{"data": data, "meta": meta})
}

func Error(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}

// ErrorRule memetakan error service yang dikenal client ke status dan kode HTTP.
type ErrorRule struct {
	Err    error
	Status int
	Code   string
}

// Fail menulis error: cocok dengan rule -> status/kode rule, selain itu 500 generik (detail hanya di log).
func Fail(c *gin.Context, err error, rules []ErrorRule) {
	for _, r := range rules {
		if errors.Is(err, r.Err) {
			body := gin.H{"code": r.Code, "message": r.Err.Error()}
			var withFields interface{ ErrorFields() map[string]any }
			if errors.As(err, &withFields) { // mis. suggested_tier, nilai harga terbaru
				for k, v := range withFields.ErrorFields() {
					body[k] = v
				}
			}
			c.AbortWithStatusJSON(r.Status, gin.H{"error": body})
			return
		}
	}
	slog.Error("unhandled error", "request_id", c.GetString("request_id"), "route", c.FullPath(), "err", err)
	Error(c, http.StatusInternalServerError, "internal_error", "terjadi kesalahan pada server")
}

// BindJSON mem-bind dan memvalidasi body (tag `binding`). Menulis 400/413 dan return false kalau gagal.
func BindJSON(c *gin.Context, dst any) bool { return bindFail(c, c.ShouldBindJSON(dst)) }

// BindForm sama seperti BindJSON untuk multipart/form-data atau query string (tag `form`).
func BindForm(c *gin.Context, dst any) bool { return bindFail(c, c.ShouldBind(dst)) }

func bindFail(c *gin.Context, err error) bool {
	if err == nil {
		return true
	}
	var tooBig *http.MaxBytesError
	var verrs validator.ValidationErrors
	switch {
	case errors.As(err, &tooBig):
		Error(c, http.StatusRequestEntityTooLarge, "payload_too_large", "body maksimal 1 MB")
	case errors.As(err, &verrs):
		fields := make([]string, 0, len(verrs))
		for _, fe := range verrs {
			fields = append(fields, fe.Field())
		}
		Error(c, http.StatusBadRequest, "validation_failed", "kolom tidak valid: "+strings.Join(fields, ", "))
	default:
		Error(c, http.StatusBadRequest, "invalid_request", "body permintaan tidak valid")
	}
	return false
}

// UserID membaca user id yang ditaruh RequireAuth.
func UserID(c *gin.Context) uuid.UUID {
	id, _ := c.Get(ctxUserID)
	u, _ := id.(uuid.UUID)
	return u
}

// ConfigureValidator dipanggil sekali saat router dibuat (dan di test yang memakai tag binding):
// pesan validasi memakai nama field JSON (account_type), dan tag `notblank` tersedia.
func ConfigureValidator() {
	v, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		return
	}
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			return f.Name
		}
		return name
	})
	_ = v.RegisterValidation("notblank", func(fl validator.FieldLevel) bool {
		return strings.TrimSpace(fl.Field().String()) != ""
	})
}

// IsAdmin membaca klaim admin dari access token (diset RequireAuth).
func IsAdmin(c *gin.Context) bool { return c.GetBool(ctxIsAdmin) }
