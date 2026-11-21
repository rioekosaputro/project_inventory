package usecase

import (
	"context"
	"net/mail"
	"strings"

	"rental-inventory-api/internal/domain"
)

const (
	maxSupplierCodeLen = 30
	maxSupplierNameLen = 150
	maxPICNameLen      = 100
	maxNPWPLen         = 30
)

// SupplierUsecase: master supplier (global). Baca: master.read; tulis: master.write.
type SupplierUsecase struct {
	tx    domain.TxManager
	repo  domain.SupplierRepository
	audit domain.AuditRepository
}

func NewSupplierUsecase(tx domain.TxManager, repo domain.SupplierRepository, audit domain.AuditRepository) *SupplierUsecase {
	return &SupplierUsecase{tx: tx, repo: repo, audit: audit}
}

type CreateSupplierInput struct {
	Code, Name                  string
	PICName, Phone, Email, NPWP *string
	Address                     *string
}

// UpdateSupplierInput: kode supplier tidak dapat diubah.
type UpdateSupplierInput struct {
	Name     *string
	PICName  Patch[string]
	Phone    Patch[string]
	Email    Patch[string]
	NPWP     Patch[string]
	Address  Patch[string]
	IsActive *bool
}

func (u *SupplierUsecase) List(ctx context.Context, f domain.SupplierFilter, p domain.PageParams) ([]domain.Supplier, int64, error) {
	return u.repo.List(ctx, f, p)
}

func (u *SupplierUsecase) Get(ctx context.Context, id int64) (*domain.Supplier, error) {
	return u.repo.FindByID(ctx, id)
}

func (u *SupplierUsecase) Create(ctx context.Context, in CreateSupplierInput) (*domain.Supplier, error) {
	s := &domain.Supplier{
		Code: normalizeCode(in.Code), Name: trimmed(in.Name),
		PICName: cleanOpt(in.PICName), Phone: cleanOpt(in.Phone), Email: normalizeEmailPtr(in.Email),
		NPWP: cleanOpt(in.NPWP), Address: cleanOpt(in.Address), IsActive: true,
	}
	errs := validateSupplier(s)
	errs = append(errs, validateCodeFormat("code", s.Code, maxSupplierCodeLen, codePattern,
		"hanya huruf besar, angka, dan tanda hubung (mis. SUP-001)")...)
	if len(errs) > 0 {
		return nil, masterInvalid("Data supplier", errs)
	}

	var created *domain.Supplier
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		var err error
		if created, err = u.repo.Create(ctx, s); err != nil {
			return err
		}
		return auditCreate(ctx, u.audit, domain.EntitySupplier, created.ID, supplierView(created))
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (u *SupplierUsecase) Update(ctx context.Context, id int64, in UpdateSupplierInput) (*domain.Supplier, error) {
	var result *domain.Supplier
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		cur, err := u.repo.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		next := *cur
		if in.Name != nil {
			next.Name = trimmed(*in.Name)
		}
		next.PICName = applyPatch(cur.PICName, in.PICName)
		next.Phone = applyPatch(cur.Phone, in.Phone)
		if in.Email.Set {
			next.Email = normalizeEmailPtr(in.Email.Value)
		}
		next.NPWP = applyPatch(cur.NPWP, in.NPWP)
		next.Address = applyPatch(cur.Address, in.Address)
		if in.IsActive != nil {
			next.IsActive = *in.IsActive
		}
		if errs := validateSupplier(&next); len(errs) > 0 {
			return masterInvalid("Data supplier", errs)
		}

		before, after := supplierView(cur), supplierView(&next)
		if _, changed := diffViews(before, after); len(changed) == 0 {
			result = cur
			return nil
		}
		if err := u.repo.Update(ctx, &next); err != nil {
			return err
		}
		if err := auditUpdate(ctx, u.audit, domain.EntitySupplier, id, before, after); err != nil {
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

// normalizeEmailPtr: trim + huruf kecil; kosong → nil.
func normalizeEmailPtr(p *string) *string {
	v := cleanOpt(p)
	if v == nil {
		return nil
	}
	lower := strings.ToLower(*v)
	return &lower
}

func validateSupplier(s *domain.Supplier) []domain.FieldError {
	var errs []domain.FieldError
	errs = append(errs, requiredText("name", s.Name, maxSupplierNameLen)...)
	errs = append(errs, optionalText("pic_name", s.PICName, maxPICNameLen)...)
	errs = append(errs, optionalText("phone", s.Phone, maxPhoneLen)...)
	errs = append(errs, optionalText("npwp", s.NPWP, maxNPWPLen)...)
	errs = append(errs, optionalText("address", s.Address, maxAddressLen)...)
	if s.Email != nil {
		if addr, err := mail.ParseAddress(*s.Email); err != nil || addr.Address != *s.Email || len(*s.Email) > maxEmailLen {
			errs = append(errs, domain.FieldError{Field: "email", Message: "format email tidak valid (maksimal 120 karakter)"})
		}
	}
	return errs
}

func supplierView(s *domain.Supplier) map[string]any {
	return map[string]any{
		"code": s.Code, "name": s.Name, "pic_name": ptrVal(s.PICName), "phone": ptrVal(s.Phone),
		"email": ptrVal(s.Email), "npwp": ptrVal(s.NPWP), "address": ptrVal(s.Address), "is_active": s.IsActive,
	}
}
