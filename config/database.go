package config

import (
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Connect membuka koneksi ke Postgres self-hosted (direct connection, bukan
// pooler transaksi ala Supavisor — jadi prepared statement GORM default aman
// dipakai, tidak perlu PreferSimpleProtocol/PrepareStmt:false lagi). Dipanggil
// sekali saja dari main() saat startup — proses ini satu binary long-lived,
// bukan spin-up per-request, jadi tidak butuh singleton/sync.Once lagi.
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

	return gormDB, nil
}
