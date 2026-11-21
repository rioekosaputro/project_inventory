//go:build integration

package seed_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/seed"
	"rental-inventory-api/internal/testutil/pgtest"
	"rental-inventory-api/pkg/password"
)

const migrationsDir = "../../migrations"

func TestRun_CreatesDataAndIsIdempotent(t *testing.T) {
	pool := pgtest.NewPool(t)
	pgtest.Apply(t, pool, migrationsDir, "up")
	ctx := context.Background()
	opts := seed.Options{AdminEmail: " Admin@Contoh.ID ", AdminPassword: "password-awal-1", AppBaseURL: "http://localhost:5173"}

	first, err := seed.Run(ctx, pool, opts)
	require.NoError(t, err)
	assert.True(t, first.AdminCreated)
	assert.Equal(t, 5, first.SettingsInserted)
	assert.Equal(t, 7, first.CategoriesInserted)

	var (
		hash       string
		role       string
		mustChange bool
	)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT password_hash, role, must_change_password FROM users WHERE email='admin@contoh.id'`).
		Scan(&hash, &role, &mustChange))
	assert.Equal(t, "ADMIN", role)
	assert.True(t, mustChange)
	assert.NotEqual(t, "password-awal-1", hash, "password tidak boleh disimpan polos")
	assert.True(t, password.Verify(hash, "password-awal-1"))

	var assets, consumables int
	require.NoError(t, pool.QueryRow(ctx, `SELECT
		count(*) FILTER (WHERE item_type='ASSET'), count(*) FILTER (WHERE item_type='CONSUMABLE')
		FROM categories`).Scan(&assets, &consumables))
	assert.Equal(t, 3, assets)
	assert.Equal(t, 4, consumables)

	// Run kedua: tidak ada duplikasi, tidak menimpa perubahan yang sudah dibuat.
	_, err = pool.Exec(ctx, `UPDATE app_settings SET value='true' WHERE key='lpb.allow_self_approve'`)
	require.NoError(t, err)
	opts.AdminPassword = "password-lain-99"

	second, err := seed.Run(ctx, pool, opts)
	require.NoError(t, err)
	assert.False(t, second.AdminCreated)
	assert.Zero(t, second.SettingsInserted)
	assert.Zero(t, second.CategoriesInserted)

	var users int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&users))
	assert.Equal(t, 1, users)

	var setting string
	require.NoError(t, pool.QueryRow(ctx, `SELECT value::text FROM app_settings WHERE key='lpb.allow_self_approve'`).Scan(&setting))
	assert.Equal(t, "true", setting, "seed tidak boleh menimpa setting yang sudah diubah")

	var stillOld string
	require.NoError(t, pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE email='admin@contoh.id'`).Scan(&stillOld))
	assert.True(t, password.Verify(stillOld, "password-awal-1"), "password admin yang ada tidak boleh diganti")
}
