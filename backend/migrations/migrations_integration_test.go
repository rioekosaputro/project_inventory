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
	assert.Equal(t, 13, tables, "7 tabel master/auth + 6 tabel audit/settings/login_attempts")
	assert.Equal(t, 2, functions)

	pgtest.Apply(t, pool, dir, "down")
	tables, functions = countObjects(t, pool)
	assert.Equal(t, 0, tables, "down harus menghapus semua tabel")
	assert.Equal(t, 0, functions, "down harus menghapus semua fungsi")

	pgtest.Apply(t, pool, dir, "up") // up lagi harus bersih
	tables, _ = countObjects(t, pool)
	assert.Equal(t, 13, tables)
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
