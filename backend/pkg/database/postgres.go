// Package database berisi koneksi PostgreSQL (pgxpool), TxManager, dan helper executor.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Mode pooler (lihat plan Bagian 3.4 dan invarian #9).
const (
	PoolerSession     = "session"
	PoolerTransaction = "transaction"
)

// Config adalah parameter koneksi pool.
type Config struct {
	URL        string
	PoolerMode string // session | transaction
	MaxConns   int
}

// buildPoolConfig menyusun pgxpool.Config. Pada transaction pooler (Supabase 6543)
// prepared statement tidak didukung, sehingga cache statement dimatikan.
func buildPoolConfig(c Config) (*pgxpool.Config, error) {
	if c.URL == "" {
		return nil, fmt.Errorf("database: DATABASE_URL kosong")
	}
	cfg, err := pgxpool.ParseConfig(c.URL)
	if err != nil {
		return nil, fmt.Errorf("database: DATABASE_URL tidak valid: %w", err)
	}

	switch c.PoolerMode {
	case "", PoolerSession:
		// default pgx (prepared statement cache) aman untuk koneksi langsung/session pooler.
	case PoolerTransaction:
		cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	default:
		return nil, fmt.Errorf("database: DB_POOLER_MODE tidak dikenal: %q", c.PoolerMode)
	}

	if c.MaxConns > 0 {
		cfg.MaxConns = int32(c.MaxConns)
	}
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = time.Minute
	cfg.ConnConfig.ConnectTimeout = 10 * time.Second
	return cfg, nil
}

// NewPool membuat pool dan memastikan database dapat dijangkau (ping).
func NewPool(ctx context.Context, c Config) (*pgxpool.Pool, error) {
	cfg, err := buildPoolConfig(c)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("database: gagal membuat pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: gagal terhubung: %w", err)
	}
	return pool, nil
}
