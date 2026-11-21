package database

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testURL = "postgres://u:p@localhost:5432/db?sslmode=disable"

func TestBuildPoolConfig_SessionKeepsDefaultExecMode(t *testing.T) {
	cfg, err := buildPoolConfig(Config{URL: testURL, PoolerMode: PoolerSession, MaxConns: 7})
	require.NoError(t, err)

	assert.Equal(t, pgx.QueryExecModeCacheStatement, cfg.ConnConfig.DefaultQueryExecMode)
	assert.EqualValues(t, 7, cfg.MaxConns)
}

func TestBuildPoolConfig_TransactionDisablesPreparedStatements(t *testing.T) {
	cfg, err := buildPoolConfig(Config{URL: testURL, PoolerMode: PoolerTransaction, MaxConns: 5})
	require.NoError(t, err)

	assert.Equal(t, pgx.QueryExecModeExec, cfg.ConnConfig.DefaultQueryExecMode)
	assert.EqualValues(t, 5, cfg.MaxConns)
}

func TestBuildPoolConfig_Errors(t *testing.T) {
	_, err := buildPoolConfig(Config{})
	assert.Error(t, err, "URL kosong")

	_, err = buildPoolConfig(Config{URL: testURL, PoolerMode: "pgbouncer"})
	assert.Error(t, err, "mode tidak dikenal")

	_, err = buildPoolConfig(Config{URL: "://rusak"})
	assert.Error(t, err, "URL tidak valid")
}

func TestMustInTx_PanicsOutsideTransaction(t *testing.T) {
	assert.Panics(t, func() { MustInTx(context.Background()) })
}
