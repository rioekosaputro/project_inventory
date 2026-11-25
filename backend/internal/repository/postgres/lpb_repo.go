package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type LPBRepo struct{ pool *pgxpool.Pool }

func NewLPBRepo(pool *pgxpool.Pool) *LPBRepo { return &LPBRepo{pool: pool} }

var _ domain.LPBRepository = (*LPBRepo)(nil)

const lpbFrom = `lpb_documents l
	JOIN warehouses w ON w.id = l.warehouse_id
	JOIN users cu ON cu.id = l.created_by
	LEFT JOIN users au ON au.id = l.approved_by
	LEFT JOIN suppliers s ON s.id = l.supplier_id
	LEFT JOIN lpb_documents cp ON cp.id = l.counterpart_lpb_id`

const lpbColumns = `l.id, l.lpb_number, l.type, l.sub_type, l.status, l.warehouse_id, w.code, w.name, w.address, l.doc_date::text,
	l.reference_type, l.reference_id, l.reference_number, l.supplier_id, s.name, l.customer_id, l.customer_site_id,
	l.requires_delivery, l.delivery_status, l.delivered_at, l.receiver_name, l.pod_photo_path, l.driver_id, l.notes, l.created_by, cu.name,
	l.submitted_by, l.submitted_at, l.approved_by, au.name, l.approved_at, l.rejected_by, l.rejected_at, l.rejection_reason,
	l.verify_token, l.created_at, l.updated_at, cp.id, cp.lpb_number`

func scanLPBInto(row pgx.Row, extra ...any) (domain.LPB, error) {
	var (
		l                domain.LPB
		typ, sub, status string
	)
	dest := append([]any{
		&l.ID, &l.Number, &typ, &sub, &status, &l.WarehouseID, &l.WarehouseCode, &l.WarehouseName, &l.WarehouseAddress, &l.DocDate,
		&l.ReferenceType, &l.ReferenceID, &l.ReferenceNumber, &l.SupplierID, &l.SupplierName, &l.CustomerID, &l.CustomerSiteID,
		&l.RequiresDelivery, &l.DeliveryStatus, &l.DeliveredAt, &l.ReceiverName, &l.PODPhotoPath, &l.DriverID, &l.Notes, &l.CreatedBy, &l.CreatedByName,
		&l.SubmittedBy, &l.SubmittedAt, &l.ApprovedBy, &l.ApprovedByName, &l.ApprovedAt, &l.RejectedBy, &l.RejectedAt, &l.RejectionReason,
		&l.VerifyToken, &l.CreatedAt, &l.UpdatedAt, &l.CounterpartID, &l.CounterpartNumber,
	}, extra...)
	err := row.Scan(dest...)
	l.Type, l.SubType, l.Status = domain.LPBType(typ), domain.LPBSubType(sub), domain.LPBStatus(status)
	return l, err
}

func (r *LPBRepo) FindByVerifyToken(ctx context.Context, token string) (*domain.LPBVerifySummary, error) {
	var (
		out              domain.LPBVerifySummary
		typ, sub, status string
	)
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		SELECT l.lpb_number, l.type, l.sub_type, l.status, l.doc_date::text,
		       w.code, w.name, s.name,
		       (SELECT count(*) FROM lpb_items WHERE lpb_id = l.id)::int,
		       (SELECT COALESCE(SUM(qty), 0) FROM lpb_items WHERE lpb_id = l.id)::int,
		       au.name, l.approved_at
		FROM lpb_documents l
		JOIN warehouses w ON w.id = l.warehouse_id
		LEFT JOIN users au ON au.id = l.approved_by
		LEFT JOIN suppliers s ON s.id = l.supplier_id
		WHERE l.verify_token = $1 AND l.status = 'APPROVED'`, token).Scan(
		&out.Number, &typ, &sub, &status, &out.DocDate,
		&out.WarehouseCode, &out.WarehouseName, &out.SupplierName,
		&out.ItemCount, &out.TotalQty, &out.ApprovedByName, &out.ApprovedAt,
	)
	if err != nil {
		if errors.Is(notFound(err), domain.ErrNotFound) {
			return nil, domain.ErrLPBNotFound
		}
		return nil, err
	}
	out.Type, out.SubType, out.Status = domain.LPBType(typ), domain.LPBSubType(sub), domain.LPBStatus(status)
	return &out, nil
}

func (r *LPBRepo) find(ctx context.Context, id int64, suffix string) (*domain.LPB, error) {
	l, err := scanLPBInto(database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT `+lpbColumns+` FROM `+lpbFrom+` WHERE l.id = $1`+suffix, id))
	if err != nil {
		return nil, notFound(err)
	}
	return &l, nil
}

