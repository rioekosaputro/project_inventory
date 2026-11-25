package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type DeliveryPODRepo struct{ pool *pgxpool.Pool }

func NewDeliveryPODRepo(pool *pgxpool.Pool) *DeliveryPODRepo {
	return &DeliveryPODRepo{pool: pool}
}

var _ domain.DeliveryPODRepository = (*DeliveryPODRepo)(nil)

func (r *DeliveryPODRepo) MarkDelivered(ctx context.Context, lpbID int64, deliveredAt time.Time, receiverName string, podPhotoPath *string) error {
	database.MustInTx(ctx)
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE lpb_documents
		SET delivery_status = 'DELIVERED', delivered_at = $2, receiver_name = $3, pod_photo_path = $4
		WHERE id = $1`, lpbID, deliveredAt, receiverName, podPhotoPath))
}

func (r *DeliveryPODRepo) MarkPickedUp(ctx context.Context, lpbID int64) error {
	database.MustInTx(ctx)
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE lpb_documents SET delivery_status = 'PICKED_UP' WHERE id = $1`, lpbID))
}

func (r *DeliveryPODRepo) LookupDOContext(ctx context.Context, doID int64) (*domain.DOContext, error) {
	var c domain.DOContext
	var otype string
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		SELECT d.id, d.so_id, so.order_type, so.contract_id, so.customer_id, so.customer_site_id
		FROM delivery_orders d
		JOIN sales_orders so ON so.id = d.so_id
		WHERE d.id = $1`, doID).Scan(
		&c.DOID, &c.SOID, &otype, &c.ContractID, &c.CustomerID, &c.SiteID,
	)
	if err != nil {
		return nil, notFound(err)
	}
	c.OrderType = domain.OrderType(otype)
	return &c, nil
}

func (r *DeliveryPODRepo) FindContractItemRate(ctx context.Context, contractID, productID, siteID int64) (*int64, decimal.Decimal, error) {
	var itemID int64
	var rate decimal.Decimal
	// Prioritaskan item kontrak yang cocok site-nya; fallback ke item kontrak tanpa site spesifik.
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		SELECT id, monthly_rate
		FROM rental_contract_items
		WHERE contract_id = $1 AND product_id = $2
		  AND (customer_site_id = $3 OR customer_site_id IS NULL)
		ORDER BY CASE WHEN customer_site_id = $3 THEN 0 ELSE 1 END, id ASC
		LIMIT 1`, contractID, productID, siteID).Scan(&itemID, &rate)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, decimal.Zero, nil
		}
		return nil, decimal.Zero, err
	}
	return &itemID, rate, nil
}

func (r *DeliveryPODRepo) MarkSerialRented(ctx context.Context, serialID, customerID, siteID int64, contractID *int64) error {
	database.MustInTx(ctx)
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE product_serials
		SET status = 'RENTED', current_warehouse_id = NULL, current_rack_id = NULL,
		    current_customer_id = $2, current_site_id = $3, current_contract_id = $4,
		    reserved_for_do_id = NULL, updated_at = NOW()
		WHERE id = $1`, serialID, customerID, siteID, contractID))
}

func (r *DeliveryPODRepo) InsertRentalPlacement(
	ctx context.Context,
	contractID int64,
	contractItemID *int64,
	serialID, siteID int64,
	monthlyRate decimal.Decimal,
	startDate string,
	startLPBID int64,
) error {
	database.MustInTx(ctx)
	_, err := database.GetExecutor(ctx, r.pool).Exec(ctx, `
		INSERT INTO rental_placements (
			contract_id, contract_item_id, product_serial_id, customer_site_id,
			monthly_rate, start_date, start_lpb_id
		)
		VALUES ($1, $2, $3, $4, $5, $6::date, $7)`,
		contractID, contractItemID, serialID, siteID, monthlyRate, startDate, startLPBID)
	return err
}

func (r *DeliveryPODRepo) MarkSerialPickupInTransit(ctx context.Context, serialID int64) error {
	database.MustInTx(ctx)
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE product_serials
		SET status = 'IN_TRANSIT', current_warehouse_id = NULL, current_rack_id = NULL, updated_at = NOW()
		WHERE id = $1 AND status = 'RENTED'`, serialID))
}

func (r *DeliveryPODRepo) CloseActivePlacement(ctx context.Context, serialID int64, endDate, endReason string, endLPBID int64) error {
	database.MustInTx(ctx)
	_, err := database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE rental_placements
		SET end_date = GREATEST(start_date, $2::date), end_reason = $3, end_lpb_id = $4
		WHERE product_serial_id = $1 AND end_date IS NULL`,
		serialID, endDate, endReason, endLPBID)
	return err
}

func (r *DeliveryPODRepo) CompleteDOAndMaybeSO(ctx context.Context, doID int64) error {
	database.MustInTx(ctx)
	exec := database.GetExecutor(ctx, r.pool)
	var soID int64
	if err := exec.QueryRow(ctx, `
		UPDATE delivery_orders SET status = 'COMPLETED' WHERE id = $1 RETURNING so_id`, doID).Scan(&soID); err != nil {
		return notFound(err)
	}
	var remaining int
	if err := exec.QueryRow(ctx, `
		SELECT COUNT(*) FROM delivery_orders
		WHERE so_id = $1 AND status NOT IN ('COMPLETED', 'CANCELLED')`, soID).Scan(&remaining); err != nil {
		return err
	}
	if remaining == 0 {
		_, err := exec.Exec(ctx, `UPDATE sales_orders SET status = 'COMPLETED' WHERE id = $1 AND status = 'IN_PROGRESS'`, soID)
		return err
	}
	return nil
}
