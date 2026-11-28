package usecase

import (
	"context"
	"net/mail"

	"rental-inventory-api/internal/domain"
)

const (
	maxCustomerCodeLen = 30
	maxCompanyNameLen  = 150
	maxPostalCodeLen   = 10
	maxSiteNameLen     = 100
	maxRegionNameLen   = 80
	maxPaymentTermDays = 365
)

// CustomerUsecase mengelola master pelanggan, cabang/site, dan daftar aset terpasang (T2.2).
// Otorisasi (customer.read / customer.write) ditegakkan di router.
type CustomerUsecase struct {
	tx    domain.TxManager
	repo  domain.CustomerRepository
	audit domain.AuditRepository
}

func NewCustomerUsecase(tx domain.TxManager, repo domain.CustomerRepository, audit domain.AuditRepository) *CustomerUsecase {
	return &CustomerUsecase{tx: tx, repo: repo, audit: audit}
}

type CustomerDetail struct {
	Customer *domain.Customer
	Sites    []domain.CustomerSite
}

type CreateCustomerInput struct {
	CustomerCode      string
	CompanyName       string
	PICName           string
	Phone             string
	Email             *string
	IsPKP             *bool
	NPWP              *string
	TaxAddress        *string
	BillingAddress    string
	BillingPostalCode string
	BillingEmail      *string
	PaymentTermDays   *int
}

// UpdateCustomerInput: perubahan parsial. customer_code tidak dapat diubah setelah dibuat.
type UpdateCustomerInput struct {
	CompanyName       *string
	PICName           *string
	Phone             *string
	Email             Patch[string]
	IsPKP             *bool
	NPWP              Patch[string]
	TaxAddress        Patch[string]
	BillingAddress    *string
	BillingPostalCode *string
	BillingEmail      Patch[string]
	PaymentTermDays   *int
	IsActive          *bool
}

type CreateCustomerSiteInput struct {
	SiteName    string
	PICName     string
	PICPhone    string
	AddressLine string
	Subdistrict *string
	District    *string
	City        string
	Province    string
	PostalCode  string
	Latitude    *float64
	Longitude   *float64
	IsPrimary   bool
}

type UpdateCustomerSiteInput struct {
	SiteName    *string
	PICName     *string
	PICPhone    *string
	AddressLine *string
	Subdistrict Patch[string]
	District    Patch[string]
	City        *string
	Province    *string
	PostalCode  *string
	Latitude    Patch[float64]
	Longitude   Patch[float64]
	IsPrimary   *bool
	IsActive    *bool
}

func (u *CustomerUsecase) List(ctx context.Context, f domain.CustomerFilter, p domain.PageParams) ([]domain.Customer, int64, error) {
	return u.repo.List(ctx, f, p)
}

func (u *CustomerUsecase) Get(ctx context.Context, id int64) (*CustomerDetail, error) {
	c, err := u.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	sites, err := u.repo.ListSites(ctx, id, domain.CustomerSiteFilter{})
	if err != nil {
		return nil, err
	}
	return &CustomerDetail{Customer: c, Sites: sites}, nil
}

