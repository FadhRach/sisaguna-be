// Package database membungkus pgxpool dan helper transaksi.
package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("database parse config: %w", err)
	}
	cfg.MaxConns = 20
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.AfterConnect = RegisterTypes

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("database connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database ping: %w", err)
	}
	return pool, nil
}

// InTx menjalankan fn dalam satu transaksi: commit kalau fn sukses, rollback kalau error.
func InTx(ctx context.Context, pool *pgxpool.Pool, fn func(tx pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op setelah commit

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// IsUniqueViolation true kalau err adalah pelanggaran UNIQUE (SQLSTATE 23505).
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// ConstraintName mengembalikan nama constraint yang dilanggar (kosong kalau bukan error PostgreSQL).
func ConstraintName(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}

// RegisterTypes mendaftarkan tipe array enum buatan kita ke koneksi pgx. Tanpa ini pgx tidak bisa
// membaca/menulis array enum ("unknown type"). Setiap enum yang dipakai sebagai array harus terdaftar
// di sini. Migrasi harus sudah jalan.
func RegisterTypes(ctx context.Context, conn *pgx.Conn) error {
	types, err := conn.LoadTypes(ctx, []string{
		"allergen", "_allergen", "safety_check", "_safety_check", // kolom array
		"order_status", "_order_status", "listing_tier", "_listing_tier", // parameter array (filter)
	})
	if err != nil {
		return fmt.Errorf("database register types: %w", err)
	}
	conn.TypeMap().RegisterTypes(types)
	return nil
}
