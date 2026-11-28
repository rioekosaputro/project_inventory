package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type TrackingRepo struct{ pool *pgxpool.Pool }

func NewTrackingRepo(pool *pgxpool.Pool) *TrackingRepo {
	return &TrackingRepo{pool: pool}
}

var _ domain.TrackingRepository = (*TrackingRepo)(nil)

func (r *TrackingRepo) ListMyDeliveries(ctx context.Context, driverID int64) ([]domain.DriverDeliveryTask, error) {
	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, `
		SELECT
			l.id, l.lpb_number, l.type, l.sub_type, l.status, l.delivery_status,
			TO_CHAR(l.doc_date, 'YYYY-MM-DD'),
			l.warehouse_id, w.code, w.name,
			l.customer_id, c.company_name,
			l.customer_site_id, cs.site_name, cs.address_line, cs.city,
			cs.pic_name, cs.pic_phone, cs.latitude::float8, cs.longitude::float8,
			COALESCE(agg.item_count, 0), COALESCE(agg.total_qty, 0),
			l.notes
		FROM lpb_documents l
		JOIN warehouses w ON w.id = l.warehouse_id
		LEFT JOIN customers c ON c.id = l.customer_id
		LEFT JOIN customer_sites cs ON cs.id = l.customer_site_id
		LEFT JOIN (
			SELECT lpb_id, COUNT(*) AS item_count, COALESCE(SUM(qty), 0) AS total_qty
			FROM lpb_items
			GROUP BY lpb_id
		) agg ON agg.lpb_id = l.id
		WHERE l.driver_id = $1
		  AND (
			(l.sub_type IN ('RENTAL_DELIVERY', 'CONSUMABLE_DELIVERY')
			 AND l.status = 'APPROVED'
			 AND l.requires_delivery = TRUE
			 AND COALESCE(l.delivery_status, '') NOT IN ('DELIVERED', 'FAILED', 'RETURNED'))
			OR
			(l.sub_type = 'RENTAL_RETURN'
			 AND l.status IN ('DRAFT', 'PENDING_APPROVAL')
			 AND COALESCE(l.delivery_status, '') NOT IN ('PICKED_UP', 'DELIVERED', 'FAILED', 'RETURNED'))
		  )
		ORDER BY l.doc_date ASC, l.id ASC`, driverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.DriverDeliveryTask
	for rows.Next() {
		var t domain.DriverDeliveryTask
		var ltype, stype, status string
		if err := rows.Scan(
			&t.LPBID, &t.LPBNumber, &ltype, &stype, &status, &t.DeliveryStatus,
			&t.DocDate,
			&t.WarehouseID, &t.WarehouseCode, &t.WarehouseName,
			&t.CustomerID, &t.CustomerName,
			&t.CustomerSiteID, &t.SiteName, &t.SiteAddress, &t.SiteCity,
			&t.SitePICName, &t.SitePICPhone, &t.SiteLatitude, &t.SiteLongitude,
			&t.ItemCount, &t.TotalQty,
			&t.Notes,
		); err != nil {
			return nil, err
		}
		t.Type = domain.LPBType(ltype)
		t.SubType = domain.LPBSubType(stype)
		t.Status = domain.LPBStatus(status)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *TrackingRepo) FindByClientEventID(ctx context.Context, clientEventID string) (*domain.ShipmentTrackingLog, error) {
	var l domain.ShipmentTrackingLog
	var st string
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		SELECT
			t.id, t.lpb_id, t.driver_id, u.name, t.status,
			t.latitude::float8, t.longitude::float8, t.accuracy_m,
			t.recorded_at, t.client_time, t.client_event_id::text,
			t.receiver_name, t.photo_path, t.failure_reason, t.notes
		FROM shipment_tracking_logs t
		JOIN users u ON u.id = t.driver_id
		WHERE t.client_event_id = $1::uuid`, clientEventID).Scan(
		&l.ID, &l.LPBID, &l.DriverID, &l.DriverName, &st,
		&l.Latitude, &l.Longitude, &l.AccuracyM,
		&l.RecordedAt, &l.ClientTime, &l.ClientEventID,
		&l.ReceiverName, &l.PhotoPath, &l.FailureReason, &l.Notes,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	l.Status = domain.TrackingStatus(st)
	return &l, nil
}

func (r *TrackingRepo) ListByLPB(ctx context.Context, lpbID int64) ([]domain.ShipmentTrackingLog, error) {
	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, `
		SELECT
			t.id, t.lpb_id, t.driver_id, u.name, t.status,
			t.latitude::float8, t.longitude::float8, t.accuracy_m,
			t.recorded_at, t.client_time, t.client_event_id::text,
			t.receiver_name, t.photo_path, t.failure_reason, t.notes
		FROM shipment_tracking_logs t
		JOIN users u ON u.id = t.driver_id
		WHERE t.lpb_id = $1
		ORDER BY t.recorded_at ASC, t.id ASC`, lpbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.ShipmentTrackingLog
	for rows.Next() {
		var l domain.ShipmentTrackingLog
		var st string
		if err := rows.Scan(
			&l.ID, &l.LPBID, &l.DriverID, &l.DriverName, &st,
			&l.Latitude, &l.Longitude, &l.AccuracyM,
			&l.RecordedAt, &l.ClientTime, &l.ClientEventID,
			&l.ReceiverName, &l.PhotoPath, &l.FailureReason, &l.Notes,
		); err != nil {
			return nil, err
		}
		l.Status = domain.TrackingStatus(st)
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *TrackingRepo) Insert(ctx context.Context, log *domain.ShipmentTrackingLog) error {
	database.MustInTx(ctx)
	return database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO shipment_tracking_logs (
			lpb_id, driver_id, status, latitude, longitude, accuracy_m,
			recorded_at, client_time, client_event_id,
			receiver_name, photo_path, failure_reason, notes
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, '')::uuid, $10, $11, $12, $13)
		RETURNING id, recorded_at`,
		log.LPBID, log.DriverID, string(log.Status), log.Latitude, log.Longitude, log.AccuracyM,
		log.RecordedAt, log.ClientTime, derefStr(log.ClientEventID),
		log.ReceiverName, log.PhotoPath, log.FailureReason, log.Notes,
	).Scan(&log.ID, &log.RecordedAt)
}

func (r *TrackingRepo) UpdateLPBDeliveryStatus(ctx context.Context, lpbID int64, status domain.TrackingStatus) error {
	database.MustInTx(ctx)
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE lpb_documents SET delivery_status = $2 WHERE id = $1`, lpbID, string(status)))
}

func (r *TrackingRepo) SiteCoordinates(ctx context.Context, siteID int64) (*domain.SiteCoords, error) {
	var c domain.SiteCoords
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		SELECT latitude::float8, longitude::float8 FROM customer_sites WHERE id = $1`, siteID).
		Scan(&c.Latitude, &c.Longitude)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
