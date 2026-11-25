package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type ContractRepo struct{ pool *pgxpool.Pool }

func NewContractRepo(pool *pgxpool.Pool) *ContractRepo { return &ContractRepo{pool: pool} }

var _ domain.ContractRepository = (*ContractRepo)(nil)

const contractFrom = `rental_contracts rc
	JOIN customers c ON c.id = rc.customer_id
	JOIN users uc ON uc.id = rc.created_by
	LEFT JOIN users ua ON ua.id = rc.activated_by
	LEFT JOIN rental_contracts rf ON rf.id = rc.renewed_from_id`

const contractColumns = `rc.id, rc.contract_number, rc.customer_id, c.customer_code, c.company_name,
	rc.start_date, rc.end_date, rc.duration_months, rc.billing_day_of_month,
	rc.billing_mode, rc.billing_in_advance, rc.is_ppn, rc.payment_term_days,
	rc.deposit_amount, rc.status, rc.terminated_at, rc.termination_reason,
	rc.renewed_from_id, rf.contract_number, rc.signed_document_path, rc.notes,
	rc.created_by, uc.name, rc.activated_by, ua.name, rc.activated_at,
	rc.created_at, rc.updated_at,
	(SELECT COUNT(*) FROM rental_contract_items rci WHERE rci.contract_id = rc.id) AS item_count,
	(SELECT COALESCE(SUM(rci.qty), 0) FROM rental_contract_items rci WHERE rci.contract_id = rc.id) AS total_qty,
	(SELECT COALESCE(SUM(rci.qty * rci.monthly_rate), 0) FROM rental_contract_items rci WHERE rci.contract_id = rc.id) AS estimated_monthly,
	(SELECT COUNT(*) FROM rental_placements rp WHERE rp.contract_id = rc.id AND rp.end_date IS NULL) AS active_placement_qty`

func scanContract(row pgx.Row) (domain.RentalContract, error) {
	var rc domain.RentalContract
	var mode, status string
	err := row.Scan(
		&rc.ID, &rc.ContractNumber, &rc.CustomerID, &rc.CustomerCode, &rc.CustomerName,
		&rc.StartDate, &rc.EndDate, &rc.DurationMonths, &rc.BillingDayOfMonth,
		&mode, &rc.BillingInAdvance, &rc.IsPPN, &rc.PaymentTermDays,
		&rc.DepositAmount, &status, &rc.TerminatedAt, &rc.TerminationReason,
		&rc.RenewedFromID, &rc.RenewedFromNumber, &rc.SignedDocumentPath, &rc.Notes,
		&rc.CreatedBy, &rc.CreatedByName, &rc.ActivatedBy, &rc.ActivatedByName, &rc.ActivatedAt,
		&rc.CreatedAt, &rc.UpdatedAt,
		&rc.ItemCount, &rc.TotalQty, &rc.EstimatedMonthly, &rc.ActivePlacementQty,
	)
	rc.BillingMode = domain.BillingMode(mode)
	rc.Status = domain.ContractStatus(status)
	return rc, err
}

func (r *ContractRepo) FindByID(ctx context.Context, id int64) (*domain.RentalContract, error) {
	rc, err := scanContract(database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT `+contractColumns+` FROM `+contractFrom+` WHERE rc.id = $1`, id))
	if err != nil {
		return nil, notFound(err)
	}
	return &rc, nil
}

func (r *ContractRepo) FindByIDForUpdate(ctx context.Context, id int64) (*domain.RentalContract, error) {
	database.MustInTx(ctx)
	// Kunci hanya baris rental_contracts agar tidak gagal FOR UPDATE pada LEFT JOIN.
	var dummy int64
	if err := database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT id FROM rental_contracts WHERE id = $1 FOR UPDATE`, id).Scan(&dummy); err != nil {
		return nil, notFound(err)
	}
	return r.FindByID(ctx, id)
}

func (r *ContractRepo) List(ctx context.Context, f domain.ContractFilter, p domain.PageParams) ([]domain.RentalContract, int64, error) {
	var b filterBuilder
	b.like(f.Query, "rc.contract_number", "c.customer_code", "c.company_name")
	if f.CustomerID != nil {
		b.add("rc.customer_id = $%d", *f.CustomerID)
	}
	if f.Status != nil {
		b.add("rc.status = $%d", string(*f.Status))
	}
	if f.ExpiringIn != nil && *f.ExpiringIn > 0 {
		deadline := f.Today.AddDate(0, 0, *f.ExpiringIn)
		b.add("rc.status = 'ACTIVE' AND rc.end_date <= $%d", deadline)
	}
	return queryPage(ctx, database.GetExecutor(ctx, r.pool), contractFrom, contractColumns, &b, p, "rc.id",
		func(row pgx.Row) (domain.RentalContract, error) { return scanContract(row) })
}