func (r *LPBRepo) FindByID(ctx context.Context, id int64) (*domain.LPB, error) {
	l, err := r.find(ctx, id, "")
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrLPBNotFound
	}
	return l, err
}

func (r *LPBRepo) FindByIDForUpdate(ctx context.Context, id int64) (*domain.LPB, error) {
	database.MustInTx(ctx)
	l, err := r.find(ctx, id, " FOR UPDATE OF l")
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrLPBNotFound
	}
	return l, err
}

// Create menyimpan dokumen baru. Status kosong = DRAFT. Dokumen yang lahir dari proses lain (pasangan
// transfer) boleh langsung PENDING_APPROVAL dengan counterpart dan pengaju terisi.
func (r *LPBRepo) Create(ctx context.Context, l *domain.LPB) (*domain.LPB, error) {
	status := string(l.Status)
	if status == "" {
		status = string(domain.LPBDraft)
	}
	var id int64
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO lpb_documents (lpb_number, type, sub_type, status, warehouse_id, doc_date, reference_type,
		                           reference_id, reference_number, supplier_id, customer_id, customer_site_id,
		                           requires_delivery, notes, created_by, counterpart_lpb_id,
		                           submitted_by, submitted_at)
		VALUES ($1, $2, $3, $4, $5, $6::date, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18) RETURNING id`,
		l.Number, string(l.Type), string(l.SubType), status, l.WarehouseID, l.DocDate, l.ReferenceType,
		l.ReferenceID, l.ReferenceNumber, l.SupplierID, l.CustomerID, l.CustomerSiteID,
		l.RequiresDelivery, l.Notes, l.CreatedBy, l.CounterpartID, l.SubmittedBy, l.SubmittedAt).Scan(&id)
	if err != nil {
		return nil, err
	}
	return r.FindByID(ctx, id)
}

func (r *LPBRepo) SetCounterpart(ctx context.Context, id, counterpartID int64) error {
	database.MustInTx(ctx)
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx,
		`UPDATE lpb_documents SET counterpart_lpb_id = $2 WHERE id = $1`, id, counterpartID))
}

func (r *LPBRepo) List(ctx context.Context, f domain.LPBFilter, p domain.PageParams) ([]domain.LPBListItem, int64, error) {
	var b filterBuilder
	b.like(f.Query, "l.lpb_number", "l.reference_number", "s.name")
	if f.Type != nil {
		b.add("l.type = $%d", string(*f.Type))
	}
	if f.SubType != nil {
		b.add("l.sub_type = $%d", string(*f.SubType))
	}
	if f.Status != nil {
		b.add("l.status = $%d", string(*f.Status))
	}
	if f.WarehouseID != nil {
		b.add("l.warehouse_id = $%d", *f.WarehouseID)
	}
	if f.ScopeWarehouseID != nil {
		b.add("l.warehouse_id = $%d", *f.ScopeWarehouseID)
	}
	if f.DateFrom != nil {
		b.add("l.doc_date >= $%d::date", *f.DateFrom)
	}
	if f.DateTo != nil {
		b.add("l.doc_date <= $%d::date", *f.DateTo)
	}
	if f.CreatedBy != nil {
		b.add("l.created_by = $%d", *f.CreatedBy)
	}

	from := lpbFrom + ` LEFT JOIN (SELECT lpb_id, count(*) AS cnt, SUM(qty) AS qty FROM lpb_items GROUP BY lpb_id) ia ON ia.lpb_id = l.id`
	columns := lpbColumns + `, COALESCE(ia.cnt, 0)::int, COALESCE(ia.qty, 0)::int`
	return queryPage(ctx, database.GetExecutor(ctx, r.pool), from, columns, &b, p, "l.id",
		func(row pgx.Row) (domain.LPBListItem, error) {
			var it domain.LPBListItem
			l, err := scanLPBInto(row, &it.ItemCount, &it.TotalQty)
			it.LPB = l
			return it, err
		})
}

func (r *LPBRepo) UpdateHeader(ctx context.Context, l *domain.LPB) error {
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE lpb_documents SET doc_date = $2::date, supplier_id = $3, reference_type = $4,
		       reference_number = $5, notes = $6
		WHERE id = $1`, l.ID, l.DocDate, l.SupplierID, l.ReferenceType, l.ReferenceNumber, l.Notes))
}

