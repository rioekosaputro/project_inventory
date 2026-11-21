package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type CategoryRepo struct{ pool *pgxpool.Pool }

func NewCategoryRepo(pool *pgxpool.Pool) *CategoryRepo { return &CategoryRepo{pool: pool} }

var _ domain.CategoryRepository = (*CategoryRepo)(nil)

const categoryColumns = `c.id, c.name, c.item_type, c.is_active`

func scanCategory(row pgx.Row) (domain.Category, error) {
	var c domain.Category
	var itemType string
	err := row.Scan(&c.ID, &c.Name, &itemType, &c.IsActive)
	c.ItemType = domain.ItemType(itemType)
	return c, err
}

func (r *CategoryRepo) find(ctx context.Context, id int64, suffix string) (*domain.Category, error) {
	c, err := scanCategory(database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT `+categoryColumns+` FROM categories c WHERE c.id = $1`+suffix, id))
	if err != nil {
		return nil, notFound(err)
	}
	return &c, nil
}

func (r *CategoryRepo) FindByID(ctx context.Context, id int64) (*domain.Category, error) {
	return r.find(ctx, id, "")
}

func (r *CategoryRepo) FindByIDForUpdate(ctx context.Context, id int64) (*domain.Category, error) {
	database.MustInTx(ctx)
	return r.find(ctx, id, " FOR UPDATE")
}

func (r *CategoryRepo) List(ctx context.Context, f domain.CategoryFilter, p domain.PageParams) ([]domain.Category, int64, error) {
	var b filterBuilder
	b.like(f.Query, "c.name")
	if f.ItemType != nil {
		b.add("c.item_type = $%d", string(*f.ItemType))
	}
	if f.IsActive != nil {
		b.add("c.is_active = $%d", *f.IsActive)
	}
	return queryPage(ctx, database.GetExecutor(ctx, r.pool), "categories c", categoryColumns, &b, p, "c.id",
		func(row pgx.Row) (domain.Category, error) { return scanCategory(row) })
}

func (r *CategoryRepo) Create(ctx context.Context, c *domain.Category) (*domain.Category, error) {
	created, err := scanCategory(database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO categories AS c (name, item_type, is_active) VALUES ($1, $2, $3)
		RETURNING `+categoryColumns, c.Name, string(c.ItemType), c.IsActive))
	if err != nil {
		return nil, duplicateOf(err, map[string]domain.DuplicateError{
			"categories_name_key": {Field: "name", Message: "nama kategori sudah dipakai"},
		})
	}
	return &created, nil
}

func (r *CategoryRepo) Update(ctx context.Context, c *domain.Category) error {
	err := mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE categories SET name = $2, item_type = $3, is_active = $4 WHERE id = $1`,
		c.ID, c.Name, string(c.ItemType), c.IsActive))
	return duplicateOf(err, map[string]domain.DuplicateError{
		"categories_name_key": {Field: "name", Message: "nama kategori sudah dipakai"},
	})
}

func (r *CategoryRepo) HasProducts(ctx context.Context, id int64) (bool, error) {
	var has bool
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM products WHERE category_id = $1)`, id).Scan(&has)
	return has, err
}
