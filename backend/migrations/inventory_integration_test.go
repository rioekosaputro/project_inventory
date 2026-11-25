//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/testutil/pgtest"
)

// fixtureInventory berisi data minimum: gudang, 2 rak, produk SN, pengguna.
type fixtureInventory struct {
	warehouseID, rackA, rackB, productID, userID int64
}

func newFixtureInventory(t *testing.T, pool *pgxpool.Pool) fixtureInventory {
	t.Helper()
	ctx := context.Background()
	var f fixtureInventory
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO warehouses (code,name) VALUES ('WH-T','Gudang Uji') RETURNING id`).Scan(&f.warehouseID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO racks (warehouse_id,code) VALUES ($1,'A-01') RETURNING id`, f.warehouseID).Scan(&f.rackA))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO racks (warehouse_id,code) VALUES ($1,'A-02') RETURNING id`, f.warehouseID).Scan(&f.rackB))
	var catID int64
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO categories (name,item_type) VALUES ('Printer','ASSET') RETURNING id`).Scan(&catID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO products (sku,name,category_id,has_serial_number) VALUES ('PRN-1','Printer',$1,TRUE) RETURNING id`,
		catID).Scan(&f.productID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO users (name,email,password_hash,role) VALUES ('u','u@x.id','h','ADMIN') RETURNING id`).Scan(&f.userID))
	return f
}

