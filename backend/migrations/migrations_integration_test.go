//go:build integration

// Package migrations_test menguji file migrasi langsung di PostgreSQL (schema sementara).
package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/testutil/pgtest"
)

const dir = "." // go test berjalan di folder paket ini

func countObjects(t *testing.T, pool *pgxpool.Pool) (tables, functions int) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.tables WHERE table_schema = current_schema()`).Scan(&tables))
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM pg_proc WHERE pronamespace = current_schema()::regnamespace`).Scan(&functions))
	return tables, functions
}

func TestMigrations_FilesArePaired(t *testing.T) {
	up := pgtest.MigrationFiles(t, dir, "up")
	down := pgtest.MigrationFiles(t, dir, "down")
	require.NotEmpty(t, up)
	assert.Len(t, down, len(up), "setiap .up.sql wajib punya .down.sql")
}

func TestMigrations_UpDownUp_LeavesNoResidue(t *testing.T) {
	pool := pgtest.NewPool(t)

	pgtest.Apply(t, pool, dir, "up")
	tables, functions := countObjects(t, pool)
	assert.Equal(t, 39, tables, "25 Fase 1 + 5 customer/contract + 5 SO/DO + 4 billing")
	assert.Equal(t, 2, functions)

	pgtest.Apply(t, pool, dir, "down")
	tables, functions = countObjects(t, pool)
	assert.Equal(t, 0, tables, "down harus menghapus semua tabel")
	assert.Equal(t, 0, functions, "down harus menghapus semua fungsi")

	pgtest.Apply(t, pool, dir, "up") // up lagi harus bersih
	tables, _ = countObjects(t, pool)
	assert.Equal(t, 39, tables)
}

func TestMigrations_Constraints(t *testing.T) {
	pool := pgtest.NewPool(t)
	pgtest.Apply(t, pool, dir, "up")
	ctx := context.Background()

	_, err := pool.Exec(ctx, `INSERT INTO users (name,email,password_hash,role) VALUES ('a','a@x.id','h','ADMIN')`)
	require.NoError(t, err)

	t.Run("role tidak valid ditolak", func(t *testing.T) {
		_, err := pool.Exec(ctx, `INSERT INTO users (name,email,password_hash,role) VALUES ('b','b@x.id','h','SUPERUSER')`)
		assert.Error(t, err)
	})
	t.Run("email ganda ditolak", func(t *testing.T) {
		_, err := pool.Exec(ctx, `INSERT INTO users (name,email,password_hash,role) VALUES ('c','a@x.id','h','ADMIN')`)
		assert.Error(t, err)
	})
	t.Run("item_type kategori tidak valid ditolak", func(t *testing.T) {
		_, err := pool.Exec(ctx, `INSERT INTO categories (name,item_type) VALUES ('X','LAINNYA')`)
		assert.Error(t, err)
	})
	t.Run("min_stock_alert negatif ditolak", func(t *testing.T) {
		_, err := pool.Exec(ctx, `INSERT INTO categories (name,item_type) VALUES ('K','ASSET')`)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO products (sku,name,category_id,min_stock_alert)
			SELECT 'S1','P',id,-1 FROM categories WHERE name='K'`)
		assert.Error(t, err)
	})
	t.Run("status job_runs tidak valid ditolak", func(t *testing.T) {
		_, err := pool.Exec(ctx, `INSERT INTO job_runs (job_name,run_key,status) VALUES ('J','1','ABC')`)
		assert.Error(t, err)
	})
	t.Run("set_updated_at memperbarui updated_at", func(t *testing.T) {
		var changed bool
		require.NoError(t, pool.QueryRow(ctx, `
			WITH old AS (SELECT updated_at AS t FROM users WHERE email='a@x.id'),
			     upd AS (UPDATE users SET name='baru', updated_at = NOW() - INTERVAL '1 day'
			             WHERE email='a@x.id' RETURNING updated_at)
			SELECT (SELECT updated_at FROM upd) >= (SELECT t FROM old)`).Scan(&changed))
		assert.True(t, changed, "trigger harus menimpa updated_at ke NOW()")
	})
}

func TestMigrations_AuditLogsAreAppendOnly(t *testing.T) {
	pool := pgtest.NewPool(t)
	pgtest.Apply(t, pool, dir, "up")
	ctx := context.Background()

	_, err := pool.Exec(ctx, `INSERT INTO audit_logs (action) VALUES ('TEST')`)
	require.NoError(t, err, "INSERT harus diizinkan")

	_, err = pool.Exec(ctx, `UPDATE audit_logs SET action = 'UBAH'`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "append-only")

	_, err = pool.Exec(ctx, `DELETE FROM audit_logs`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "append-only")

	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='TEST'`).Scan(&n))
	assert.Equal(t, 1, n)
}

