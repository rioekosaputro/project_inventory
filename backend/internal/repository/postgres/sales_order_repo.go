package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type SalesOrderRepo struct{ pool *pgxpool.Pool }

func NewSalesOrderRepo(pool *pgxpool.Pool) *SalesOrderRepo { return &SalesOrderRepo{pool: pool} }

var _ domain.SalesOrderRepository = (*SalesOrderRepo)(nil)

const soFrom = `sales_orders so
	JOIN customers c ON c.id = so.customer_id
	JOIN customer_sites cs ON cs.id = so.customer_site_id
	JOIN users uc ON uc.id = so.created_by
	LEFT JOIN rental_contracts rc ON rc.id = so.contract_id
	LEFT JOIN users ucf ON ucf.id = so.confirmed_by
	LEFT JOIN users ucn ON ucn.id = so.cancelled_by`

const soColumns = `so.id, so.so_number, so.order_type, so.customer_id, c.customer_code, c.company_name,
	so.customer_site_id, cs.site_name, cs.city, so.contract_id, rc.contract_number,
	so.status, so.order_date, so.requested_date, so.is_ppn,
	so.subtotal, so.discount_amount, so.dpp, so.dpp_other_value, so.tax_rate, so.tax_amount, so.grand_total,
	so.related_ticket_id, so.notes,
	so.created_by, uc.name, so.confirmed_by, ucf.name, so.confirmed_at,
	so.cancelled_by, ucn.name, so.cancelled_at, so.cancel_reason,
	so.created_at, so.updated_at,
	(SELECT COUNT(*) FROM sales_order_items soi WHERE soi.so_id = so.id) AS item_count,
	(SELECT COALESCE(SUM(soi.qty), 0) FROM sales_order_items soi WHERE soi.so_id = so.id) AS total_qty,
	(SELECT COUNT(*) FROM delivery_orders d WHERE d.so_id = so.id AND d.status <> 'CANCELLED') AS do_count`

func scanSO(row pgx.Row) (domain.SalesOrder, error) {
	var so domain.SalesOrder
	var otype, status string
	err := row.Scan(
		&so.ID, &so.SONumber, &otype, &so.CustomerID, &so.CustomerCode, &so.CustomerName,
		&so.CustomerSiteID, &so.SiteName, &so.SiteCity, &so.ContractID, &so.ContractNumber,
		&status, &so.OrderDate, &so.RequestedDate, &so.IsPPN,
		&so.Subtotal, &so.DiscountAmount, &so.DPP, &so.DPPOtherValue, &so.TaxRate, &so.TaxAmount, &so.GrandTotal,
		&so.RelatedTicketID, &so.Notes,
		&so.CreatedBy, &so.CreatedByName, &so.ConfirmedBy, &so.ConfirmedByName, &so.ConfirmedAt,
		&so.CancelledBy, &so.CancelledByName, &so.CancelledAt, &so.CancelReason,
		&so.CreatedAt, &so.UpdatedAt,
		&so.ItemCount, &so.TotalQty, &so.DOCount,
	)
	so.OrderType = domain.OrderType(otype)
	so.Status = domain.SOStatus(status)
	return so, err
}

func (r *SalesOrderRepo) FindByID(ctx context.Context, id int64) (*domain.SalesOrder, error) {
	so, err := scanSO(database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT `+soColumns+` FROM `+soFrom+` WHERE so.id = $1`, id))
	if err != nil {
		return nil, notFound(err)
	}
	return &so, nil
}

func (r *SalesOrderRepo) FindByIDForUpdate(ctx context.Context, id int64) (*domain.SalesOrder, error) {
	database.MustInTx(ctx)
	var dummy int64
	if err := database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT id FROM sales_orders WHERE id = $1 FOR UPDATE`, id).Scan(&dummy); err != nil {
		return nil, notFound(err)
	}
	return r.FindByID(ctx, id)
}