func (f fixtureInventory) insertLPB(pool *pgxpool.Pool, number, typ, subType string) (lpbID, itemID int64, err error) {
	ctx := context.Background()
	err = pool.QueryRow(ctx,
		`INSERT INTO lpb_documents (lpb_number,type,sub_type,warehouse_id,created_by) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		number, typ, subType, f.warehouseID, f.userID).Scan(&lpbID)
	if err != nil {
		return 0, 0, err
	}
	err = pool.QueryRow(ctx,
		`INSERT INTO lpb_items (lpb_id,product_id,rack_id,qty) VALUES ($1,$2,$3,1) RETURNING id`,
		lpbID, f.productID, f.rackA).Scan(&itemID)
	return lpbID, itemID, err
}

func TestMigrations_InventoryConstraints(t *testing.T) {
	pool := pgtest.NewPool(t)
	pgtest.Apply(t, pool, dir, "up")
	ctx := context.Background()
	f := newFixtureInventory(t, pool)

	t.Run("saldo stok negatif ditolak", func(t *testing.T) {
		_, err := pool.Exec(ctx, `INSERT INTO stock_balances (product_id,rack_id,qty_on_hand) VALUES ($1,$2,-1)`,
			f.productID, f.rackA)
		assert.Error(t, err)
	})
	t.Run("qty_reserved melebihi qty_on_hand ditolak", func(t *testing.T) {
		_, err := pool.Exec(ctx, `INSERT INTO stock_balances (product_id,rack_id,qty_on_hand,qty_reserved) VALUES ($1,$2,1,2)`,
			f.productID, f.rackA)
		assert.Error(t, err)
	})
	t.Run("saldo per (produk, rak) unik", func(t *testing.T) {
		_, err := pool.Exec(ctx, `INSERT INTO stock_balances (product_id,rack_id,qty_on_hand) VALUES ($1,$2,5)`,
			f.productID, f.rackA)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO stock_balances (product_id,rack_id,qty_on_hand) VALUES ($1,$2,1)`,
			f.productID, f.rackA)
		assert.Error(t, err)
	})

	t.Run("lokasi SN dijaga chk_serial_location", func(t *testing.T) {
		var custID int64
		require.NoError(t, pool.QueryRow(ctx, `
			INSERT INTO customers (customer_code, company_name, pic_name, phone, billing_address, billing_postal_code)
			VALUES ('C-LOC', 'PT Uji Lokasi', 'PIC', '0812', 'Jl. A', '50111') RETURNING id`).Scan(&custID))
		ins := func(tag, sn, status string, rack, customer any) error {
			_, err := pool.Exec(ctx, `INSERT INTO product_serials
				(product_id,asset_tag,serial_number,status,current_rack_id,current_customer_id)
				VALUES ($1,$2,$3,$4,$5,$6)`, f.productID, tag, sn, status, rack, customer)
			return err
		}
		assert.Error(t, ins("AST-1", "SN-1", "AVAILABLE", nil, nil), "AVAILABLE tanpa rak")
		assert.Error(t, ins("AST-2", "SN-2", "RENTED", nil, nil), "RENTED tanpa pelanggan")
		assert.Error(t, ins("AST-3", "SN-3", "RENTED", f.rackA, custID), "RENTED tetapi masih punya rak")
		assert.Error(t, ins("AST-4", "SN-4", "AVAILABLE", f.rackA, custID), "AVAILABLE tetapi punya pelanggan")
		assert.Error(t, ins("AST-5", "SN-5", "IN_TRANSIT", f.rackA, nil), "IN_TRANSIT tidak boleh punya rak")
		assert.Error(t, ins("AST-6", "SN-6", "HILANG", f.rackA, nil), "status tidak dikenal")
		require.NoError(t, ins("AST-7", "SN-7", "AVAILABLE", f.rackA, nil))
		require.NoError(t, ins("AST-8", "SN-8", "RENTED", nil, custID))
		require.NoError(t, ins("AST-9", "SN-9", "LOST", nil, nil))
	})
	t.Run("asset_tag unik dan SN unik per produk", func(t *testing.T) {
		_, err := pool.Exec(ctx, `INSERT INTO product_serials (product_id,asset_tag,serial_number,current_rack_id)
			VALUES ($1,'AST-7','SN-BEDA',$2)`, f.productID, f.rackA)
		assert.Error(t, err, "asset_tag ganda")
		_, err = pool.Exec(ctx, `INSERT INTO product_serials (product_id,asset_tag,serial_number,current_rack_id)
			VALUES ($1,'AST-BEDA','SN-7',$2)`, f.productID, f.rackA)
		assert.Error(t, err, "SN ganda pada produk yang sama")
	})

	t.Run("pasangan type/sub_type LPB divalidasi", func(t *testing.T) {
		_, _, err := f.insertLPB(pool, "LPBI/1", "INBOUND", "DISPOSAL")
		assert.Error(t, err, "INBOUND dengan sub_type OUTBOUND")
		_, _, err = f.insertLPB(pool, "LPBO/1", "OUTBOUND", "PURCHASE")
		assert.Error(t, err, "OUTBOUND dengan sub_type INBOUND")
		_, _, err = f.insertLPB(pool, "LPBI/2", "INBOUND", "NGAWUR")
		assert.Error(t, err, "sub_type tidak dikenal")
		_, _, err = f.insertLPB(pool, "LPBI/3", "INBOUND", "PURCHASE")
		assert.NoError(t, err)
	})
	t.Run("nomor LPB unik dan status awal DRAFT", func(t *testing.T) {
		_, _, err := f.insertLPB(pool, "LPBI/3", "INBOUND", "PURCHASE")
		assert.Error(t, err)
		var status string
		require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM lpb_documents WHERE lpb_number='LPBI/3'`).Scan(&status))
		assert.Equal(t, "DRAFT", status)
	})
	t.Run("lpb_items: qty harus > 0 dan produk/rak unik per LPB", func(t *testing.T) {
		var lpbID int64
		require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM lpb_documents WHERE lpb_number='LPBI/3'`).Scan(&lpbID))
		_, err := pool.Exec(ctx, `INSERT INTO lpb_items (lpb_id,product_id,rack_id,qty) VALUES ($1,$2,$3,0)`,
			lpbID, f.productID, f.rackB)
		assert.Error(t, err, "qty 0")
		_, err = pool.Exec(ctx, `INSERT INTO lpb_items (lpb_id,product_id,rack_id,qty) VALUES ($1,$2,$3,1)`,
			lpbID, f.productID, f.rackA)
		assert.Error(t, err, "(lpb, produk, rak) ganda")
	})
}

