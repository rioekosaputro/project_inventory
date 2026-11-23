package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type RackRepo struct{ pool *pgxpool.Pool }

func NewRackRepo(pool *pgxpool.Pool) *RackRepo { return &RackRepo{pool: pool} }

var _ domain.RackRepository = (*RackRepo)(nil)

const rackFrom = `racks r JOIN warehouses w ON w.id = r.warehouse_id`

const rackColumns = `r.id, r.warehouse_id, w.code, r.code, r.zone, r.is_sellable, r.description,
	r.is_active, r.locked_by_opname_id, r.created_at, r.updated_at`

func scanRack(row pgx.Row) (domain.Rack, error) {
	var r domain.Rack
	err := row.Scan(&r.ID, &r.WarehouseID, &r.WarehouseCode, &r.Code, &r.Zone, &r.IsSellable, &r.Description,
		&r.IsActive, &r.LockedByOpnameID, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func (r *RackRepo) find(ctx context.Context, id int64, suffix string) (*domain.Rack, error) {
	rack, err := scanRack(database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT `+rackColumns+` FROM `+rackFrom+` WHERE r.id = $1`+suffix, id))
	if err != nil {
		return nil, notFound(err)
	}
	return &rack, nil
}

func (r *RackRepo) FindByID(ctx context.Context, id int64) (*domain.Rack, error) {
	return r.find(ctx, id, "")
}

func (r *RackRepo) FindByIDForUpdate(ctx context.Context, id int64) (*domain.Rack, error) {
	database.MustInTx(ctx)
	return r.find(ctx, id, " FOR UPDATE OF r")
}

func (r *RackRepo) List(ctx context.Context, f domain.RackFilter, scopeWarehouseID *int64, p domain.PageParams) ([]domain.Rack, int64, error) {
	var b filterBuilder
	b.like(f.Query, "r.code", "r.zone")
	if f.WarehouseID != nil {
		b.add("r.warehouse_id = $%d", *f.WarehouseID)
	}
	if scopeWarehouseID != nil {
		b.add("r.warehouse_id = $%d", *scopeWarehouseID)
	}
	if f.Zone != "" {
		b.add("r.zone = $%d", f.Zone)
	}
	if f.IsSellable != nil {
		b.add("r.is_sellable = $%d", *f.IsSellable)
	}
	if f.IsActive != nil {
		b.add("r.is_active = $%d", *f.IsActive)
	}
	return queryPage(ctx, database.GetExecutor(ctx, r.pool), rackFrom, rackColumns, &b, p, "r.id",
		func(row pgx.Row) (domain.Rack, error) { return scanRack(row) })
}

func (r *RackRepo) Create(ctx context.Context, rack *domain.Rack) (*domain.Rack, error) {
	var id int64
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO racks (warehouse_id, code, zone, is_sellable, description, is_active)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		rack.WarehouseID, rack.Code, rack.Zone, rack.IsSellable, rack.Description, rack.IsActive).Scan(&id)
	if err != nil {
		return nil, duplicateOf(err, map[string]domain.DuplicateError{
			"racks_warehouse_id_code_key": {Field: "code", Message: "kode rak sudah dipakai di gudang ini"},
		})
	}
	return r.FindByID(ctx, id)
}

func (r *RackRepo) Update(ctx context.Context, rack *domain.Rack) error {
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE racks SET zone = $2, is_sellable = $3, description = $4, is_active = $5
		WHERE id = $1`, rack.ID, rack.Zone, rack.IsSellable, rack.Description, rack.IsActive))
}

func (r *RackRepo) HasStock(ctx context.Context, id int64) (bool, error) {
	var has bool
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM stock_balances WHERE rack_id = $1 AND qty_on_hand > 0)
		    OR EXISTS (SELECT 1 FROM product_serials WHERE current_rack_id = $1)`, id).Scan(&has)
	return has, err
}

func (r *RackRepo) LockForShare(ctx context.Context, ids []int64) ([]domain.Rack, error) {
	database.MustInTx(ctx)
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx,
		`SELECT `+rackColumns+` FROM `+rackFrom+` WHERE r.id = ANY($1) ORDER BY r.id FOR SHARE OF r`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Rack
	for rows.Next() {
		rack, err := scanRack(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rack)
	}
	return out, rows.Err()
}
