package pgtest

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// AssertInvariants menjalankan pemeriksaan konsistensi stok (plan Bagian 5.9) dan gagal bila ada
// baris yang melanggar. Panggil di akhir setiap test integrasi yang mengubah stok (CLAUDE.md).
//
//	I1: saldo rak produk SN = jumlah unit fisik di rak itu
//	I2: qty_reserved produk SN = jumlah unit RESERVED di rak itu
//	I3: saldo rak = Σ ledger IN − Σ ledger OUT
//	I4: tidak ada saldo negatif / reserved > on hand (dijaga CHECK, diperiksa ulang)
//	I5: setiap baris ledger berantai: balance_after = balance_before ± qty
func AssertInvariants(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	checks := []struct{ name, sql string }{
		{"I1 saldo SN = unit fisik di rak", `
			SELECT sb.product_id, sb.rack_id, sb.qty_on_hand, COUNT(ps.id) AS serial_count
			FROM stock_balances sb
			JOIN products p ON p.id = sb.product_id AND p.has_serial_number
			LEFT JOIN product_serials ps ON ps.product_id = sb.product_id AND ps.current_rack_id = sb.rack_id
			     AND ps.status IN ('AVAILABLE','RESERVED','MAINTENANCE','BROKEN')
			GROUP BY sb.product_id, sb.rack_id, sb.qty_on_hand
			HAVING sb.qty_on_hand <> COUNT(ps.id)`},
		{"I2 qty_reserved SN = unit RESERVED", `
			SELECT sb.product_id, sb.rack_id, sb.qty_reserved, COUNT(ps.id) AS reserved_units
			FROM stock_balances sb
			JOIN products p ON p.id = sb.product_id AND p.has_serial_number
			LEFT JOIN product_serials ps ON ps.product_id = sb.product_id AND ps.current_rack_id = sb.rack_id
			     AND ps.status = 'RESERVED'
			GROUP BY sb.product_id, sb.rack_id, sb.qty_reserved
			HAVING sb.qty_reserved <> COUNT(ps.id)`},
		{"I3 saldo = Σ ledger", `
			SELECT sb.product_id, sb.rack_id, sb.qty_on_hand
			FROM stock_balances sb
			LEFT JOIN stock_ledgers l ON l.product_id = sb.product_id AND l.rack_id = sb.rack_id
			GROUP BY sb.product_id, sb.rack_id, sb.qty_on_hand
			HAVING sb.qty_on_hand <> COALESCE(SUM(CASE WHEN l.direction = 'IN' THEN l.qty ELSE -l.qty END), 0)`},
		{"I3b ledger tanpa saldo", `
			SELECT l.product_id, l.rack_id
			FROM stock_ledgers l
			LEFT JOIN stock_balances sb ON sb.product_id = l.product_id AND sb.rack_id = l.rack_id
			WHERE sb.id IS NULL
			GROUP BY l.product_id, l.rack_id`},
		{"I4 saldo negatif / reserved melebihi on hand", `
			SELECT product_id, rack_id FROM stock_balances
			WHERE qty_on_hand < 0 OR qty_reserved < 0 OR qty_reserved > qty_on_hand`},
		{"I5 rantai ledger konsisten", `
			SELECT id FROM stock_ledgers
			WHERE (direction = 'IN'  AND balance_after <> balance_before + qty)
			   OR (direction = 'OUT' AND balance_after <> balance_before - qty)`},
		{"I6 unit di gudang punya rak, unit tersewa punya pelanggan", `
			SELECT id FROM product_serials
			WHERE (status IN ('AVAILABLE','RESERVED','MAINTENANCE','BROKEN') AND current_rack_id IS NULL)
			   OR (status = 'RENTED' AND current_customer_id IS NULL)`},
	}
	for _, c := range checks {
		rows, err := pool.Query(ctx, c.sql)
		require.NoError(t, err, c.name)
		var violations []string
		for rows.Next() {
			vals, err := rows.Values()
			require.NoError(t, err, c.name)
			violations = append(violations, fmt.Sprint(vals...))
		}
		require.NoError(t, rows.Err(), c.name)
		rows.Close()
		require.Empty(t, violations, "invarian dilanggar — %s: %v", c.name, violations)
	}
}
