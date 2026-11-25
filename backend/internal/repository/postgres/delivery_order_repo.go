package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type DeliveryOrderRepo struct{ pool *pgxpool.Pool }

func NewDeliveryOrderRepo(pool *pgxpool.Pool) *DeliveryOrderRepo {
	return &DeliveryOrderRepo{pool: pool}
}

var _ domain.DeliveryOrderRepository = (*DeliveryOrderRepo)(nil)

const doFrom = `delivery_orders d
	JOIN sales_orders so ON so.id = d.so_id
	JOIN customers c ON c.id = so.customer_id
	JOIN customer_sites cs ON cs.id = so.customer_site_id
	JOIN warehouses w ON w.id = d.warehouse_id
	JOIN users uc ON uc.id = d.created_by
	LEFT JOIN lpb_documents l ON l.id = d.lpb_id`

const doColumns = `d.id, d.do_number, d.so_id, so.so_number, so.order_type,
	so.customer_id, c.customer_code, c.company_name, so.customer_site_id, cs.site_name, cs.city, so.contract_id,
	d.warehouse_id, w.code, w.name, d.direction, d.status, d.scheduled_date,
	d.lpb_id, l.lpb_number, d.notes, d.created_by, uc.name, d.created_at, d.updated_at,
	(SELECT COUNT(*) FROM delivery_order_items di WHERE di.do_id = d.id) AS item_count,
	(SELECT COALESCE(SUM(di.qty), 0) FROM delivery_order_items di WHERE di.do_id = d.id) AS total_qty,
	(SELECT COALESCE(SUM(r.qty), 0)
	 FROM delivery_order_reservations r
	 JOIN delivery_order_items di ON di.id = r.do_item_id
	 WHERE di.do_id = d.id AND r.released_at IS NULL) AS reserved_qty`

func scanDO(row pgx.Row) (domain.DeliveryOrder, error) {
	var d domain.DeliveryOrder
	var otype, dir, st string
	err := row.Scan(
		&d.ID, &d.DONumber, &d.SOID, &d.SONumber, &otype,
		&d.CustomerID, &d.CustomerCode, &d.CustomerName, &d.CustomerSiteID, &d.SiteName, &d.SiteCity, &d.ContractID,
		&d.WarehouseID, &d.WarehouseCode, &d.WarehouseName, &dir, &st, &d.ScheduledDate,
		&d.LPBID, &d.LPBNumber, &d.Notes, &d.CreatedBy, &d.CreatedByName, &d.CreatedAt, &d.UpdatedAt,
		&d.ItemCount, &d.TotalQty, &d.ReservedQty,
	)
	d.OrderType = domain.OrderType(otype)
	d.Direction = domain.DODirection(dir)
	d.Status = domain.DOStatus(st)
	return d, err
}

func (r *DeliveryOrderRepo) FindByID(ctx context.Context, id int64) (*domain.DeliveryOrder, error) {
	d, err := scanDO(database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT `+doColumns+` FROM `+doFrom+` WHERE d.id = $1`, id))
	if err != nil {
		return nil, notFound(err)
	}
	return &d, nil
}

func (r *DeliveryOrderRepo) FindByIDForUpdate(ctx context.Context, id int64) (*domain.DeliveryOrder, error) {
	database.MustInTx(ctx)
	var dummy int64
	if err := database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT id FROM delivery_orders WHERE id = $1 FOR UPDATE`, id).Scan(&dummy); err != nil {
		return nil, notFound(err)
	}
	return r.FindByID(ctx, id)
}

func (r *DeliveryOrderRepo) List(ctx context.Context, f domain.DeliveryOrderFilter, scopeWarehouseID *int64, p domain.PageParams) ([]domain.DeliveryOrder, int64, error) {
	var b filterBuilder
	b.like(f.Query, "d.do_number", "so.so_number", "c.company_name", "cs.site_name")
	if scopeWarehouseID != nil {
		b.add("d.warehouse_id = $%d", *scopeWarehouseID)
	} else if f.WarehouseID != nil {
		b.add("d.warehouse_id = $%d", *f.WarehouseID)
	}
	if f.Direction != nil {
		b.add("d.direction = $%d", string(*f.Direction))
	}
	if f.Status != nil {
		b.add("d.status = $%d", string(*f.Status))
	}
	if f.SOID != nil {
		b.add("d.so_id = $%d", *f.SOID)
	}
	return queryPage(ctx, database.GetExecutor(ctx, r.pool), doFrom, doColumns, &b, p, "d.id",
		func(row pgx.Row) (domain.DeliveryOrder, error) { return scanDO(row) })
}

