package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type JobRepo struct {
	pool *pgxpool.Pool
}

func NewJobRepo(pool *pgxpool.Pool) *JobRepo {
	return &JobRepo{pool: pool}
}

func (r *JobRepo) TryAcquire(ctx context.Context, jobName, runKey string) (*domain.JobRun, bool, error) {
	db := database.GetExecutor(ctx, r.pool)

	// 1. Coba insert baru.
	run, err := scanJobRun(db.QueryRow(ctx, `
		INSERT INTO job_runs (job_name, run_key, status, started_at)
		VALUES ($1, $2, 'RUNNING', NOW())
		ON CONFLICT (job_name, run_key) DO NOTHING
		RETURNING id, job_name, run_key, status, summary, error, started_at, finished_at
	`, jobName, runKey))
	if err == nil {
		return run, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf("job_runs insert: %w", err)
	}

	// 2. Sudah ada: coba ambil alih bila status sebelumnya FAILED (retry atomik).
	run, err = scanJobRun(db.QueryRow(ctx, `
		UPDATE job_runs
		SET status = 'RUNNING', started_at = NOW(), finished_at = NULL, error = NULL, summary = NULL
		WHERE job_name = $1 AND run_key = $2 AND status = 'FAILED'
		RETURNING id, job_name, run_key, status, summary, error, started_at, finished_at
	`, jobName, runKey))
	if err == nil {
		return run, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf("job_runs retry: %w", err)
	}

	// 3. Sudah SUCCESS atau sedang RUNNING: kembalikan catatan yang ada (acquired=false).
	existing, err := scanJobRun(db.QueryRow(ctx, `
		SELECT id, job_name, run_key, status, summary, error, started_at, finished_at
		FROM job_runs
		WHERE job_name = $1 AND run_key = $2
	`, jobName, runKey))
	if err != nil {
		return nil, false, fmt.Errorf("job_runs get existing: %w", err)
	}
	return existing, false, nil
}

func (r *JobRepo) MarkSuccess(ctx context.Context, id int64, summary map[string]any) error {
	db := database.GetExecutor(ctx, r.pool)
	raw, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("marshal job summary: %w", err)
	}
	_, err = db.Exec(ctx, `
		UPDATE job_runs
		SET status = 'SUCCESS', summary = $2::jsonb, error = NULL, finished_at = NOW()
		WHERE id = $1
	`, id, raw)
	return err
}

func (r *JobRepo) MarkFailed(ctx context.Context, id int64, summary map[string]any, errMsg string) error {
	db := database.GetExecutor(ctx, r.pool)
	var raw []byte
	if summary != nil {
		var err error
		raw, err = json.Marshal(summary)
		if err != nil {
			return fmt.Errorf("marshal job summary: %w", err)
		}
	}
	_, err := db.Exec(ctx, `
		UPDATE job_runs
		SET status = 'FAILED', summary = $2::jsonb, error = $3, finished_at = NOW()
		WHERE id = $1
	`, id, raw, errMsg)
	return err
}

