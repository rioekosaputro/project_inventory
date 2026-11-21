// Package domain berisi entity, konstanta status, sentinel error, dan interface repository.
// Package ini tidak boleh mengimpor Gin maupun pgx.
package domain

import "context"

// TxManager membungkus satu usecase dalam satu transaksi. Implementasi: pkg/database.TxManager.
//
// Context yang diterima fn membawa transaksi aktif; repository membacanya lewat
// database.GetExecutor(ctx, pool). Pemanggilan bersarang memakai transaksi yang sama.
type TxManager interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}
