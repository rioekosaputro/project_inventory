package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type InvoiceRepo struct{ pool *pgxpool.Pool }

func NewInvoiceRepo(pool *pgxpool.Pool) *InvoiceRepo { return &InvoiceRepo{pool: pool} }

var _ domain.InvoiceRepository = (*InvoiceRepo)(nil)

func (r *InvoiceRepo) ListBillableContracts(ctx context.Context, billingDay int, contractID *int64) ([]domain.BillableContract, error) {
	args := []any{billingDay}
	q := `SELECT rc.id, rc.contract_number, rc.customer_id, c.company_name,
	             rc.billing_day_of_month, rc.billing_mode, rc.is_ppn, rc.payment_term_days
	      FROM rental_contracts rc
	      JOIN customers c ON c.id = rc.customer_id
	      WHERE rc.status IN ('ACTIVE', 'EXPIRED', 'TERMINATED')
	        AND rc.billing_day_of_month = $1`
	if contractID != nil {
		args = append(args, *contractID)
		q += fmt.Sprintf(" AND rc.id = $%d", len(args))
	}
	q += ` ORDER BY rc.id ASC`

	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.BillableContract, 0)
	for rows.Next() {
		var bc domain.BillableContract
		var mode string
		if err := rows.Scan(
			&bc.ID, &bc.ContractNumber, &bc.CustomerID, &bc.CustomerName,
			&bc.BillingDayOfMonth, &mode, &bc.IsPPN, &bc.PaymentTermDays,
		); err != nil {
			return nil, err
		}
		bc.BillingMode = domain.BillingMode(mode)
		out = append(out, bc)
	}
	return out, rows.Err()
}

func (r *InvoiceRepo) HasActiveRentalInvoice(ctx context.Context, contractID int64, periodStart, periodEnd time.Time) (bool, error) {
	var exists bool
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM invoices
			WHERE contract_id = $1
			  AND invoice_type = 'RENTAL'
			  AND billing_period_start = $2::date
			  AND billing_period_end = $3::date
			  AND status <> 'VOID'
		)`, contractID, periodStart.Format("2006-01-02"), periodEnd.Format("2006-01-02"),
	).Scan(&exists)
	return exists, err
}

// ListPlacementsForPeriod menjalankan query irisan periode [start_date, end_date) dengan [ps, pe + 1) (plan Bagian 6.5).
func (r *InvoiceRepo) ListPlacementsForPeriod(ctx context.Context, contractID int64, periodStart, periodEnd time.Time) ([]domain.BillablePlacement, error) {
	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, `
		WITH p AS (SELECT $2::date AS ps, $3::date AS pe)
		SELECT rp.id, ps.product_id, pr.name, rp.product_serial_id, ps.serial_number,
		       rp.customer_site_id, cs.site_name, rp.monthly_rate,
		       GREATEST(rp.start_date, p.ps)                                                    AS bill_from,
		       LEAST(COALESCE(rp.end_date, p.pe + 1), p.pe + 1)                                 AS bill_to_excl,
		       (LEAST(COALESCE(rp.end_date, p.pe + 1), p.pe + 1) - GREATEST(rp.start_date, p.ps))::int AS days_billed,
		       (p.pe - p.ps + 1)::int                                                           AS days_in_period
		FROM rental_placements rp
		JOIN product_serials ps ON ps.id = rp.product_serial_id
		JOIN products pr ON pr.id = ps.product_id
		JOIN customer_sites cs ON cs.id = rp.customer_site_id, p
		WHERE rp.contract_id = $1
		  AND rp.start_date <= p.pe
		  AND (rp.end_date IS NULL OR rp.end_date > p.ps)
		ORDER BY rp.customer_site_id ASC, rp.product_serial_id ASC, rp.id ASC`,
		contractID, periodStart.Format("2006-01-02"), periodEnd.Format("2006-01-02"),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.BillablePlacement, 0)
	for rows.Next() {
		var bp domain.BillablePlacement
		if err := rows.Scan(
			&bp.PlacementID, &bp.ProductID, &bp.ProductName, &bp.ProductSerialID, &bp.SerialNumber,
			&bp.CustomerSiteID, &bp.SiteName, &bp.MonthlyRate,
			&bp.BillFrom, &bp.BillToExcl, &bp.DaysBilled, &bp.DaysInPeriod,
		); err != nil {
			return nil, err
		}
		if bp.DaysBilled > 0 {
			out = append(out, bp)
		}
	}
	return out, rows.Err()
}

func (r *InvoiceRepo) CreateInvoiceWithItems(ctx context.Context, inv *domain.Invoice, items []domain.InvoiceItem) (*domain.Invoice, error) {
	database.MustInTx(ctx)
	exec := database.GetExecutor(ctx, r.pool)
	err := exec.QueryRow(ctx, `
		INSERT INTO invoices (
			invoice_number, invoice_type, customer_id, contract_id, so_id,
			billing_period_start, billing_period_end, invoice_date, due_date,
			status, is_ppn, subtotal, discount_amount, dpp, dpp_other_value,
			tax_rate, tax_amount, grand_total, paid_amount, notes,
			generated_by_job, created_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, 0, $19, $20, $21)
		RETURNING id, created_at, updated_at`,
		inv.InvoiceNumber, string(inv.InvoiceType), inv.CustomerID, inv.ContractID, inv.SOID,
		inv.BillingPeriodStart, inv.BillingPeriodEnd, inv.InvoiceDate, inv.DueDate,
		string(inv.Status), inv.IsPPN, inv.Subtotal, inv.DiscountAmount, inv.DPP, inv.DPPOtherValue,
		inv.TaxRate, inv.TaxAmount, inv.GrandTotal, inv.Notes,
		inv.GeneratedByJob, inv.CreatedBy,
	).Scan(&inv.ID, &inv.CreatedAt, &inv.UpdatedAt)
	if err != nil {
		return nil, duplicateOf(err, map[string]domain.DuplicateError{
			"uq_invoice_rental_period": {Field: "contract_id", Message: "invoice sewa untuk kontrak dan periode ini sudah ada"},
		})
	}

	for _, it := range items {
		if _, err := exec.Exec(ctx, `
			INSERT INTO invoice_items (
				invoice_id, description, product_id, product_serial_id, customer_site_id,
				rental_placement_id, period_start, period_end, days_billed, days_in_period,
				qty, unit_price, line_total
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
			inv.ID, it.Description, it.ProductID, it.ProductSerialID, it.CustomerSiteID,
			it.RentalPlacementID, it.PeriodStart, it.PeriodEnd, it.DaysBilled, it.DaysInPeriod,
			it.Qty, it.UnitPrice, it.LineTotal,
		); err != nil {
			return nil, err
		}
	}
	return inv, nil
}
