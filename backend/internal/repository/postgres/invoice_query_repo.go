package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

const invoiceFrom = `invoices i
	JOIN customers c ON c.id = i.customer_id
	LEFT JOIN rental_contracts rc ON rc.id = i.contract_id
	LEFT JOIN sales_orders so ON so.id = i.so_id`

const invoiceColumns = `i.id, i.invoice_number, i.invoice_type, i.customer_id, c.customer_code, c.company_name,
	c.npwp, c.billing_address, i.contract_id, rc.contract_number, i.so_id, so.so_number,
	i.billing_period_start, i.billing_period_end, i.invoice_date, i.due_date, i.status, i.is_ppn,
	i.subtotal, i.discount_amount, i.dpp, i.dpp_other_value, i.tax_rate, i.tax_amount, i.grand_total,
	i.paid_amount, i.faktur_pajak_number, i.pdf_path, i.notes, i.generated_by_job,
	i.created_by, i.issued_by, i.issued_at, i.voided_by, i.voided_at, i.void_reason,
	i.created_at, i.updated_at`

func scanInvoice(row pgx.Row) (domain.Invoice, error) {
	var inv domain.Invoice
	var itype, status string
	err := row.Scan(
		&inv.ID, &inv.InvoiceNumber, &itype, &inv.CustomerID, &inv.CustomerCode, &inv.CustomerName,
		&inv.CustomerNPWP, &inv.CustomerBillingAddress, &inv.ContractID, &inv.ContractNumber, &inv.SOID, &inv.SONumber,
		&inv.BillingPeriodStart, &inv.BillingPeriodEnd, &inv.InvoiceDate, &inv.DueDate, &status, &inv.IsPPN,
		&inv.Subtotal, &inv.DiscountAmount, &inv.DPP, &inv.DPPOtherValue, &inv.TaxRate, &inv.TaxAmount, &inv.GrandTotal,
		&inv.PaidAmount, &inv.FakturPajakNumber, &inv.PDFPath, &inv.Notes, &inv.GeneratedByJob,
		&inv.CreatedBy, &inv.IssuedBy, &inv.IssuedAt, &inv.VoidedBy, &inv.VoidedAt, &inv.VoidReason,
		&inv.CreatedAt, &inv.UpdatedAt,
	)
	inv.InvoiceType = domain.InvoiceType(itype)
	inv.Status = domain.InvoiceStatus(status)
	return inv, err
}

func buildInvoiceFilter(f domain.InvoiceFilter) filterBuilder {
	var b filterBuilder
	b.like(f.Query, "i.invoice_number", "c.customer_code", "c.company_name", "i.faktur_pajak_number")
	if f.InvoiceType != nil {
		b.add("i.invoice_type = $%d", string(*f.InvoiceType))
	}
	if f.Status != nil {
		b.add("i.status = $%d", string(*f.Status))
	}
	if f.CustomerID != nil {
		b.add("i.customer_id = $%d", *f.CustomerID)
	}
	if f.ContractID != nil {
		b.add("i.contract_id = $%d", *f.ContractID)
	}
	if f.DateFrom != nil {
		b.add("i.invoice_date >= $%d::date", f.DateFrom.Format("2006-01-02"))
	}
	if f.DateTo != nil {
		b.add("i.invoice_date <= $%d::date", f.DateTo.Format("2006-01-02"))
	}
	if f.OverdueOnly {
		b.add("i.status IN ('ISSUED', 'PARTIALLY_PAID') AND i.due_date < $%d::date", f.Today.Format("2006-01-02"))
	}
	return b
}

func (r *InvoiceRepo) FindByID(ctx context.Context, id int64) (*domain.Invoice, error) {
	inv, err := scanInvoice(database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT `+invoiceColumns+` FROM `+invoiceFrom+` WHERE i.id = $1`, id))
	if err != nil {
		return nil, notFound(err)
	}
	return &inv, nil
}

func (r *InvoiceRepo) FindByIDForUpdate(ctx context.Context, id int64) (*domain.Invoice, error) {
	database.MustInTx(ctx)
	var dummy int64
	if err := database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT id FROM invoices WHERE id = $1 FOR UPDATE`, id).Scan(&dummy); err != nil {
		return nil, notFound(err)
	}
	return r.FindByID(ctx, id)
}

func (r *InvoiceRepo) List(ctx context.Context, f domain.InvoiceFilter, p domain.PageParams) ([]domain.Invoice, int64, error) {
	b := buildInvoiceFilter(f)
	return queryPage(ctx, database.GetExecutor(ctx, r.pool), invoiceFrom, invoiceColumns, &b, p, "i.id",
		func(row pgx.Row) (domain.Invoice, error) { return scanInvoice(row) })
}

