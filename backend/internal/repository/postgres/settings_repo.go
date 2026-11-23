package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type SettingsRepo struct{ pool *pgxpool.Pool }

func NewSettingsRepo(pool *pgxpool.Pool) *SettingsRepo { return &SettingsRepo{pool: pool} }

var _ domain.SettingsRepository = (*SettingsRepo)(nil)

func (r *SettingsRepo) Bool(ctx context.Context, key string, def bool) (bool, error) {
	var raw []byte
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `SELECT value FROM app_settings WHERE key = $1`, key).Scan(&raw)
	if err != nil {
		if errors.Is(notFound(err), domain.ErrNotFound) {
			return def, nil
		}
		return false, err
	}
	var v bool
	if json.Unmarshal(raw, &v) != nil {
		return def, nil // bukan boolean: pakai bawaan (yang lebih ketat)
	}
	return v, nil
}
