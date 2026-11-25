package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type OpnameRepo struct{ pool *pgxpool.Pool }

func NewOpnameRepo(pool *pgxpool.Pool) *OpnameRepo { return &OpnameRepo{pool: pool} }

var _ domain.OpnameRepository = (*OpnameRepo)(nil)

const opnameFrom = `stock_opnames o
	JOIN warehouses w ON w.id = o.warehouse_id
	JOIN users su ON su.id = o.started_by
	LEFT JOIN lpb_documents li ON li.id = o.adjustment_in_lpb_id
	LEFT JOIN lpb_documents lo ON lo.id = o.adjustment_out_lpb_id`

const opnameColumns = `o.id, o.opname_number, o.warehouse_id, w.code, w.name, o.status, o.is_blind_count,
	o.started_by, su.name, o.started_at, o.submitted_by, o.submitted_at, o.finalized_by, o.finalized_at,
	o.adjustment_in_lpb_id, li.lpb_number, o.adjustment_out_lpb_id, lo.lpb_number, o.notes`

func scanOpnameInto(row pgx.Row, extra ...any) (domain.Opname, error) {
	var (
		o      domain.Opname
		status string
	)
	dest := append([]any{
		&o.ID, &o.Number, &o.WarehouseID, &o.WarehouseCode, &o.WarehouseName, &status, &o.IsBlindCount,
		&o.StartedBy, &o.StartedByName, &o.StartedAt, &o.SubmittedBy, &o.SubmittedAt, &o.FinalizedBy, &o.FinalizedAt,
		&o.AdjustmentInLPBID, &o.AdjustmentInLPBNumber, &o.AdjustmentOutLPBID, &o.AdjustmentOutLPBNumber, &o.Notes,
	}, extra...)
	err := row.Scan(dest...)
	o.Status = domain.OpnameStatus(status)
	return o, err
}

func (r *OpnameRepo) find(ctx context.Context, id int64, suffix string) (*domain.Opname, error) {
	o, err := scanOpnameInto(database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT `+opnameColumns+` FROM `+opnameFrom+` WHERE o.id = $1`+suffix, id))
	if err != nil {
		return nil, notFound(err)
	}
	return &o, nil
}

func (r *OpnameRepo) FindByID(ctx context.Context, id int64) (*domain.Opname, error) {
	return r.find(ctx, id, "")
}

func (r *OpnameRepo) FindByIDForUpdate(ctx context.Context, id int64) (*domain.Opname, error) {
	database.MustInTx(ctx)
	return r.find(ctx, id, " FOR UPDATE OF o")
}

func (r *OpnameRepo) Create(ctx context.Context, o *domain.Opname) (*domain.Opname, error) {
	database.MustInTx(ctx)
	var id int64
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO stock_opnames (opname_number, warehouse_id, status, is_blind_count, started_by, started_at, notes)
		VALUES ($1, $2, 'IN_PROGRESS', $3, $4, $5, $6) RETURNING id`,
		o.Number, o.WarehouseID, o.IsBlindCount, o.StartedBy, o.StartedAt, o.Notes).Scan(&id)
	if err != nil {
		return nil, err
	}
	return r.FindByID(ctx, id)
}

func (r *OpnameRepo) List(ctx context.Context, f domain.OpnameFilter, p domain.PageParams) ([]domain.OpnameListItem, int64, error) {
	var b filterBuilder
	b.like(f.Query, "o.opname_number", "w.code", "w.name")
	if f.Status != nil {
		b.add("o.status = $%d", string(*f.Status))
	}
	if f.WarehouseID != nil {
		b.add("o.warehouse_id = $%d", *f.WarehouseID)
	}
	if f.ScopeWarehouseID != nil {
		b.add("o.warehouse_id = $%d", *f.ScopeWarehouseID)
	}
	cols := opnameColumns + `,
		(SELECT count(*) FROM stock_opname_racks WHERE opname_id = o.id)::int,
		(SELECT count(*) FROM stock_opname_items WHERE opname_id = o.id)::int,
		(SELECT count(*) FROM stock_opname_items WHERE opname_id = o.id AND counted_qty IS NOT NULL)::int,
		(SELECT count(*) FROM stock_opname_serials WHERE opname_id = o.id)::int,
		(SELECT count(*) FROM stock_opname_serials WHERE opname_id = o.id AND result = 'FOUND')::int`
	return queryPage(ctx, database.GetExecutor(ctx, r.pool), opnameFrom, cols, &b, p, "o.id",
		func(row pgx.Row) (domain.OpnameListItem, error) {
			var it domain.OpnameListItem
			var err error
			it.Opname, err = scanOpnameInto(row, &it.RackCount, &it.ItemCount, &it.CountedCount, &it.SerialCount, &it.FoundCount)
			return it, err
		})
}

func (r *OpnameRepo) AddRacks(ctx context.Context, opnameID int64, rackIDs []int64) error {
	database.MustInTx(ctx)
	db := database.GetExecutor(ctx, r.pool)
	for _, rid := range rackIDs {
		if _, err := db.Exec(ctx, `INSERT INTO stock_opname_racks (opname_id, rack_id) VALUES ($1, $2)`, opnameID, rid); err != nil {
			return err
		}
	}
	return nil
}

