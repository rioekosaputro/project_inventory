package database

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type txCtxKey struct{}

// DBTX adalah subset method yang dimiliki bersama oleh *pgxpool.Pool dan pgx.Tx,
// sehingga repository tidak perlu tahu sedang berjalan di dalam transaksi atau tidak.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
	CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error)
}

var (
	_ DBTX = (*pgxpool.Pool)(nil)
	_ DBTX = pgx.Tx(nil)
)

// GetExecutor mengembalikan tx aktif dari context bila ada, selain itu pool.
func GetExecutor(ctx context.Context, pool *pgxpool.Pool) DBTX {
	if tx, ok := txFromContext(ctx); ok {
		return tx
	}
	return pool
}

// MustInTx dipakai repository yang WAJIB dipanggil di dalam transaksi (method Lock*).
func MustInTx(ctx context.Context) pgx.Tx {
	tx, ok := txFromContext(ctx)
	if !ok {
		panic("database: method ini wajib dipanggil di dalam WithinTransaction")
	}
	return tx
}

func txFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txCtxKey{}).(pgx.Tx)
	return tx, ok
}
