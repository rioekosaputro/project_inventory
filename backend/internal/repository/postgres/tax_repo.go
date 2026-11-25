package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type TaxSettingRepo struct{ pool *pgxpool.Pool }

func NewTaxSettingRepo(pool *pgxpool.Pool) *TaxSettingRepo { return &TaxSettingRepo{pool: pool} }

var _ domain.TaxSettingRepository = (*TaxSettingRepo)(nil)

const taxSettingColumns = `id, tax_code, rate, dpp_numerator, dpp_denominator, faktur_code,
	effective_from, effective_to, notes`

func scanTaxSetting(row pgx.Row) (domain.TaxSetting, error) {
	var t domain.TaxSetting
	err := row.Scan(
		&t.ID, &t.TaxCode, &t.Rate, &t.DPPNum, &t.DPPDen, &t.FakturCode,
		&t.EffectiveFrom, &t.EffectiveTo, &t.Notes,
	)
	return t, err
}

func (r *TaxSettingRepo) FindEffective(ctx context.Context, taxCode string, onDate time.Time) (*domain.TaxSetting, error) {
	row := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		SELECT `+taxSettingColumns+`
		FROM tax_settings
		WHERE tax_code = $1
		  AND effective_from <= $2::date
		  AND (effective_to IS NULL OR effective_to >= $2::date)
		ORDER BY effective_from DESC
		LIMIT 1`,
		taxCode, onDate.Format("2006-01-02"),
	)
	ts, err := scanTaxSetting(row)
	if err != nil {
		return nil, notFound(err)
	}
	return &ts, nil
}

func (r *TaxSettingRepo) List(ctx context.Context, taxCode string) ([]domain.TaxSetting, error) {
	q := `SELECT ` + taxSettingColumns + ` FROM tax_settings`
	var args []any
	if taxCode != "" {
		q += ` WHERE tax_code = $1`
		args = append(args, taxCode)
	}
	q += ` ORDER BY tax_code ASC, effective_from DESC`

	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.TaxSetting, 0)
	for rows.Next() {
		ts, err := scanTaxSetting(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, ts)
	}
	return items, rows.Err()
}
