// Package seed mengisi data awal yang wajib ada (admin, app_settings, kategori).
// Idempotent: aman dijalankan berulang, data yang sudah ada tidak ditimpa.
package seed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/pkg/database"
	"rental-inventory-api/pkg/password"
)

// Options adalah masukan seed.
type Options struct {
	AdminEmail    string
	AdminPassword string
	AppBaseURL    string // dasar URL verifikasi QR: {AppBaseURL}/verify/lpb
}

// Result melaporkan jumlah baris BARU yang dibuat pada run ini.
type Result struct {
	AdminCreated       bool
	SettingsInserted   int
	CategoriesInserted int
}

type category struct{ name, itemType string }

var defaultCategories = []category{
	{"Printer", "ASSET"}, {"PC", "ASSET"}, {"Monitor", "ASSET"},
	{"Toner", "CONSUMABLE"}, {"Tinta", "CONSUMABLE"}, {"Kertas", "CONSUMABLE"}, {"Sparepart", "CONSUMABLE"},
}

// Run menjalankan seed dalam satu transaksi.
func Run(ctx context.Context, pool *pgxpool.Pool, opts Options) (Result, error) {
	email := strings.ToLower(strings.TrimSpace(opts.AdminEmail))
	if !strings.Contains(email, "@") {
		return Result{}, errors.New("SEED_ADMIN_EMAIL belum diisi atau tidak valid")
	}
	if opts.AdminPassword == "" {
		return Result{}, errors.New("SEED_ADMIN_PASSWORD belum diisi")
	}
	hash, err := password.Hash(opts.AdminPassword)
	if err != nil {
		return Result{}, fmt.Errorf("SEED_ADMIN_PASSWORD tidak valid: %w", err)
	}
	settings, err := defaultSettings(email, opts.AppBaseURL)
	if err != nil {
		return Result{}, err
	}

	var res Result
	tm := database.NewTxManager(pool)
	err = tm.WithinTransaction(ctx, func(ctx context.Context) error {
		db := database.GetExecutor(ctx, pool)

		tag, err := db.Exec(ctx, `
			INSERT INTO users (name, email, password_hash, role, must_change_password)
			VALUES ('Administrator', $1, $2, 'ADMIN', TRUE)
			ON CONFLICT (email) DO NOTHING`, email, hash)
		if err != nil {
			return fmt.Errorf("seed admin: %w", err)
		}
		res.AdminCreated = tag.RowsAffected() == 1

		for _, s := range settings {
			tag, err := db.Exec(ctx, `
				INSERT INTO app_settings (key, value) VALUES ($1, $2::jsonb)
				ON CONFLICT (key) DO NOTHING`, s.key, s.value)
			if err != nil {
				return fmt.Errorf("seed setting %s: %w", s.key, err)
			}
			res.SettingsInserted += int(tag.RowsAffected())
		}

		for _, c := range defaultCategories {
			tag, err := db.Exec(ctx, `
				INSERT INTO categories (name, item_type) VALUES ($1, $2)
				ON CONFLICT (name) DO NOTHING`, c.name, c.itemType)
			if err != nil {
				return fmt.Errorf("seed kategori %s: %w", c.name, err)
			}
			res.CategoriesInserted += int(tag.RowsAffected())
		}
		return nil
	})
	return res, err
}

type setting struct{ key, value string }

func defaultSettings(adminEmail, appBaseURL string) ([]setting, error) {
	profile, err := json.Marshal(map[string]string{
		"name": "PT Contoh Rental", "address": "Semarang", "phone": "024-xxx", "email": adminEmail, "npwp": "",
	})
	if err != nil {
		return nil, err
	}
	verifyURL, err := json.Marshal(strings.TrimRight(appBaseURL, "/") + "/verify/lpb")
	if err != nil {
		return nil, err
	}
	return []setting{
		{"company.profile", string(profile)},
		{"lpb.allow_self_approve", "false"},
		{"billing.run_day_offset", "0"},
		{"billing.rounding", `"HALF_UP"`},
		{"pdf.verify_base_url", string(verifyURL)},
	}, nil
}
