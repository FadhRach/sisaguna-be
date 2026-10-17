// Command seed mengisi database. `-env prod`: admin + voucher umum awal.
// `-env dev`: admin + salinan data dummy app Android (cmd/seed/data, dihasilkan scripts/extract_android_seed.py).
// Idempoten: ID deterministik; menjalankan ulang menyegarkan waktu relatif (jendela pickup, umur order) dan stok.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5"

	"github.com/FadhRach/sisaguna-be/internal/config"
	"github.com/FadhRach/sisaguna-be/internal/platform/auth"
	"github.com/FadhRach/sisaguna-be/internal/platform/database"
)

func main() {
	env := flag.String("env", "", "dev | prod")
	flag.Parse()
	if err := run(*env); err != nil {
		slog.Error("seed gagal", "err", err)
		os.Exit(1)
	}
}

func run(env string) error {
	if env != "dev" && env != "prod" {
		return fmt.Errorf("flag -env wajib: dev atau prod")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if env == "dev" && cfg.IsProduction() && !cfg.AllowDemoSeed {
		return fmt.Errorf("seed dev ditolak saat APP_ENV=production (VPS uji: set ALLOW_DEMO_SEED=true, lihat deploy/demo.sh)")
	}
	if cfg.AdminEmail == "" || cfg.AdminPassword == "" {
		return fmt.Errorf("ADMIN_EMAIL dan ADMIN_PASSWORD wajib diisi")
	}

	ctx := context.Background()
	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	pw, err := auth.NewPasswords()
	if err != nil {
		return err
	}

	err = database.InTx(ctx, pool, func(tx pgx.Tx) error {
		if err := seedProd(ctx, tx, pw, cfg, env == "prod"); err != nil {
			return fmt.Errorf("seed prod: %w", err)
		}
		if env == "dev" {
			if err := seedDev(ctx, tx, pw, cfg); err != nil {
				return fmt.Errorf("seed dev: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	slog.Info("seed selesai", "env", env)
	if env == "dev" {
		slog.Info("akun dev", "password", devPassword, "contoh", "budi.santoso@gmail.com (pembeli + pemilik Dapur Budi), owner-m1@example.com (Warung Bu Sari), wati@example.com (toko pending), admin = ADMIN_EMAIL")
	}
	return nil
}

// seedProd: admin awal; voucher umum awal hanya untuk produksi (dev memakai voucher dari dummy Android).
func seedProd(ctx context.Context, tx pgx.Tx, pw *auth.Passwords, cfg *config.Config, vouchers bool) error {
	hash, err := pw.Hash(cfg.AdminPassword)
	if err != nil {
		return err
	}
	// ON CONFLICT DO NOTHING: password admin yang sudah ada tidak ditimpa.
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, name, email, phone, password_hash, is_admin)
		VALUES ($1, $2, $3, '081200000000', $4, true)
		ON CONFLICT (email) DO NOTHING`,
		sid("user:admin"), cfg.AdminName, cfg.AdminEmail, hash); err != nil {
		return fmt.Errorf("admin: %w", err)
	}
	if !vouchers {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO vouchers (code, title, description, percent_off, max_discount, min_spend, expires_at, terms, accent_hex)
		VALUES ('SISAGUNA10', 'Diskon 10%', 'Diskon 10% untuk semua makanan, maksimal Rp10.000', 10, 10000, 20000,
		        now() + interval '1 year', ARRAY['Berlaku untuk semua toko','Minimal belanja Rp20.000'], '#55B931')
		ON CONFLICT (code) DO NOTHING`); err != nil {
		return fmt.Errorf("voucher SISAGUNA10: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO vouchers (code, title, description, flat_off, min_spend, expires_at, terms, accent_hex)
		VALUES ('HEMAT5K', 'Potongan Rp5.000', 'Potongan Rp5.000 untuk belanja minimal Rp30.000', 5000, 30000,
		        now() + interval '1 year', ARRAY['Berlaku untuk semua toko'], '#F5A524')
		ON CONFLICT (code) DO NOTHING`); err != nil {
		return fmt.Errorf("voucher HEMAT5K: %w", err)
	}
	return nil
}
