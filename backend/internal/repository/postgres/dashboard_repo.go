package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type DashboardRepo struct{ pool *pgxpool.Pool }

func NewDashboardRepo(pool *pgxpool.Pool) *DashboardRepo { return &DashboardRepo{pool: pool} }

var _ domain.DashboardRepository = (*DashboardRepo)(nil)

func (r *DashboardRepo) WarehouseSummary(ctx context.Context, warehouseID *int64) (*domain.WarehouseDashboardSummary, error) {
	db := database.GetExecutor(ctx, r.pool)
	out := &domain.WarehouseDashboardSummary{
		LowStockItems:     make([]domain.LowStockAlertItem, 0),
		RecentPendingLPBs: make([]domain.PendingLPBSummary, 0),
		RecentMovements:   make([]domain.LedgerRow, 0),
	}

	// 1. Produk aktif: aman vs menipis + total tersedia (per scope gudang bila diset).
	err := db.QueryRow(ctx, `
		WITH avail AS (
			SELECT p.id, p.min_stock_alert,
			       COALESCE(SUM(CASE WHEN rk.is_sellable THEN sb.qty_on_hand - sb.qty_reserved ELSE 0 END), 0)::int AS qty_available
			FROM products p
			LEFT JOIN stock_balances sb ON sb.product_id = p.id
			LEFT JOIN racks rk ON rk.id = sb.rack_id AND ($1::bigint IS NULL OR rk.warehouse_id = $1)
			WHERE p.is_active = TRUE
			GROUP BY p.id, p.min_stock_alert
		)
		SELECT count(*)::int,
		       count(*) FILTER (WHERE min_stock_alert > 0 AND qty_available < min_stock_alert)::int,
		       COALESCE(SUM(qty_available), 0)::int
		FROM avail`, warehouseID).Scan(&out.TotalActiveProducts, &out.LowStockProducts, &out.TotalQtyAvailable)
	if err != nil {
		return nil, err
	}
	out.SafeStockProducts = out.TotalActiveProducts - out.LowStockProducts

	// 2. Unit SN aktif (non-RETIRED) & tersedia.
	err = db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status <> 'RETIRED')::int,
		       count(*) FILTER (WHERE status = 'AVAILABLE')::int
		FROM product_serials
		WHERE ($1::bigint IS NULL OR current_warehouse_id = $1 OR current_rack_id IS NULL)`, warehouseID).
		Scan(&out.SerialUnitsTotal, &out.SerialUnitsAvailable)
	if err != nil {
		return nil, err
	}

	// 3. Antrean LPB menunggu persetujuan & opname berjalan.
	if err := db.QueryRow(ctx, `
		SELECT count(*)::int FROM lpb_documents
		WHERE status = 'PENDING_APPROVAL' AND ($1::bigint IS NULL OR warehouse_id = $1)`, warehouseID).
		Scan(&out.PendingLPBApprovals); err != nil {
		return nil, err
	}
	if err := db.QueryRow(ctx, `
		SELECT count(*)::int FROM stock_opnames
		WHERE status IN ('IN_PROGRESS', 'SUBMITTED') AND ($1::bigint IS NULL OR warehouse_id = $1)`, warehouseID).
		Scan(&out.ActiveOpnames); err != nil {
		return nil, err
	}

	// 4. Top 5 produk stok menipis.
	rows, err := db.Query(ctx, `
		WITH avail AS (
			SELECT p.id, p.sku, p.name, p.unit, p.min_stock_alert,
			       COALESCE(SUM(CASE WHEN rk.is_sellable THEN sb.qty_on_hand - sb.qty_reserved ELSE 0 END), 0)::int AS qty_available
			FROM products p
			LEFT JOIN stock_balances sb ON sb.product_id = p.id
			LEFT JOIN racks rk ON rk.id = sb.rack_id AND ($1::bigint IS NULL OR rk.warehouse_id = $1)
			WHERE p.is_active = TRUE AND p.min_stock_alert > 0
			GROUP BY p.id, p.sku, p.name, p.unit, p.min_stock_alert
		)
		SELECT id, sku, name, unit, qty_available, min_stock_alert
		FROM avail
		WHERE qty_available < min_stock_alert
		ORDER BY (min_stock_alert - qty_available) DESC, sku ASC
		LIMIT 5`, warehouseID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var it domain.LowStockAlertItem
		if err := rows.Scan(&it.ProductID, &it.SKU, &it.ProductName, &it.Unit, &it.QtyAvailable, &it.MinStockAlert); err != nil {
			rows.Close()
			return nil, err
		}
		out.LowStockItems = append(out.LowStockItems, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 5. 5 LPB menunggu persetujuan tertua.
	pRows, err := db.Query(ctx, `
		SELECT l.id, l.lpb_number, l.sub_type, w.code, u.name, l.submitted_at
		FROM lpb_documents l
		JOIN warehouses w ON w.id = l.warehouse_id
		JOIN users u ON u.id = l.created_by
		WHERE l.status = 'PENDING_APPROVAL' AND ($1::bigint IS NULL OR l.warehouse_id = $1)
		ORDER BY l.submitted_at ASC NULLS LAST, l.id ASC
		LIMIT 5`, warehouseID)
	if err != nil {
		return nil, err
	}
	for pRows.Next() {
		var p domain.PendingLPBSummary
		var sub string
		if err := pRows.Scan(&p.ID, &p.Number, &sub, &p.WarehouseCode, &p.CreatedByName, &p.SubmittedAt); err != nil {
			pRows.Close()
			return nil, err
		}
		p.SubType = domain.LPBSubType(sub)
		out.RecentPendingLPBs = append(out.RecentPendingLPBs, p)
	}
	pRows.Close()
	if err := pRows.Err(); err != nil {
		return nil, err
	}

	// 6. 5 mutasi ledger terbaru.
	lRows, err := db.Query(ctx, `
		SELECT `+ledgerColumns+` FROM `+ledgerFrom+`
		WHERE ($1::bigint IS NULL OR l.warehouse_id = $1)
		ORDER BY l.occurred_at DESC, l.id DESC
		LIMIT 5`, warehouseID)
	if err != nil {
		return nil, err
	}
	for lRows.Next() {
		lr, err := scanLedgerRow(lRows)
		if err != nil {
			lRows.Close()
			return nil, err
		}
		out.RecentMovements = append(out.RecentMovements, lr)
	}
	lRows.Close()
	return out, lRows.Err()
}
