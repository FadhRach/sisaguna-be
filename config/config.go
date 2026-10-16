// Package config memuat environment variable jadi satu struct yang tervalidasi
// (config.go) dan mengelola koneksi database (database.go) — dua tanggung
// jawab, dua file, satu package flat di root supaya sesuai konvensi proyek.
package config

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
)

// AppConfig adalah config yang sudah di-Load, dibaca package lain yang butuh
// (niru pola reference project: config.AppConfig).
var AppConfig *Config

// Config menampung seluruh environment variable yang dipakai aplikasi.
type Config struct {
	DatabaseURL        string
	JWTSecret          string
	JWTAccessTokenTTL  time.Duration
	JWTRefreshTokenTTL time.Duration
	CronSecret         string
	Port               string
}

// Load membaca .env (kalau ada) lalu environment variable asli, dan
// memvalidasi variabel yang wajib diisi. Di Docker, env var di-inject
// langsung lewat docker-compose, jadi error dari godotenv.Load() diabaikan.
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		JWTSecret:          os.Getenv("JWT_SECRET"),
		JWTAccessTokenTTL:  15 * time.Minute,
		JWTRefreshTokenTTL: 30 * 24 * time.Hour,
		CronSecret:         os.Getenv("CRON_SECRET"),
		Port:               envOrDefault("PORT", "3030"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("config load: DATABASE_URL wajib diisi")
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("config load: JWT_SECRET wajib diisi")
	}

	AppConfig = cfg
	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
