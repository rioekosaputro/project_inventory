package usecase

import (
	"context"
	"errors"
	"strings"

	"rental-inventory-api/internal/domain"
)

const (
	maxSKULen        = 50
	maxProductName   = 150
	maxBrandModelLen = 80
	maxUnitLen       = 20
	defaultUnit      = "unit"
)

// ProductUsecase: master produk (global). Baca: product.read (TECHNICIAN ikut); tulis: master.write.
type ProductUsecase struct {
	tx         domain.TxManager
	repo       domain.ProductRepository
	categories domain.CategoryRepository
	audit      domain.AuditRepository
}

func NewProductUsecase(tx domain.TxManager, repo domain.ProductRepository, categories domain.CategoryRepository, audit domain.AuditRepository) *ProductUsecase {
	return &ProductUsecase{tx: tx, repo: repo, categories: categories, audit: audit}
}

// CreateProductInput: tarif/harga berupa teks desimal ("550000.00"), tanpa float (CLAUDE.md invarian #4).
type CreateProductInput struct {
	SKU                string
	Name               string
	CategoryID         int64
	Brand              *string
	Model              *string
	Unit               *string // kosong → "unit"
	HasSerialNumber    bool
	MinStockAlert      int
	DefaultMonthlyRate *string
	DefaultSellPrice   *string
	Description        *string
}

type UpdateProductInput struct {
	SKU                *string
	Name               *string
	CategoryID         *int64
	Brand              Patch[string]
	Model              Patch[string]
	Unit               *string
	HasSerialNumber    *bool
	MinStockAlert      *int
	DefaultMonthlyRate Patch[string]
	DefaultSellPrice   Patch[string]
	Description        Patch[string]
	IsActive           *bool
}

func (u *ProductUsecase) List(ctx context.Context, f domain.ProductFilter, p domain.PageParams) ([]domain.Product, int64, error) {
	return u.repo.List(ctx, f, p)
}

func (u *ProductUsecase) Get(ctx context.Context, id int64) (*domain.Product, error) {
	return u.repo.FindByID(ctx, id)
}

func (u *ProductUsecase) Create(ctx context.Context, in CreateProductInput) (*domain.Product, error) {
	p := &domain.Product{
		SKU: normalizeCode(in.SKU), Name: trimmed(in.Name), CategoryID: in.CategoryID,
		Brand: cleanOpt(in.Brand), Model: cleanOpt(in.Model), Unit: defaultUnit,
		HasSerialNumber: in.HasSerialNumber, MinStockAlert: in.MinStockAlert,
		Description: cleanOpt(in.Description), IsActive: true,
	}
	if in.Unit != nil && trimmed(*in.Unit) != "" {
		p.Unit = trimmed(*in.Unit)
	}
	var errs []domain.FieldError
	var moneyErrs []domain.FieldError
	p.DefaultMonthlyRate, moneyErrs = parseMoney("default_monthly_rate", in.DefaultMonthlyRate)
	errs = append(errs, moneyErrs...)
	p.DefaultSellPrice, moneyErrs = parseMoney("default_sell_price", in.DefaultSellPrice)
	errs = append(errs, moneyErrs...)
	errs = append(errs, validateProduct(p)...)
	if len(errs) > 0 {
		return nil, masterInvalid("Data produk", errs)
	}

	var created *domain.Product
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		cat, errs, err := u.activeCategory(ctx, p.CategoryID)
		if err != nil {
			return err
		}
		if len(errs) == 0 {
			errs = serialRule(cat.ItemType, p.HasSerialNumber)
		}
		if len(errs) > 0 {
			return masterInvalid("Data produk", errs)
		}
		if created, err = u.repo.Create(ctx, p); err != nil {
			return err
		}
		return auditCreate(ctx, u.audit, domain.EntityProduct, created.ID, productView(created))
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// Update: SKU, mode serial number, dan jenis barang dikunci setelah produk dipakai stok/SN/LPB/opname
// (ErrProductInUse). Memindahkan produk ke kategori lain hanya ke kategori aktif.
func (u *ProductUsecase) Update(ctx context.Context, id int64, in UpdateProductInput) (*domain.Product, error) {
	var result *domain.Product
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		cur, err := u.repo.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}

		next := *cur
		var errs []domain.FieldError
		if in.SKU != nil {
			next.SKU = normalizeCode(*in.SKU)
		}
		if in.Name != nil {
			next.Name = trimmed(*in.Name)
		}
		if in.CategoryID != nil {
			next.CategoryID = *in.CategoryID
		}
		next.Brand = applyPatch(cur.Brand, in.Brand)
		next.Model = applyPatch(cur.Model, in.Model)
		if in.Unit != nil {
			next.Unit = trimmed(*in.Unit)
		}
		if in.HasSerialNumber != nil {
			next.HasSerialNumber = *in.HasSerialNumber
		}
		if in.MinStockAlert != nil {
			next.MinStockAlert = *in.MinStockAlert
		}
		next.Description = applyPatch(cur.Description, in.Description)
		if in.IsActive != nil {
			next.IsActive = *in.IsActive
		}
		var moneyErrs []domain.FieldError
		if in.DefaultMonthlyRate.Set {
			next.DefaultMonthlyRate, moneyErrs = parseMoney("default_monthly_rate", in.DefaultMonthlyRate.Value)
			errs = append(errs, moneyErrs...)
		}
		if in.DefaultSellPrice.Set {
			next.DefaultSellPrice, moneyErrs = parseMoney("default_sell_price", in.DefaultSellPrice.Value)
			errs = append(errs, moneyErrs...)
		}
		errs = append(errs, validateProduct(&next)...)

		if next.CategoryID != cur.CategoryID {
			cat, catErrs, err := u.activeCategory(ctx, next.CategoryID)
			if err != nil {
				return err
			}
			errs = append(errs, catErrs...)
			if cat != nil {
				next.CategoryName, next.ItemType = cat.Name, cat.ItemType
			}
		}
		if len(errs) == 0 {
			errs = serialRule(next.ItemType, next.HasSerialNumber)
		}
		if len(errs) > 0 {
			return masterInvalid("Data produk", errs)
		}

		locked := next.SKU != cur.SKU || next.HasSerialNumber != cur.HasSerialNumber || next.ItemType != cur.ItemType
		if locked {
			used, err := u.repo.HasMovements(ctx, id)
			if err != nil {
				return err
			}
			if used {
				return domain.ErrProductInUse
			}
		}

		before, after := productView(cur), productView(&next)
		if _, changed := diffViews(before, after); len(changed) == 0 {
			result = cur
			return nil
		}
		if err := u.repo.Update(ctx, &next); err != nil {
			return err
		}
		if err := auditUpdate(ctx, u.audit, domain.EntityProduct, id, before, after); err != nil {
			return err
		}
		result = &next
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// activeCategory: (kategori, kesalahan input, kegagalan infrastruktur).
func (u *ProductUsecase) activeCategory(ctx context.Context, id int64) (*domain.Category, []domain.FieldError, error) {
	cat, err := u.categories.FindByID(ctx, id)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return nil, []domain.FieldError{{Field: "category_id", Message: "kategori tidak ditemukan"}}, nil
	case err != nil:
		return nil, nil, err
	case !cat.IsActive:
		return cat, []domain.FieldError{{Field: "category_id", Message: "kategori tidak aktif"}}, nil
	}
	return cat, nil, nil
}

