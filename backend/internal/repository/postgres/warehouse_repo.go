package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type WarehouseRepo struct{ pool *pgxpool.Pool }

func NewWarehouseRepo(pool *pgxpool.Pool) *WarehouseRepo { return &WarehouseRepo{pool: pool} }

var _ domain.WarehouseRepository = (*WarehouseRepo)(nil)

const warehouseColumns = `w.id, w.code, w.name, w.address, w.phone, w.latitude, w.longitude,
	w.is_active, w.created_at, w.updated_at`

func scanWarehouse(row pgx.Row) (domain.Warehouse, error) {
	var w domain.Warehouse
	err := row.Scan(&w.ID, &w.Code, &w.Name, &w.Address, &w.Phone, &w.Latitude, &w.Longitude,
		&w.IsActive, &w.CreatedAt, &w.UpdatedAt)
	return w, err
}

func (r *WarehouseRepo) IsActive(ctx context.Context, id int64) (bool, error) {
	var active bool
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `SELECT is_active FROM warehouses WHERE id = $1`, id).Scan(&active)
	if err != nil {
		return false, notFound(err)
	}
	return active, nil
}

func (r *WarehouseRepo) find(ctx context.Context, id int64, suffix string) (*domain.Warehouse, error) {
	w, err := scanWarehouse(database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT `+warehouseColumns+` FROM warehouses w WHERE w.id = $1`+suffix, id))
	if err != nil {
		return nil, notFound(err)
	}
	return &w, nil
}

func (r *WarehouseRepo) FindByID(ctx context.Context, id int64) (*domain.Warehouse, error) {
	return r.find(ctx, id, "")
}

func (r *WarehouseRepo) FindByIDForUpdate(ctx context.Context, id int64) (*domain.Warehouse, error) {
	database.MustInTx(ctx)
	return r.find(ctx, id, " FOR UPDATE")
}

func (r *WarehouseRepo) List(ctx context.Context, f domain.WarehouseFilter, p domain.PageParams) ([]domain.Warehouse, int64, error) {
	var b filterBuilder
	b.like(f.Query, "w.code", "w.name")
	if f.IsActive != nil {
		b.add("w.is_active = $%d", *f.IsActive)
	}
	if f.OnlyID != nil {
		b.add("w.id = $%d", *f.OnlyID)
	}
	return queryPage(ctx, database.GetExecutor(ctx, r.pool), "warehouses w", warehouseColumns, &b, p, "w.id",
		func(row pgx.Row) (domain.Warehouse, error) { return scanWarehouse(row) })
}

func (r *WarehouseRepo) Create(ctx context.Context, w *domain.Warehouse) (*domain.Warehouse, error) {
	created, err := scanWarehouse(database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO warehouses AS w (code, name, address, phone, latitude, longitude, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+warehouseColumns,
		w.Code, w.Name, w.Address, w.Phone, w.Latitude, w.Longitude, w.IsActive))
	if err != nil {
		return nil, duplicateOf(err, map[string]domain.DuplicateError{
			"warehouses_code_key": {Field: "code", Message: "kode gudang sudah dipakai"},
		})
	}
	return &created, nil
}

func (r *WarehouseRepo) Update(ctx context.Context, w *domain.Warehouse) error {
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE warehouses SET name = $2, address = $3, phone = $4, latitude = $5, longitude = $6, is_active = $7
		WHERE id = $1`, w.ID, w.Name, w.Address, w.Phone, w.Latitude, w.Longitude, w.IsActive))
}

func (r *WarehouseRepo) HasStock(ctx context.Context, id int64) (bool, error) {
	var has bool
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM stock_balances sb JOIN racks r ON r.id = sb.rack_id
			WHERE r.warehouse_id = $1 AND sb.qty_on_hand > 0
		) OR EXISTS (
			SELECT 1 FROM product_serials ps JOIN racks r ON r.id = ps.current_rack_id
			WHERE r.warehouse_id = $1
		)`, id).Scan(&has)
	return has, err
}
