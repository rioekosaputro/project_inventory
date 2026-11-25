package domain

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"rental-inventory-api/pkg/money"
)

const TaxCodePPN = "PPN"

// TaxSetting adalah baris konfigurasi tarif pajak efektif per tanggal (tabel tax_settings).
type TaxSetting struct {
	ID            int64
	TaxCode       string
	Rate          decimal.Decimal
	DPPNum        int
	DPPDen        int
	FakturCode    string
	EffectiveFrom time.Time
	EffectiveTo   *time.Time
	Notes         *string
}

// ToMoneyRule mengonversi TaxSetting menjadi money.TaxRule untuk kalkulator pajak.
func (t *TaxSetting) ToMoneyRule() money.TaxRule {
	return money.TaxRule{
		TaxCode:    t.TaxCode,
		Rate:       t.Rate,
		DPPNum:     t.DPPNum,
		DPPDen:     t.DPPDen,
		FakturCode: t.FakturCode,
	}
}

type TaxSettingRepository interface {
	// FindEffective mencari tarif pajak yang berlaku pada tanggal bisnis onDate (YYYY-MM-DD WIB).
	FindEffective(ctx context.Context, taxCode string, onDate time.Time) (*TaxSetting, error)
	List(ctx context.Context, taxCode string) ([]TaxSetting, error)
}
