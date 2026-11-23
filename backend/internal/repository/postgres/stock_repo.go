package postgres

import (
	"context"
	"errors"

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
