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
	assert.Equal(t, 43, tables, "25 Fase 1 + 14 Fase 2 + 4 Fase 3 (tracking & tickets)")
	assert.Equal(t, 2, functions)

	pgtest.Apply(t, pool, dir, "down")
	tables, functions = countObjects(t, pool)
	assert.Equal(t, 0, tables, "down harus menghapus semua tabel")
	assert.Equal(t, 0, functions, "down harus menghapus semua fungsi")

	pgtest.Apply(t, pool, dir, "up") // up lagi harus bersih
	tables, _ = countObjects(t, pool)
	assert.Equal(t, 43, tables)
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

func TestMigrations_Phase3TrackingTicketsConstraints(t *testing.T) {
	pool := pgtest.NewPool(t)
	pgtest.Apply(t, pool, dir, "up")
	ctx := context.Background()

	var userID, whID, catID, prodID, serialID, custID, siteID, lpbID, ticketID int64
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO users (name,email,password_hash,role) VALUES ('drv','drv@p3.id','h','DRIVER') RETURNING id`).Scan(&userID))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO warehouses (code,name) VALUES ('WH-P3','Gudang P3') RETURNING id`).Scan(&whID))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO categories (name,item_type) VALUES ('Printer','ASSET') RETURNING id`).Scan(&catID))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO products (sku,name,category_id,has_serial_number) VALUES ('PRT-01','Printer A',$1,TRUE) RETURNING id`, catID).Scan(&prodID))
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO customers (customer_code, company_name, pic_name, phone, billing_address, billing_postal_code, created_by)
		VALUES ('CUST-P3', 'PT P3', 'Cici', '0811', 'Jl. C', '50113', $1) RETURNING id`, userID).Scan(&custID))
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO customer_sites (customer_id, site_name, pic_name, pic_phone, address_line, city, province, postal_code, is_primary)
		VALUES ($1, 'Cabang P3', 'Cici', '0811', 'Jl. C', 'Semarang', 'Jawa Tengah', '50113', TRUE) RETURNING id`, custID).Scan(&siteID))
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO product_serials (product_id, serial_number, asset_tag, status, current_customer_id)
		VALUES ($1, 'SN-P3-1', 'AST-P3-1', 'RENTED', $2) RETURNING id`, prodID, custID).Scan(&serialID))
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO lpb_documents (lpb_number, type, sub_type, warehouse_id, created_by)
		VALUES ('LPB/P3/01', 'OUTBOUND', 'RENTAL_DELIVERY', $1, $2) RETURNING id`, whID, userID).Scan(&lpbID))

	t.Run("shipment_tracking_logs: status tidak valid ditolak dan client_event_id unik", func(t *testing.T) {
		_, err := pool.Exec(ctx, `
			INSERT INTO shipment_tracking_logs (lpb_id, driver_id, status, latitude, longitude)
			VALUES ($1, $2, 'UNKNOWN', -6.9932, 110.4203)`, lpbID, userID)
		assert.Error(t, err)

		const evID = "11111111-1111-4111-8111-111111111111"
		_, err = pool.Exec(ctx, `
			INSERT INTO shipment_tracking_logs (lpb_id, driver_id, status, latitude, longitude, client_event_id)
			VALUES ($1, $2, 'IN_TRANSIT', -6.9932, 110.4203, $3)`, lpbID, userID, evID)
		require.NoError(t, err)

		_, err = pool.Exec(ctx, `
			INSERT INTO shipment_tracking_logs (lpb_id, driver_id, status, latitude, longitude, client_event_id)
			VALUES ($1, $2, 'IN_TRANSIT', -6.9932, 110.4203, $3)`, lpbID, userID, evID)
		assert.Error(t, err, "client_event_id duplikat harus ditolak")
	})

	t.Run("maintenance_tickets: insert valid, trigger updated_at, dan FK sales_orders.related_ticket_id", func(t *testing.T) {
		require.NoError(t, pool.QueryRow(ctx, `
			INSERT INTO maintenance_tickets (ticket_number, customer_id, customer_site_id, product_serial_id, title, created_by)
			VALUES ('TKT/2026/0001', $1, $2, $3, 'Paper jam berat', $4) RETURNING id`,
			custID, siteID, serialID, userID).Scan(&ticketID))

		var changed bool
		require.NoError(t, pool.QueryRow(ctx, `
			WITH old AS (SELECT updated_at AS t FROM maintenance_tickets WHERE id = $1),
			     upd AS (UPDATE maintenance_tickets SET status = 'ASSIGNED', updated_at = NOW() - INTERVAL '1 day'
			             WHERE id = $1 RETURNING updated_at)
			SELECT (SELECT updated_at FROM upd) >= (SELECT t FROM old)`, ticketID).Scan(&changed))
		assert.True(t, changed, "trg_tickets_updated harus menimpa updated_at ke NOW()")

		_, err := pool.Exec(ctx, `
			INSERT INTO sales_orders (so_number, order_type, customer_id, customer_site_id, related_ticket_id, created_by)
			VALUES ('SO/P3/BAD', 'CONSUMABLE', $1, $2, 999999, $3)`, custID, siteID, userID)
		assert.Error(t, err, "fk_so_ticket harus menolak ticket_id yang tidak ada")

		_, err = pool.Exec(ctx, `
			INSERT INTO sales_orders (so_number, order_type, customer_id, customer_site_id, related_ticket_id, created_by)
			VALUES ('SO/P3/OK', 'CONSUMABLE', $1, $2, $3, $4)`, custID, siteID, ticketID, userID)
		require.NoError(t, err)
	})

	t.Run("ticket_spareparts & ticket_activities: constraint qty dan activity_type", func(t *testing.T) {
		_, err := pool.Exec(ctx, `
			INSERT INTO ticket_spareparts (ticket_id, product_id, qty, requested_by)
			VALUES ($1, $2, 0, $3)`, ticketID, prodID, userID)
		assert.Error(t, err, "qty <= 0 harus ditolak")

		_, err = pool.Exec(ctx, `
			INSERT INTO ticket_spareparts (ticket_id, product_id, qty, is_billable, requested_by)
			VALUES ($1, $2, 2, TRUE, $3)`, ticketID, prodID, userID)
		require.NoError(t, err)

		_, err = pool.Exec(ctx, `
			INSERT INTO ticket_activities (ticket_id, user_id, activity_type, message)
			VALUES ($1, $2, 'INVALID_TYPE', 'test')`, ticketID, userID)
		assert.Error(t, err)

		_, err = pool.Exec(ctx, `
			INSERT INTO ticket_activities (ticket_id, user_id, activity_type, message, latitude, longitude)
			VALUES ($1, $2, 'VISIT_CHECKIN', 'Tiba di lokasi', -6.9932, 110.4203)`, ticketID, userID)
		require.NoError(t, err)
	})
}
