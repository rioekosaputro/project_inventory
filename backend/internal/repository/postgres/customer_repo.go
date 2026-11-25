package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type CustomerRepo struct{ pool *pgxpool.Pool }

func NewCustomerRepo(pool *pgxpool.Pool) *CustomerRepo { return &CustomerRepo{pool: pool} }

var _ domain.CustomerRepository = (*CustomerRepo)(nil)

const customerBaseColumns = `c.id, c.customer_code, c.company_name, c.pic_name, c.phone, c.email,
	c.is_pkp, c.npwp, c.tax_address, c.billing_address, c.billing_postal_code, c.billing_email,
	c.payment_term_days, c.is_active, c.created_by, c.created_at, c.updated_at`

const customerListColumns = customerBaseColumns + `,
	(SELECT COUNT(*) FROM customer_sites cs WHERE cs.customer_id = c.id AND cs.is_active = TRUE) AS site_count,
	(SELECT COUNT(*) FROM product_serials ps WHERE ps.current_customer_id = c.id AND ps.status = 'RENTED') AS active_asset_count`

const siteColumns = `s.id, s.customer_id, s.site_name, s.pic_name, s.pic_phone, s.address_line,
	s.subdistrict, s.district, s.city, s.province, s.postal_code,
	s.latitude, s.longitude, s.is_primary, s.is_active, s.created_at, s.updated_at`

func scanCustomerBase(row pgx.Row) (domain.Customer, error) {
	var c domain.Customer
	err := row.Scan(
		&c.ID, &c.CustomerCode, &c.CompanyName, &c.PICName, &c.Phone, &c.Email,
		&c.IsPKP, &c.NPWP, &c.TaxAddress, &c.BillingAddress, &c.BillingPostalCode, &c.BillingEmail,
		&c.PaymentTermDays, &c.IsActive, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
	)
	return c, err
}

func scanCustomerWithCounts(row pgx.Row) (domain.Customer, error) {
	var c domain.Customer
	err := row.Scan(
		&c.ID, &c.CustomerCode, &c.CompanyName, &c.PICName, &c.Phone, &c.Email,
		&c.IsPKP, &c.NPWP, &c.TaxAddress, &c.BillingAddress, &c.BillingPostalCode, &c.BillingEmail,
		&c.PaymentTermDays, &c.IsActive, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
		&c.SiteCount, &c.ActiveAssetCount,
	)
	return c, err
}

func scanCustomerSite(row pgx.Row) (domain.CustomerSite, error) {
	var s domain.CustomerSite
	err := row.Scan(
		&s.ID, &s.CustomerID, &s.SiteName, &s.PICName, &s.PICPhone, &s.AddressLine,
		&s.Subdistrict, &s.District, &s.City, &s.Province, &s.PostalCode,
		&s.Latitude, &s.Longitude, &s.IsPrimary, &s.IsActive, &s.CreatedAt, &s.UpdatedAt,
	)
	return s, err
}

func (r *CustomerRepo) FindByID(ctx context.Context, id int64) (*domain.Customer, error) {
	c, err := scanCustomerWithCounts(database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT `+customerListColumns+` FROM customers c WHERE c.id = $1`, id))
	if err != nil {
		return nil, notFound(err)
	}
	return &c, nil
}

func (r *CustomerRepo) FindByIDForUpdate(ctx context.Context, id int64) (*domain.Customer, error) {
	database.MustInTx(ctx)
	c, err := scanCustomerBase(database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT `+customerBaseColumns+` FROM customers c WHERE c.id = $1 FOR UPDATE`, id))
	if err != nil {
		return nil, notFound(err)
	}
	return &c, nil
}

