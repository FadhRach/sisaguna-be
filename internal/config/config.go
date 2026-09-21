// Package config memuat environment variable jadi satu struct yang tervalidasi.
// Tidak menyentuh koneksi database — itu tanggung jawab package internal/db.
package config

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
)

// Config menampung seluruh environment variable yang dipakai aplikasi.
type Config struct {
	DatabaseURL          string
	MigrationDatabaseURL string
	JWTSecret            string
	JWTAccessTokenTTL    time.Duration
	JWTRefreshTokenTTL   time.Duration
	CronSecret           string
	BlobReadWriteToken   string
	Port                 string
}

// Load membaca .env (kalau ada) lalu environment variable asli, dan
// memvalidasi variabel yang wajib diisi. Di Vercel, env var di-inject
// langsung tanpa file .env, jadi error dari godotenv.Load() diabaikan.
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		MigrationDatabaseURL: os.Getenv("MIGRATION_DATABASE_URL"),
		JWTSecret:            os.Getenv("JWT_SECRET"),
		JWTAccessTokenTTL:    15 * time.Minute,
		JWTRefreshTokenTTL:   30 * 24 * time.Hour,
		CronSecret:           os.Getenv("CRON_SECRET"),
		BlobReadWriteToken:   os.Getenv("BLOB_READ_WRITE_TOKEN"),
		Port:                 envOrDefault("PORT", "3030"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("config load: DATABASE_URL wajib diisi")
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("config load: JWT_SECRET wajib diisi")
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
