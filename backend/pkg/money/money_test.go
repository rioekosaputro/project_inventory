package money

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestCalculateTax_TableCases(t *testing.T) {
	pmk131 := PMK131Rule()
	ppn11Old := TaxRule{
		TaxCode:    "PPN",
		Rate:       d("11.00"),
		DPPNum:     1,
		DPPDen:     1,
		FakturCode: "01",
	}

	tests := []struct {
		name           string
		subtotal       decimal.Decimal
		discount       decimal.Decimal
		isPPN          bool
		rule           TaxRule
		wantDPP        string
		wantDPPOther   string
		wantTaxRate    string
		wantTaxAmount  string
		wantGrandTotal string
		wantFakturCode string
	}{
		{
			name:           "PMK 131/2024 contoh plan 5.6 (10 printer x Rp500.000)",
			subtotal:       d("5000000"),
			discount:       d("0"),
			isPPN:          true,
			rule:           pmk131,
			wantDPP:        "5000000.00",
			wantDPPOther:   "4583333.00",
			wantTaxRate:    "12.00",
			wantTaxAmount:  "550000.00",
			wantGrandTotal: "5550000.00",
			wantFakturCode: "04",
		},
		{
			name:           "Non-PPN menghasilkan pajak 0 dan grand_total sama dengan DPP",
			subtotal:       d("5000000"),
			discount:       d("250000"),
			isPPN:          false,
			rule:           pmk131,
			wantDPP:        "4750000.00",
			wantDPPOther:   "0.00",
			wantTaxRate:    "0.00",
			wantTaxAmount:  "0.00",
			wantGrandTotal: "4750000.00",
			wantFakturCode: "",
		},
		{
			name:           "PPN dengan diskon (DPP = subtotal - diskon)",
			subtotal:       d("1000000"),
			discount:       d("100000"),
			isPPN:          true,
			rule:           pmk131,
			wantDPP:        "900000.00",
			wantDPPOther:   "825000.00",
			wantTaxRate:    "12.00",
			wantTaxAmount:  "99000.00",
			wantGrandTotal: "999000.00",
			wantFakturCode: "04",
		},
		{
			name:           "Diskon penuh 100% menghasilkan pajak dan grand total 0",
			subtotal:       d("750000"),
			discount:       d("750000"),
			isPPN:          true,
			rule:           pmk131,
			wantDPP:        "0.00",
			wantDPPOther:   "0.00",
			wantTaxRate:    "12.00",
			wantTaxAmount:  "0.00",
			wantGrandTotal: "0.00",
			wantFakturCode: "04",
		},
		{
			name:           "Pembulatan HALF_UP pada pecahan 11/12 (DPP 100 -> 91.666.. -> 92; 92*12%=11.04 -> 11)",
			subtotal:       d("100"),
			discount:       d("0"),
			isPPN:          true,
			rule:           pmk131,
			wantDPP:        "100.00",
			wantDPPOther:   "92.00",
			wantTaxRate:    "12.00",
			wantTaxAmount:  "11.00",
			wantGrandTotal: "111.00",
			wantFakturCode: "04",
		},
		{
			name:           "Pembulatan HALF_UP tepat .5 naik ke atas (DPP 6 -> 6*11/12=5.5 -> 6; 6*12%=0.72 -> 1)",
			subtotal:       d("6"),
			discount:       d("0"),
			isPPN:          true,
			rule:           pmk131,
			wantDPP:        "6.00",
			wantDPPOther:   "6.00",
			wantTaxRate:    "12.00",
			wantTaxAmount:  "1.00",
			wantGrandTotal: "7.00",
			wantFakturCode: "04",
		},
		{
			name:           "Tarif PPN lama 11% (DPP 1/1, faktur 01)",
			subtotal:       d("2000000"),
			discount:       d("0"),
			isPPN:          true,
			rule:           ppn11Old,
			wantDPP:        "2000000.00",
			wantDPPOther:   "2000000.00",
			wantTaxRate:    "11.00",
			wantTaxAmount:  "220000.00",
			wantGrandTotal: "2220000.00",
			wantFakturCode: "01",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CalculateTax(tc.subtotal, tc.discount, tc.isPPN, tc.rule)
			require.NoError(t, err)
			assert.Equal(t, tc.wantDPP, got.DPP.StringFixed(2))
			assert.Equal(t, tc.wantDPPOther, got.DPPOtherValue.StringFixed(2))
			assert.Equal(t, tc.wantTaxRate, got.TaxRate.StringFixed(2))
			assert.Equal(t, tc.wantTaxAmount, got.TaxAmount.StringFixed(2))
			assert.Equal(t, tc.wantGrandTotal, got.GrandTotal.StringFixed(2))
			assert.Equal(t, tc.wantFakturCode, got.FakturCode)
		})
	}
}

func TestCalculateTax_ValidationErrors(t *testing.T) {
	pmk := PMK131Rule()
	_, err := CalculateTax(d("-1"), Zero, true, pmk)
	assert.ErrorIs(t, err, ErrNegativeAmount)

	_, err = CalculateTax(d("1000"), d("1001"), true, pmk)
	assert.ErrorIs(t, err, ErrDiscountExceeds)

	_, err = CalculateTax(d("1000"), Zero, true, TaxRule{Rate: d("12"), DPPNum: 0, DPPDen: 12})
	assert.ErrorIs(t, err, ErrInvalidTaxRule)
}

func TestProrata(t *testing.T) {
	rate := d("500000")

	full, err := Prorata(rate, 30, 30)
	require.NoError(t, err)
	assert.Equal(t, "500000.00", full.StringFixed(2))

	half, err := Prorata(rate, 15, 30)
	require.NoError(t, err)
	assert.Equal(t, "250000.00", half.StringFixed(2))

	nineDays, err := Prorata(rate, 9, 30)
	require.NoError(t, err)
	assert.Equal(t, "150000.00", nineDays.StringFixed(2))

	odd, err := Prorata(rate, 10, 31)
	require.NoError(t, err)
	assert.Equal(t, "161290.32", odd.StringFixed(2))

	_, err = Prorata(rate, 32, 31)
	assert.ErrorIs(t, err, ErrInvalidDayPeriod)
}

func TestParseAndFormatRupiah(t *testing.T) {
	v, err := ParseNonNegative(" 550000.50 ")
	require.NoError(t, err)
	assert.Equal(t, "550000.50", v.StringFixed(2))
	assert.Equal(t, "Rp 550.000,50", FormatRupiah(v))
	assert.Equal(t, "Rp 5.000.000", FormatRupiah(d("5000000")))

	_, err = ParseNonNegative("1e5")
	assert.ErrorIs(t, err, ErrInvalidAmount)

	_, err = ParseNonNegative("-100")
	assert.ErrorIs(t, err, ErrNegativeAmount)
}