func (r *CustomerRepo) List(ctx context.Context, f domain.CustomerFilter, p domain.PageParams) ([]domain.Customer, int64, error) {
	var b filterBuilder
	b.like(f.Query, "c.customer_code", "c.company_name", "c.pic_name", "c.npwp")
	if f.IsPKP != nil {
		b.add("c.is_pkp = $%d", *f.IsPKP)
	}
	if f.IsActive != nil {
		b.add("c.is_active = $%d", *f.IsActive)
	}
	return queryPage(ctx, database.GetExecutor(ctx, r.pool), "customers c", customerListColumns, &b, p, "c.id",
		func(row pgx.Row) (domain.Customer, error) { return scanCustomerWithCounts(row) })
}

func (r *CustomerRepo) Create(ctx context.Context, c *domain.Customer) (*domain.Customer, error) {
	created, err := scanCustomerBase(database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO customers AS c (
			customer_code, company_name, pic_name, phone, email,
			is_pkp, npwp, tax_address, billing_address, billing_postal_code, billing_email,
			payment_term_days, is_active, created_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING `+customerBaseColumns,
		c.CustomerCode, c.CompanyName, c.PICName, c.Phone, c.Email,
		c.IsPKP, c.NPWP, c.TaxAddress, c.BillingAddress, c.BillingPostalCode, c.BillingEmail,
		c.PaymentTermDays, c.IsActive, c.CreatedBy,
	))
	if err != nil {
		return nil, duplicateOf(err, map[string]domain.DuplicateError{
			"customers_customer_code_key": {Field: "customer_code", Message: "kode pelanggan sudah dipakai"},
		})
	}
	return &created, nil
}

func (r *CustomerRepo) Update(ctx context.Context, c *domain.Customer) error {
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE customers
		SET company_name = $2, pic_name = $3, phone = $4, email = $5,
		    is_pkp = $6, npwp = $7, tax_address = $8,
		    billing_address = $9, billing_postal_code = $10, billing_email = $11,
		    payment_term_days = $12, is_active = $13
		WHERE id = $1`,
		c.ID, c.CompanyName, c.PICName, c.Phone, c.Email,
		c.IsPKP, c.NPWP, c.TaxAddress,
		c.BillingAddress, c.BillingPostalCode, c.BillingEmail,
		c.PaymentTermDays, c.IsActive,
	))
}

func (r *CustomerRepo) HasActiveAssets(ctx context.Context, customerID int64) (bool, error) {
	var exists bool
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM product_serials WHERE current_customer_id = $1 AND status = 'RENTED')`,
		customerID,
	).Scan(&exists)
	return exists, err
}

// ---------------------------------------------------------------- sites

func (r *CustomerRepo) ListSites(ctx context.Context, customerID int64, f domain.CustomerSiteFilter) ([]domain.CustomerSite, error) {
	args := []any{customerID}
	q := `SELECT ` + siteColumns + ` FROM customer_sites s WHERE s.customer_id = $1`
	if f.IsActive != nil {
		args = append(args, *f.IsActive)
		q += fmt.Sprintf(" AND s.is_active = $%d", len(args))
	}
	q += ` ORDER BY s.is_primary DESC, s.is_active DESC, s.site_name ASC, s.id ASC`

	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.CustomerSite, 0)
	for rows.Next() {
		s, err := scanCustomerSite(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, s)
	}
	return items, rows.Err()
}

func (r *CustomerRepo) findSite(ctx context.Context, siteID int64, suffix string) (*domain.CustomerSite, error) {
	s, err := scanCustomerSite(database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT `+siteColumns+` FROM customer_sites s WHERE s.id = $1`+suffix, siteID))
	if err != nil {
		return nil, notFound(err)
	}
	return &s, nil
}

func (r *CustomerRepo) FindSiteByID(ctx context.Context, siteID int64) (*domain.CustomerSite, error) {
	return r.findSite(ctx, siteID, "")
}

func (r *CustomerRepo) FindSiteByIDForUpdate(ctx context.Context, siteID int64) (*domain.CustomerSite, error) {
	database.MustInTx(ctx)
	return r.findSite(ctx, siteID, " FOR UPDATE")
}

