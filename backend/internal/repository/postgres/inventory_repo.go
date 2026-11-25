package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

// InventoryRepo: pembacaan saldo stok dan unit SN. Tidak menulis apa pun.
type InventoryRepo struct{ pool *pgxpool.Pool }

func NewInventoryRepo(pool *pgxpool.Pool) *InventoryRepo { return &InventoryRepo{pool: pool} }

var _ domain.InventoryRepository = (*InventoryRepo)(nil)

// ---------------------------------------------------------------- balances

const balanceColumns = `sb.product_id, p.sku, p.name, p.unit, p.has_serial_number, p.min_stock_alert,
	sb.rack_id, r.code, r.zone, r.is_sellable, r.warehouse_id, w.code,
	sb.qty_on_hand, sb.qty_reserved,
	(CASE WHEN r.is_sellable THEN sb.qty_on_hand - sb.qty_reserved ELSE 0 END) AS qty_available,
	(p.min_stock_alert > 0 AND tot.available < p.min_stock_alert),
	sb.updated_at`

func scanBalance(row pgx.Row) (domain.StockBalance, error) {
	var s domain.StockBalance
	err := row.Scan(&s.ProductID, &s.SKU, &s.ProductName, &s.Unit, &s.HasSerialNumber, &s.MinStockAlert,
		&s.RackID, &s.RackCode, &s.Zone, &s.IsSellable, &s.WarehouseID, &s.WarehouseCode,
		&s.QtyOnHand, &s.QtyReserved, &s.QtyAvailable, &s.IsLowStock, &s.UpdatedAt)
	return s, err
}

func (r *InventoryRepo) ListBalances(ctx context.Context, f domain.BalanceFilter, p domain.PageParams) ([]domain.StockBalance, int64, error) {
	var b filterBuilder
	scope := b.warehouseScope(f.ScopeWarehouseID, f.WarehouseID)

	// Total tersedia per PRODUK pada cakupan gudang (bukan hanya rak yang sedang ditampilkan):
	// min_stock_alert melekat pada produk, jadi menipis/tidak dinilai dari totalnya.
	from := `stock_balances sb
		JOIN products p ON p.id = sb.product_id
		JOIN racks r ON r.id = sb.rack_id
		JOIN warehouses w ON w.id = r.warehouse_id
		JOIN (
			SELECT sb2.product_id,
			       SUM(CASE WHEN r2.is_sellable THEN sb2.qty_on_hand - sb2.qty_reserved ELSE 0 END) AS available
			FROM stock_balances sb2 JOIN racks r2 ON r2.id = sb2.rack_id
			WHERE ` + scope("r2.warehouse_id") + `
			GROUP BY sb2.product_id
		) tot ON tot.product_id = sb.product_id`

	if f.ScopeWarehouseID != nil || f.WarehouseID != nil {
		b.raw(scope("r.warehouse_id"))
	}
	b.like(f.Query, "p.sku", "p.name")
	if f.ProductID != nil {
		b.add("sb.product_id = $%d", *f.ProductID)
	}
	if f.RackID != nil {
		b.add("sb.rack_id = $%d", *f.RackID)
	}
	if !f.IncludeZero {
		b.raw("sb.qty_on_hand > 0")
	}
	if f.LowStock {
		b.raw("(p.min_stock_alert > 0 AND tot.available < p.min_stock_alert)")
	}

	return queryPage(ctx, database.GetExecutor(ctx, r.pool), from, balanceColumns, &b, p, "sb.id",
		func(row pgx.Row) (domain.StockBalance, error) { return scanBalance(row) })
}

// ---------------------------------------------------------------- ringkasan per produk

const stockColumns = `t.product_id, t.sku, t.name, t.unit, t.category_id, t.category_name, t.item_type,
	t.has_serial_number, t.min_stock_alert, t.is_active,
	t.qty_on_hand, t.qty_reserved, t.qty_available, t.rack_count,
	(t.min_stock_alert > 0 AND t.qty_available < t.min_stock_alert)`