func (r *OpnameRepo) Racks(ctx context.Context, opnameID int64) ([]domain.OpnameRack, error) {
	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, `
		SELECT r.id, r.code, r.zone, r.is_sellable
		FROM stock_opname_racks sor
		JOIN racks r ON r.id = sor.rack_id
		WHERE sor.opname_id = $1
		ORDER BY r.code`, opnameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.OpnameRack, 0)
	for rows.Next() {
		var rk domain.OpnameRack
		if err := rows.Scan(&rk.RackID, &rk.RackCode, &rk.Zone, &rk.IsSellable); err != nil {
			return nil, err
		}
		out = append(out, rk)
	}
	return out, rows.Err()
}

func (r *OpnameRepo) LockRacksForOpname(ctx context.Context, opnameID int64, rackIDs []int64) error {
	database.MustInTx(ctx)
	_, err := database.GetExecutor(ctx, r.pool).Exec(ctx,
		`UPDATE racks SET locked_by_opname_id = $1 WHERE id = ANY($2)`, opnameID, rackIDs)
	return err
}

func (r *OpnameRepo) UnlockRacksForOpname(ctx context.Context, opnameID int64) error {
	database.MustInTx(ctx)
	_, err := database.GetExecutor(ctx, r.pool).Exec(ctx,
		`UPDATE racks SET locked_by_opname_id = NULL WHERE locked_by_opname_id = $1`, opnameID)
	return err
}

func (r *OpnameRepo) SnapshotNonSerialBalances(ctx context.Context, opnameID int64, rackIDs []int64) error {
	database.MustInTx(ctx)
	_, err := database.GetExecutor(ctx, r.pool).Exec(ctx, `
		INSERT INTO stock_opname_items (opname_id, product_id, rack_id, system_qty)
		SELECT $1, sb.product_id, sb.rack_id, sb.qty_on_hand
		FROM stock_balances sb
		JOIN products p ON p.id = sb.product_id
		WHERE sb.rack_id = ANY($2) AND p.has_serial_number = FALSE AND sb.qty_on_hand > 0`, opnameID, rackIDs)
	return err
}

func (r *OpnameRepo) SnapshotSerialUnits(ctx context.Context, opnameID int64, rackIDs []int64) error {
	database.MustInTx(ctx)
	_, err := database.GetExecutor(ctx, r.pool).Exec(ctx, `
		INSERT INTO stock_opname_serials (opname_id, rack_id, product_serial_id, result)
		SELECT $1, ps.current_rack_id, ps.id, 'MISSING'
		FROM product_serials ps
		WHERE ps.current_rack_id = ANY($2)
		  AND ps.status IN ('AVAILABLE','RESERVED','MAINTENANCE','BROKEN')`, opnameID, rackIDs)
	return err
}

func (r *OpnameRepo) Items(ctx context.Context, opnameID int64) ([]domain.OpnameItem, error) {
	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, `
		SELECT i.id, i.opname_id, i.product_id, p.sku, p.name, p.unit, i.rack_id, rk.code,
		       i.system_qty, i.counted_qty, i.diff_qty, i.counted_by, i.counted_at, i.notes
		FROM stock_opname_items i
		JOIN products p ON p.id = i.product_id
		JOIN racks rk ON rk.id = i.rack_id
		WHERE i.opname_id = $1
		ORDER BY rk.code, p.sku`, opnameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.OpnameItem, 0)
	for rows.Next() {
		var it domain.OpnameItem
		if err := rows.Scan(&it.ID, &it.OpnameID, &it.ProductID, &it.SKU, &it.ProductName, &it.Unit,
			&it.RackID, &it.RackCode, &it.SystemQty, &it.CountedQty, &it.DiffQty, &it.CountedBy, &it.CountedAt, &it.Notes); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *OpnameRepo) UpsertItemCount(ctx context.Context, opnameID, productID, rackID int64, countedQty int, userID int64, at time.Time, notes *string) error {
	database.MustInTx(ctx)
	_, err := database.GetExecutor(ctx, r.pool).Exec(ctx, `
		INSERT INTO stock_opname_items (opname_id, product_id, rack_id, system_qty, counted_qty, counted_by, counted_at, notes)
		VALUES ($1, $2, $3, 0, $4, $5, $6, $7)
		ON CONFLICT (opname_id, product_id, rack_id)
		DO UPDATE SET counted_qty = EXCLUDED.counted_qty, counted_by = EXCLUDED.counted_by,
		              counted_at = EXCLUDED.counted_at, notes = EXCLUDED.notes`,
		opnameID, productID, rackID, countedQty, userID, at, notes)
	return err
}