func (r *ContractRepo) Create(ctx context.Context, rc *domain.RentalContract) (*domain.RentalContract, error) {
	var id int64
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO rental_contracts (
			contract_number, customer_id, start_date, end_date, duration_months,
			billing_day_of_month, billing_mode, billing_in_advance, is_ppn,
			payment_term_days, deposit_amount, status, renewed_from_id,
			signed_document_path, notes, created_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING id`,
		rc.ContractNumber, rc.CustomerID, rc.StartDate, rc.EndDate, rc.DurationMonths,
		rc.BillingDayOfMonth, string(rc.BillingMode), rc.BillingInAdvance, rc.IsPPN,
		rc.PaymentTermDays, rc.DepositAmount, string(rc.Status), rc.RenewedFromID,
		rc.SignedDocumentPath, rc.Notes, rc.CreatedBy,
	).Scan(&id)
	if err != nil {
		return nil, duplicateOf(err, map[string]domain.DuplicateError{
			"rental_contracts_contract_number_key": {Field: "contract_number", Message: "nomor kontrak sudah dipakai"},
		})
	}
	return r.FindByID(ctx, id)
}

func (r *ContractRepo) Update(ctx context.Context, rc *domain.RentalContract) error {
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE rental_contracts
		SET start_date = $2, end_date = $3, duration_months = $4,
		    billing_day_of_month = $5, billing_mode = $6, billing_in_advance = $7,
		    is_ppn = $8, payment_term_days = $9, deposit_amount = $10,
		    signed_document_path = $11, notes = $12
		WHERE id = $1`,
		rc.ID, rc.StartDate, rc.EndDate, rc.DurationMonths,
		rc.BillingDayOfMonth, string(rc.BillingMode), rc.BillingInAdvance,
		rc.IsPPN, rc.PaymentTermDays, rc.DepositAmount,
		rc.SignedDocumentPath, rc.Notes,
	))
}

func (r *ContractRepo) UpdateStatus(ctx context.Context, rc *domain.RentalContract) error {
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE rental_contracts
		SET status = $2, activated_by = $3, activated_at = $4,
		    terminated_at = $5, termination_reason = $6
		WHERE id = $1`,
		rc.ID, string(rc.Status), rc.ActivatedBy, rc.ActivatedAt,
		rc.TerminatedAt, rc.TerminationReason,
	))
}

// ---------------------------------------------------------------- items

func (r *ContractRepo) ListItems(ctx context.Context, contractID int64) ([]domain.RentalContractItem, error) {
	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, `
		SELECT rci.id, rci.contract_id, rci.product_id, p.sku, p.name,
		       rci.customer_site_id, cs.site_name, rci.qty, rci.monthly_rate, rci.notes
		FROM rental_contract_items rci
		JOIN products p ON p.id = rci.product_id
		LEFT JOIN customer_sites cs ON cs.id = rci.customer_site_id
		WHERE rci.contract_id = $1
		ORDER BY rci.id ASC`, contractID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.RentalContractItem, 0)
	for rows.Next() {
		var it domain.RentalContractItem
		if err := rows.Scan(
			&it.ID, &it.ContractID, &it.ProductID, &it.ProductSKU, &it.ProductName,
			&it.CustomerSiteID, &it.SiteName, &it.Qty, &it.MonthlyRate, &it.Notes,
		); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func (r *ContractRepo) ReplaceItems(ctx context.Context, contractID int64, items []domain.RentalContractItem) ([]domain.RentalContractItem, error) {
	database.MustInTx(ctx)
	exec := database.GetExecutor(ctx, r.pool)
	if _, err := exec.Exec(ctx, `DELETE FROM rental_contract_items WHERE contract_id = $1`, contractID); err != nil {
		return nil, err
	}
	for _, it := range items {
		_, err := exec.Exec(ctx, `
			INSERT INTO rental_contract_items (contract_id, product_id, customer_site_id, qty, monthly_rate, notes)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			contractID, it.ProductID, it.CustomerSiteID, it.Qty, it.MonthlyRate, it.Notes,
		)
		if err != nil {
			return nil, duplicateOf(err, map[string]domain.DuplicateError{
				"rental_contract_items_contract_id_product_id_customer_site__key": {
					Field: "items", Message: "kombinasi produk dan site tidak boleh duplikat dalam satu kontrak",
				},
			})
		}
	}
	return r.ListItems(ctx, contractID)
}
