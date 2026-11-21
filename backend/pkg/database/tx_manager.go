package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TxManager menjalankan fungsi di dalam satu transaksi database.
// Implementasi domain.TxManager (usecase hanya bergantung pada interface itu).
type TxManager struct {
	pool *pgxpool.Pool
}

func NewTxManager(pool *pgxpool.Pool) *TxManager {
	return &TxManager{pool: pool}
}

// WithinTransaction menjalankan fn di dalam transaksi (isolation default: READ COMMITTED).
//
//   - fn mengembalikan nil        → COMMIT.
//   - fn mengembalikan error      → ROLLBACK, error asli dikembalikan apa adanya (errors.Is tetap bekerja).
//   - fn panic                    → ROLLBACK, lalu panic diteruskan.
//   - dipanggil bersarang         → fn memakai transaksi yang sudah ada (tanpa savepoint).
//     Konsekuensinya: error dari fn bersarang yang di-swallow oleh pemanggil luar TIDAK
//     membatalkan tulisan fn bersarang; selalu propagasikan error ke atas.
//
// Context yang diteruskan ke fn membawa tx; repository membacanya lewat GetExecutor.
func (m *TxManager) WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if _, ok := txFromContext(ctx); ok {
		return fn(ctx)
	}

	tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("database: gagal memulai transaksi: %w", err)
	}
	txCtx := context.WithValue(ctx, txCtxKey{}, tx)

	// Rollback memakai context yang tidak ikut batal agar tetap berjalan
	// ketika ctx request sudah di-cancel.
	rollback := func() error {
		rbErr := tx.Rollback(context.WithoutCancel(ctx))
		if errors.Is(rbErr, pgx.ErrTxClosed) {
			return nil
		}
		return rbErr
	}

	defer func() {
		if r := recover(); r != nil {
			_ = rollback()
			panic(r)
		}
	}()

	if fnErr := fn(txCtx); fnErr != nil {
		if rbErr := rollback(); rbErr != nil {
			return errors.Join(fnErr, fmt.Errorf("database: rollback gagal: %w", rbErr))
		}
		return fnErr
	}

	if err := tx.Commit(ctx); err != nil {
		_ = rollback()
		return fmt.Errorf("database: commit gagal: %w", err)
	}
	return nil
}