func scanProductStock(row pgx.Row) (domain.ProductStock, error) {
	var s domain.ProductStock
	var itemType string
	err := row.Scan(&s.ProductID, &s.SKU, &s.ProductName, &s.Unit, &s.CategoryID, &s.CategoryName, &itemType,
		&s.HasSerialNumber, &s.MinStockAlert, &s.IsActive,
		&s.QtyOnHand, &s.QtyReserved, &s.QtyAvailable, &s.RackCount, &s.IsLowStock)
	s.ItemType = domain.ItemType(itemType)
	return s, err
}

func (r *InventoryRepo) ListProductStock(ctx context.Context, f domain.StockSummaryFilter, p domain.PageParams) ([]domain.ProductStock, int64, error) {
	var b filterBuilder
	scope := b.warehouseScope(f.ScopeWarehouseID, f.WarehouseID)

	// Semua produk ikut (LEFT JOIN) agar produk yang stoknya 0 tetap tampil dan dinilai menipis.
	from := `(
		SELECT p.id AS product_id, p.sku, p.name, p.unit, p.category_id, c.name AS category_name, c.item_type,
		       p.has_serial_number, p.min_stock_alert, p.is_active,
		       COALESCE(SUM(s.qty_on_hand), 0)::int AS qty_on_hand,
		       COALESCE(SUM(s.qty_reserved), 0)::int AS qty_reserved,
		       COALESCE(SUM(CASE WHEN s.is_sellable THEN s.qty_on_hand - s.qty_reserved ELSE 0 END), 0)::int AS qty_available,
		       (COUNT(s.rack_id) FILTER (WHERE s.qty_on_hand > 0))::int AS rack_count
		FROM products p
		JOIN categories c ON c.id = p.category_id
		LEFT JOIN (
			SELECT sb.product_id, sb.rack_id, sb.qty_on_hand, sb.qty_reserved, r.is_sellable
			FROM stock_balances sb JOIN racks r ON r.id = sb.rack_id
			WHERE ` + scope("r.warehouse_id") + `
		) s ON s.product_id = p.id
		GROUP BY p.id, c.id
	) t`

	b.like(f.Query, "t.sku", "t.name")
	if f.CategoryID != nil {
		b.add("t.category_id = $%d", *f.CategoryID)
	}
	if f.ItemType != nil {
		b.add("t.item_type = $%d", string(*f.ItemType))
	}
	if f.LowStock {
		b.raw("(t.min_stock_alert > 0 AND t.qty_available < t.min_stock_alert)")
	}
	if f.HasStock {
		b.raw("t.qty_on_hand > 0")
	}
	if f.IsActive != nil {
		b.add("t.is_active = $%d", *f.IsActive)
	} else {
		// Produk nonaktif yang masih menyimpan stok tetap terlihat.
		b.raw("(t.is_active OR t.qty_on_hand > 0)")
	}

	return queryPage(ctx, database.GetExecutor(ctx, r.pool), from, stockColumns, &b, p, "t.product_id",
		func(row pgx.Row) (domain.ProductStock, error) { return scanProductStock(row) })
}

// ---------------------------------------------------------------- unit SN

const serialFrom = `product_serials ps
	JOIN products p ON p.id = ps.product_id
	LEFT JOIN racks r ON r.id = ps.current_rack_id
	LEFT JOIN warehouses w ON w.id = r.warehouse_id`

const serialColumns = `ps.id, ps.product_id, p.sku, p.name, ps.asset_tag, ps.serial_number, ps.status, ps.condition,
	ps.current_rack_id, r.code, r.warehouse_id, w.code,
	ps.current_customer_id, ps.current_site_id, ps.current_contract_id, ps.reserved_for_do_id,
	ps.warranty_until::text, ps.notes, ps.updated_at`

func scanSerial(row pgx.Row) (domain.Serial, error) {
	var s domain.Serial
	var status string
	err := row.Scan(&s.ID, &s.ProductID, &s.SKU, &s.ProductName, &s.AssetTag, &s.SerialNumber, &status, &s.Condition,
		&s.RackID, &s.RackCode, &s.WarehouseID, &s.WarehouseCode,
		&s.CustomerID, &s.SiteID, &s.ContractID, &s.ReservedForDOID,
		&s.WarrantyUntil, &s.Notes, &s.UpdatedAt)
	s.Status = domain.SerialStatus(status)
	return s, err
}