func (u *CustomerUsecase) Create(ctx context.Context, actor domain.Principal, in CreateCustomerInput) (*domain.Customer, error) {
	isPKP := true
	if in.IsPKP != nil {
		isPKP = *in.IsPKP
	}
	termDays := 30
	if in.PaymentTermDays != nil {
		termDays = *in.PaymentTermDays
	}
	createdBy := actor.UserID
	c := &domain.Customer{
		CustomerCode:      normalizeCode(in.CustomerCode),
		CompanyName:       trimmed(in.CompanyName),
		PICName:           trimmed(in.PICName),
		Phone:             trimmed(in.Phone),
		Email:             cleanOpt(in.Email),
		IsPKP:             isPKP,
		NPWP:              cleanOpt(in.NPWP),
		TaxAddress:        cleanOpt(in.TaxAddress),
		BillingAddress:    trimmed(in.BillingAddress),
		BillingPostalCode: trimmed(in.BillingPostalCode),
		BillingEmail:      cleanOpt(in.BillingEmail),
		PaymentTermDays:   termDays,
		IsActive:          true,
		CreatedBy:         &createdBy,
	}
	errs := validateCustomer(c)
	errs = append(errs, validateCodeFormat("customer_code", c.CustomerCode, maxCustomerCodeLen, codePattern,
		"hanya huruf besar, angka, dan tanda hubung (mis. CUST-001)")...)
	if len(errs) > 0 {
		return nil, masterInvalid("Data pelanggan", errs)
	}

	var created *domain.Customer
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		var err error
		if created, err = u.repo.Create(ctx, c); err != nil {
			return err
		}
		return auditCreate(ctx, u.audit, domain.EntityCustomer, created.ID, customerView(created))
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (u *CustomerUsecase) Update(ctx context.Context, id int64, in UpdateCustomerInput) (*domain.Customer, error) {
	var result *domain.Customer
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		cur, err := u.repo.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		next := *cur
		if in.CompanyName != nil {
			next.CompanyName = trimmed(*in.CompanyName)
		}
		if in.PICName != nil {
			next.PICName = trimmed(*in.PICName)
		}
		if in.Phone != nil {
			next.Phone = trimmed(*in.Phone)
		}
		next.Email = applyPatch(cur.Email, in.Email)
		if in.IsPKP != nil {
			next.IsPKP = *in.IsPKP
		}
		next.NPWP = applyPatch(cur.NPWP, in.NPWP)
		next.TaxAddress = applyPatch(cur.TaxAddress, in.TaxAddress)
		if in.BillingAddress != nil {
			next.BillingAddress = trimmed(*in.BillingAddress)
		}
		if in.BillingPostalCode != nil {
			next.BillingPostalCode = trimmed(*in.BillingPostalCode)
		}
		next.BillingEmail = applyPatch(cur.BillingEmail, in.BillingEmail)
		if in.PaymentTermDays != nil {
			next.PaymentTermDays = *in.PaymentTermDays
		}
		if in.IsActive != nil {
			next.IsActive = *in.IsActive
		}

		if errs := validateCustomer(&next); len(errs) > 0 {
			return masterInvalid("Data pelanggan", errs)
		}

		if cur.IsActive && !next.IsActive {
			hasAssets, err := u.repo.HasActiveAssets(ctx, id)
			if err != nil {
				return err
			}
			if hasAssets {
				return &domain.ValidationError{
					Message: "Pelanggan tidak dapat dinonaktifkan",
					Errors:  []domain.FieldError{{Field: "is_active", Message: "pelanggan masih memiliki unit sewa aktif; tarik unit terlebih dahulu"}},
				}
			}
		}

		before, after := customerView(cur), customerView(&next)
		if _, changed := diffViews(before, after); len(changed) == 0 {
			result = cur
			return nil
		}
		if err := u.repo.Update(ctx, &next); err != nil {
			return err
		}
		if err := auditUpdate(ctx, u.audit, domain.EntityCustomer, id, before, after); err != nil {
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

// ---------------------------------------------------------------- sites

func (u *CustomerUsecase) ListSites(ctx context.Context, customerID int64, f domain.CustomerSiteFilter) ([]domain.CustomerSite, error) {
	if _, err := u.repo.FindByID(ctx, customerID); err != nil {
		return nil, err
	}
	return u.repo.ListSites(ctx, customerID, f)
}

func (u *CustomerUsecase) CreateSite(ctx context.Context, customerID int64, in CreateCustomerSiteInput) (*domain.CustomerSite, error) {
	s := &domain.CustomerSite{
		CustomerID:  customerID,
		SiteName:    trimmed(in.SiteName),
		PICName:     trimmed(in.PICName),
		PICPhone:    trimmed(in.PICPhone),
		AddressLine: trimmed(in.AddressLine),
		Subdistrict: cleanOpt(in.Subdistrict),
		District:    cleanOpt(in.District),
		City:        trimmed(in.City),
		Province:    trimmed(in.Province),
		PostalCode:  trimmed(in.PostalCode),
		Latitude:    in.Latitude,
		Longitude:   in.Longitude,
		IsPrimary:   in.IsPrimary,
		IsActive:    true,
	}
	if errs := validateCustomerSite(s); len(errs) > 0 {
		return nil, masterInvalid("Data site pelanggan", errs)
	}

	var created *domain.CustomerSite
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		// Kunci baris pelanggan agar pergantian site utama aman dari race condition.
		if _, err := u.repo.FindByIDForUpdate(ctx, customerID); err != nil {
			return err
		}
		if s.IsPrimary {
			if err := u.repo.ClearPrimarySite(ctx, customerID, 0); err != nil {
				return err
			}
		}
		var err error
		if created, err = u.repo.CreateSite(ctx, s); err != nil {
			return err
		}
		return auditCreate(ctx, u.audit, domain.EntityCustomerSite, created.ID, customerSiteView(created))
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (u *CustomerUsecase) UpdateSite(ctx context.Context, siteID int64, in UpdateCustomerSiteInput) (*domain.CustomerSite, error) {
	var result *domain.CustomerSite
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		// Baca site dulu untuk mengetahui customer_id, lalu kunci customer → site secara konsisten.
		meta, err := u.repo.FindSiteByID(ctx, siteID)
		if err != nil {
			return err
		}
		if _, err := u.repo.FindByIDForUpdate(ctx, meta.CustomerID); err != nil {
			return err
		}
		cur, err := u.repo.FindSiteByIDForUpdate(ctx, siteID)
		if err != nil {
			return err
		}

		next := *cur
		if in.SiteName != nil {
			next.SiteName = trimmed(*in.SiteName)
		}
		if in.PICName != nil {
			next.PICName = trimmed(*in.PICName)
		}
		if in.PICPhone != nil {
			next.PICPhone = trimmed(*in.PICPhone)
		}
		if in.AddressLine != nil {
			next.AddressLine = trimmed(*in.AddressLine)
		}
		next.Subdistrict = applyPatch(cur.Subdistrict, in.Subdistrict)
		next.District = applyPatch(cur.District, in.District)
		if in.City != nil {
			next.City = trimmed(*in.City)
		}
		if in.Province != nil {
			next.Province = trimmed(*in.Province)
		}
		if in.PostalCode != nil {
			next.PostalCode = trimmed(*in.PostalCode)
		}
		if in.Latitude.Set {
			next.Latitude = in.Latitude.Value
		}
		if in.Longitude.Set {
			next.Longitude = in.Longitude.Value
		}
		if in.IsPrimary != nil {
			next.IsPrimary = *in.IsPrimary
		}
		if in.IsActive != nil {
			next.IsActive = *in.IsActive
		}
		// Site nonaktif tidak boleh tetap menjadi site utama.
		if !next.IsActive {
			next.IsPrimary = false
		}

		if errs := validateCustomerSite(&next); len(errs) > 0 {
			return masterInvalid("Data site pelanggan", errs)
		}

		if cur.IsActive && !next.IsActive {
			hasAssets, err := u.repo.SiteHasActiveAssets(ctx, siteID)
			if err != nil {
				return err
			}
			if hasAssets {
				return &domain.ValidationError{
					Message: "Site pelanggan tidak dapat dinonaktifkan",
					Errors:  []domain.FieldError{{Field: "is_active", Message: "site masih memiliki unit sewa aktif; tarik unit terlebih dahulu"}},
				}
			}
		}

		before, after := customerSiteView(cur), customerSiteView(&next)
		if _, changed := diffViews(before, after); len(changed) == 0 {
			result = cur
			return nil
		}

		if next.IsActive && next.IsPrimary && (!cur.IsActive || !cur.IsPrimary) {
			if err := u.repo.ClearPrimarySite(ctx, cur.CustomerID, cur.ID); err != nil {
				return err
			}
		}
		if err := u.repo.UpdateSite(ctx, &next); err != nil {
			return err
		}
		if err := auditUpdate(ctx, u.audit, domain.EntityCustomerSite, siteID, before, after); err != nil {
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

// ---------------------------------------------------------------- assets

func (u *CustomerUsecase) ListAssets(ctx context.Context, customerID int64, siteID *int64) ([]domain.CustomerAsset, error) {
	if _, err := u.repo.FindByID(ctx, customerID); err != nil {
		return nil, err
	}
	return u.repo.ListAssets(ctx, customerID, siteID)
}

// ---------------------------------------------------------------- validation & audit views

func validateCustomer(c *domain.Customer) []domain.FieldError {
	var errs []domain.FieldError
	errs = append(errs, requiredText("company_name", c.CompanyName, maxCompanyNameLen)...)
	errs = append(errs, requiredText("pic_name", c.PICName, maxPICNameLen)...)
	errs = append(errs, requiredText("phone", c.Phone, maxPhoneLen)...)
	errs = append(errs, optionalEmail("email", c.Email)...)
	errs = append(errs, optionalText("npwp", c.NPWP, maxNPWPLen)...)
	errs = append(errs, optionalText("tax_address", c.TaxAddress, maxAddressLen)...)
	errs = append(errs, requiredText("billing_address", c.BillingAddress, maxAddressLen)...)
	errs = append(errs, requiredText("billing_postal_code", c.BillingPostalCode, maxPostalCodeLen)...)
	errs = append(errs, optionalEmail("billing_email", c.BillingEmail)...)
	if c.PaymentTermDays < 0 || c.PaymentTermDays > maxPaymentTermDays {
		errs = append(errs, domain.FieldError{Field: "payment_term_days", Message: "termin pembayaran harus di antara 0 dan 365 hari"})
	}
	return errs
}

func validateCustomerSite(s *domain.CustomerSite) []domain.FieldError {
	var errs []domain.FieldError
	errs = append(errs, requiredText("site_name", s.SiteName, maxSiteNameLen)...)
	errs = append(errs, requiredText("pic_name", s.PICName, maxPICNameLen)...)
	errs = append(errs, requiredText("pic_phone", s.PICPhone, maxPhoneLen)...)
	errs = append(errs, requiredText("address_line", s.AddressLine, maxAddressLen)...)
	errs = append(errs, optionalText("subdistrict", s.Subdistrict, maxRegionNameLen)...)
	errs = append(errs, optionalText("district", s.District, maxRegionNameLen)...)
	errs = append(errs, requiredText("city", s.City, maxRegionNameLen)...)
	errs = append(errs, requiredText("province", s.Province, maxRegionNameLen)...)
	errs = append(errs, requiredText("postal_code", s.PostalCode, maxPostalCodeLen)...)
	if (s.Latitude == nil) != (s.Longitude == nil) {
		errs = append(errs, domain.FieldError{Field: "latitude", Message: "latitude dan longitude harus diisi bersamaan"})
	}
	if s.Latitude != nil && (*s.Latitude < -90 || *s.Latitude > 90) {
		errs = append(errs, domain.FieldError{Field: "latitude", Message: "harus di antara -90 dan 90"})
	}
	if s.Longitude != nil && (*s.Longitude < -180 || *s.Longitude > 180) {
		errs = append(errs, domain.FieldError{Field: "longitude", Message: "harus di antara -180 dan 180"})
	}
	return errs
}

func optionalEmail(field string, p *string) []domain.FieldError {
	if p == nil {
		return nil
	}
	if errs := optionalText(field, p, maxEmailLen); len(errs) > 0 {
		return errs
	}
	addr, err := mail.ParseAddress(*p)
	if err != nil || addr.Address != *p {
		return []domain.FieldError{{Field: field, Message: "format email tidak valid"}}
	}
	return nil
}

func customerView(c *domain.Customer) map[string]any {
	return map[string]any{
		"customer_code":       c.CustomerCode,
		"company_name":        c.CompanyName,
		"pic_name":            c.PICName,
		"phone":               c.Phone,
		"email":               ptrVal(c.Email),
		"is_pkp":              c.IsPKP,
		"npwp":                ptrVal(c.NPWP),
		"tax_address":         ptrVal(c.TaxAddress),
		"billing_address":     c.BillingAddress,
		"billing_postal_code": c.BillingPostalCode,
		"billing_email":       ptrVal(c.BillingEmail),
		"payment_term_days":   c.PaymentTermDays,
		"is_active":           c.IsActive,
	}
}

func customerSiteView(s *domain.CustomerSite) map[string]any {
	return map[string]any{
		"customer_id":  s.CustomerID,
		"site_name":    s.SiteName,
		"pic_name":     s.PICName,
		"pic_phone":    s.PICPhone,
		"address_line": s.AddressLine,
		"subdistrict":  ptrVal(s.Subdistrict),
		"district":     ptrVal(s.District),
		"city":         s.City,
		"province":     s.Province,
		"postal_code":  s.PostalCode,
		"latitude":     ptrVal(s.Latitude),
		"longitude":    ptrVal(s.Longitude),
		"is_primary":   s.IsPrimary,
		"is_active":    s.IsActive,
	}
}
