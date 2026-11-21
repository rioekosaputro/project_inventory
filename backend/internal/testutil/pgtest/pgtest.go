// Package pgtest berisi helper test integrasi berbasis PostgreSQL lokal.
//
// Set TEST_DATABASE_URL (boleh sama dengan DATABASE_URL lokal). Setiap test mendapat
// schema sementara (search_path) yang dihapus di akhir, sehingga tabel di schema public
// tidak tersentuh. Tanpa TEST_DATABASE_URL, test di-skip.
package pgtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// NewPool membuat pool yang terkunci ke schema sementara.
func NewPool(t *testing.T) *pgxpool.Pool {
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

	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		_ = admin.Close(ctx)
	})
	return pool
}

// MigrationFiles mengembalikan path file *.up.sql (urut naik) atau *.down.sql (urut turun).
func MigrationFiles(t *testing.T, dir, kind string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*."+kind+".sql"))
	require.NoError(t, err)
	sort.Strings(files)
	if kind == "down" {
		for i, j := 0, len(files)-1; i < j; i, j = i+1, j-1 {
			files[i], files[j] = files[j], files[i]
		}
	}
	return files
}

// Apply menjalankan file migrasi ("up" naik, "down" turun) pada pool.
func Apply(t *testing.T, pool *pgxpool.Pool, dir, kind string) {
	t.Helper()
	for _, f := range MigrationFiles(t, dir, kind) {
		b, err := os.ReadFile(f)
		require.NoError(t, err)
		_, err = pool.Exec(context.Background(), string(b))
		require.NoError(t, err, strings.TrimSuffix(filepath.Base(f), ".sql"))
	}
}