func (r *OpnameRepo) Serials(ctx context.Context, opnameID int64) ([]domain.OpnameSerial, error) {
	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, `
		SELECT s.id, s.opname_id, s.rack_id, rk.code, s.product_serial_id,
		       ps.asset_tag, ps.serial_number, p.sku, p.name,
		       s.scanned_code, s.result, s.scanned_by, s.scanned_at
		FROM stock_opname_serials s
		JOIN racks rk ON rk.id = s.rack_id
		LEFT JOIN product_serials ps ON ps.id = s.product_serial_id
		LEFT JOIN products p ON p.id = ps.product_id
		WHERE s.opname_id = $1
		ORDER BY rk.code, s.result, COALESCE(ps.asset_tag, s.scanned_code, '')`, opnameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.OpnameSerial, 0)
	for rows.Next() {
		var s domain.OpnameSerial
		var res string
		if err := rows.Scan(&s.ID, &s.OpnameID, &s.RackID, &s.RackCode, &s.ProductSerialID,
			&s.AssetTag, &s.SerialNumber, &s.SKU, &s.ProductName,
			&s.ScannedCode, &res, &s.ScannedBy, &s.ScannedAt); err != nil {
			return nil, err
		}
		s.Result = domain.OpnameSerialResult(res)
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *OpnameRepo) UpsertSerialScan(ctx context.Context, opnameID, rackID int64, productSerialID *int64, scannedCode string, result domain.OpnameSerialResult, userID int64, at time.Time) (*domain.OpnameSerial, error) {
	database.MustInTx(ctx)
	db := database.GetExecutor(ctx, r.pool)
	var id int64
	var err error
	if productSerialID != nil {
		err = db.QueryRow(ctx, `
			INSERT INTO stock_opname_serials (opname_id, rack_id, product_serial_id, scanned_code, result, scanned_by, scanned_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (opname_id, product_serial_id)
			DO UPDATE SET rack_id = EXCLUDED.rack_id, scanned_code = EXCLUDED.scanned_code,
			              result = EXCLUDED.result, scanned_by = EXCLUDED.scanned_by, scanned_at = EXCLUDED.scanned_at
			RETURNING id`, opnameID, rackID, *productSerialID, scannedCode, string(result), userID, at).Scan(&id)
	} else {
		// Kode tak dikenal (product_serial_id NULL): hindari duplikat kode yang sama pada rak yang sama.
		err = db.QueryRow(ctx, `
			SELECT id FROM stock_opname_serials
			WHERE opname_id = $1 AND rack_id = $2 AND product_serial_id IS NULL AND upper(scanned_code) = upper($3)
			LIMIT 1`, opnameID, rackID, scannedCode).Scan(&id)
		if err == pgx.ErrNoRows {
			err = db.QueryRow(ctx, `
				INSERT INTO stock_opname_serials (opname_id, rack_id, product_serial_id, scanned_code, result, scanned_by, scanned_at)
				VALUES ($1, $2, NULL, $3, $4, $5, $6) RETURNING id`,
				opnameID, rackID, scannedCode, string(result), userID, at).Scan(&id)
		}
	}
	if err != nil {
		return nil, err
	}

	var out domain.OpnameSerial
	var res string
	err = db.QueryRow(ctx, `
		SELECT s.id, s.opname_id, s.rack_id, rk.code, s.product_serial_id,
		       ps.asset_tag, ps.serial_number, p.sku, p.name,
		       s.scanned_code, s.result, s.scanned_by, s.scanned_at
		FROM stock_opname_serials s
		JOIN racks rk ON rk.id = s.rack_id
		LEFT JOIN product_serials ps ON ps.id = s.product_serial_id
		LEFT JOIN products p ON p.id = ps.product_id
		WHERE s.id = $1`, id).Scan(
		&out.ID, &out.OpnameID, &out.RackID, &out.RackCode, &out.ProductSerialID,
		&out.AssetTag, &out.SerialNumber, &out.SKU, &out.ProductName,
		&out.ScannedCode, &res, &out.ScannedBy, &out.ScannedAt)
	if err != nil {
		return nil, err
	}
	out.Result = domain.OpnameSerialResult(res)
	return &out, nil
}

func (r *OpnameRepo) MarkSubmitted(ctx context.Context, id, userID int64, at time.Time) error {
	database.MustInTx(ctx)
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx,
		`UPDATE stock_opnames SET status = 'SUBMITTED', submitted_by = $2, submitted_at = $3 WHERE id = $1`,
		id, userID, at))
}

func (r *OpnameRepo) MarkFinalized(ctx context.Context, id, userID int64, at time.Time, adjInLPBID, adjOutLPBID *int64) error {
	database.MustInTx(ctx)
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE stock_opnames
		SET status = 'FINALIZED', finalized_by = $2, finalized_at = $3,
		    adjustment_in_lpb_id = $4, adjustment_out_lpb_id = $5
		WHERE id = $1`, id, userID, at, adjInLPBID, adjOutLPBID))
}

func (r *OpnameRepo) MarkCancelled(ctx context.Context, id int64) error {
	database.MustInTx(ctx)
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx,
		`UPDATE stock_opnames SET status = 'CANCELLED' WHERE id = $1`, id))
}
