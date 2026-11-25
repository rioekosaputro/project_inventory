package money

import "github.com/shopspring/decimal"

// TaxRule adalah aturan pajak efektif pada tanggal transaksi (berasal dari tabel tax_settings).
// Untuk PMK 131/2024: Rate=12.00, DPPNum=11, DPPDen=12, FakturCode="04".
type TaxRule struct {
	TaxCode    string
	Rate       decimal.Decimal // persen, mis. 12.00
	DPPNum     int             // pembilang DPP Nilai Lain, mis. 11
	DPPDen     int             // penyebut DPP Nilai Lain, mis. 12
	FakturCode string          // kode faktur pajak, mis. "04"
}

// PMK131Rule mengembalikan aturan PPN PMK 131/2024 (tarif 12% × DPP Nilai Lain 11/12, kode faktur 04).
func PMK131Rule() TaxRule {
	return TaxRule{
		TaxCode:    "PPN",
		Rate:       decimal.NewFromInt(12),
		DPPNum:     11,
		DPPDen:     12,
		FakturCode: "04",
	}
}

// TaxBreakdown adalah hasil kalkulasi pajak lengkap yang disnapshot ke SO / Invoice.
type TaxBreakdown struct {
	Subtotal       decimal.Decimal
	DiscountAmount decimal.Decimal
	DPP            decimal.Decimal // subtotal - discount_amount
	DPPOtherValue  decimal.Decimal // round(dpp * DPPNum / DPPDen), 0 bila non-PPN
	TaxRate        decimal.Decimal // snapshot tarif (mis. 12.00), 0 bila non-PPN
	TaxAmount      decimal.Decimal // round(dpp_other_value * rate / 100), 0 bila non-PPN
	GrandTotal     decimal.Decimal // dpp + tax_amount
	FakturCode     string          // kosong bila non-PPN
}

// CalculateTax menghitung DPP, DPP Nilai Lain, PPN, dan Grand Total sesuai plan Bagian 5.6:
//
//	dpp             = subtotal - discount
//	dpp_other_value = RoundRupiah(dpp * DPPNum / DPPDen)   (0 bila non-PPN)
//	tax_amount      = RoundRupiah(dpp_other_value * Rate / 100) (0 bila non-PPN)
//	grand_total     = dpp + tax_amount
func CalculateTax(subtotal, discount decimal.Decimal, isPPN bool, rule TaxRule) (TaxBreakdown, error) {
	if subtotal.IsNegative() || discount.IsNegative() {
		return TaxBreakdown{}, ErrNegativeAmount
	}
	sub := Round2(subtotal)
	disc := Round2(discount)
	if disc.GreaterThan(sub) {
		return TaxBreakdown{}, ErrDiscountExceeds
	}

	dpp := Round2(sub.Sub(disc))
	if !isPPN {
		return TaxBreakdown{
			Subtotal:       sub,
			DiscountAmount: disc,
			DPP:            dpp,
			DPPOtherValue:  Zero,
			TaxRate:        Zero,
			TaxAmount:      Zero,
			GrandTotal:     dpp,
			FakturCode:     "",
		}, nil
	}

	if rule.Rate.IsNegative() || rule.DPPNum <= 0 || rule.DPPDen <= 0 {
		return TaxBreakdown{}, ErrInvalidTaxRule
	}

	dppOther := RoundRupiah(
		dpp.Mul(decimal.NewFromInt(int64(rule.DPPNum))).Div(decimal.NewFromInt(int64(rule.DPPDen))),
	)
	taxAmount := RoundRupiah(dppOther.Mul(rule.Rate).Div(Hundred))
	grandTotal := Round2(dpp.Add(taxAmount))

	return TaxBreakdown{
		Subtotal:       sub,
		DiscountAmount: disc,
		DPP:            dpp,
		DPPOtherValue:  dppOther,
		TaxRate:        Round2(rule.Rate),
		TaxAmount:      taxAmount,
		GrandTotal:     grandTotal,
		FakturCode:     rule.FakturCode,
	}, nil
}

// CalculateLinesTax menjumlahkan semua baris (line totals) lalu menghitung pajaknya.
func CalculateLinesTax(lines []decimal.Decimal, discount decimal.Decimal, isPPN bool, rule TaxRule) (TaxBreakdown, error) {
	subtotal := Zero
	for _, l := range lines {
		if l.IsNegative() {
			return TaxBreakdown{}, ErrNegativeAmount
		}
		subtotal = subtotal.Add(Round2(l))
	}
	return CalculateTax(subtotal, discount, isPPN, rule)
}