func (r *SalesOrderRepo) List(ctx context.Context, f domain.SalesOrderFilter, p domain.PageParams) ([]domain.SalesOrder, int64, error) {
	var b filterBuilder
	b.like(f.Query, "so.so_number", "c.customer_code", "c.company_name", "cs.site_name")
	if f.OrderType != nil {
		b.add("so.order_type = $%d", string(*f.OrderType))
	}
	if f.Status != nil {
		b.add("so.status = $%d", string(*f.Status))
	}
	if f.CustomerID != nil {
		b.add("so.customer_id = $%d", *f.CustomerID)
	}
	if f.ContractID != nil {
		b.add("so.contract_id = $%d", *f.ContractID)
	}
	return queryPage(ctx, database.GetExecutor(ctx, r.pool), soFrom, soColumns, &b, p, "so.id",
		func(row pgx.Row) (domain.SalesOrder, error) { return scanSO(row) })
}

func (r *SalesOrderRepo) Create(ctx context.Context, so *domain.SalesOrder) (*domain.SalesOrder, error) {
	var id int64
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO sales_orders (
			so_number, order_type, customer_id, customer_site_id, contract_id,
			status, order_date, requested_date, is_ppn,
			subtotal, discount_amount, dpp, dpp_other_value, tax_rate, tax_amount, grand_total,
			notes, created_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		RETURNING id`,
		so.SONumber, string(so.OrderType), so.CustomerID, so.CustomerSiteID, so.ContractID,
		string(so.Status), so.OrderDate, so.RequestedDate, so.IsPPN,
		so.Subtotal, so.DiscountAmount, so.DPP, so.DPPOtherValue, so.TaxRate, so.TaxAmount, so.GrandTotal,
		so.Notes, so.CreatedBy,
	).Scan(&id)
	if err != nil {
		return nil, duplicateOf(err, map[string]domain.DuplicateError{
			"sales_orders_so_number_key": {Field: "so_number", Message: "nomor SO sudah dipakai"},
		})
	}
	return r.FindByID(ctx, id)
}

func (r *SalesOrderRepo) Update(ctx context.Context, so *domain.SalesOrder) error {
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE sales_orders
		SET customer_site_id = $2, order_date = $3, requested_date = $4, is_ppn = $5,
		    subtotal = $6, discount_amount = $7, dpp = $8, dpp_other_value = $9,
		    tax_rate = $10, tax_amount = $11, grand_total = $12, notes = $13
		WHERE id = $1`,
		so.ID, so.CustomerSiteID, so.OrderDate, so.RequestedDate, so.IsPPN,
		so.Subtotal, so.DiscountAmount, so.DPP, so.DPPOtherValue,
		so.TaxRate, so.TaxAmount, so.GrandTotal, so.Notes,
	))
}

func (r *SalesOrderRepo) UpdateStatus(ctx context.Context, so *domain.SalesOrder) error {
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE sales_orders
		SET status = $2, confirmed_by = $3, confirmed_at = $4,
		    cancelled_by = $5, cancelled_at = $6, cancel_reason = $7
		WHERE id = $1`,
		so.ID, string(so.Status), so.ConfirmedBy, so.ConfirmedAt,
		so.CancelledBy, so.CancelledAt, so.CancelReason,
	))
}

// ---------------------------------------------------------------- items

func (r *SalesOrderRepo) ListItems(ctx context.Context, soID int64) ([]domain.SalesOrderItem, error) {
	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, `
		SELECT soi.id, soi.so_id, soi.product_id, p.sku, p.name, soi.direction,
		       soi.qty, soi.unit_price, soi.line_total,
		       soi.product_serial_id, ps.serial_number, ps.asset_tag, soi.notes
		FROM sales_order_items soi
		JOIN products p ON p.id = soi.product_id
		LEFT JOIN product_serials ps ON ps.id = soi.product_serial_id
		WHERE soi.so_id = $1
		ORDER BY soi.direction DESC, soi.id ASC`, soID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.SalesOrderItem, 0)
	for rows.Next() {
		var it domain.SalesOrderItem
		var dir string
		if err := rows.Scan(
			&it.ID, &it.SOID, &it.ProductID, &it.ProductSKU, &it.ProductName, &dir,
			&it.Qty, &it.UnitPrice, &it.LineTotal,
			&it.ProductSerialID, &it.SerialNumber, &it.AssetTag, &it.Notes,
		); err != nil {
			return nil, err
		}
		it.Direction = domain.ItemDirection(dir)
		items = append(items, it)
	}
	return items, rows.Err()
}

func (r *SalesOrderRepo) ReplaceItems(ctx context.Context, soID int64, items []domain.SalesOrderItem) ([]domain.SalesOrderItem, error) {
	database.MustInTx(ctx)
	exec := database.GetExecutor(ctx, r.pool)
	if _, err := exec.Exec(ctx, `DELETE FROM sales_order_items WHERE so_id = $1`, soID); err != nil {
		return nil, err
	}
	for _, it := range items {
		if _, err := exec.Exec(ctx, `
			INSERT INTO sales_order_items (so_id, product_id, direction, qty, unit_price, line_total, product_serial_id, notes)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			soID, it.ProductID, string(it.Direction), it.Qty, it.UnitPrice, it.LineTotal, it.ProductSerialID, it.Notes,
		); err != nil {
			return nil, err
		}
	}
	return r.ListItems(ctx, soID)
}

