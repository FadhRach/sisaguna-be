// Package testutil menyediakan Postgres+PostGIS asli (testcontainers) untuk test integrasi.
package testutil

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/FadhRach/sisaguna-be/internal/platform/database"
)

// NewPool menjalankan container postgis/postgis:16-3.4, menerapkan semua migrasi *.up.sql,
// dan mengembalikan pool. Container dihentikan saat test selesai.
func NewPool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx, "postgis/postgis:16-3.4",
		tcpostgres.WithDatabase("sisaguna_test"), tcpostgres.WithUsername("test"), tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies())
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(ctr) })

	url, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	// Migrasi dijalankan lewat pool polos dulu; tipe enum baru ada setelahnya, baru pool aplikasi
	// (dengan RegisterTypes) dibuat.
	boot, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	ctxM, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, f := range migrationFiles(t) {
		sql, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := boot.Exec(ctxM, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", filepath.Base(f), err)
		}
	}
	boot.Close()

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AfterConnect = database.RegisterTypes
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func migrationFiles(t testing.TB) []string {
	_, here, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(here), "..", "..", "db", "migrations")
	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations found in %s: %v", dir, err)
	}
	sort.Strings(files)
	return files
}