func (r *LPBRepo) MarkSubmitted(ctx context.Context, id, userID int64, at time.Time) error {
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE lpb_documents SET status = 'PENDING_APPROVAL', submitted_by = $2, submitted_at = $3,
		       rejected_by = NULL, rejected_at = NULL, rejection_reason = NULL
		WHERE id = $1`, id, userID, at))
}

func (r *LPBRepo) MarkCancelled(ctx context.Context, id int64) error {
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx,
		`UPDATE lpb_documents SET status = 'CANCELLED' WHERE id = $1`, id))
}

func (r *LPBRepo) MarkRevised(ctx context.Context, id int64) error {
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE lpb_documents SET status = 'DRAFT', submitted_by = NULL, submitted_at = NULL WHERE id = $1`, id))
}

// ---------------------------------------------------------------- item

const itemFrom = `lpb_items i
	JOIN products p ON p.id = i.product_id
	JOIN racks r ON r.id = i.rack_id
	LEFT JOIN racks dr ON dr.id = i.dest_rack_id
	LEFT JOIN warehouses dw ON dw.id = dr.warehouse_id
	LEFT JOIN stock_balances sb ON sb.product_id = i.product_id AND sb.rack_id = i.rack_id
	LEFT JOIN stock_ledgers lg ON lg.lpb_item_id = i.id`

const itemColumns = `i.id, i.lpb_id, i.product_id, p.sku, p.name, p.unit, p.has_serial_number,
	i.rack_id, r.code, r.is_sellable, i.dest_rack_id, dr.code, dr.is_sellable, dr.warehouse_id, dw.code,
	i.qty, i.unit_cost::text, i.notes,
	COALESCE(sb.qty_on_hand, 0), lg.balance_before, lg.balance_after`

func scanItem(row pgx.Row) (domain.LPBItem, error) {
	var it domain.LPBItem
	var cost *string
	err := row.Scan(&it.ID, &it.LPBID, &it.ProductID, &it.SKU, &it.ProductName, &it.Unit, &it.HasSerialNumber,
		&it.RackID, &it.RackCode, &it.RackIsSellable, &it.DestRackID, &it.DestRackCode, &it.DestRackIsSellable,
		&it.DestWarehouseID, &it.DestWarehouseCode, &it.Qty, &cost, &it.Notes,
		&it.CurrentQty, &it.BalanceBefore, &it.BalanceAfter)
	if err != nil {
		return it, err
	}
	it.UnitCost, err = moneyFrom(cost)
	return it, err
}

func (r *LPBRepo) Items(ctx context.Context, lpbID int64) ([]domain.LPBItem, error) {
	db := database.GetExecutor(ctx, r.pool)
	rows, err := db.Query(ctx, `SELECT `+itemColumns+` FROM `+itemFrom+` WHERE i.lpb_id = $1 ORDER BY i.id`, lpbID)
	if err != nil {
		return nil, err
	}
	var items []domain.LPBItem
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	serials, err := r.serialsOf(ctx, `i.lpb_id = $1`, lpbID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Serials = serials[items[i].ID]
	}
	return items, nil
}

