package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

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
	if err := db.QueryRow(ctx, `
		SELECT count(*)::int FROM delivery_orders
		WHERE status = 'OPEN' AND ($1::bigint IS NULL OR warehouse_id = $1)`, warehouseID).
		Scan(&out.PendingDOReservations); err != nil {
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

// CommercialSummary menghitung metrik komersial (Bagian 9.5, T2.11):
// utilisasi aset, omzet bulan berjalan, piutang/overdue, dan kontrak berakhir <= 30 hari.
func (r *DashboardRepo) CommercialSummary(ctx context.Context, today time.Time) (*domain.CommercialDashboardSummary, error) {
	db := database.GetExecutor(ctx, r.pool)
	todayStr := today.Format("2006-01-02")
	monthStart := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	monthEnd := time.Date(today.Year(), today.Month()+1, 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	dueSoonStr := today.AddDate(0, 0, 7).Format("2006-01-02")
	expiring30Str := today.AddDate(0, 0, 30).Format("2006-01-02")

	out := &domain.CommercialDashboardSummary{
		ExpiringContracts: make([]domain.ExpiringContractAlert, 0),
		OverdueInvoices:   make([]domain.OverdueInvoiceAlert, 0),
	}

	// 1. Pelanggan & kontrak aktif.
	if err := db.QueryRow(ctx, `SELECT count(*)::int FROM customers WHERE is_active = TRUE`).Scan(&out.ActiveCustomers); err != nil {
		return nil, err
	}
	if err := db.QueryRow(ctx, `SELECT count(*)::int FROM rental_contracts WHERE status = 'ACTIVE'`).Scan(&out.ActiveContracts); err != nil {
		return nil, err
	}

	// 2. Utilisasi aset: RENTED / total unit SN non-RETIRED.
	if err := db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status = 'RENTED')::int,
		       count(*) FILTER (WHERE status <> 'RETIRED')::int
		FROM product_serials`).Scan(&out.RentedUnits, &out.TotalActiveAssets); err != nil {
		return nil, err
	}
	if out.TotalActiveAssets > 0 {
		out.AssetUtilizationPct = decimal.NewFromInt(int64(out.RentedUnits)).
			Mul(decimal.NewFromInt(100)).
			DivRound(decimal.NewFromInt(int64(out.TotalActiveAssets)), 2)
	} else {
		out.AssetUtilizationPct = decimal.Zero
	}

	// 3. Omzet bulan berjalan (invoice terbit/dibayar di bulan ini) & piutang/overdue.
	var revStr, outStr, overdueAmtStr string
	if err := db.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN status IN ('ISSUED','PARTIALLY_PAID','PAID')
			                  AND invoice_date >= $1::date AND invoice_date <= $2::date
			             THEN grand_total ELSE 0 END), 0)::text,
			COALESCE(SUM(CASE WHEN status IN ('ISSUED','PARTIALLY_PAID')
			             THEN grand_total - paid_amount ELSE 0 END), 0)::text,
			count(*) FILTER (WHERE status IN ('ISSUED','PARTIALLY_PAID') AND due_date < $3::date)::int,
			COALESCE(SUM(CASE WHEN status IN ('ISSUED','PARTIALLY_PAID') AND due_date < $3::date
			             THEN grand_total - paid_amount ELSE 0 END), 0)::text,
			count(*) FILTER (WHERE status IN ('ISSUED','PARTIALLY_PAID') AND due_date >= $3::date AND due_date <= $4::date)::int
		FROM invoices`, monthStart, monthEnd, todayStr, dueSoonStr).Scan(
		&revStr, &outStr, &out.OverdueInvoiceCount, &overdueAmtStr, &out.DueSoonInvoiceCount,
	); err != nil {
		return nil, err
	}
	out.MonthlyRevenue, _ = decimal.NewFromString(revStr)
	out.OutstandingReceivables, _ = decimal.NewFromString(outStr)
	out.OverdueInvoiceAmount, _ = decimal.NewFromString(overdueAmtStr)

	// 4. Pembayaran tertagih bulan berjalan (amount_received + pph23_amount).
	var collectedStr string
	if err := db.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount_received + pph23_amount), 0)::text
		FROM payments
		WHERE payment_date >= $1::date AND payment_date <= $2::date`, monthStart, monthEnd).Scan(&collectedStr); err != nil {
		return nil, err
	}
	out.MonthlyCollected, _ = decimal.NewFromString(collectedStr)

	// 5. Jumlah & top 5 kontrak akan berakhir <= 30 hari.
	if err := db.QueryRow(ctx, `
		SELECT count(*)::int FROM rental_contracts
		WHERE status = 'ACTIVE' AND end_date >= $1::date AND end_date <= $2::date`,
		todayStr, expiring30Str).Scan(&out.ExpiringContractsCount); err != nil {
		return nil, err
	}
	cRows, err := db.Query(ctx, `
		SELECT rc.id, rc.contract_number, rc.customer_id, c.company_name,
		       TO_CHAR(rc.end_date, 'YYYY-MM-DD'),
		       (rc.end_date - $1::date) AS days_remaining
		FROM rental_contracts rc
		JOIN customers c ON c.id = rc.customer_id
		WHERE rc.status = 'ACTIVE'
		  AND rc.end_date >= $1::date
		  AND rc.end_date <= $2::date
		ORDER BY rc.end_date ASC, rc.id ASC
		LIMIT 5`, todayStr, expiring30Str)
	if err != nil {
		return nil, err
	}
	for cRows.Next() {
		var a domain.ExpiringContractAlert
		if err := cRows.Scan(&a.ContractID, &a.ContractNumber, &a.CustomerID, &a.CustomerName, &a.EndDate, &a.DaysRemaining); err != nil {
			cRows.Close()
			return nil, err
		}
		out.ExpiringContracts = append(out.ExpiringContracts, a)
	}
	cRows.Close()
	if err := cRows.Err(); err != nil {
		return nil, err
	}

	// 6. Top 5 invoice melewati jatuh tempo (terlama dulu).
	iRows, err := db.Query(ctx, `
		SELECT i.id, i.invoice_number, i.customer_id, c.company_name,
		       TO_CHAR(i.due_date, 'YYYY-MM-DD'),
		       ($1::date - i.due_date) AS days_overdue,
		       TO_CHAR(i.grand_total - i.paid_amount, 'FM999999999999990.00') AS remaining_amount
		FROM invoices i
		JOIN customers c ON c.id = i.customer_id
		WHERE i.status IN ('ISSUED', 'PARTIALLY_PAID')
		  AND i.due_date < $1::date
		ORDER BY i.due_date ASC, i.id ASC
		LIMIT 5`, todayStr)
	if err != nil {
		return nil, err
	}
	for iRows.Next() {
		var a domain.OverdueInvoiceAlert
		if err := iRows.Scan(&a.InvoiceID, &a.InvoiceNumber, &a.CustomerID, &a.CustomerName, &a.DueDate, &a.DaysOverdue, &a.RemainingAmount); err != nil {
			iRows.Close()
			return nil, err
		}
		out.OverdueInvoices = append(out.OverdueInvoices, a)
	}
	iRows.Close()
	return out, iRows.Err()
}

func (r *DashboardRepo) FieldSummary(ctx context.Context, now time.Time) (*domain.FieldDashboardSummary, error) {
	db := database.GetExecutor(ctx, r.pool)
	out := &domain.FieldDashboardSummary{
		SLACompliancePct: decimal.NewFromInt(100),
		SLABreachAlerts:  make([]domain.SLABreachTicketAlert, 0),
	}

	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// 1. Metrik tiket & kepatuhan SLA bulan ini.
	var resolvedOnTime int
	if err := db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status IN ('OPEN','ASSIGNED','IN_PROGRESS','WAITING_PART'))::int,
			COUNT(*) FILTER (WHERE status = 'WAITING_PART')::int,
			COUNT(*) FILTER (WHERE status IN ('OPEN','ASSIGNED','IN_PROGRESS','WAITING_PART')
			                  AND sla_due_at IS NOT NULL AND sla_due_at < $1)::int,
			COUNT(*) FILTER (WHERE resolved_at >= $2)::int,
			COUNT(*) FILTER (WHERE resolved_at >= $2 AND (sla_due_at IS NULL OR resolved_at <= sla_due_at))::int
		FROM maintenance_tickets`, now, monthStart).Scan(
		&out.OpenTickets, &out.WaitingPartTickets, &out.SLABreachedTickets,
		&out.ResolvedThisMonth, &resolvedOnTime,
	); err != nil {
		return nil, err
	}
	if out.ResolvedThisMonth > 0 {
		out.SLACompliancePct = decimal.NewFromInt(int64(resolvedOnTime*100)).
			DivRound(decimal.NewFromInt(int64(out.ResolvedThisMonth)), 2)
	}

	// 2. Metrik pengiriman lapangan.
	if err := db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (
				WHERE sub_type IN ('RENTAL_DELIVERY','CONSUMABLE_DELIVERY')
				  AND status = 'APPROVED' AND requires_delivery = TRUE
				  AND COALESCE(delivery_status, '') NOT IN ('DELIVERED','FAILED','RETURNED')
			)::int,
			COUNT(*) FILTER (WHERE delivery_status = 'DELIVERED' AND delivered_at >= $1)::int,
			COUNT(*) FILTER (WHERE delivery_status = 'FAILED' AND updated_at >= $2)::int
		FROM lpb_documents`, dayStart, monthStart).Scan(
		&out.ActiveDeliveries, &out.DeliveredToday, &out.FailedDeliveriesThisMonth,
	); err != nil {
		return nil, err
	}

	// 3. Permintaan sparepart yang masih menunggu dibuatkan LPB.
	if err := db.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM ticket_spareparts WHERE status = 'REQUESTED'`).
		Scan(&out.PendingSparepartRequests); err != nil {
		return nil, err
	}

	// 4. Top 5 tiket aktif yang melewati SLA (terlama dulu).
	tRows, err := db.Query(ctx, `
		SELECT t.id, t.ticket_number, t.title, c.company_name, ps.asset_tag,
		       t.priority, t.status, t.sla_due_at,
		       GREATEST(0, FLOOR(EXTRACT(EPOCH FROM ($1 - t.sla_due_at)) / 3600))::int AS hours_overdue
		FROM maintenance_tickets t
		JOIN customers c ON c.id = t.customer_id
		JOIN product_serials ps ON ps.id = t.product_serial_id
		WHERE t.status IN ('OPEN','ASSIGNED','IN_PROGRESS','WAITING_PART')
		  AND t.sla_due_at IS NOT NULL
		  AND t.sla_due_at < $1
		ORDER BY t.sla_due_at ASC, t.id ASC
		LIMIT 5`, now)
	if err != nil {
		return nil, err
	}
	for tRows.Next() {
		var a domain.SLABreachTicketAlert
		if err := tRows.Scan(
			&a.TicketID, &a.TicketNumber, &a.Title, &a.CustomerName, &a.AssetTag,
			&a.Priority, &a.Status, &a.SLADueAt, &a.HoursOverdue,
		); err != nil {
			tRows.Close()
			return nil, err
		}
		out.SLABreachAlerts = append(out.SLABreachAlerts, a)
	}
	tRows.Close()
	return out, tRows.Err()
}