// serialRule: barang habis pakai (CONSUMABLE) dikelola per qty, tidak per unit ber-serial number.
func serialRule(t domain.ItemType, hasSerial bool) []domain.FieldError {
	if t == domain.ItemTypeConsumable && hasSerial {
		return []domain.FieldError{{Field: "has_serial_number", Message: "barang habis pakai (CONSUMABLE) tidak memakai serial number"}}
	}
	return nil
}

func validateProduct(p *domain.Product) []domain.FieldError {
	var errs []domain.FieldError
	errs = append(errs, validateCodeFormat("sku", p.SKU, maxSKULen, skuPattern,
		"hanya huruf besar, angka, titik, garis miring, garis bawah, dan tanda hubung")...)
	errs = append(errs, requiredText("name", p.Name, maxProductName)...)
	if p.CategoryID < 1 {
		errs = append(errs, domain.FieldError{Field: "category_id", Message: "wajib diisi"})
	}
	errs = append(errs, optionalText("brand", p.Brand, maxBrandModelLen)...)
	errs = append(errs, optionalText("model", p.Model, maxBrandModelLen)...)
	errs = append(errs, requiredText("unit", p.Unit, maxUnitLen)...)
	errs = append(errs, optionalText("description", p.Description, maxDescriptionLen)...)
	if p.MinStockAlert < 0 {
		errs = append(errs, domain.FieldError{Field: "min_stock_alert", Message: "tidak boleh negatif"})
	}
	return errs
}

func productView(p *domain.Product) map[string]any {
	return map[string]any{
		"sku": p.SKU, "name": p.Name, "category_id": p.CategoryID, "brand": ptrVal(p.Brand), "model": ptrVal(p.Model),
		"unit": p.Unit, "has_serial_number": p.HasSerialNumber, "min_stock_alert": p.MinStockAlert,
		"default_monthly_rate": decVal(p.DefaultMonthlyRate), "default_sell_price": decVal(p.DefaultSellPrice),
		"description": ptrVal(p.Description), "is_active": p.IsActive,
	}
}

// trimmed menyederhanakan pemanggilan strings.TrimSpace pada input teks.
func trimmed(s string) string { return strings.TrimSpace(s) }