func (r *LPBRepo) FindItem(ctx context.Context, lpbID, itemID int64) (*domain.LPBItem, error) {
	it, err := scanItem(database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT `+itemColumns+` FROM `+itemFrom+` WHERE i.lpb_id = $1 AND i.id = $2`, lpbID, itemID))
	if err != nil {
		return nil, notFound(err)
	}
	serials, err := r.serialsOf(ctx, `i.id = $1`, itemID)
	if err != nil {
		return nil, err
	}
	it.Serials = serials[it.ID]
	return &it, nil
}

// serialsOf membaca SN item berdasarkan kondisi atas alias i (lpb_items), dikelompokkan per item.
func (r *LPBRepo) serialsOf(ctx context.Context, cond string, arg int64) (map[int64][]domain.LPBItemSerial, error) {
	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, `
		SELECT s.id, s.lpb_item_id, s.product_serial_id, s.serial_number, ps.asset_tag,
		       s.condition_on_scan, s.scanned_by, s.scanned_at
		FROM lpb_item_serials s
		JOIN lpb_items i ON i.id = s.lpb_item_id
		LEFT JOIN product_serials ps ON ps.id = s.product_serial_id
		WHERE `+cond+` ORDER BY s.id`, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int64][]domain.LPBItemSerial{}
	for rows.Next() {
		var s domain.LPBItemSerial
		var cond string
		if err := rows.Scan(&s.ID, &s.LPBItemID, &s.ProductSerialID, &s.SerialNumber, &s.AssetTag, &cond, &s.ScannedBy, &s.ScannedAt); err != nil {
			return nil, err
		}
		s.Condition = domain.SerialCondition(cond)
		out[s.LPBItemID] = append(out[s.LPBItemID], s)
	}
	return out, rows.Err()
}

func (r *LPBRepo) InsertItem(ctx context.Context, lpbID int64, it *domain.LPBItem) (int64, error) {
	var id int64
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO lpb_items (lpb_id, product_id, rack_id, dest_rack_id, qty, unit_cost, notes)
		VALUES ($1, $2, $3, $4, $5, $6::text::numeric, $7) RETURNING id`,
		lpbID, it.ProductID, it.RackID, it.DestRackID, it.Qty, moneyArg(it.UnitCost), it.Notes).Scan(&id)
	return id, duplicateOf(err, map[string]domain.DuplicateError{
		"lpb_items_lpb_id_product_id_rack_id_key": {Field: "items", Message: "produk dan rak yang sama muncul lebih dari sekali"},
	})
}

func (r *LPBRepo) UpdateItem(ctx context.Context, it *domain.LPBItem) error {
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE lpb_items SET qty = $2, unit_cost = $3::text::numeric, notes = $4, dest_rack_id = $5 WHERE id = $1`,
		it.ID, it.Qty, moneyArg(it.UnitCost), it.Notes, it.DestRackID))
}

func (r *LPBRepo) DeleteItem(ctx context.Context, itemID int64) error {
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `DELETE FROM lpb_items WHERE id = $1`, itemID))
}

// ---------------------------------------------------------------- serial pada item

func (r *LPBRepo) InsertItemSerial(ctx context.Context, s *domain.LPBItemSerial) (*domain.LPBItemSerial, error) {
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO lpb_item_serials (lpb_item_id, product_serial_id, serial_number, condition_on_scan, scanned_by)
		VALUES ($1, $2, $3, $4, $5) RETURNING id, scanned_at`,
		s.LPBItemID, s.ProductSerialID, s.SerialNumber, string(s.Condition), s.ScannedBy).Scan(&s.ID, &s.ScannedAt)
	if err != nil {
		return nil, duplicateOf(err, map[string]domain.DuplicateError{
			"lpb_item_serials_lpb_item_id_serial_number_key": {Field: "serial_number", Message: "serial number sudah di-scan pada item ini"},
		})
	}
	return s, nil
}

func (r *LPBRepo) DeleteItemSerial(ctx context.Context, itemID, serialRowID int64) error {
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx,
		`DELETE FROM lpb_item_serials WHERE id = $2 AND lpb_item_id = $1`, itemID, serialRowID))
}