func (r *DeliveryOrderRepo) ListItems(ctx context.Context, doID int64) ([]domain.DeliveryOrderItem, error) {
	db := database.GetExecutor(ctx, r.pool)
	rows, err := db.Query(ctx, `
		SELECT di.id, di.do_id, di.so_item_id, di.product_id, p.sku, p.name, p.has_serial_number,
		       di.qty,
		       COALESCE((SELECT SUM(r.qty) FROM delivery_order_reservations r WHERE r.do_item_id = di.id AND r.released_at IS NULL), 0) AS reserved_qty,
		       soi.product_serial_id, ps.serial_number
		FROM delivery_order_items di
		JOIN products p ON p.id = di.product_id
		JOIN sales_order_items soi ON soi.id = di.so_item_id
		LEFT JOIN product_serials ps ON ps.id = soi.product_serial_id
		WHERE di.do_id = $1
		ORDER BY di.id ASC`, doID)
	if err != nil {
		return nil, err
	}
	items := make([]domain.DeliveryOrderItem, 0)
	for rows.Next() {
		var it domain.DeliveryOrderItem
		if err := rows.Scan(
			&it.ID, &it.DOID, &it.SOItemID, &it.ProductID, &it.ProductSKU, &it.ProductName,
			&it.HasSerialNumber, &it.Qty, &it.ReservedQty, &it.SOProductSerial, &it.SOSerialNumber,
		); err != nil {
			rows.Close()
			return nil, err
		}
		it.Reservations = []domain.DOReservation{}
		items = append(items, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	resList, err := r.ListActiveReservations(ctx, doID)
	if err != nil {
		return nil, err
	}
	byItem := make(map[int64][]domain.DOReservation, len(items))
	for _, rs := range resList {
		byItem[rs.DOItemID] = append(byItem[rs.DOItemID], rs)
	}
	for i := range items {
		if rs, ok := byItem[items[i].ID]; ok {
			items[i].Reservations = rs
		}
	}
	return items, nil
}

func (r *DeliveryOrderRepo) ListActiveReservations(ctx context.Context, doID int64) ([]domain.DOReservation, error) {
	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, `
		SELECT r.id, r.do_item_id, di.product_id, r.rack_id, rk.code,
		       r.product_serial_id, ps.serial_number, ps.asset_tag,
		       r.qty, r.reserved_by, r.reserved_at, r.released_at
		FROM delivery_order_reservations r
		JOIN delivery_order_items di ON di.id = r.do_item_id
		JOIN racks rk ON rk.id = r.rack_id
		LEFT JOIN product_serials ps ON ps.id = r.product_serial_id
		WHERE di.do_id = $1 AND r.released_at IS NULL
		ORDER BY r.id ASC`, doID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.DOReservation, 0)
	for rows.Next() {
		var rs domain.DOReservation
		if err := rows.Scan(
			&rs.ID, &rs.DOItemID, &rs.ProductID, &rs.RackID, &rs.RackCode,
			&rs.ProductSerialID, &rs.SerialNumber, &rs.AssetTag,
			&rs.Qty, &rs.ReservedBy, &rs.ReservedAt, &rs.ReleasedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, rs)
	}
	return out, rows.Err()
}

func (r *DeliveryOrderRepo) InsertReservation(ctx context.Context, res *domain.DOReservation) (*domain.DOReservation, error) {
	database.MustInTx(ctx)
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO delivery_order_reservations (do_item_id, rack_id, product_serial_id, qty, reserved_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, reserved_at`,
		res.DOItemID, res.RackID, res.ProductSerialID, res.Qty, res.ReservedBy,
	).Scan(&res.ID, &res.ReservedAt)
	if err != nil {
		return nil, duplicateOf(err, map[string]domain.DuplicateError{
			"uq_active_serial_reservation": {Field: "serial_ids", Message: "serial number sudah direservasi untuk DO lain"},
		})
	}
	return res, nil
}

func (r *DeliveryOrderRepo) MarkReservationsReleased(ctx context.Context, doID int64, at time.Time) error {
	database.MustInTx(ctx)
	_, err := database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE delivery_order_reservations r
		SET released_at = $2
		FROM delivery_order_items di
		WHERE di.id = r.do_item_id AND di.do_id = $1 AND r.released_at IS NULL`,
		doID, at,
	)
	return err
}

func (r *DeliveryOrderRepo) UpdateStatus(ctx context.Context, doID int64, status domain.DOStatus, lpbID *int64) error {
	database.MustInTx(ctx)
	if lpbID != nil {
		return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx,
			`UPDATE delivery_orders SET status = $2, lpb_id = $3 WHERE id = $1`,
			doID, string(status), *lpbID))
	}
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx,
		`UPDATE delivery_orders SET status = $2 WHERE id = $1`,
		doID, string(status)))
}
