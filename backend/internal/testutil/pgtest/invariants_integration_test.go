//go:build integration

package pgtest_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/testutil/pgtest"
)

func TestCheckInvariants_DetectsViolations(t *testing.T) {
	pool := pgtest.NewPool(t)
	pgtest.Apply(t, pool, filepath.Join("..", "..", "..", "migrations"), "up")
	ctx := context.Background()

	// Database kosong: 0 pelanggaran.
	violations, err := pgtest.CheckInvariants(ctx, pool)
	require.NoError(t, err)
	assert.Empty(t, violations)

	// Siapkan master minimal untuk memicu pelanggaran invarian satu per satu.
	var whID, rackID, catID, prodSN, prodNonSN, userID int64
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO warehouses (code, name) VALUES ('WH-INV', 'Gudang Uji') RETURNING id`).Scan(&whID))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO racks (warehouse_id, code, zone) VALUES ($1, 'A-01', 'PRINTER') RETURNING id`, whID).Scan(&rackID))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO categories (name, item_type) VALUES ('Printer', 'ASSET') RETURNING id`).Scan(&catID))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO products (sku, name, category_id, has_serial_number) VALUES ('PRT-1', 'Printer', $1, TRUE) RETURNING id`, catID).Scan(&prodSN))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO products (sku, name, category_id, has_serial_number) VALUES ('TNR-1', 'Toner', $1, FALSE) RETURNING id`, catID).Scan(&prodNonSN))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO users (name, email, password_hash, role) VALUES ('Admin', 'admin@inv.test', 'x', 'ADMIN') RETURNING id`).Scan(&userID))

	hasCheck := func(list []pgtest.InvariantViolation, prefix string) bool {
		for _, v := range list {
			if len(v.Check) >= len(prefix) && v.Check[:len(prefix)] == prefix {
				return true
			}
		}
		return false
	}

	// I1b: unit fisik di rak tanpa baris stock_balances.
	var serialID int64
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO product_serials (product_id, asset_tag, serial_number, status, current_warehouse_id, current_rack_id)
		VALUES ($1, 'AST-2611-00001', 'SN-INV-1', 'AVAILABLE', $2, $3) RETURNING id`, prodSN, whID, rackID).Scan(&serialID))
	violations, err = pgtest.CheckInvariants(ctx, pool)
	require.NoError(t, err)
	assert.True(t, hasCheck(violations, "I1b"), "harus mendeteksi unit tanpa saldo: %+v", violations)

	// I1 & I2 & I3: saldo SN = 2, reserved = 1, padahal hanya ada 1 unit AVAILABLE dan 0 ledger.
	_, err = pool.Exec(ctx, `INSERT INTO stock_balances (product_id, rack_id, qty_on_hand, qty_reserved) VALUES ($1, $2, 2, 1)`, prodSN, rackID)
	require.NoError(t, err)
	violations, err = pgtest.CheckInvariants(ctx, pool)
	require.NoError(t, err)
	assert.True(t, hasCheck(violations, "I1 "), "harus mendeteksi I1: %+v", violations)
	assert.True(t, hasCheck(violations, "I2 "), "harus mendeteksi I2: %+v", violations)
	assert.True(t, hasCheck(violations, "I3 "), "harus mendeteksi I3: %+v", violations)

	// Rapikan SN agar tinggal uji I5b (celah rantai ledger pada non-SN).
	_, err = pool.Exec(ctx, `DELETE FROM product_serials WHERE id = $1`, serialID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `DELETE FROM stock_balances WHERE product_id = $1`, prodSN)
	require.NoError(t, err)

	// Buat LPB & item untuk ledger dengan celah (baris 1: 0→5, baris 2: 6→8).
	var lpbID, itemID int64
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO lpb_documents (lpb_number, type, sub_type, status, warehouse_id, created_by)
		VALUES ('LPBI/WH-INV/2026/11/0001', 'INBOUND', 'INITIAL_STOCK', 'APPROVED', $1, $2) RETURNING id`, whID, userID).Scan(&lpbID))
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO lpb_items (lpb_id, product_id, rack_id, qty) VALUES ($1, $2, $3, 5) RETURNING id`, lpbID, prodNonSN, rackID).Scan(&itemID))
	_, err = pool.Exec(ctx, `INSERT INTO stock_balances (product_id, rack_id, qty_on_hand) VALUES ($1, $2, 7)`, prodNonSN, rackID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO stock_ledgers (product_id, warehouse_id, rack_id, direction, qty, balance_before, balance_after, movement_type, lpb_id, lpb_item_id, created_by)
		VALUES ($1, $2, $3, 'IN', 5, 0, 5, 'INITIAL_STOCK', $4, $5, $6),
		       ($1, $2, $3, 'IN', 2, 6, 8, 'INITIAL_STOCK', $4, $5, $6)`, prodNonSN, whID, rackID, lpbID, itemID, userID)
	require.NoError(t, err)

	violations, err = pgtest.CheckInvariants(ctx, pool)
	require.NoError(t, err)
	assert.True(t, hasCheck(violations, "I5b"), "harus mendeteksi celah rantai ledger: %+v", violations)
}
