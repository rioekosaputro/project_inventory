package usecase

import (
	"context"

	"rental-inventory-api/internal/domain"
)

const (
	maxWarehouseCodeLen = 20 // = batas scope docnum & kolom warehouses.code
	maxWarehouseNameLen = 100
)

// WarehouseUsecase: master gudang. Baca: master.read; tulis: master.write (ditegakkan di router).
// Warehouse scoping (plan 5.1): akun yang dibatasi ke satu gudang hanya melihat/mengubah gudangnya
// dan tidak boleh membuat gudang baru.
type WarehouseUsecase struct {
	tx    domain.TxManager
	repo  domain.WarehouseRepository
	audit domain.AuditRepository
}

func NewWarehouseUsecase(tx domain.TxManager, repo domain.WarehouseRepository, audit domain.AuditRepository) *WarehouseUsecase {
	return &WarehouseUsecase{tx: tx, repo: repo, audit: audit}
}

type CreateWarehouseInput struct {
	Code      string
	Name      string
	Address   *string
	Phone     *string
	Latitude  *float64
	Longitude *float64
}

// UpdateWarehouseInput: perubahan parsial. Kode gudang tidak dapat diubah (dipakai di nomor dokumen).
type UpdateWarehouseInput struct {
	Name      *string
	Address   Patch[string]
	Phone     Patch[string]
	Latitude  Patch[float64]
	Longitude Patch[float64]
	IsActive  *bool
}

func (u *WarehouseUsecase) List(ctx context.Context, actor domain.Principal, f domain.WarehouseFilter, p domain.PageParams) ([]domain.Warehouse, int64, error) {
	f.OnlyID = scopeOf(actor)
	return u.repo.List(ctx, f, p)
}

func (u *WarehouseUsecase) Get(ctx context.Context, actor domain.Principal, id int64) (*domain.Warehouse, error) {
	if !actor.CanAccessWarehouse(id) {
		return nil, domain.ErrForbiddenWarehouse
	}
	return u.repo.FindByID(ctx, id)
}

func (u *WarehouseUsecase) Create(ctx context.Context, actor domain.Principal, in CreateWarehouseInput) (*domain.Warehouse, error) {
	if scopeOf(actor) != nil {
		return nil, domain.ErrForbiddenWarehouse // membuat gudang baru bukan hak akun yang dibatasi ke satu gudang
	}
	w := &domain.Warehouse{
		Code: normalizeCode(in.Code), Name: trimmed(in.Name),
		Address: cleanOpt(in.Address), Phone: cleanOpt(in.Phone),
		Latitude: in.Latitude, Longitude: in.Longitude, IsActive: true,
	}
	errs := validateWarehouse(w)
	errs = append(errs, validateCodeFormat("code", w.Code, maxWarehouseCodeLen, codePattern,
		"hanya huruf besar, angka, dan tanda hubung (mis. WH-SMG-01)")...)
	if len(errs) > 0 {
		return nil, masterInvalid("Data gudang", errs)
	}

	var created *domain.Warehouse
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		var err error
		if created, err = u.repo.Create(ctx, w); err != nil {
			return err
		}
		return auditCreate(ctx, u.audit, domain.EntityWarehouse, created.ID, warehouseView(created))
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// Update: menonaktifkan gudang yang masih menyimpan stok ditolak (ErrLocationHasStock).
func (u *WarehouseUsecase) Update(ctx context.Context, actor domain.Principal, id int64, in UpdateWarehouseInput) (*domain.Warehouse, error) {
	if !actor.CanAccessWarehouse(id) {
		return nil, domain.ErrForbiddenWarehouse
	}

	var result *domain.Warehouse
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		cur, err := u.repo.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		next := *cur
		if in.Name != nil {
			next.Name = trimmed(*in.Name)
		}
		next.Address = applyPatch(cur.Address, in.Address)
		next.Phone = applyPatch(cur.Phone, in.Phone)
		if in.Latitude.Set {
			next.Latitude = in.Latitude.Value
		}
		if in.Longitude.Set {
			next.Longitude = in.Longitude.Value
		}
		if in.IsActive != nil {
			next.IsActive = *in.IsActive
		}
		if errs := validateWarehouse(&next); len(errs) > 0 {
			return masterInvalid("Data gudang", errs)
		}

		if cur.IsActive && !next.IsActive {
			has, err := u.repo.HasStock(ctx, id)
			if err != nil {
				return err
			}
			if has {
				return domain.ErrLocationHasStock
			}
		}

		before, after := warehouseView(cur), warehouseView(&next)
		if _, changed := diffViews(before, after); len(changed) == 0 {
			result = cur
			return nil
		}
		if err := u.repo.Update(ctx, &next); err != nil {
			return err
		}
		if err := auditUpdate(ctx, u.audit, domain.EntityWarehouse, id, before, after); err != nil {
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

func validateWarehouse(w *domain.Warehouse) []domain.FieldError {
	var errs []domain.FieldError
	errs = append(errs, requiredText("name", w.Name, maxWarehouseNameLen)...)
	errs = append(errs, optionalText("address", w.Address, maxAddressLen)...)
	errs = append(errs, optionalText("phone", w.Phone, maxPhoneLen)...)
	if (w.Latitude == nil) != (w.Longitude == nil) {
		errs = append(errs, domain.FieldError{Field: "latitude", Message: "latitude dan longitude harus diisi bersamaan"})
	}
	if w.Latitude != nil && (*w.Latitude < -90 || *w.Latitude > 90) {
		errs = append(errs, domain.FieldError{Field: "latitude", Message: "harus di antara -90 dan 90"})
	}
	if w.Longitude != nil && (*w.Longitude < -180 || *w.Longitude > 180) {
		errs = append(errs, domain.FieldError{Field: "longitude", Message: "harus di antara -180 dan 180"})
	}
	return errs
}

func warehouseView(w *domain.Warehouse) map[string]any {
	return map[string]any{
		"code": w.Code, "name": w.Name, "address": ptrVal(w.Address), "phone": ptrVal(w.Phone),
		"latitude": ptrVal(w.Latitude), "longitude": ptrVal(w.Longitude), "is_active": w.IsActive,
	}
}
