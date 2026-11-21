//go:build integration

package database

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test ini butuh PostgreSQL. Set TEST_DATABASE_URL, mis.:
//
//	TEST_DATABASE_URL=postgres://postgres:***@localhost:5432/project_inventory?sslmode=disable
//
// Setiap test memakai schema sementara (search_path) yang dihapus di akhir,
// sehingga tabel asli di schema public tidak tersentuh.
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL tidak diset")
	}
	ctx := context.Background()

	admin, err := pgx.Connect(ctx, url)
	require.NoError(t, err)

	buf := make([]byte, 6)
	_, _ = rand.Read(buf)
	schema := "t_" + hex.EncodeToString(buf)
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)

	cfg, err := pgxpool.ParseConfig(url)
	require.NoError(t, err)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `CREATE TABLE items (id SERIAL PRIMARY KEY, name TEXT NOT NULL)`)
	require.NoError(t, err)

	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		_ = admin.Close(ctx)
	})
	return pool
}

func countItems(t *testing.T, pool *pgxpool.Pool, name string) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM items WHERE name = $1`, name).Scan(&n))
	return n
}

func insertItem(ctx context.Context, pool *pgxpool.Pool, name string) error {
	_, err := GetExecutor(ctx, pool).Exec(ctx, `INSERT INTO items (name) VALUES ($1)`, name)
	return err
}

func TestTxManager_CommitOnSuccess(t *testing.T) {
	pool := newTestPool(t)
	m := NewTxManager(pool)

	err := m.WithinTransaction(context.Background(), func(ctx context.Context) error {
		return insertItem(ctx, pool, "commit")
	})

	require.NoError(t, err)
	assert.Equal(t, 1, countItems(t, pool, "commit"))
}

func TestTxManager_RollbackOnError(t *testing.T) {
	pool := newTestPool(t)
	m := NewTxManager(pool)
	sentinel := errors.New("aturan bisnis dilanggar")

	err := m.WithinTransaction(context.Background(), func(ctx context.Context) error {
		require.NoError(t, insertItem(ctx, pool, "rollback"))
		return sentinel
	})

	require.ErrorIs(t, err, sentinel)
	assert.Equal(t, 0, countItems(t, pool, "rollback"))
}

func TestTxManager_RollbackOnPanicAndRepanics(t *testing.T) {
	pool := newTestPool(t)
	m := NewTxManager(pool)

	assert.PanicsWithValue(t, "boom", func() {
		_ = m.WithinTransaction(context.Background(), func(ctx context.Context) error {
			require.NoError(t, insertItem(ctx, pool, "panic"))
			panic("boom")
		})
	})

	assert.Equal(t, 0, countItems(t, pool, "panic"))
	assert.EqualValues(t, 0, pool.Stat().AcquiredConns(), "koneksi harus dikembalikan ke pool")
}

func TestTxManager_NestedUsesSameTransaction(t *testing.T) {
	pool := newTestPool(t)
	m := NewTxManager(pool)
	sentinel := errors.New("gagal di dalam")

	t.Run("tx yang sama", func(t *testing.T) {
		err := m.WithinTransaction(context.Background(), func(outer context.Context) error {
			outerTx := MustInTx(outer)
			return m.WithinTransaction(outer, func(inner context.Context) error {
				assert.Same(t, outerTx, MustInTx(inner), "bersarang harus memakai tx yang sama")
				return nil
			})
		})
		require.NoError(t, err)
	})

	t.Run("error di dalam membatalkan tulisan luar dan dalam", func(t *testing.T) {
		err := m.WithinTransaction(context.Background(), func(outer context.Context) error {
			require.NoError(t, insertItem(outer, pool, "luar"))
			return m.WithinTransaction(outer, func(inner context.Context) error {
				require.NoError(t, insertItem(inner, pool, "dalam"))
				return sentinel
			})
		})

		require.ErrorIs(t, err, sentinel)
		assert.Equal(t, 0, countItems(t, pool, "luar"))
		assert.Equal(t, 0, countItems(t, pool, "dalam"))
	})

	t.Run("commit bersama bila sukses", func(t *testing.T) {
		err := m.WithinTransaction(context.Background(), func(outer context.Context) error {
			require.NoError(t, insertItem(outer, pool, "luar-ok"))
			return m.WithinTransaction(outer, func(inner context.Context) error {
				return insertItem(inner, pool, "dalam-ok")
			})
		})

		require.NoError(t, err)
		assert.Equal(t, 1, countItems(t, pool, "luar-ok"))
		assert.Equal(t, 1, countItems(t, pool, "dalam-ok"))
	})
}

func TestTxManager_RollbackStillWorksWhenContextCancelled(t *testing.T) {
	pool := newTestPool(t)
	m := NewTxManager(pool)
	ctx, cancel := context.WithCancel(context.Background())

	err := m.WithinTransaction(ctx, func(txCtx context.Context) error {
		require.NoError(t, insertItem(txCtx, pool, "cancel"))
		cancel()
		return txCtx.Err()
	})

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 0, countItems(t, pool, "cancel"))
	assert.Eventually(t, func() bool { return pool.Stat().AcquiredConns() == 0 },
		2*time.Second, 20*time.Millisecond, "koneksi harus dikembalikan ke pool")
}

func TestGetExecutor_ReturnsPoolOutsideAndTxInside(t *testing.T) {
	pool := newTestPool(t)
	m := NewTxManager(pool)

	assert.Same(t, pool, GetExecutor(context.Background(), pool))

	_ = m.WithinTransaction(context.Background(), func(ctx context.Context) error {
		assert.NotSame(t, pool, GetExecutor(ctx, pool))
		assert.Same(t, MustInTx(ctx), GetExecutor(ctx, pool))
		return nil
	})
}

func TestNewPool_BothPoolerModes(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL tidak diset")
	}
	for _, mode := range []string{PoolerSession, PoolerTransaction} {
		t.Run(mode, func(t *testing.T) {
			pool, err := NewPool(context.Background(), Config{URL: url, PoolerMode: mode, MaxConns: 3})
			require.NoError(t, err)
			defer pool.Close()

			var got int
			require.NoError(t, pool.QueryRow(context.Background(), `SELECT $1::int + 1`, 41).Scan(&got))
			assert.Equal(t, 42, got)
		})
	}
}
