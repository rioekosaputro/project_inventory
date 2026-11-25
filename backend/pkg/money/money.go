// Package money menyediakan utilitas uang dan kalkulator pajak berbasis shopspring/decimal.
// Semua perhitungan moneter WAJIB memakai decimal.Decimal (dilarang float64, CLAUDE.md invarian #3).
package money

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/shopspring/decimal"
)

var (
	Zero    = decimal.Zero
	Hundred = decimal.NewFromInt(100)

	// Menerima angka desimal polos opsional tanda minus, tanpa notasi ilmiah (e/E).
	decimalPattern = regexp.MustCompile(`^-?\d+(\.\d+)?$`)

	ErrInvalidAmount    = errors.New("money: format angka tidak valid")
	ErrNegativeAmount   = errors.New("money: nominal tidak boleh negatif")
	ErrDiscountExceeds  = errors.New("money: diskon tidak boleh melebihi subtotal")
	ErrInvalidTaxRule   = errors.New("money: aturan pajak tidak valid")
	ErrInvalidDayPeriod = errors.New("money: periode hari tidak valid")
)

// Parse mengubah string angka desimal polos (tanpa notasi ilmiah) menjadi decimal.Decimal.
func Parse(raw string) (decimal.Decimal, error) {
	s := strings.TrimSpace(raw)
	if !decimalPattern.MatchString(s) {
		return Zero, ErrInvalidAmount
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return Zero, ErrInvalidAmount
	}
	return d, nil
}

// ParseNonNegative mem-parse nominal uang yang tidak boleh negatif.
func ParseNonNegative(raw string) (decimal.Decimal, error) {
	d, err := Parse(raw)
	if err != nil {
		return Zero, err
	}
	if d.IsNegative() {
		return Zero, ErrNegativeAmount
	}
	return d, nil
}

// RoundRupiah membulatkan nominal ke rupiah penuh dengan metode HALF_UP (plan Bagian 5.6).
func RoundRupiah(d decimal.Decimal) decimal.Decimal {
	return d.Round(0)
}

// Round2 membulatkan nominal ke 2 angka desimal (HALF_UP).
func Round2(d decimal.Decimal) decimal.Decimal {
	return d.Round(2)
}

// LineTotal menghitung qty * unitPrice dibulatkan 2 desimal.
func LineTotal(qty int, unitPrice decimal.Decimal) decimal.Decimal {
	return Round2(unitPrice.Mul(decimal.NewFromInt(int64(qty))))
}

// Prorata menghitung tagihan proporsional: round(monthlyRate * billableDays / periodDays, 2).
func Prorata(monthlyRate decimal.Decimal, billableDays, periodDays int) (decimal.Decimal, error) {
	if monthlyRate.IsNegative() {
		return Zero, ErrNegativeAmount
	}
	if periodDays <= 0 || billableDays < 0 || billableDays > periodDays {
		return Zero, ErrInvalidDayPeriod
	}
	if billableDays == 0 {
		return Zero, nil
	}
	if billableDays == periodDays {
		return Round2(monthlyRate), nil
	}
	num := monthlyRate.Mul(decimal.NewFromInt(int64(billableDays)))
	return Round2(num.Div(decimal.NewFromInt(int64(periodDays)))), nil
}

// FormatRupiah memformat nominal menjadi string Rupiah Indonesia (mis. "Rp 5.000.000" atau "Rp 5.000.000,50").
func FormatRupiah(d decimal.Decimal) string {
	neg := d.IsNegative()
	abs := d.Abs().Round(2)
	parts := strings.Split(abs.StringFixed(2), ".")
	intPart, fracPart := parts[0], parts[1]

	// Sisipkan pemisah ribuan '.' dari belakang.
	var b strings.Builder
	n := len(intPart)
	for i := 0; i < n; i++ {
		if i > 0 && (n-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteByte(intPart[i])
	}

	prefix := "Rp "
	if neg {
		prefix = "-Rp "
	}
	if fracPart == "00" {
		return fmt.Sprintf("%s%s", prefix, b.String())
	}
	return fmt.Sprintf("%s%s,%s", prefix, b.String(), fracPart)
}