func TestMigrations_Phase2CommercialConstraints(t *testing.T) {
	pool := pgtest.NewPool(t)
	pgtest.Apply(t, pool, dir, "up")
	ctx := context.Background()

	var userID, custID, siteID, contractID, invID int64
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO users (name,email,password_hash,role) VALUES ('u','u@p2.id','h','BUSINESS_ADMIN') RETURNING id`).Scan(&userID))
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO customers (customer_code, company_name, pic_name, phone, billing_address, billing_postal_code, created_by)
		VALUES ('CUST-01', 'PT Pelanggan Satu', 'Andi', '0812', 'Jl. A', '50111', $1) RETURNING id`, userID).Scan(&custID))
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO customer_sites (customer_id, site_name, pic_name, pic_phone, address_line, city, province, postal_code, is_primary)
		VALUES ($1, 'Kantor Pusat', 'Andi', '0812', 'Jl. A', 'Semarang', 'Jawa Tengah', '50111', TRUE) RETURNING id`, custID).Scan(&siteID))

	t.Run("hapus pelanggan yang masih punya site ditolak (ON DELETE RESTRICT)", func(t *testing.T) {
		_, err := pool.Exec(ctx, `DELETE FROM customers WHERE id = $1`, custID)
		assert.Error(t, err)
	})
	t.Run("site utama aktif harus unik per pelanggan", func(t *testing.T) {
		_, err := pool.Exec(ctx, `
			INSERT INTO customer_sites (customer_id, site_name, pic_name, pic_phone, address_line, city, province, postal_code, is_primary)
			VALUES ($1, 'Cabang 2', 'Budi', '0813', 'Jl. B', 'Semarang', 'Jawa Tengah', '50112', TRUE)`, custID)
		assert.Error(t, err)
	})
	t.Run("SO sewa tanpa kontrak ditolak (chk_so_contract)", func(t *testing.T) {
		_, err := pool.Exec(ctx, `
			INSERT INTO sales_orders (so_number, order_type, customer_id, customer_site_id, created_by)
			VALUES ('SO/01', 'NEW_DELIVERY', $1, $2, $3)`, custID, siteID, userID)
		assert.Error(t, err)
	})
	t.Run("invoice RENTAL ganda per periode aktif ditolak (uq_invoice_rental_period)", func(t *testing.T) {
		require.NoError(t, pool.QueryRow(ctx, `
			INSERT INTO rental_contracts (contract_number, customer_id, start_date, end_date, duration_months, created_by)
			VALUES ('KTR/01', $1, '2026-01-01', '2026-12-31', 12, $2) RETURNING id`, custID, userID).Scan(&contractID))
		require.NoError(t, pool.QueryRow(ctx, `
			INSERT INTO invoices (invoice_number, invoice_type, customer_id, contract_id, billing_period_start, billing_period_end, invoice_date, due_date, is_ppn)
			VALUES ('INV/01', 'RENTAL', $1, $2, '2026-11-01', '2026-11-30', '2026-12-01', '2026-12-31', TRUE) RETURNING id`, custID, contractID).Scan(&invID))
		_, err := pool.Exec(ctx, `
			INSERT INTO invoices (invoice_number, invoice_type, customer_id, contract_id, billing_period_start, billing_period_end, invoice_date, due_date, is_ppn)
			VALUES ('INV/02', 'RENTAL', $1, $2, '2026-11-01', '2026-11-30', '2026-12-01', '2026-12-31', TRUE)`, custID, contractID)
		assert.Error(t, err)
	})
	t.Run("pembayaran nol ditolak (chk_payment_positive)", func(t *testing.T) {
		_, err := pool.Exec(ctx, `
			INSERT INTO payments (invoice_id, payment_date, amount_received, pph23_amount, method, recorded_by)
			VALUES ($1, '2026-12-05', 0, 0, 'TRANSFER', $2)`, invID, userID)
		assert.Error(t, err)
	})
}
