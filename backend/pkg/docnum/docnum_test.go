package docnum

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/clock"
)

var _ domain.DocNumberGenerator = (*Generator)(nil)

func TestFormat(t *testing.T) {
	assert.Equal(t, "LPBO/WH-SMG-01/2026/11/0007", Format("LPBO", "WH-SMG-01", "202611", 7))
	assert.Equal(t, "INV/GLOBAL/2026/01/0001", Format("INV", "GLOBAL", "202601", 1))
	assert.Equal(t, "SO/GLOBAL/2026/12/12345", Format("SO", "GLOBAL", "202612", 12345), "lebar bertambah, tidak dipotong")
}

// Bulan ditentukan oleh zona WIB: 30 Nov 23:30 UTC sudah 1 Des di Jakarta.
func TestPeriod_UsesWIBNotUTC(t *testing.T) {
	assert.Equal(t, "202612", Period(time.Date(2026, 11, 30, 17, 0, 0, 0, time.UTC)))
	assert.Equal(t, "202611", Period(time.Date(2026, 11, 30, 16, 59, 59, 0, time.UTC)))
	assert.Equal(t, "202701", Period(time.Date(2026, 12, 31, 17, 0, 0, 0, time.UTC)))
	assert.Equal(t, "202611", Period(time.Date(2026, 11, 15, 12, 0, 0, 0, clock.WIB)))
}

// Validasi berjalan sebelum menyentuh database (pool nil aman).
func TestNextNumber_RejectsInvalidInput(t *testing.T) {
	g := New(nil)
	ctx := context.Background()

	cases := []struct {
		name              string
		docType, scope, p string
		want              error
	}{
		{"jenis kosong", "", "GLOBAL", "202611", ErrInvalidDocType},
		{"jenis huruf kecil", "lpbi", "GLOBAL", "202611", ErrInvalidDocType},
		{"jenis mengandung garis miring", "LPB/I", "GLOBAL", "202611", ErrInvalidDocType},
		{"jenis terlalu panjang", strings.Repeat("A", 21), "GLOBAL", "202611", ErrInvalidDocType},
		{"scope kosong", "LPBI", "", "202611", ErrInvalidScope},
		{"scope mengandung garis miring", "LPBI", "WH/01", "202611", ErrInvalidScope},
		{"scope mengandung spasi", "LPBI", "WH 01", "202611", ErrInvalidScope},
		{"scope terlalu panjang", "LPBI", strings.Repeat("W", 41), "202611", ErrInvalidScope},
		{"periode pendek", "LPBI", "GLOBAL", "2026", ErrInvalidPeriod},
		{"periode bukan angka", "LPBI", "GLOBAL", "2026AB", ErrInvalidPeriod},
		{"periode kosong", "LPBI", "GLOBAL", "", ErrInvalidPeriod},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := g.NextNumber(ctx, tc.docType, tc.scope, tc.p)
			assert.ErrorIs(t, err, tc.want)
		})
	}
}

func TestNext_PropagatesValidationError(t *testing.T) {
	_, err := New(nil).Next(context.Background(), "lpbi", "GLOBAL", time.Now())
	assert.ErrorIs(t, err, ErrInvalidDocType)
}

func TestFormatAssetTag(t *testing.T) {
	nov := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	assert.Equal(t, "AST-2611-00012", FormatAssetTag(nov, 12))
	assert.Equal(t, "AST-2611-00001", FormatAssetTag(nov, 1))
	assert.Equal(t, "AST-2611-123456", FormatAssetTag(nov, 123456), "lebar bertambah, tidak dipotong")
	assert.LessOrEqual(t, len(FormatAssetTag(nov, 99999999)), 30, "harus muat di kolom asset_tag VARCHAR(30)")
}

// Bulan pada label mengikuti WIB: 30 Nov 17:30 UTC sudah Desember di Jakarta.
func TestFormatAssetTag_MonthUsesWIB(t *testing.T) {
	assert.Equal(t, "AST-2612-00007", FormatAssetTag(time.Date(2026, 11, 30, 17, 30, 0, 0, time.UTC), 7))
	assert.Equal(t, "AST-2701-00007", FormatAssetTag(time.Date(2026, 12, 31, 17, 0, 0, 0, time.UTC), 7))
}
