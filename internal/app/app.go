// Package app merakit semua dependency (constructor injection) menjadi router HTTP dan scheduler.
// main.go hanya memuat config, membuka koneksi, lalu menjalankan hasil rakitan ini.
package app

import (
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FadhRach/sisaguna-be/internal/address"
	"github.com/FadhRach/sisaguna-be/internal/admin"
	"github.com/FadhRach/sisaguna-be/internal/chat"
	"github.com/FadhRach/sisaguna-be/internal/complaint"
	"github.com/FadhRach/sisaguna-be/internal/config"
	"github.com/FadhRach/sisaguna-be/internal/listing"
	"github.com/FadhRach/sisaguna-be/internal/media"
	"github.com/FadhRach/sisaguna-be/internal/merchant"
	"github.com/FadhRach/sisaguna-be/internal/notification"
	"github.com/FadhRach/sisaguna-be/internal/order"
	"github.com/FadhRach/sisaguna-be/internal/payment"
	"github.com/FadhRach/sisaguna-be/internal/platform/auth"
	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
	pmedia "github.com/FadhRach/sisaguna-be/internal/platform/media"
	"github.com/FadhRach/sisaguna-be/internal/platform/realtime"
	"github.com/FadhRach/sisaguna-be/internal/platform/scheduler"
	"github.com/FadhRach/sisaguna-be/internal/review"
	"github.com/FadhRach/sisaguna-be/internal/saved"
	"github.com/FadhRach/sisaguna-be/internal/user"
	"github.com/FadhRach/sisaguna-be/internal/voucher"
)

// writesPerMinute: batas semua request tulis (POST/PUT/PATCH/DELETE) per akun; chat dan upload punya batas sendiri yang lebih ketat.
const writesPerMinute = 120

type App struct {
	Router    *gin.Engine
	Scheduler *scheduler.Scheduler
	Hub       *realtime.Hub // main.go menjalankan Hub.Run (LISTEN sg_events)
}

func New(cfg *config.Config, pool *pgxpool.Pool) (*App, error) {
	tokens := auth.NewTokens(cfg.JWTSecret, cfg.AccessTTL)
	passwords, err := auth.NewPasswords()
	if err != nil {
		return nil, err
	}
	disk := pmedia.NewDisk(cfg.MediaRoot)
	urls := pmedia.URLs{Base: cfg.PublicBaseURL}

	mediaSvc := media.NewService(media.NewRepository(pool), disk, urls)
	if cfg.R2Enabled() {
		endpoint := cfg.R2Endpoint
		if endpoint == "" {
			endpoint = pmedia.R2Endpoint(cfg.R2AccountID)
		}
		mediaSvc.WithRemote(pmedia.NewS3(endpoint, cfg.R2AccessKeyID, cfg.R2Secret, cfg.R2Bucket))
	}
	listingSvc := listing.NewService(listing.NewRepository(pool), urls).WithPartners(cfg.PartnersDummy)
	orderSvc := order.NewService(order.NewRepository(pool), urls).WithPartners(cfg.PartnersDummy)

	router, err := httpx.NewRouter(cfg.IsProduction(), cfg.TrustedProxies)
	if err != nil {
		return nil, err
	}
	router.GET("/healthz", httpx.Health(func(c *gin.Context) error {
		if err := pool.Ping(c.Request.Context()); err != nil {
			return err
		}
		return disk.Writable()
	}))
	if !cfg.IsProduction() {
		// Di produksi foto publik disajikan Caddy; di lokal Go yang menyajikan (folder public saja).
		router.Static("/media", filepath.Join(cfg.MediaRoot, "public"))
	}

	requireAuth := httpx.RequireAuthLimited(tokens, writesPerMinute)
	optionalAuth := httpx.OptionalAuth(tokens)
	api := router.Group("/api/v1")
	user.NewHandler(user.NewService(user.NewRepository(pool), tokens, passwords, cfg.RefreshTTL), urls).Routes(api, requireAuth, user.Limits{
		Login:    httpx.RateLimit(max(cfg.LoginRate, 1), time.Minute, httpx.ByIP),
		Register: httpx.RateLimit(max(cfg.RegisterRate, 1), time.Minute, httpx.ByIP),
		Password: httpx.RateLimit(5, time.Minute, httpx.ByUser),
	})
	address.NewHandler(address.NewService(address.NewRepository(pool))).Routes(api, requireAuth)
	media.NewHandler(mediaSvc).Routes(api, requireAuth, httpx.RateLimit(30, time.Hour, httpx.ByUser))
	merchant.NewHandler(merchant.NewService(merchant.NewRepository(pool), listingSvc, urls)).Routes(api, requireAuth, optionalAuth)
	listing.NewHandler(listingSvc).Routes(api, requireAuth, optionalAuth)
	order.NewHandler(orderSvc).Routes(api, requireAuth)
	voucher.NewHandler(voucher.NewService(pool, urls)).Routes(api, requireAuth)
	payment.NewHandler(payment.NewService(pool)).Routes(api, requireAuth)
	notification.NewHandler(notification.NewService(pool)).Routes(api, requireAuth)
	admin.NewHandler(admin.NewService(pool)).Routes(api, requireAuth)
	review.NewHandler(review.NewService(pool, urls)).Routes(api, requireAuth)
	complaint.NewHandler(complaint.NewService(pool, urls)).Routes(api, requireAuth)
	saved.NewHandler(saved.NewService(pool, urls)).Routes(api, requireAuth)

	chatSvc := chat.NewService(pool)
	chat.NewHandler(chatSvc).Routes(api, requireAuth, httpx.RateLimit(20, time.Minute, httpx.ByUser))
	hub := realtime.NewHub(chatSvc)
	hub.Routes(router, api, requireAuth, httpx.RateLimit(30, time.Minute, httpx.ByUser))

	// Job tiap menit di bawah advisory lock: no-show, reminder pickup, hapus media yatim, salin media ke R2.
	jobs := scheduler.New(pool, time.Minute,
		scheduler.Job{Name: "order.no_show", Run: orderSvc.ExpireNoShows},
		scheduler.Job{Name: "order.reminder", Run: orderSvc.SendReminders},
		scheduler.Job{Name: "media.orphans", Run: mediaSvc.CleanupOrphans},
		scheduler.Job{Name: "media.r2sync", Run: mediaSvc.SyncToRemote}) // tidak melakukan apa-apa kalau R2 belum dikonfigurasi
	return &App{Router: router, Scheduler: jobs, Hub: hub}, nil
}