func (r *InventoryRepo) ListSerials(ctx context.Context, f domain.SerialFilter, p domain.PageParams) ([]domain.Serial, int64, error) {
	var b filterBuilder
	b.like(f.Query, "ps.asset_tag", "ps.serial_number", "p.sku", "p.name")
	if f.ProductID != nil {
		b.add("ps.product_id = $%d", *f.ProductID)
	}
	if f.Status != nil {
		b.add("ps.status = $%d", string(*f.Status))
	}
	if f.CustomerID != nil {
		b.add("ps.current_customer_id = $%d", *f.CustomerID)
	}
	if f.WarehouseID != nil {
		b.add("r.warehouse_id = $%d", *f.WarehouseID)
	}
	if f.RackID != nil {
		b.add("ps.current_rack_id = $%d", *f.RackID)
	}
	if f.ScopeWarehouseID != nil {
		// Akun satu gudang: unit di gudangnya + unit yang tidak berada di gudang mana pun (disewa, dalam perjalanan, hilang, pensiun).
		b.add("(r.warehouse_id = $%d OR ps.current_rack_id IS NULL)", *f.ScopeWarehouseID)
	}
	return queryPage(ctx, database.GetExecutor(ctx, r.pool), serialFrom, serialColumns, &b, p, "ps.id",
		func(row pgx.Row) (domain.Serial, error) { return scanSerial(row) })
}