func (r *JobRepo) ListRecent(ctx context.Context, limit int) ([]domain.JobRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	db := database.GetExecutor(ctx, r.pool)
	rows, err := db.Query(ctx, `
		SELECT id, job_name, run_key, status, summary, error, started_at, finished_at
		FROM job_runs
		ORDER BY started_at DESC, id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.JobRun{}
	for rows.Next() {
		run, err := scanJobRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *run)
	}
	return out, rows.Err()
}

func (r *JobRepo) ExpireElapsedContracts(ctx context.Context, today time.Time) (int64, error) {
	db := database.GetExecutor(ctx, r.pool)
	tag, err := db.Exec(ctx, `
		UPDATE rental_contracts
		SET status = 'EXPIRED', updated_at = NOW()
		WHERE status = 'ACTIVE' AND end_date < $1::date
	`, today.Format("2006-01-02"))
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *JobRepo) FindExpiringContracts(ctx context.Context, today time.Time, withinDays int) ([]domain.ExpiringContractAlert, error) {
	db := database.GetExecutor(ctx, r.pool)
	todayStr := today.Format("2006-01-02")
	untilStr := today.AddDate(0, 0, withinDays).Format("2006-01-02")

	rows, err := db.Query(ctx, `
		SELECT rc.id, rc.contract_number, rc.customer_id, c.company_name,
		       TO_CHAR(rc.end_date, 'YYYY-MM-DD'),
		       (rc.end_date - $1::date) AS days_remaining
		FROM rental_contracts rc
		JOIN customers c ON c.id = rc.customer_id
		WHERE rc.status = 'ACTIVE'
		  AND rc.end_date >= $1::date
		  AND rc.end_date <= $2::date
		ORDER BY rc.end_date ASC, rc.id ASC
	`, todayStr, untilStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.ExpiringContractAlert{}
	for rows.Next() {
		var a domain.ExpiringContractAlert
		if err := rows.Scan(&a.ContractID, &a.ContractNumber, &a.CustomerID, &a.CustomerName, &a.EndDate, &a.DaysRemaining); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *JobRepo) FindOverdueInvoices(ctx context.Context, today time.Time) ([]domain.OverdueInvoiceAlert, error) {
	db := database.GetExecutor(ctx, r.pool)
	todayStr := today.Format("2006-01-02")
	rows, err := db.Query(ctx, `
		SELECT i.id, i.invoice_number, i.customer_id, c.company_name,
		       TO_CHAR(i.due_date, 'YYYY-MM-DD'),
		       ($1::date - i.due_date) AS days_overdue,
		       TO_CHAR(i.grand_total - i.paid_amount, 'FM999999999999990.00') AS remaining_amount
		FROM invoices i
		JOIN customers c ON c.id = i.customer_id
		WHERE i.status IN ('ISSUED', 'PARTIALLY_PAID')
		  AND i.due_date < $1::date
		ORDER BY i.due_date ASC, i.id ASC
	`, todayStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.OverdueInvoiceAlert{}
	for rows.Next() {
		var a domain.OverdueInvoiceAlert
		if err := rows.Scan(&a.InvoiceID, &a.InvoiceNumber, &a.CustomerID, &a.CustomerName, &a.DueDate, &a.DaysOverdue, &a.RemainingAmount); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *JobRepo) FindLowStockProducts(ctx context.Context) ([]domain.JobLowStockItem, error) {
	db := database.GetExecutor(ctx, r.pool)
	rows, err := db.Query(ctx, `
		SELECT p.id, p.sku, p.name,
		       COALESCE(SUM(CASE WHEN r.is_sellable THEN sb.qty_on_hand - sb.qty_reserved ELSE 0 END), 0)::int AS qty_available,
		       p.min_stock_alert
		FROM products p
		LEFT JOIN stock_balances sb ON sb.product_id = p.id
		LEFT JOIN racks r ON r.id = sb.rack_id
		WHERE p.is_active = TRUE AND p.min_stock_alert > 0
		GROUP BY p.id, p.sku, p.name, p.min_stock_alert
		HAVING COALESCE(SUM(CASE WHEN r.is_sellable THEN sb.qty_on_hand - sb.qty_reserved ELSE 0 END), 0) < p.min_stock_alert
		ORDER BY qty_available ASC, p.id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.JobLowStockItem{}
	for rows.Next() {
		var item domain.JobLowStockItem
		if err := rows.Scan(&item.ProductID, &item.SKU, &item.ProductName, &item.QtyAvailable, &item.MinStockAlert); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *JobRepo) FindStaleDOReservations(ctx context.Context, olderThan time.Time) ([]domain.StaleDOAlertItem, error) {
	db := database.GetExecutor(ctx, r.pool)
	rows, err := db.Query(ctx, `
		SELECT d.id, d.do_number, d.warehouse_id, d.status,
		       MIN(res.reserved_at) AS oldest_reserved_at,
		       COUNT(res.id)::int AS reserved_count
		FROM delivery_orders d
		JOIN delivery_order_items di ON di.do_id = d.id
		JOIN delivery_order_reservations res ON res.do_item_id = di.id
		WHERE d.status IN ('OPEN', 'RESERVED')
		  AND res.released_at IS NULL
		GROUP BY d.id, d.do_number, d.warehouse_id, d.status
		HAVING MIN(res.reserved_at) < $1
		ORDER BY oldest_reserved_at ASC, d.id ASC
	`, olderThan)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.StaleDOAlertItem{}
	for rows.Next() {
		var item domain.StaleDOAlertItem
		if err := rows.Scan(&item.DOID, &item.DONumber, &item.WarehouseID, &item.Status, &item.OldestReservedAt, &item.ReservedCount); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *JobRepo) CleanupExpiredIdempotencyKeys(ctx context.Context, olderThan time.Time) (int64, error) {
	db := database.GetExecutor(ctx, r.pool)
	tag, err := db.Exec(ctx, `DELETE FROM idempotency_keys WHERE created_at < $1`, olderThan)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func scanJobRun(row pgx.Row) (*domain.JobRun, error) {
	var run domain.JobRun
	var rawSummary []byte
	if err := row.Scan(
		&run.ID, &run.JobName, &run.RunKey, &run.Status,
		&rawSummary, &run.Error, &run.StartedAt, &run.FinishedAt,
	); err != nil {
		return nil, err
	}
	if len(rawSummary) > 0 {
		var m map[string]any
		if err := json.Unmarshal(rawSummary, &m); err == nil {
			run.Summary = m
		}
	}
	return &run, nil
}
