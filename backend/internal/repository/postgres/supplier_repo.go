package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type SupplierRepo struct{ pool *pgxpool.Pool }

func NewSupplierRepo(pool *pgxpool.Pool) *SupplierRepo { return &SupplierRepo{pool: pool} }

var _ domain.SupplierRepository = (*SupplierRepo)(nil)

const supplierColumns = `s.id, s.code, s.name, s.pic_name, s.phone, s.email, s.npwp, s.address,
	s.is_active, s.created_at, s.updated_at`

func scanSupplier(row pgx.Row) (domain.Supplier, error) {
	var s domain.Supplier
	err := row.Scan(&s.ID, &s.Code, &s.Name, &s.PICName, &s.Phone, &s.Email, &s.NPWP, &s.Address,
		&s.IsActive, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func (r *SupplierRepo) find(ctx context.Context, id int64, suffix string) (*domain.Supplier, error) {
	s, err := scanSupplier(database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT `+supplierColumns+` FROM suppliers s WHERE s.id = $1`+suffix, id))
	if err != nil {
		return nil, notFound(err)
	}
	return &s, nil
}

func (r *SupplierRepo) FindByID(ctx context.Context, id int64) (*domain.Supplier, error) {
	return r.find(ctx, id, "")
}

func (r *SupplierRepo) FindByIDForUpdate(ctx context.Context, id int64) (*domain.Supplier, error) {
	database.MustInTx(ctx)
	return r.find(ctx, id, " FOR UPDATE")
}

func (r *SupplierRepo) List(ctx context.Context, f domain.SupplierFilter, p domain.PageParams) ([]domain.Supplier, int64, error) {
	var b filterBuilder
	b.like(f.Query, "s.code", "s.name", "s.pic_name")
	if f.IsActive != nil {
		b.add("s.is_active = $%d", *f.IsActive)
	}
	return queryPage(ctx, database.GetExecutor(ctx, r.pool), "suppliers s", supplierColumns, &b, p, "s.id",
		func(row pgx.Row) (domain.Supplier, error) { return scanSupplier(row) })
}

func (r *SupplierRepo) Create(ctx context.Context, s *domain.Supplier) (*domain.Supplier, error) {
	created, err := scanSupplier(database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO suppliers AS s (code, name, pic_name, phone, email, npwp, address, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+supplierColumns,
		s.Code, s.Name, s.PICName, s.Phone, s.Email, s.NPWP, s.Address, s.IsActive))
	if err != nil {
		return nil, duplicateOf(err, map[string]domain.DuplicateError{
			"suppliers_code_key": {Field: "code", Message: "kode supplier sudah dipakai"},
		})
	}
	return &created, nil
}

func (r *SupplierRepo) Update(ctx context.Context, s *domain.Supplier) error {
	return mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE suppliers SET name = $2, pic_name = $3, phone = $4, email = $5, npwp = $6, address = $7, is_active = $8
		WHERE id = $1`, s.ID, s.Name, s.PICName, s.Phone, s.Email, s.NPWP, s.Address, s.IsActive))
}