// ---------------------------------------------------------------- delivery orders

func (r *SalesOrderRepo) ListDeliveryOrdersBySO(ctx context.Context, soID int64) ([]domain.DeliveryOrderSummary, error) {
	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, `
		SELECT d.id, d.do_number, d.so_id, d.warehouse_id, w.code, w.name,
		       d.direction, d.status, d.scheduled_date, d.lpb_id, d.notes, d.created_at,
		       (SELECT COUNT(*) FROM delivery_order_items di WHERE di.do_id = d.id) AS item_count,
		       (SELECT COALESCE(SUM(di.qty), 0) FROM delivery_order_items di WHERE di.do_id = d.id) AS total_qty
		FROM delivery_orders d
		JOIN warehouses w ON w.id = d.warehouse_id
		WHERE d.so_id = $1
		ORDER BY d.id ASC`, soID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.DeliveryOrderSummary, 0)
	for rows.Next() {
		var d domain.DeliveryOrderSummary
		var dir, st string
		if err := rows.Scan(
			&d.ID, &d.DONumber, &d.SOID, &d.WarehouseID, &d.WarehouseCode, &d.WarehouseName,
			&dir, &st, &d.ScheduledDate, &d.LPBID, &d.Notes, &d.CreatedAt,
			&d.ItemCount, &d.TotalQty,
		); err != nil {
			return nil, err
		}
		d.Direction = domain.DODirection(dir)
		d.Status = domain.DOStatus(st)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *SalesOrderRepo) HasActiveDeliveryOrders(ctx context.Context, soID int64) (bool, error) {
	var exists bool
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM delivery_orders WHERE so_id = $1 AND status <> 'CANCELLED')`,
		soID,
	).Scan(&exists)
	return exists, err
}

func (r *SalesOrderRepo) CreateDeliveryOrder(
	ctx context.Context,
	do *domain.DeliveryOrderSummary,
	items []domain.SalesOrderItem,
	createdBy int64,
) (*domain.DeliveryOrderSummary, error) {
	database.MustInTx(ctx)
	exec := database.GetExecutor(ctx, r.pool)
	var doID int64
	err := exec.QueryRow(ctx, `
		INSERT INTO delivery_orders (do_number, so_id, warehouse_id, direction, status, scheduled_date, notes, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at`,
		do.DONumber, do.SOID, do.WarehouseID, string(do.Direction), string(domain.DOStatusOpen),
		do.ScheduledDate, do.Notes, createdBy,
	).Scan(&doID, &do.CreatedAt)
	if err != nil {
		return nil, err
	}
	totalQty := 0
	for _, it := range items {
		if _, err := exec.Exec(ctx, `
			INSERT INTO delivery_order_items (do_id, so_item_id, product_id, qty)
			VALUES ($1, $2, $3, $4)`,
			doID, it.ID, it.ProductID, it.Qty,
		); err != nil {
			return nil, err
		}
		totalQty += it.Qty
	}
	do.ID = doID
	do.Status = domain.DOStatusOpen
	do.ItemCount = len(items)
	do.TotalQty = totalQty
	return do, nil
}