func (r *LPBRepo) SerialClaimedByOtherLPB(ctx context.Context, productID int64, serialNumber string, productSerialID *int64, excludeLPBID int64) (string, bool, error) {
	var number string
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		SELECT l.lpb_number
		FROM lpb_item_serials s
		JOIN lpb_items i ON i.id = s.lpb_item_id
		JOIN lpb_documents l ON l.id = i.lpb_id
		WHERE i.product_id = $1 AND l.status IN ('DRAFT', 'PENDING_APPROVAL') AND l.id <> $3
		  AND (upper(s.serial_number) = upper($2) OR ($4::bigint IS NOT NULL AND s.product_serial_id = $4::bigint))
		ORDER BY l.id LIMIT 1`, productID, serialNumber, excludeLPBID, productSerialID).Scan(&number)
	if err != nil {
		if errors.Is(notFound(err), domain.ErrNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	return number, true, nil
}

func (r *LPBRepo) ProductSerialExists(ctx context.Context, productID int64, serialNumber string) (bool, error) {
	var exists bool
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM product_serials WHERE product_id = $1 AND upper(serial_number) = upper($2))`,
		productID, serialNumber).Scan(&exists)
	return exists, err
}

func (r *LPBRepo) SerialStates(ctx context.Context, ids []int64) ([]domain.SerialState, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx,
		`SELECT id, status, current_rack_id, asset_tag FROM product_serials WHERE id = ANY($1) ORDER BY id`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.SerialState
	for rows.Next() {
		var s domain.SerialState
		var status string
		if err := rows.Scan(&s.ID, &status, &s.RackID, &s.AssetTag); err != nil {
			return nil, err
		}
		s.Status = domain.SerialStatus(status)
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *LPBRepo) FreeQty(ctx context.Context, productID, rackID int64) (int, error) {
	var n int
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		SELECT COALESCE(SUM(qty_on_hand - qty_reserved), 0)::int
		FROM stock_balances WHERE product_id = $1 AND rack_id = $2`, productID, rackID).Scan(&n)
	return n, err
}

// ---------------------------------------------------------------- timeline

func (r *LPBRepo) Timeline(ctx context.Context, lpbID int64) ([]domain.LPBEvent, error) {
	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, `
		SELECT a.action, a.user_id, u.name, a.after_data::text, a.created_at
		FROM audit_logs a LEFT JOIN users u ON u.id = a.user_id
		WHERE a.entity_type = 'lpb' AND a.entity_id = $1
		ORDER BY a.created_at, a.id`, lpbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.LPBEvent{}
	for rows.Next() {
		var e domain.LPBEvent
		var data *string
		if err := rows.Scan(&e.Action, &e.UserID, &e.UserName, &data, &e.CreatedAt); err != nil {
			return nil, err
		}
		if data != nil {
			e.Data = json.RawMessage(*data)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *LPBRepo) LockSerialKey(ctx context.Context, productID int64, serialNumber string) error {
	database.MustInTx(ctx)
	_, err := database.GetExecutor(ctx, r.pool).Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`,
		fmt.Sprintf("lpb-serial:%d:%s", productID, strings.ToUpper(serialNumber)))
	return err
}

func (r *LPBRepo) MarkApproved(ctx context.Context, id, userID int64, at time.Time, verifyToken string) error {
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE lpb_documents
		SET status = 'APPROVED', approved_by = $2, approved_at = $3, verify_token = $4,
		    delivery_status = CASE WHEN requires_delivery THEN 'READY' ELSE delivery_status END
		WHERE id = $1`, id, userID, at, verifyToken))
}

func (r *LPBRepo) MarkRejected(ctx context.Context, id, userID int64, at time.Time, reason string) error {
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE lpb_documents SET status = 'REJECTED', rejected_by = $2, rejected_at = $3, rejection_reason = $4
		WHERE id = $1`, id, userID, at, reason))
}

func (r *LPBRepo) SetItemSerialLink(ctx context.Context, itemSerialID, productSerialID int64) error {
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx,
		`UPDATE lpb_item_serials SET product_serial_id = $2 WHERE id = $1`, itemSerialID, productSerialID))
}
