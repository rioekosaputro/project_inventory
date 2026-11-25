//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/repository/postgres"
	"rental-inventory-api/internal/testutil/pgtest"
	"rental-inventory-api/pkg/money"
)

func TestTaxSettingRepo_FindEffectiveByDateAndCalculate(t *testing.T) {
	pool := pgtest.NewPool(t)
	pgtest.Apply(t, pool, "../../../migrations", "up")
	ctx := context.Background()

	// Bersihkan dan isi dua periode aturan PPN: 11% (2022–2024) dan PMK 131/2024 (2025+).
	_, err := pool.Exec(ctx, `DELETE FROM tax_settings WHERE tax_code = 'PPN'`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO tax_settings (tax_code, rate, dpp_numerator, dpp_denominator, faktur_code, effective_from, effective_to, notes)
		VALUES
			('PPN', 11.00, 1, 1, '01', '2022-04-01', '2024-12-31', 'PPN 11% UU HPP'),
			('PPN', 12.00, 11, 12, '04', '2025-01-01', NULL, 'PMK 131/2024')`)
	require.NoError(t, err)

	repo := postgres.NewTaxSettingRepo(pool)

	// Tanggal sebelum aturan mana pun → ErrNotFound.
	_, err = repo.FindEffective(ctx, domain.TaxCodePPN, time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC))
	assert.ErrorIs(t, err, domain.ErrNotFound)

	// Tanggal 2024-12-31 → PPN 11% (1/1, faktur 01).
	oldRule, err := repo.FindEffective(ctx, domain.TaxCodePPN, time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Equal(t, "11.00", oldRule.Rate.StringFixed(2))
	assert.Equal(t, 1, oldRule.DPPNum)
	assert.Equal(t, "01", oldRule.FakturCode)

	// Tanggal 2026-11-25 → PMK 131/2024 (12%, 11/12, faktur 04) + kalkulasi pajak Rp5.000.000.
	newRule, err := repo.FindEffective(ctx, domain.TaxCodePPN, time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Equal(t, "12.00", newRule.Rate.StringFixed(2))
	assert.Equal(t, 11, newRule.DPPNum)
	assert.Equal(t, 12, newRule.DPPDen)
	assert.Equal(t, "04", newRule.FakturCode)

	tb, err := money.CalculateTax(decimal.NewFromInt(5000000), decimal.Zero, true, newRule.ToMoneyRule())
	require.NoError(t, err)
	assert.Equal(t, "4583333.00", tb.DPPOtherValue.StringFixed(2))
	assert.Equal(t, "550000.00", tb.TaxAmount.StringFixed(2))
	assert.Equal(t, "5550000.00", tb.GrandTotal.StringFixed(2))
}