func (r *InventoryRepo) FindSerialsByCode(ctx context.Context, code string) ([]domain.Serial, error) {
	db := database.GetExecutor(ctx, r.pool)
	// Berurutan dari yang paling spesifik: asset tag (unik) â†’ SN persis â†’ SN tanpa peduli huruf.
	for _, cond := range []string{
		`ps.asset_tag = upper($1)`,
		`ps.serial_number = $1`,
		`upper(ps.serial_number) = upper($1)`,
	} {
		rows, err := db.Query(ctx, `SELECT `+serialColumns+` FROM `+serialFrom+` WHERE `+cond+` ORDER BY ps.id LIMIT 10`, code)
		if err != nil {
			return nil, err
		}
		var found []domain.Serial
		for rows.Next() {
			s, err := scanSerial(rows)
			if err != nil {
				rows.Close()
				return nil, err
			}
			found = append(found, s)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if len(found) > 0 {
			return found, nil
		}
	}
	return nil, nil
}

func (r *InventoryRepo) FindSerialsByIDs(ctx context.Context, ids []int64) ([]domain.Serial, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx,
		`SELECT `+serialColumns+` FROM `+serialFrom+` WHERE ps.id = ANY($1) ORDER BY ps.asset_tag`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Serial, 0, len(ids))
	for rows.Next() {
		s, err := scanSerial(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *InventoryRepo) RecentSerialHistory(ctx context.Context, serialID int64, limit int) ([]domain.SerialHistory, error) {
	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, `
		SELECT h.id, h.from_status, h.to_status, fr.code, tr.code, h.customer_id, h.event_type,
		       h.reference_type, h.reference_id, h.user_id, u.name, h.notes, h.created_at
		FROM product_serial_histories h
		LEFT JOIN racks fr ON fr.id = h.from_rack_id
		LEFT JOIN racks tr ON tr.id = h.to_rack_id
		LEFT JOIN users u ON u.id = h.user_id
		WHERE h.product_serial_id = $1
		ORDER BY h.created_at DESC, h.id DESC
		LIMIT $2`, serialID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.SerialHistory, 0, limit)
	for rows.Next() {
		var h domain.SerialHistory
		if err := rows.Scan(&h.ID, &h.FromStatus, &h.ToStatus, &h.FromRackCode, &h.ToRackCode, &h.CustomerID,
			&h.EventType, &h.ReferenceType, &h.ReferenceID, &h.UserID, &h.UserName, &h.Notes, &h.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (r *InventoryRepo) FindSerialByID(ctx context.Context, id int64) (*domain.Serial, error) {
	s, err := scanSerial(database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT `+serialColumns+` FROM `+serialFrom+` WHERE ps.id = $1`, id))
	if err != nil {
		return nil, notFound(err)
	}
	return &s, nil
}

const ledgerFrom = `stock_ledgers l
	JOIN products p ON p.id = l.product_id
	JOIN warehouses w ON w.id = l.warehouse_id
	JOIN racks r ON r.id = l.rack_id
	JOIN lpb_documents d ON d.id = l.lpb_id
	JOIN users u ON u.id = l.created_by`

const ledgerColumns = `l.id, l.occurred_at, l.product_id, p.sku, p.name, p.unit,
	l.warehouse_id, w.code, l.rack_id, r.code, l.direction, l.qty,
	l.balance_before, l.balance_after, l.movement_type, l.lpb_id, d.lpb_number,
	COALESCE(array_length(l.serial_ids, 1), 0)::int, l.created_by, u.name, l.notes`

func scanLedgerRow(row pgx.Row) (domain.LedgerRow, error) {
	var (
		lr  domain.LedgerRow
		mov string
	)
	err := row.Scan(
		&lr.ID, &lr.OccurredAt, &lr.ProductID, &lr.SKU, &lr.ProductName, &lr.Unit,
		&lr.WarehouseID, &lr.WarehouseCode, &lr.RackID, &lr.RackCode, &lr.Direction, &lr.Qty,
		&lr.BalanceBefore, &lr.BalanceAfter, &mov, &lr.LPBID, &lr.LPBNumber,
		&lr.SerialCount, &lr.CreatedBy, &lr.CreatedByName, &lr.Notes,
	)
	lr.MovementType = domain.LPBSubType(mov)
	return lr, err
}

func (r *InventoryRepo) ListLedgers(ctx context.Context, f domain.LedgerFilter, p domain.PageParams) ([]domain.LedgerRow, int64, error) {
	var b filterBuilder
	b.like(f.Query, "p.sku", "p.name", "d.lpb_number")
	if f.ProductID != nil {
		b.add("l.product_id = $%d", *f.ProductID)
	}
	if f.RackID != nil {
		b.add("l.rack_id = $%d", *f.RackID)
	}
	if f.WarehouseID != nil {
		b.add("l.warehouse_id = $%d", *f.WarehouseID)
	}
	if f.ScopeWarehouseID != nil {
		b.add("l.warehouse_id = $%d", *f.ScopeWarehouseID)
	}
	if f.Direction != nil {
		b.add("l.direction = $%d", *f.Direction)
	}
	if f.MovementType != nil {
		b.add("l.movement_type = $%d", string(*f.MovementType))
	}
	if f.DateFrom != nil {
		b.add("(l.occurred_at AT TIME ZONE 'Asia/Jakarta')::date >= $%d::date", *f.DateFrom)
	}
	if f.DateTo != nil {
		b.add("(l.occurred_at AT TIME ZONE 'Asia/Jakarta')::date <= $%d::date", *f.DateTo)
	}
	return queryPage(ctx, database.GetExecutor(ctx, r.pool), ledgerFrom, ledgerColumns, &b, p, "l.id", scanLedgerRow)
}

func (r *InventoryRepo) OpeningBalance(ctx context.Context, f domain.StockCardFilter) (int, error) {
	if f.DateFrom == nil || *f.DateFrom == "" {
		return 0, nil
	}
	var b filterBuilder
	b.add("l.product_id = $%d", f.ProductID)
	b.add("(l.occurred_at AT TIME ZONE 'Asia/Jakarta')::date < $%d::date", *f.DateFrom)
	if f.RackID != nil {
		b.add("l.rack_id = $%d", *f.RackID)
	}
	if f.WarehouseID != nil {
		b.add("l.warehouse_id = $%d", *f.WarehouseID)
	}
	if f.ScopeWarehouseID != nil {
		b.add("l.warehouse_id = $%d", *f.ScopeWarehouseID)
	}
	var bal int
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT COALESCE(SUM(CASE WHEN l.direction = 'IN' THEN l.qty ELSE -l.qty END), 0)::int FROM stock_ledgers l`+b.where(),
		b.args...).Scan(&bal)
	return bal, err
}
