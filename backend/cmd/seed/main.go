// Command seed mengisi data awal (admin, app_settings, kategori). Aman dijalankan berulang.
//
//	go run ./cmd/seed
//
// Butuh DATABASE_URL, SEED_ADMIN_EMAIL, SEED_ADMIN_PASSWORD (lihat backend/.env.example).
// Admin dibuat dengan must_change_password = TRUE.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"rental-inventory-api/config"
	"rental-inventory-api/internal/seed"
	"rental-inventory-api/pkg/database"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Seed ditulis ke DATABASE_URL (koneksi runtime); transaksi biasa aman di semua pooler mode.
	pool, err := database.NewPool(ctx, database.Config{
		URL: cfg.DatabaseURL, PoolerMode: cfg.DBPoolerMode, MaxConns: 2,
	})
	if err != nil {
		return err
	}
	defer pool.Close()

	res, err := seed.Run(ctx, pool, seed.Options{
		AdminEmail:    cfg.SeedAdminEmail,
		AdminPassword: cfg.SeedAdminPassword,
		AppBaseURL:    cfg.AppBaseURL,
	})
	if err != nil {
		return err
	}

	if res.AdminCreated {
		fmt.Printf("admin dibuat: %s (wajib ganti password saat login pertama)\n", cfg.SeedAdminEmail)
	} else {
		fmt.Printf("admin sudah ada: %s (tidak diubah)\n", cfg.SeedAdminEmail)
	}
	fmt.Printf("app_settings baru: %d, kategori baru: %d\n", res.SettingsInserted, res.CategoriesInserted)
	return nil
}
