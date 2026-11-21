package usecase

import (
	"context"
	"errors"
	"strings"

	"rental-inventory-api/internal/domain"
)

const (
	maxRackCodeLen = 30
	maxRackZoneLen = 30
)

// Zona yang rak-nya secara bawaan tidak dihitung "available" untuk DO (plan Bagian 4: racks.is_sellable).
var nonSellableZones = map[string]bool{"KARANTINA": true, "WORKSHOP": true}

// RackUsecase: master rak. Baca: master.read; tulis: rack.write (STAFF_WAREHOUSE & HEAD_WAREHOUSE).
// Setiap akses dibatasi ke gudang pemanggil (warehouse scoping).
type RackUsecase struct {
	tx         domain.TxManager
	repo       domain.RackRepository
	warehouses domain.WarehouseRepository
	audit      domain.AuditRepository
}

func NewRackUsecase(tx domain.TxManager, repo domain.RackRepository, warehouses domain.WarehouseRepository, audit domain.AuditRepository) *RackUsecase {
	return &RackUsecase{tx: tx, repo: repo, warehouses: warehouses, audit: audit}
}

type CreateRackInput struct {
	WarehouseID int64
	Code        string
	Zone        *string
	IsSellable  *bool // nil: false untuk zona KARANTINA/WORKSHOP, selain itu true
	Description *string
}

// UpdateRackInput: kode dan gudang rak tidak dapat diubah (kode tercetak di label rak).
type UpdateRackInput struct {
	Zone        Patch[string]
	IsSellable  *bool
	Description Patch[string]
	IsActive    *bool
}

func (u *RackUsecase) List(ctx context.Context, actor domain.Principal, f domain.RackFilter, p domain.PageParams) ([]domain.Rack, int64, error) {
	if f.WarehouseID != nil && !actor.CanAccessWarehouse(*f.WarehouseID) {
		return nil, 0, domain.ErrForbiddenWarehouse
	}
	f.Zone = strings.ToUpper(strings.TrimSpace(f.Zone))
	return u.repo.List(ctx, f, scopeOf(actor), p)
}

func (u *RackUsecase) Get(ctx context.Context, actor domain.Principal, id int64) (*domain.Rack, error) {
	r, err := u.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !actor.CanAccessWarehouse(r.WarehouseID) {
		return nil, domain.ErrForbiddenWarehouse
	}
	return r, nil
}

func (u *RackUsecase) Create(ctx context.Context, actor domain.Principal, in CreateRackInput) (*domain.Rack, error) {
	if !actor.CanAccessWarehouse(in.WarehouseID) {
		return nil, domain.ErrForbiddenWarehouse
	}
	r := &domain.Rack{
		WarehouseID: in.WarehouseID, Code: normalizeCode(in.Code),
		Zone: normalizeZone(in.Zone), Description: cleanOpt(in.Description), IsActive: true,
	}
	r.IsSellable = r.Zone == nil || !nonSellableZones[*r.Zone]
	if in.IsSellable != nil {
		r.IsSellable = *in.IsSellable
	}
	errs := validateRack(r)
	errs = append(errs, validateCodeFormat("code", r.Code, maxRackCodeLen, rackCodePattern,
		"hanya huruf besar, angka, titik, garis bawah, dan tanda hubung (mis. A-02-01)")...)
	if len(errs) > 0 {
		return nil, masterInvalid("Data rak", errs)
	}

	var created *domain.Rack
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		active, err := u.warehouses.IsActive(ctx, in.WarehouseID)
		switch {
		case errors.Is(err, domain.ErrNotFound):
			return masterInvalid("Data rak", []domain.FieldError{{Field: "warehouse_id", Message: "gudang tidak ditemukan"}})
		case err != nil:
			return err
		case !active:
			return masterInvalid("Data rak", []domain.FieldError{{Field: "warehouse_id", Message: "gudang tidak aktif"}})
		}
		if created, err = u.repo.Create(ctx, r); err != nil {
			return err
		}
		return auditCreate(ctx, u.audit, domain.EntityRack, created.ID, rackView(created))
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// Update: rak yang sedang dikunci stock opname tidak dapat diubah (ErrRackLocked); rak yang masih
// menyimpan stok tidak dapat dinonaktifkan (ErrLocationHasStock).
func (u *RackUsecase) Update(ctx context.Context, actor domain.Principal, id int64, in UpdateRackInput) (*domain.Rack, error) {
	var result *domain.Rack
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		cur, err := u.repo.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if !actor.CanAccessWarehouse(cur.WarehouseID) {
			return domain.ErrForbiddenWarehouse
		}
		if cur.LockedByOpnameID != nil {
			return domain.ErrRackLocked
		}

		next := *cur
		if in.Zone.Set {
			next.Zone = normalizeZone(in.Zone.Value)
		}
		if in.IsSellable != nil {
			next.IsSellable = *in.IsSellable
		}
		next.Description = applyPatch(cur.Description, in.Description)
		if in.IsActive != nil {
			next.IsActive = *in.IsActive
		}
		if errs := validateRack(&next); len(errs) > 0 {
			return masterInvalid("Data rak", errs)
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
		if !cur.IsActive && next.IsActive { // rak tidak boleh hidup kembali di gudang yang nonaktif
			active, err := u.warehouses.IsActive(ctx, cur.WarehouseID)
			if err != nil {
				return err
			}
			if !active {
				return masterInvalid("Data rak", []domain.FieldError{{Field: "is_active", Message: "gudang rak ini tidak aktif"}})
			}
		}

		before, after := rackView(cur), rackView(&next)
		if _, changed := diffViews(before, after); len(changed) == 0 {
			result = cur
			return nil
		}
		if err := u.repo.Update(ctx, &next); err != nil {
			return err
		}
		if err := auditUpdate(ctx, u.audit, domain.EntityRack, id, before, after); err != nil {
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

// normalizeZone: huruf besar & trim; kosong → nil.
func normalizeZone(p *string) *string {
	v := cleanOpt(p)
	if v == nil {
		return nil
	}
	up := strings.ToUpper(*v)
	return &up
}

func validateRack(r *domain.Rack) []domain.FieldError {
	var errs []domain.FieldError
	errs = append(errs, optionalText("zone", r.Zone, maxRackZoneLen)...)
	errs = append(errs, optionalText("description", r.Description, maxDescriptionLen)...)
	return errs
}

func rackView(r *domain.Rack) map[string]any {
	return map[string]any{
		"warehouse_id": r.WarehouseID, "code": r.Code, "zone": ptrVal(r.Zone), "is_sellable": r.IsSellable,
		"description": ptrVal(r.Description), "is_active": r.IsActive,
	}
}
