package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type ReconcileRepo struct{ pool *pgxpool.Pool }

func NewReconcileRepo(pool *pgxpool.Pool) *ReconcileRepo { return &ReconcileRepo{pool: pool} }

var _ domain.ReconcileRepository = (*ReconcileRepo)(nil)

func (r *ReconcileRepo) CheckStockInvariants(ctx context.Context, now time.Time) (*domain.ReconcileReport, error) {
	checks := []struct{ code, name, sql string }{
		{"I1", "Saldo rak produk SN = jumlah unit fisik di rak", `
			SELECT sb.product_id, sb.rack_id, sb.qty_on_hand, COUNT(ps.id) AS serial_count
			FROM stock_balances sb
			JOIN products p ON p.id = sb.product_id AND p.has_serial_number
			LEFT JOIN product_serials ps ON ps.product_id = sb.product_id AND ps.current_rack_id = sb.rack_id
			     AND ps.status IN ('AVAILABLE','RESERVED','MAINTENANCE','BROKEN')
			GROUP BY sb.product_id, sb.rack_id, sb.qty_on_hand
			HAVING sb.qty_on_hand <> COUNT(ps.id)`},
		{"I1b", "Tidak ada unit fisik SN di rak tanpa baris saldo", `
			SELECT ps.product_id, ps.current_rack_id, COUNT(ps.id) AS serial_count
			FROM product_serials ps
			LEFT JOIN stock_balances sb ON sb.product_id = ps.product_id AND sb.rack_id = ps.current_rack_id
			WHERE ps.status IN ('AVAILABLE','RESERVED','MAINTENANCE','BROKEN')
			  AND sb.id IS NULL
			GROUP BY ps.product_id, ps.current_rack_id`},
		{"I2", "qty_reserved produk SN = jumlah unit RESERVED di rak", `
			SELECT sb.product_id, sb.rack_id, sb.qty_reserved, COUNT(ps.id) AS reserved_units
			FROM stock_balances sb
			JOIN products p ON p.id = sb.product_id AND p.has_serial_number
			LEFT JOIN product_serials ps ON ps.product_id = sb.product_id AND ps.current_rack_id = sb.rack_id
			     AND ps.status = 'RESERVED'
			GROUP BY sb.product_id, sb.rack_id, sb.qty_reserved
			HAVING sb.qty_reserved <> COUNT(ps.id)`},
		{"I3", "Saldo rak = Σ ledger IN − Σ ledger OUT", `
			SELECT sb.product_id, sb.rack_id, sb.qty_on_hand
			FROM stock_balances sb
			LEFT JOIN stock_ledgers l ON l.product_id = sb.product_id AND l.rack_id = sb.rack_id
			GROUP BY sb.product_id, sb.rack_id, sb.qty_on_hand
			HAVING sb.qty_on_hand <> COALESCE(SUM(CASE WHEN l.direction = 'IN' THEN l.qty ELSE -l.qty END), 0)`},
		{"I3b", "Tidak ada baris ledger tanpa baris saldo", `
			SELECT l.product_id, l.rack_id
			FROM stock_ledgers l
			LEFT JOIN stock_balances sb ON sb.product_id = l.product_id AND sb.rack_id = l.rack_id
			WHERE sb.id IS NULL
			GROUP BY l.product_id, l.rack_id`},
		{"I4", "Tidak ada saldo negatif atau reserved > on_hand", `
			SELECT product_id, rack_id, qty_on_hand, qty_reserved FROM stock_balances
			WHERE qty_on_hand < 0 OR qty_reserved < 0 OR qty_reserved > qty_on_hand`},
		{"I5", "Setiap baris ledger memenuhi balance_after = balance_before ± qty", `
			SELECT id, product_id, rack_id FROM stock_ledgers
			WHERE (direction = 'IN'  AND balance_after <> balance_before + qty)
			   OR (direction = 'OUT' AND balance_after <> balance_before - qty)`},
		{"I5b", "Rantai ledger per (produk, rak) berkesinambungan tanpa celah", `
			WITH ordered AS (
				SELECT id, product_id, rack_id, balance_before,
				       COALESCE(LAG(balance_after) OVER (PARTITION BY product_id, rack_id ORDER BY id), 0) AS prev_after
				FROM stock_ledgers
			)
			SELECT id, product_id, rack_id, prev_after, balance_before
			FROM ordered
			WHERE balance_before <> prev_after`},
		{"I6", "Unit di gudang punya rak, unit tersewa punya pelanggan", `
			SELECT id, asset_tag, status FROM product_serials
			WHERE (status IN ('AVAILABLE','RESERVED','MAINTENANCE','BROKEN') AND current_rack_id IS NULL)
			   OR (status = 'RENTED' AND current_customer_id IS NULL)`},
	}

	exec := database.GetExecutor(ctx, r.pool)
	violations := make([]domain.ReconcileViolation, 0)
	for _, c := range checks {
		rows, err := exec.Query(ctx, c.sql)
		if err != nil {
			return nil, fmt.Errorf("reconcile %s: %w", c.code, err)
		}
		for rows.Next() {
			vals, err := rows.Values()
			if err != nil {
				rows.Close()
				return nil, fmt.Errorf("reconcile %s: %w", c.code, err)
			}
			violations = append(violations, domain.ReconcileViolation{
				Code:   c.code,
				Check:  c.name,
				Detail: fmt.Sprint(vals...),
			})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("reconcile %s: %w", c.code, err)
		}
		rows.Close()
	}

	return &domain.ReconcileReport{
		CheckedAt:      now,
		Healthy:        len(violations) == 0,
		TotalChecks:    len(checks),
		ViolationCount: len(violations),
		Violations:     violations,
	}, nil
}