func (r *CustomerRepo) CreateSite(ctx context.Context, s *domain.CustomerSite) (*domain.CustomerSite, error) {
	created, err := scanCustomerSite(database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO customer_sites AS s (
			customer_id, site_name, pic_name, pic_phone, address_line,
			subdistrict, district, city, province, postal_code,
			latitude, longitude, is_primary, is_active
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING `+siteColumns,
		s.CustomerID, s.SiteName, s.PICName, s.PICPhone, s.AddressLine,
		s.Subdistrict, s.District, s.City, s.Province, s.PostalCode,
		s.Latitude, s.Longitude, s.IsPrimary, s.IsActive,
	))
	if err != nil {
		return nil, duplicateOf(err, map[string]domain.DuplicateError{
			"uq_customer_primary_site": {Field: "is_primary", Message: "pelanggan sudah memiliki site utama aktif"},
		})
	}
	return &created, nil
}

func (r *CustomerRepo) UpdateSite(ctx context.Context, s *domain.CustomerSite) error {
	tag, err := database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE customer_sites
		SET site_name = $2, pic_name = $3, pic_phone = $4, address_line = $5,
		    subdistrict = $6, district = $7, city = $8, province = $9, postal_code = $10,
		    latitude = $11, longitude = $12, is_primary = $13, is_active = $14
		WHERE id = $1`,
		s.ID, s.SiteName, s.PICName, s.PICPhone, s.AddressLine,
		s.Subdistrict, s.District, s.City, s.Province, s.PostalCode,
		s.Latitude, s.Longitude, s.IsPrimary, s.IsActive,
	)
	if err != nil {
		return duplicateOf(err, map[string]domain.DuplicateError{
			"uq_customer_primary_site": {Field: "is_primary", Message: "pelanggan sudah memiliki site utama aktif"},
		})
	}
	return mustAffectOne(tag, nil)
}

func (r *CustomerRepo) ClearPrimarySite(ctx context.Context, customerID int64, exceptSiteID int64) error {
	database.MustInTx(ctx)
	_, err := database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE customer_sites
		SET is_primary = FALSE
		WHERE customer_id = $1 AND is_primary = TRUE AND id <> $2`,
		customerID, exceptSiteID,
	)
	return err
}

func (r *CustomerRepo) SiteHasActiveAssets(ctx context.Context, siteID int64) (bool, error) {
	var exists bool
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM product_serials WHERE current_site_id = $1 AND status = 'RENTED')`,
		siteID,
	).Scan(&exists)
	return exists, err
}

// ---------------------------------------------------------------- assets

func (r *CustomerRepo) ListAssets(ctx context.Context, customerID int64, siteID *int64) ([]domain.CustomerAsset, error) {
	args := []any{customerID}
	q := `SELECT ps.id, ps.serial_number, ps.asset_tag, ps.product_id, p.sku, p.name, ps.condition,
	             ps.current_site_id, cs.site_name, cs.city,
	             ps.current_contract_id, rc.contract_number, ps.updated_at
	      FROM product_serials ps
	      JOIN products p ON p.id = ps.product_id
	      LEFT JOIN customer_sites cs ON cs.id = ps.current_site_id
	      LEFT JOIN rental_contracts rc ON rc.id = ps.current_contract_id
	      WHERE ps.current_customer_id = $1 AND ps.status = 'RENTED'`
	if siteID != nil {
		args = append(args, *siteID)
		q += fmt.Sprintf(" AND ps.current_site_id = $%d", len(args))
	}
	q += ` ORDER BY cs.site_name ASC NULLS LAST, p.name ASC, ps.serial_number ASC`

	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.CustomerAsset, 0)
	for rows.Next() {
		var a domain.CustomerAsset
		if err := rows.Scan(
			&a.SerialID, &a.SerialNumber, &a.AssetTag, &a.ProductID, &a.ProductSKU, &a.ProductName, &a.Condition,
			&a.SiteID, &a.SiteName, &a.SiteCity,
			&a.ContractID, &a.ContractNumber, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, rows.Err()
}
