package usecase

import (
	"context"

	"rental-inventory-api/internal/domain"
)

const maxCategoryNameLen = 100

// CategoryUsecase: master kategori (global, tidak dibatasi gudang). Baca: master.read; tulis: master.write.
type CategoryUsecase struct {
	tx    domain.TxManager
	repo  domain.CategoryRepository
	audit domain.AuditRepository
}

func NewCategoryUsecase(tx domain.TxManager, repo domain.CategoryRepository, audit domain.AuditRepository) *CategoryUsecase {
	return &CategoryUsecase{tx: tx, repo: repo, audit: audit}
}

type CreateCategoryInput struct {
	Name     string
	ItemType domain.ItemType
}

type UpdateCategoryInput struct {
	Name     *string
	ItemType *domain.ItemType
	IsActive *bool
}

func (u *CategoryUsecase) List(ctx context.Context, f domain.CategoryFilter, p domain.PageParams) ([]domain.Category, int64, error) {
	return u.repo.List(ctx, f, p)
}

func (u *CategoryUsecase) Get(ctx context.Context, id int64) (*domain.Category, error) {
	return u.repo.FindByID(ctx, id)
}

func (u *CategoryUsecase) Create(ctx context.Context, in CreateCategoryInput) (*domain.Category, error) {
	c := &domain.Category{Name: trimmed(in.Name), ItemType: in.ItemType, IsActive: true}
	if errs := validateCategory(c); len(errs) > 0 {
		return nil, masterInvalid("Data kategori", errs)
	}

	var created *domain.Category
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		var err error
		if created, err = u.repo.Create(ctx, c); err != nil {
			return err
		}
		return auditCreate(ctx, u.audit, domain.EntityCategory, created.ID, categoryView(created))
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// Update: jenis barang (ASSET/CONSUMABLE) tidak dapat diubah bila kategori sudah dipakai produk,
// karena menentukan apakah produknya boleh ber-serial number (ErrCategoryInUse).
func (u *CategoryUsecase) Update(ctx context.Context, id int64, in UpdateCategoryInput) (*domain.Category, error) {
	var result *domain.Category
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		cur, err := u.repo.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		next := *cur
		if in.Name != nil {
			next.Name = trimmed(*in.Name)
		}
		if in.ItemType != nil {
			next.ItemType = *in.ItemType
		}
		if in.IsActive != nil {
			next.IsActive = *in.IsActive
		}
		if errs := validateCategory(&next); len(errs) > 0 {
			return masterInvalid("Data kategori", errs)
		}

		if next.ItemType != cur.ItemType {
			used, err := u.repo.HasProducts(ctx, id)
			if err != nil {
				return err
			}
			if used {
				return domain.ErrCategoryInUse
			}
		}

		before, after := categoryView(cur), categoryView(&next)
		if _, changed := diffViews(before, after); len(changed) == 0 {
			result = cur
			return nil
		}
		if err := u.repo.Update(ctx, &next); err != nil {
			return err
		}
		if err := auditUpdate(ctx, u.audit, domain.EntityCategory, id, before, after); err != nil {
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

func validateCategory(c *domain.Category) []domain.FieldError {
	errs := requiredText("name", c.Name, maxCategoryNameLen)
	if !c.ItemType.Valid() {
		errs = append(errs, domain.FieldError{Field: "item_type", Message: "harus ASSET atau CONSUMABLE"})
	}
	return errs
}

func categoryView(c *domain.Category) map[string]any {
	return map[string]any{"name": c.Name, "item_type": string(c.ItemType), "is_active": c.IsActive}
}
