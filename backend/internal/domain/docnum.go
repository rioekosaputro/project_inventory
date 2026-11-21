package domain

import (
	"context"
	"time"
)

// DocNumberGenerator menerbitkan nomor dokumen berurutan (LPBI, LPBO, SO, DO, INV, OPN, TKT, KTR).
// Wajib dipanggil di dalam transaksi usecase yang sama dengan penyimpanan dokumen
// (CLAUDE.md invarian #6). Implementasi: pkg/docnum.Generator.
type DocNumberGenerator interface {
	// Next mengembalikan {docType}/{scope}/{YYYY}/{MM}/{NNNN}; bulan dihitung dari t dalam WIB.
	Next(ctx context.Context, docType, scope string, t time.Time) (string, error)
	// NextAssetTag mengembalikan label unit AST-{YYMM}-{NNNNN} (counter global tanpa reset).
	NextAssetTag(ctx context.Context, t time.Time) (string, error)
	// NextNumber menaikkan counter (docType, scope, period) dan mengembalikan nilainya (mulai 1).
	NextNumber(ctx context.Context, docType, scope, period string) (int, error)
}
