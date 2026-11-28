package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

// StockRepo adalah satu-satunya penulis saldo, unit baru, riwayat unit, dan ledger (lihat
// domain.StockRepository). Semua method wajib dipanggil di dalam transaksi usecase.
type StockRepo struct{ pool *pgxpool.Pool }

func NewStockRepo(pool *pgxpool.Pool) *StockRepo { return &StockRepo{pool: pool} }

var _ domain.StockRepository = (*StockRepo)(nil)

func (r *StockRepo) AddStock(ctx context.Context, productID, rackID int64, qty int) (int, int, error) {
	database.MustInTx(ctx)
	var after int
	// Upsert mengunci baris saldo sampai transaksi selesai (urutan lock dijaga pemanggil).
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO stock_balances (product_id, rack_id, qty_on_hand) VALUES ($1, $2, $3)
		ON CONFLICT (product_id, rack_id)
		DO UPDATE SET qty_on_hand = stock_balances.qty_on_hand + EXCLUDED.qty_on_hand, updated_at = NOW()
		RETURNING qty_on_hand`, productID, rackID, qty).Scan(&after)
	if err != nil {
		return 0, 0, err
	}
	return after - qty, after, nil
}

func (r *StockRepo) RemoveStock(ctx context.Context, productID, rackID int64, qty int) (int, int, error) {
	database.MustInTx(ctx)
	var after int
	// Satu UPDATE bersyarat: mengunci baris saldo dan menolak bila stok bebas (on hand − reserved) kurang.
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		UPDATE stock_balances SET qty_on_hand = qty_on_hand - $3, updated_at = NOW()
		WHERE product_id = $1 AND rack_id = $2 AND qty_on_hand - qty_reserved >= $3
		RETURNING qty_on_hand`, productID, rackID, qty).Scan(&after)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, domain.ErrInsufficientStock
		}
		return 0, 0, err
	}
	return after + qty, after, nil
}

func (r *StockRepo) ReserveStock(ctx context.Context, productID, rackID int64, qty int) error {
	database.MustInTx(ctx)
	tag, err := database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE stock_balances SET qty_reserved = qty_reserved + $3, updated_at = NOW()
		WHERE product_id = $1 AND rack_id = $2 AND qty_on_hand - qty_reserved >= $3`,
		productID, rackID, qty)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrInsufficientStock
	}
	return nil
}

func (r *StockRepo) ReleaseStock(ctx context.Context, productID, rackID int64, qty int) error {
	database.MustInTx(ctx)
	tag, err := database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE stock_balances SET qty_reserved = qty_reserved - $3, updated_at = NOW()
		WHERE product_id = $1 AND rack_id = $2 AND qty_reserved >= $3`,
		productID, rackID, qty)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrReservationMismatch
	}
	return nil
}

func (r *StockRepo) ConsumeReservedStock(ctx context.Context, productID, rackID int64, qty int) (int, int, error) {
	database.MustInTx(ctx)
	var after int
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		UPDATE stock_balances
		SET qty_on_hand = qty_on_hand - $3, qty_reserved = qty_reserved - $3, updated_at = NOW()
		WHERE product_id = $1 AND rack_id = $2 AND qty_on_hand >= $3 AND qty_reserved >= $3
		RETURNING qty_on_hand`, productID, rackID, qty).Scan(&after)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, domain.ErrReservationMismatch
		}
		return 0, 0, err
	}
	return after + qty, after, nil
}

func (r *StockRepo) LockSerials(ctx context.Context, ids []int64) ([]domain.SerialLock, error) {
	database.MustInTx(ctx)
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, `
		SELECT id, product_id, asset_tag, serial_number, status, current_rack_id, current_warehouse_id, reserved_for_do_id
		FROM product_serials WHERE id = ANY($1) ORDER BY id FOR UPDATE`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.SerialLock
	for rows.Next() {
		var s domain.SerialLock
		var status string
		if err := rows.Scan(&s.ID, &s.ProductID, &s.AssetTag, &s.Number, &status, &s.RackID, &s.WarehouseID, &s.ReservedForDOID); err != nil {
			return nil, err
		}
		s.Status = domain.SerialStatus(status)
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *StockRepo) MoveSerial(ctx context.Context, id int64, status domain.SerialStatus, warehouseID, rackID *int64) error {
	database.MustInTx(ctx)
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE product_serials SET status = $2, current_warehouse_id = $3, current_rack_id = $4, updated_at = NOW()
		WHERE id = $1`, id, string(status), warehouseID, rackID))
}

