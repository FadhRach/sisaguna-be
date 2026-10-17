package httpx

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/time/rate"

	"github.com/FadhRach/sisaguna-be/internal/platform/auth"
)

const maxBodyBytes = 1 << 20

// NewRouter membuat engine dengan middleware global: request id, log, recover, batas body 1 MB.
func NewRouter(production bool, trustedProxies []string) (*gin.Engine, error) {
	if production {
		gin.SetMode(gin.ReleaseMode)
	}
	ConfigureValidator()
	r := gin.New()
	if err := r.SetTrustedProxies(trustedProxies); err != nil {
		return nil, err
	}
	r.Use(requestID(), accessLog(), recoverJSON(), limitBody())
	return r, nil
}

func requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if _, err := uuid.Parse(id); err != nil {
			id = uuid.NewString()
		}
		c.Set("request_id", id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

func accessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		attrs := []any{
			"request_id", c.GetString("request_id"),
			"route", c.FullPath(),
			"method", c.Request.Method,
			"status", c.Writer.Status(),
			"ms", time.Since(start).Milliseconds(),
		}
		if uid := UserID(c); uid != uuid.Nil {
			attrs = append(attrs, "user_id", uid.String())
		}
		slog.Info("request", attrs...)
	}
}

func recoverJSON() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		slog.Error("panic", "request_id", c.GetString("request_id"), "route", c.FullPath(), "panic", recovered)
		Error(c, http.StatusInternalServerError, "internal_error", "terjadi kesalahan pada server")
	})
}

func limitBody() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/") {
			c.Next() // upload file mengatur batasnya sendiri
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
		c.Next()
	}
}

// RequireAuth memvalidasi Bearer access token dan menaruh user id di context.
func RequireAuth(tokens *auth.Tokens) gin.HandlerFunc {
	return func(c *gin.Context) {
		if authenticate(c, tokens) {
			c.Next()
		}
	}
}

// RequireAuthLimited seperti RequireAuth, ditambah batas request tulis (selain GET): perMinute per akun,
// satu ember untuk semua endpoint tulis. Dipasang sebagai requireAuth standar supaya tak ada endpoint tulis yang terlewat.
func RequireAuthLimited(tokens *auth.Tokens, perMinute int) gin.HandlerFunc {
	limit := RateLimit(perMinute, time.Minute, ByUser)
	return func(c *gin.Context) {
		if !authenticate(c, tokens) {
			return
		}
		if c.Request.Method == http.MethodGet {
			c.Next()
			return
		}
		limit(c) // memanggil c.Next() kalau masih dalam batas
	}
}

// authenticate menulis 401 dan mengembalikan false kalau token tidak valid.
func authenticate(c *gin.Context, tokens *auth.Tokens) bool {
	raw, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	if !ok || raw == "" {
		Error(c, http.StatusUnauthorized, "unauthorized", "header Authorization Bearer wajib diisi")
		return false
	}
	claims, err := tokens.Parse(raw)
	if err != nil {
		Error(c, http.StatusUnauthorized, "unauthorized", "token tidak valid atau kedaluwarsa")
		return false
	}
	uid, err := claims.UserID()
	if err != nil {
		Error(c, http.StatusUnauthorized, "unauthorized", "token tidak valid atau kedaluwarsa")
		return false
	}
	c.Set(ctxUserID, uid)
	c.Set(ctxIsAdmin, claims.IsAdmin)
	return true
}

// OptionalAuth mengisi user id kalau ada Bearer token yang valid; tanpa token (atau token rusak) request
// tetap lanjut sebagai anonim (UserID = uuid.Nil). Untuk endpoint publik yang menambah data khusus pengguna.
func OptionalAuth(tokens *auth.Tokens) gin.HandlerFunc {
	return func(c *gin.Context) {
		if raw, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer "); ok {
			if claims, err := tokens.Parse(raw); err == nil {
				if uid, err := claims.UserID(); err == nil {
					c.Set(ctxUserID, uid)
					c.Set(ctxIsAdmin, claims.IsAdmin)
				}
			}
		}
		c.Next()
	}
}

// ByIP dan ByUser adalah kunci untuk RateLimit. ByUser harus dipasang setelah RequireAuth.
func ByIP(c *gin.Context) string   { return c.ClientIP() }
func ByUser(c *gin.Context) string { return UserID(c).String() }

// RateLimit membatasi n request per jendela per kunci (IP atau akun). State ada di closure.
func RateLimit(n int, per time.Duration, key func(*gin.Context) string) gin.HandlerFunc {
	type entry struct {
		lim  *rate.Limiter
		seen time.Time
	}
	var mu sync.Mutex
	clients := map[string]*entry{}

	return func(c *gin.Context) {
		k := key(c)
		now := time.Now()

		mu.Lock()
		if len(clients) > 10_000 { // shortcut: sapu sederhana, ganti store eksternal kalau multi-instance
			for ck, e := range clients {
				if now.Sub(e.seen) > 10*time.Minute {
					delete(clients, ck)
				}
			}
		}
		e, ok := clients[k]
		if !ok {
			e = &entry{lim: rate.NewLimiter(rate.Every(per/time.Duration(n)), n)}
			clients[k] = e
		}
		e.seen = now
		allowed := e.lim.Allow()
		mu.Unlock()

		if !allowed {
			slog.Warn("rate limited", "request_id", c.GetString("request_id"), "route", c.FullPath())
			Error(c, http.StatusTooManyRequests, "rate_limited", "terlalu banyak permintaan, coba lagi sebentar")
			return
		}
		c.Next()
	}
}

// Health menjawab /healthz dengan memanggil check (mis. ping DB).
func Health(check func(c *gin.Context) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := check(c); err != nil {
			slog.Error("healthz", "err", err)
			Error(c, http.StatusServiceUnavailable, "unhealthy", "dependensi tidak sehat")
			return
		}
		OK(c, http.StatusOK, gin.H{"status": "ok"})
	}
}

// RequireAdmin harus dipasang setelah RequireAuth.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !IsAdmin(c) {
			Error(c, http.StatusForbidden, "forbidden", "hanya admin yang boleh mengakses")
			return
		}
		c.Next()
	}
}
