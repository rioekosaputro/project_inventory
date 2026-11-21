package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/database"
)

type ProductRepo struct{ pool *pgxpool.Pool }

func NewProductRepo(pool *pgxpool.Pool) *ProductRepo { return &ProductRepo{pool: pool} }

var _ domain.ProductRepository = (*ProductRepo)(nil)

const productFrom = `products p JOIN categories c ON c.id = p.category_id`

// Kolom uang di-cast ::text agar nilai NUMERIC(15,2) sampai ke Go tanpa float.
const productColumns = `p.id, p.sku, p.name, p.category_id, c.name, c.item_type, p.brand, p.model, p.unit,
	p.has_serial_number, p.min_stock_alert, p.default_monthly_rate::text, p.default_sell_price::text,
	p.description, p.is_active, p.created_at, p.updated_at`

func scanProduct(row pgx.Row) (domain.Product, error) {
	var (
		p          domain.Product
		itemType   string
		rate, sell *string
	)
	err := row.Scan(&p.ID, &p.SKU, &p.Name, &p.CategoryID, &p.CategoryName, &itemType, &p.Brand, &p.Model, &p.Unit,
		&p.HasSerialNumber, &p.MinStockAlert, &rate, &sell, &p.Description, &p.IsActive, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return p, err
	}
	p.ItemType = domain.ItemType(itemType)
	if p.DefaultMonthlyRate, err = moneyFrom(rate); err != nil {
		return p, err
	}
	p.DefaultSellPrice, err = moneyFrom(sell)
	return p, err
}

func (r *ProductRepo) find(ctx context.Context, id int64, suffix string) (*domain.Product, error) {
	p, err := scanProduct(database.GetExecutor(ctx, r.pool).QueryRow(ctx,
		`SELECT `+productColumns+` FROM `+productFrom+` WHERE p.id = $1`+suffix, id))
	if err != nil {
		return nil, notFound(err)
	}
	return &p, nil
}

func (r *ProductRepo) FindByID(ctx context.Context, id int64) (*domain.Product, error) {
	return r.find(ctx, id, "")
}

func (r *ProductRepo) FindByIDForUpdate(ctx context.Context, id int64) (*domain.Product, error) {
	database.MustInTx(ctx)
	return r.find(ctx, id, " FOR UPDATE OF p")
}

func (r *ProductRepo) List(ctx context.Context, f domain.ProductFilter, p domain.PageParams) ([]domain.Product, int64, error) {
	var b filterBuilder
	b.like(f.Query, "p.sku", "p.name", "p.brand", "p.model")
	if f.CategoryID != nil {
		b.add("p.category_id = $%d", *f.CategoryID)
	}
	if f.ItemType != nil {
		b.add("c.item_type = $%d", string(*f.ItemType))
	}
	if f.HasSerialNumber != nil {
		b.add("p.has_serial_number = $%d", *f.HasSerialNumber)
	}
	if f.IsActive != nil {
		b.add("p.is_active = $%d", *f.IsActive)
	}
	return queryPage(ctx, database.GetExecutor(ctx, r.pool), productFrom, productColumns, &b, p, "p.id",
		func(row pgx.Row) (domain.Product, error) { return scanProduct(row) })
}

var productDuplicates = map[string]domain.DuplicateError{
	"products_sku_key": {Field: "sku", Message: "SKU sudah dipakai"},
}

func (r *ProductRepo) Create(ctx context.Context, p *domain.Product) (*domain.Product, error) {
	var id int64
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO products (sku, name, category_id, brand, model, unit, has_serial_number, min_stock_alert,
		                      default_monthly_rate, default_sell_price, description, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::text::numeric, $10::text::numeric, $11, $12)
		RETURNING id`,
		p.SKU, p.Name, p.CategoryID, p.Brand, p.Model, p.Unit, p.HasSerialNumber, p.MinStockAlert,
		moneyArg(p.DefaultMonthlyRate), moneyArg(p.DefaultSellPrice), p.Description, p.IsActive).Scan(&id)
	if err != nil {
		return nil, duplicateOf(err, productDuplicates)
	}
	return r.FindByID(ctx, id)
}

func (r *ProductRepo) Update(ctx context.Context, p *domain.Product) error {
	err := mustAffectOne(database.GetExecutor(ctx, r.pool).Exec(ctx, `
		UPDATE products SET sku = $2, name = $3, category_id = $4, brand = $5, model = $6, unit = $7,
		       has_serial_number = $8, min_stock_alert = $9,
		       default_monthly_rate = $10::text::numeric, default_sell_price = $11::text::numeric,
		       description = $12, is_active = $13
		WHERE id = $1`,
		p.ID, p.SKU, p.Name, p.CategoryID, p.Brand, p.Model, p.Unit, p.HasSerialNumber, p.MinStockAlert,
		moneyArg(p.DefaultMonthlyRate), moneyArg(p.DefaultSellPrice), p.Description, p.IsActive))
	return duplicateOf(err, productDuplicates)
}

func (r *ProductRepo) HasMovements(ctx context.Context, id int64) (bool, error) {
	var has bool
	err := database.GetExecutor(ctx, r.pool).QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM stock_balances      WHERE product_id = $1)
		    OR EXISTS (SELECT 1 FROM product_serials     WHERE product_id = $1)
		    OR EXISTS (SELECT 1 FROM lpb_items           WHERE product_id = $1)
		    OR EXISTS (SELECT 1 FROM stock_opname_items  WHERE product_id = $1)`, id).Scan(&has)
	return has, err
}