func (r *StockRepo) ReserveSerial(ctx context.Context, id, doID int64) error {
	database.MustInTx(ctx)
	tag, err := database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE product_serials SET status = 'RESERVED', reserved_for_do_id = $2, updated_at = NOW()
		WHERE id = $1 AND status = 'AVAILABLE'`, id, doID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrSerialNotValid
	}
	return nil
}

func (r *StockRepo) ReleaseSerial(ctx context.Context, id, doID int64) error {
	database.MustInTx(ctx)
	tag, err := database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE product_serials SET status = 'AVAILABLE', reserved_for_do_id = NULL, updated_at = NOW()
		WHERE id = $1 AND status = 'RESERVED' AND reserved_for_do_id = $2`, id, doID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrReservationMismatch
	}
	return nil
}

func (r *StockRepo) DispatchReservedSerial(ctx context.Context, id, doID int64, customerID, siteID *int64) error {
	database.MustInTx(ctx)
	tag, err := database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE product_serials
		SET status = 'IN_TRANSIT', current_warehouse_id = NULL, current_rack_id = NULL,
		    reserved_for_do_id = NULL, current_customer_id = $3, current_site_id = $4, updated_at = NOW()
		WHERE id = $1 AND status = 'RESERVED' AND reserved_for_do_id = $2`, id, doID, customerID, siteID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrReservationMismatch
	}
	return nil
}

func (r *StockRepo) ReturnSerialToRack(ctx context.Context, id int64, status domain.SerialStatus, condition domain.SerialCondition, warehouseID, rackID int64) error {
	database.MustInTx(ctx)
	tag, err := database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE product_serials
		SET status = $2, condition = $3, current_warehouse_id = $4, current_rack_id = $5,
		    current_customer_id = NULL, current_site_id = NULL, current_contract_id = NULL,
		    reserved_for_do_id = NULL, updated_at = NOW()
		WHERE id = $1`, id, string(status), string(condition), warehouseID, rackID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrSerialNotValid
	}
	return nil
}

func (r *StockRepo) UpdateSerialQC(ctx context.Context, id int64, status domain.SerialStatus, condition domain.SerialCondition) error {
	database.MustInTx(ctx)
	tag, err := database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE product_serials
		SET status = $2, condition = $3, updated_at = NOW()
		WHERE id = $1`, id, string(status), string(condition))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrSerialNotValid
	}
	return nil
}

func (r *StockRepo) InsertSerial(ctx context.Context, s *domain.NewSerial) (int64, error) {
	database.MustInTx(ctx)
	var id int64
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO product_serials (product_id, asset_tag, serial_number, status, condition,
		                             current_warehouse_id, current_rack_id, supplier_id, purchase_date, purchase_price)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::date, $10::text::numeric) RETURNING id`,
		s.ProductID, s.AssetTag, s.SerialNumber, string(s.Status), string(s.Condition),
		s.WarehouseID, s.RackID, s.SupplierID, s.PurchaseDate, moneyArg(s.PurchasePrice)).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "product_serials_product_id_serial_number_key" {
			return 0, domain.ErrSerialAlreadyExists
		}
		return 0, err
	}
	return id, nil
}

func (r *StockRepo) InsertSerialHistory(ctx context.Context, h domain.SerialHistoryEntry) error {
	database.MustInTx(ctx)
	_, err := database.GetExecutor(ctx, r.pool).Exec(ctx, `
		INSERT INTO product_serial_histories (product_serial_id, from_status, to_status, from_rack_id, to_rack_id,
		                                      event_type, reference_type, reference_id, user_id, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		h.SerialID, h.FromStatus, h.ToStatus, h.FromRackID, h.ToRackID,
		h.EventType, h.ReferenceType, h.ReferenceID, h.UserID, h.Notes)
	return err
}

func (r *StockRepo) InsertLedger(ctx context.Context, e domain.LedgerEntry) error {
	database.MustInTx(ctx)
	_, err := database.GetExecutor(ctx, r.pool).Exec(ctx, `
		INSERT INTO stock_ledgers (product_id, warehouse_id, rack_id, direction, qty, balance_before, balance_after,
		                           movement_type, lpb_id, lpb_item_id, serial_ids, unit_cost, created_by, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::text::numeric, $13, $14)`,
		e.ProductID, e.WarehouseID, e.RackID, e.Direction, e.Qty, e.Before, e.After,
		string(e.MovementType), e.LPBID, e.LPBItemID, e.SerialIDs, moneyArg(e.UnitCost), e.CreatedBy, e.Notes)
	return err
}
