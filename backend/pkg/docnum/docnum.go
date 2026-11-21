// Package docnum membuat nomor dokumen berurutan (LPBI, LPBO, SO, DO, INV, OPN, TKT, KTR, AST).
//
// Nomor diambil dari tabel document_sequences dengan satu pernyataan atomik
// (INSERT ... ON CONFLICT DO UPDATE ... RETURNING). Baris urutan terkunci sampai transaksi
// selesai, sehingga:
//   - dua transaksi paralel tidak pernah mendapat nomor yang sama (yang kedua menunggu);
//   - tidak ada celah: transaksi yang rollback ikut membatalkan kenaikan counter-nya.
//
// Karena itu Next WAJIB dipanggil di dalam transaksi usecase yang sama dengan penyimpanan
// dokumennya (CLAUDE.md invarian #6). Konsekuensinya, penerbitan nomor untuk (jenis, scope,
// periode) yang sama berjalan serial sampai commit — wajar untuk dokumen bisnis, jadi
// jangan menahan transaksi terlalu lama setelah memanggil Next.
//
// Periode dihitung dalam zona Asia/Jakarta (pkg/clock), bukan UTC.
package docnum

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/pkg/clock"
	"rental-inventory-api/pkg/database"
)

// PeriodNone dipakai untuk urutan yang tidak di-reset per bulan (mis. asset tag, scope GLOBAL).
const PeriodNone = "000000"

// ScopeGlobal adalah scope untuk dokumen yang tidak terikat gudang.
const ScopeGlobal = "GLOBAL"

// Batas panjang mengikuti kolom document_sequences (doc_type 20, scope_key 40).
const (
	maxDocTypeLen = 20
	maxScopeLen   = 40
)

var (
	docTypeRe = regexp.MustCompile(`^[A-Z]+$`)
	periodRe  = regexp.MustCompile(`^\d{6}$`)

	ErrInvalidDocType = errors.New("docnum: jenis dokumen harus huruf kapital A-Z, maksimal 20 karakter")
	ErrInvalidScope   = errors.New("docnum: scope wajib diisi, maksimal 40 karakter, tanpa '/' atau spasi")
	ErrInvalidPeriod  = errors.New("docnum: periode harus 6 digit (YYYYMM atau 000000)")
)

// Generator menerbitkan nomor dokumen. Implementasi domain.DocNumberGenerator.
type Generator struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Generator { return &Generator{pool: pool} }

// Period mengembalikan periode YYYYMM dari t dalam zona WIB.
func Period(t time.Time) string { return t.In(clock.WIB).Format("200601") }

// Format menyusun nomor: {DOCTYPE}/{SCOPE}/{YYYY}/{MM}/{NNNN}, mis. LPBO/WH-SMG-01/2026/11/0007.
// Counter melebihi 9999 tetap valid (lebar bertambah, tidak dipotong).
func Format(docType, scope, period string, n int) string {
	return fmt.Sprintf("%s/%s/%s/%s/%04d", docType, scope, period[:4], period[4:], n)
}

// Next menerbitkan nomor dokumen berikutnya untuk (docType, scope) pada bulan t (WIB).
// Wajib dipanggil di dalam transaksi (database.MustInTx panik bila tidak).
func (g *Generator) Next(ctx context.Context, docType, scope string, t time.Time) (string, error) {
	period := Period(t)
	n, err := g.NextNumber(ctx, docType, scope, period)
	if err != nil {
		return "", err
	}
	return Format(docType, scope, period, n), nil
}

// NextNumber menaikkan counter (docType, scope, period) dan mengembalikan nilainya (mulai 1).
// Dipakai langsung oleh pemanggil yang menyusun format sendiri atau memakai PeriodNone.
// Wajib dipanggil di dalam transaksi.
func (g *Generator) NextNumber(ctx context.Context, docType, scope, period string) (int, error) {
	if err := validate(docType, scope, period); err != nil {
		return 0, err
	}
	tx := database.MustInTx(ctx)

	var n int
	err := tx.QueryRow(ctx, `
		INSERT INTO document_sequences (doc_type, scope_key, period, last_number)
		VALUES ($1, $2, $3, 1)
		ON CONFLICT (doc_type, scope_key, period)
		DO UPDATE SET last_number = document_sequences.last_number + 1
		RETURNING last_number`, docType, scope, period).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("docnum: gagal menerbitkan nomor %s/%s: %w", docType, scope, err)
	}
	return n, nil
}

func validate(docType, scope, period string) error {
	if len(docType) == 0 || len(docType) > maxDocTypeLen || !docTypeRe.MatchString(docType) {
		return ErrInvalidDocType
	}
	if scope == "" || len(scope) > maxScopeLen || strings.ContainsAny(scope, "/ \t\r\n") {
		return ErrInvalidScope
	}
	if !periodRe.MatchString(period) {
		return ErrInvalidPeriod
	}
	return nil
}
