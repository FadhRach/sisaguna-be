// Package config memuat environment variable jadi satu struct yang tervalidasi.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv               string // development | production
	DatabaseURL          string
	JWTSecret            string
	AccessTTL            time.Duration
	RefreshTTL           time.Duration
	Port                 string
	PublicBaseURL        string
	MediaRoot            string
	Timezone             string
	TrustedProxies       []string
	LoginRate            int // percobaan login per menit per IP
	RegisterRate         int // pendaftaran per menit per IP
	AdminEmail           string
	AdminPassword        string
	AdminName            string
	R2AccountID          string
	R2AccessKeyID        string
	R2Secret             string
	R2Bucket             string
	FCMCredentialsFile   string // service account JSON; kosong = push mati
	FCMProjectID         string
	EmailMode            string // smtp | log | off
	SMTPHost, SMTPPort   string
	SMTPUser, SMTPPass   string
	SMTPFrom             string
	AutoApproveMerchants bool   // MERCHANT_AUTO_APPROVE=true: toko baru langsung approved tanpa admin (hanya untuk uji)
	AllowDemoSeed        bool   // ALLOW_DEMO_SEED=true: izinkan seed dev (data dummy + akun berpassword umum) di APP_ENV=production, hanya untuk VPS uji
	PartnersDummy        bool   // kurir + pembayaran non-tunai tiruan (PARTNERS_MODE=dummy); default dummy di development, off di produksi
	R2Endpoint           string // opsional; default https://<account>.r2.cloudflarestorage.com (diisi hanya untuk tes dengan MinIO)
}

// Load membaca .env (kalau ada) lalu environment asli. Di Docker env sudah
// di-inject compose, jadi error godotenv diabaikan.
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		AppEnv:         envOr("APP_ENV", "development"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		JWTSecret:      os.Getenv("JWT_SECRET"),
		AccessTTL:      15 * time.Minute,
		RefreshTTL:     30 * 24 * time.Hour,
		Port:           envOr("PORT", "3030"),
		PublicBaseURL:  strings.TrimRight(envOr("PUBLIC_BASE_URL", "http://localhost:3030"), "/"),
		MediaRoot:      envOr("MEDIA_ROOT", "./data/media"),
		Timezone:       envOr("APP_TIMEZONE", "Asia/Jakarta"),
		TrustedProxies: strings.Split(envOr("TRUSTED_PROXIES", "127.0.0.1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16"), ","),
		AdminEmail:     os.Getenv("ADMIN_EMAIL"),
		AdminPassword:  os.Getenv("ADMIN_PASSWORD"),
		AdminName:      envOr("ADMIN_NAME", "Admin SisaGuna"),
		R2AccountID:    os.Getenv("R2_ACCOUNT_ID"),
		R2AccessKeyID:  os.Getenv("R2_ACCESS_KEY_ID"),
		R2Secret:       os.Getenv("R2_SECRET_ACCESS_KEY"),
		R2Bucket:       os.Getenv("R2_BUCKET"),
		R2Endpoint:     os.Getenv("R2_ENDPOINT"),
	}

	// Produksi: 5 login dan 3 register per menit per IP. Development lebih longgar supaya tes manual
	// dengan banyak peran tidak terkunci; bisa diatur lewat LOGIN_RATE_PER_MIN / REGISTER_RATE_PER_MIN.
	strict := os.Getenv("APP_ENV") == "production"
	cfg.PartnersDummy = envOr("PARTNERS_MODE", map[bool]string{true: "off", false: "dummy"}[strict]) == "dummy"
	cfg.SMTPHost, cfg.SMTPPort = os.Getenv("SMTP_HOST"), envOr("SMTP_PORT", "587")
	cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFrom = os.Getenv("SMTP_USER"), os.Getenv("SMTP_PASSWORD"), envOr("SMTP_FROM", "SisaGuna <no-reply@sisaguna.id>")
	// default: development mencetak email ke log; produksi mengirim lewat SMTP bila SMTP_HOST diisi, kalau tidak mati.
	defMode := "log"
	if strict {
		defMode = "off"
		if cfg.SMTPHost != "" {
			defMode = "smtp"
		}
	}
	cfg.EmailMode = envOr("EMAIL_MODE", defMode)
	cfg.FCMCredentialsFile, cfg.FCMProjectID = os.Getenv("FCM_CREDENTIALS_FILE"), os.Getenv("FCM_PROJECT_ID")
	cfg.AllowDemoSeed = os.Getenv("ALLOW_DEMO_SEED") == "true"
	cfg.AutoApproveMerchants = os.Getenv("MERCHANT_AUTO_APPROVE") == "true"
	cfg.LoginRate = envInt("LOGIN_RATE_PER_MIN", map[bool]int{true: 5, false: 60}[strict])
	cfg.RegisterRate = envInt("REGISTER_RATE_PER_MIN", map[bool]int{true: 3, false: 30}[strict])

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("config load: DATABASE_URL wajib diisi")
	}
	if len(cfg.JWTSecret) < 32 {
		return nil, fmt.Errorf("config load: JWT_SECRET minimal 32 byte")
	}
	if cfg.EmailMode != "smtp" && cfg.EmailMode != "log" && cfg.EmailMode != "off" {
		return nil, fmt.Errorf("config load: EMAIL_MODE harus smtp, log, atau off")
	}
	if cfg.EmailMode == "smtp" && cfg.SMTPHost == "" {
		return nil, fmt.Errorf("config load: EMAIL_MODE=smtp butuh SMTP_HOST")
	}
	if cfg.AppEnv != "development" && cfg.AppEnv != "production" {
		return nil, fmt.Errorf("config load: APP_ENV harus development atau production")
	}
	return cfg, nil
}

// R2Enabled: salinan media ke R2 hanya jalan kalau keempat variabel R2 terisi.
func (c *Config) R2Enabled() bool {
	return c.R2AccountID != "" && c.R2AccessKeyID != "" && c.R2Secret != "" && c.R2Bucket != ""
}

func (c *Config) IsProduction() bool { return c.AppEnv == "production" }

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil && n > 0 {
		return n
	}
	return fallback
}
