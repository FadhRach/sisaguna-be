// Package db mengelola lifecycle koneksi GORM ke Postgres (Supabase Supavisor
// pooler di production, Postgres biasa di lokal — lihat README.md).
package db

import (
	"fmt"
	"sync"
	"time"

	"github.com/FadhRach/sisaguna-be/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	once     sync.Once
	instance *gorm.DB
	initErr  error
)

// Connect membuka koneksi baru ke Supavisor pooler (transaction mode).
// PreferSimpleProtocol dan PrepareStmt:false wajib mati karena Supavisor
// transaction mode tidak mendukung prepared statement (lihat batasan kritis #2 di CLAUDE.md).
func Connect(cfg *config.Config) (*gorm.DB, error) {
	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  cfg.DatabaseURL,
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		PrepareStmt: false,
	})
	if err != nil {
		return nil, fmt.Errorf("db connect: %w", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("db connect: %w", err)
	}
	sqlDB.SetMaxOpenConns(5)
	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	return gormDB, nil
}

// Get mengembalikan pool koneksi singleton untuk proses ini. Karena tiap file
// di /api adalah Vercel Function (proses) terpisah, singleton ini berlaku
// per-function, bukan satu pool yang dibagi lintas seluruh aplikasi.
func Get(cfg *config.Config) (*gorm.DB, error) {
	once.Do(func() {
		instance, initErr = Connect(cfg)
	})
	return instance, initErr
}
