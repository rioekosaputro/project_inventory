package main

import (
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToPgx5URL(t *testing.T) {
	got, err := toPgx5URL("postgres://u:p@localhost:5432/db?sslmode=disable")
	require.NoError(t, err)
	assert.Equal(t, "pgx5://u:p@localhost:5432/db?sslmode=disable", got)

	got, err = toPgx5URL("postgresql://u:p@h/db")
	require.NoError(t, err)
	assert.Equal(t, "pgx5://u:p@h/db", got)

	_, err = toPgx5URL("mysql://u@h/db")
	assert.Error(t, err)
}

func TestNewMigration_NumbersSequentiallyAndCreatesPair(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, newMigration(dir, "init_schema"))
	require.NoError(t, newMigration(dir, "add_products"))

	for _, f := range []string{
		"000001_init_schema.up.sql", "000001_init_schema.down.sql",
		"000002_add_products.up.sql", "000002_add_products.down.sql",
	} {
		_, err := os.Stat(filepath.Join(dir, f))
		assert.NoError(t, err, f)
	}
}

func TestNewMigration_RejectsBadName(t *testing.T) {
	for _, name := range []string{"Add Products", "../evil", "UPPER", "a-b", ""} {
		assert.Error(t, newMigration(t.TempDir(), name), name)
	}
}

func TestSourceURL(t *testing.T) {
	dir := t.TempDir()
	u, err := sourceURL(dir)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(u, "file://"))
	// Cara golang-migrate membaca path: host + path dari URL.
	parsed, err := neturl.Parse(u)
	require.NoError(t, err)
	assert.Equal(t, filepath.ToSlash(dir), parsed.Host+parsed.Path)

	_, err = sourceURL(filepath.Join(t.TempDir(), "tidak-ada"))
	assert.Error(t, err)
}
