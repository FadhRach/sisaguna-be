package config

import (
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// DB adalah koneksi database yang sudah di-Connect, dibaca langsung oleh
// repositories (niru pola reference project: config.DB).
var DB *gorm.DB

// Connect membuka koneksi ke Postgres self-hosted (direct connection, bukan
// pooler transaksi ala Supavisor — jadi prepared statement GORM default aman
// dipakai, tidak perlu PreferSimpleProtocol/PrepareStmt:false lagi). Dipanggil
// sekali saja dari main() saat startup.
func Connect(cfg *Config) (*gorm.DB, error) {
	gormDB, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("db connect: %w", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("db connect: %w", err)
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	DB = gormDB
	return gormDB, nil
}