func (r *InvoiceRepo) ListAll(ctx context.Context, f domain.InvoiceFilter) ([]domain.Invoice, error) {
	b := buildInvoiceFilter(f)
	q := `SELECT ` + invoiceColumns + ` FROM ` + invoiceFrom + b.where() + ` ORDER BY i.invoice_date DESC, i.id DESC LIMIT 5000`
	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, q, b.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Invoice, 0)
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

func (r *InvoiceRepo) ListItems(ctx context.Context, invoiceID int64) ([]domain.InvoiceItem, error) {
	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, `
		SELECT id, invoice_id, description, product_id, product_serial_id, customer_site_id,
		       rental_placement_id, period_start, period_end, days_billed, days_in_period,
		       qty, unit_price, line_total
		FROM invoice_items
		WHERE invoice_id = $1
		ORDER BY id ASC`, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.InvoiceItem, 0)
	for rows.Next() {
		var it domain.InvoiceItem
		if err := rows.Scan(
			&it.ID, &it.InvoiceID, &it.Description, &it.ProductID, &it.ProductSerialID, &it.CustomerSiteID,
			&it.RentalPlacementID, &it.PeriodStart, &it.PeriodEnd, &it.DaysBilled, &it.DaysInPeriod,
			&it.Qty, &it.UnitPrice, &it.LineTotal,
		); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *InvoiceRepo) ListPayments(ctx context.Context, invoiceID int64) ([]domain.Payment, error) {
	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, `
		SELECT p.id, p.invoice_id, p.payment_date, p.amount_received, p.pph23_amount,
		       p.pph23_slip_number, p.method, p.reference_number, p.proof_path, p.notes,
		       p.recorded_by, u.name, p.created_at
		FROM payments p
		JOIN users u ON u.id = p.recorded_by
		WHERE p.invoice_id = $1
		ORDER BY p.payment_date ASC, p.id ASC`, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Payment, 0)
	for rows.Next() {
		var pm domain.Payment
		var method string
		if err := rows.Scan(
			&pm.ID, &pm.InvoiceID, &pm.PaymentDate, &pm.AmountReceived, &pm.PPh23Amount,
			&pm.PPh23SlipNumber, &method, &pm.ReferenceNumber, &pm.ProofPath, &pm.Notes,
			&pm.RecordedBy, &pm.RecordedByName, &pm.CreatedAt,
		); err != nil {
			return nil, err
		}
		pm.Method = domain.PaymentMethod(method)
		out = append(out, pm)
	}
	return out, rows.Err()
}

func (r *InvoiceRepo) MarkIssued(ctx context.Context, id, userID int64, at time.Time, fakturPajak *string) error {
	database.MustInTx(ctx)
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE invoices
		SET status = 'ISSUED', issued_by = $2, issued_at = $3,
		    faktur_pajak_number = COALESCE($4, faktur_pajak_number)
		WHERE id = $1`, id, userID, at, fakturPajak))
}

func (r *InvoiceRepo) MarkVoided(ctx context.Context, id, userID int64, at time.Time, reason string) error {
	database.MustInTx(ctx)
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE invoices
		SET status = 'VOID', voided_by = $2, voided_at = $3, void_reason = $4
		WHERE id = $1`, id, userID, at, reason))
}

func (r *InvoiceRepo) InsertPaymentAndUpdateInvoice(ctx context.Context, p *domain.Payment, newPaid decimal.Decimal, newStatus domain.InvoiceStatus) (*domain.Payment, error) {
	database.MustInTx(ctx)
	exec := database.GetExecutor(ctx, r.pool)
	err := exec.QueryRow(ctx, `
		INSERT INTO payments (
			invoice_id, payment_date, amount_received, pph23_amount, pph23_slip_number,
			method, reference_number, proof_path, notes, recorded_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at`,
		p.InvoiceID, p.PaymentDate, p.AmountReceived, p.PPh23Amount, p.PPh23SlipNumber,
		string(p.Method), p.ReferenceNumber, p.ProofPath, p.Notes, p.RecordedBy,
	).Scan(&p.ID, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	if err := mustAffectOne(exec.Exec(ctx, `
		UPDATE invoices SET paid_amount = $2, status = $3 WHERE id = $1`,
		p.InvoiceID, newPaid, string(newStatus))); err != nil {
		return nil, err
	}
	return p, nil
}
