package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

// WarehouseRepo baru memuat yang dibutuhkan T0.7; CRUD gudang menyusul di T1.2.
type WarehouseRepo struct{ pool *pgxpool.Pool }

func NewWarehouseRepo(pool *pgxpool.Pool) *WarehouseRepo { return &WarehouseRepo{pool: pool} }

var _ domain.WarehouseRepository = (*WarehouseRepo)(nil)

func (r *WarehouseRepo) IsActive(ctx context.Context, id int64) (bool, error) {
	var active bool
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `SELECT is_active FROM warehouses WHERE id = $1`, id).Scan(&active)
	if err != nil {
		return false, notFound(err)
	}
	return active, nil
}