func TestMigrations_LedgerAndHistoriesAreAppendOnly(t *testing.T) {
	pool := pgtest.NewPool(t)
	pgtest.Apply(t, pool, dir, "up")
	ctx := context.Background()
	f := newFixtureInventory(t, pool)

	lpbID, itemID, err := f.insertLPB(pool, "LPBI/1", "INBOUND", "PURCHASE")
	require.NoError(t, err)

	ledger := func(direction string, qty, before, after int) error {
		_, err := pool.Exec(ctx, `INSERT INTO stock_ledgers
			(product_id,warehouse_id,rack_id,direction,qty,balance_before,balance_after,movement_type,lpb_id,lpb_item_id,created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,'PURCHASE',$8,$9,$10)`,
			f.productID, f.warehouseID, f.rackA, direction, qty, before, after, lpbID, itemID, f.userID)
		return err
	}

	t.Run("saldo ledger harus konsisten dengan arah dan qty", func(t *testing.T) {
		assert.Error(t, ledger("IN", 5, 0, 4), "IN: after != before+qty")
		assert.Error(t, ledger("OUT", 5, 10, 6), "OUT: after != before-qty")
		assert.Error(t, ledger("IN", 0, 0, 0), "qty 0")
		assert.Error(t, ledger("SAMPING", 1, 0, 1), "arah tidak dikenal")
		require.NoError(t, ledger("IN", 5, 0, 5))
		require.NoError(t, ledger("OUT", 2, 5, 3))
	})
	t.Run("UPDATE dan DELETE stock_ledgers ditolak", func(t *testing.T) {
		_, err := pool.Exec(ctx, `UPDATE stock_ledgers SET qty = 99`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "append-only")
		_, err = pool.Exec(ctx, `DELETE FROM stock_ledgers`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "append-only")
		var n int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM stock_ledgers`).Scan(&n))
		assert.Equal(t, 2, n)
	})
	t.Run("UPDATE dan DELETE product_serial_histories ditolak", func(t *testing.T) {
		var serialID int64
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO product_serials
			(product_id,asset_tag,serial_number,current_rack_id) VALUES ($1,'AST-H','SN-H',$2) RETURNING id`,
			f.productID, f.rackA).Scan(&serialID))
		_, err := pool.Exec(ctx, `INSERT INTO product_serial_histories (product_serial_id,to_status,event_type)
			VALUES ($1,'AVAILABLE','LPB_APPROVED')`, serialID)
		require.NoError(t, err, "INSERT harus diizinkan")
		_, err = pool.Exec(ctx, `UPDATE product_serial_histories SET to_status = 'LOST'`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "append-only")
		_, err = pool.Exec(ctx, `DELETE FROM product_serial_histories`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "append-only")
	})
}

func TestMigrations_StockOpname(t *testing.T) {
	pool := pgtest.NewPool(t)
	pgtest.Apply(t, pool, dir, "up")
	ctx := context.Background()
	f := newFixtureInventory(t, pool)

	var opnameID int64
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO stock_opnames (opname_number,warehouse_id,started_by) VALUES ('OPN/1',$1,$2) RETURNING id`,
		f.warehouseID, f.userID).Scan(&opnameID))

	t.Run("status awal IN_PROGRESS dan blind count aktif", func(t *testing.T) {
		var status string
		var blind bool
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT status,is_blind_count FROM stock_opnames WHERE id=$1`, opnameID).Scan(&status, &blind))
		assert.Equal(t, "IN_PROGRESS", status)
		assert.True(t, blind)
	})
	t.Run("status opname tidak dikenal ditolak", func(t *testing.T) {
		_, err := pool.Exec(ctx, `UPDATE stock_opnames SET status='SELESAI' WHERE id=$1`, opnameID)
		assert.Error(t, err)
	})
	t.Run("diff_qty dihitung otomatis dari counted - system", func(t *testing.T) {
		var diff int
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO stock_opname_items
			(opname_id,product_id,rack_id,system_qty,counted_qty) VALUES ($1,$2,$3,10,7) RETURNING diff_qty`,
			opnameID, f.productID, f.rackA).Scan(&diff))
		assert.Equal(t, -3, diff)
	})
	t.Run("hasil scan SN divalidasi", func(t *testing.T) {
		_, err := pool.Exec(ctx, `INSERT INTO stock_opname_serials (opname_id,rack_id,scanned_code,result)
			VALUES ($1,$2,'X','HILANG')`, opnameID, f.rackA)
		assert.Error(t, err, "result tidak dikenal")
		_, err = pool.Exec(ctx, `INSERT INTO stock_opname_serials (opname_id,rack_id,scanned_code,result)
			VALUES ($1,$2,'X','UNEXPECTED')`, opnameID, f.rackA)
		assert.NoError(t, err)
	})
	t.Run("rak dapat dikunci opname, FK menolak opname yang tidak ada", func(t *testing.T) {
		_, err := pool.Exec(ctx, `UPDATE racks SET locked_by_opname_id=$1 WHERE id=$2`, opnameID, f.rackA)
		assert.NoError(t, err)
		_, err = pool.Exec(ctx, `UPDATE racks SET locked_by_opname_id=999999 WHERE id=$1`, f.rackB)
		assert.Error(t, err)
	})
}
