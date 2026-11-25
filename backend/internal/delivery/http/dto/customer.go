package dto

import (
	"time"

	"rental-inventory-api/internal/domain"
)

// ---------------------------------------------------------------- customer

type ListCustomersQuery struct {
	Q        string `form:"q" binding:"omitempty,max=100"`
	IsPKP    *bool  `form:"is_pkp"`
	IsActive *bool  `form:"is_active"`
}

type CreateCustomerRequest struct {
	CustomerCode      string  `json:"customer_code" binding:"required,max=30"`
	CompanyName       string  `json:"company_name" binding:"required,max=150"`
	PICName           string  `json:"pic_name" binding:"required,max=100"`
	Phone             string  `json:"phone" binding:"required,max=30"`
	Email             *string `json:"email" binding:"omitempty,max=120"`
	IsPKP             *bool   `json:"is_pkp"`
	NPWP              *string `json:"npwp" binding:"omitempty,max=30"`
	TaxAddress        *string `json:"tax_address" binding:"omitempty,max=500"`
	BillingAddress    string  `json:"billing_address" binding:"required,max=500"`
	BillingPostalCode string  `json:"billing_postal_code" binding:"required,max=10"`
	BillingEmail      *string `json:"billing_email" binding:"omitempty,max=120"`
	PaymentTermDays   *int    `json:"payment_term_days" binding:"omitempty,gte=0,lte=365"`
}

type UpdateCustomerRequest struct {
	CompanyName       *string          `json:"company_name" binding:"omitempty,min=1,max=150"`
	PICName           *string          `json:"pic_name" binding:"omitempty,min=1,max=100"`
	Phone             *string          `json:"phone" binding:"omitempty,min=1,max=30"`
	Email             Optional[string] `json:"email"`
	IsPKP             *bool            `json:"is_pkp"`
	NPWP              Optional[string] `json:"npwp"`
	TaxAddress        Optional[string] `json:"tax_address"`
	BillingAddress    *string          `json:"billing_address" binding:"omitempty,min=1,max=500"`
	BillingPostalCode *string          `json:"billing_postal_code" binding:"omitempty,min=1,max=10"`
	BillingEmail      Optional[string] `json:"billing_email"`
	PaymentTermDays   *int             `json:"payment_term_days" binding:"omitempty,gte=0,lte=365"`
	IsActive          *bool            `json:"is_active"`
}

type CustomerResponse struct {
	ID                int64     `json:"id"`
	CustomerCode      string    `json:"customer_code"`
	CompanyName       string    `json:"company_name"`
	PICName           string    `json:"pic_name"`
	Phone             string    `json:"phone"`
	Email             *string   `json:"email"`
	IsPKP             bool      `json:"is_pkp"`
	NPWP              *string   `json:"npwp"`
	TaxAddress        *string   `json:"tax_address"`
	BillingAddress    string    `json:"billing_address"`
	BillingPostalCode string    `json:"billing_postal_code"`
	BillingEmail      *string   `json:"billing_email"`
	PaymentTermDays   int       `json:"payment_term_days"`
	IsActive          bool      `json:"is_active"`
	SiteCount         int       `json:"site_count"`
	ActiveAssetCount  int       `json:"active_asset_count"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type CustomerDetailResponse struct {
	CustomerResponse
	Sites []CustomerSiteResponse `json:"sites"`
}

func NewCustomerResponse(c *domain.Customer) CustomerResponse {
	return CustomerResponse{
		ID:                c.ID,
		CustomerCode:      c.CustomerCode,
		CompanyName:       c.CompanyName,
		PICName:           c.PICName,
		Phone:             c.Phone,
		Email:             c.Email,
		IsPKP:             c.IsPKP,
		NPWP:              c.NPWP,
		TaxAddress:        c.TaxAddress,
		BillingAddress:    c.BillingAddress,
		BillingPostalCode: c.BillingPostalCode,
		BillingEmail:      c.BillingEmail,
		PaymentTermDays:   c.PaymentTermDays,
		IsActive:          c.IsActive,
		SiteCount:         c.SiteCount,
		ActiveAssetCount:  c.ActiveAssetCount,
		CreatedAt:         c.CreatedAt,
		UpdatedAt:         c.UpdatedAt,
	}
}

// ---------------------------------------------------------------- customer sites

type ListCustomerSitesQuery struct {
	IsActive *bool `form:"is_active"`
}

type CreateCustomerSiteRequest struct {
	SiteName    string   `json:"site_name" binding:"required,max=100"`
	PICName     string   `json:"pic_name" binding:"required,max=100"`
	PICPhone    string   `json:"pic_phone" binding:"required,max=30"`
	AddressLine string   `json:"address_line" binding:"required,max=500"`
	Subdistrict *string  `json:"subdistrict" binding:"omitempty,max=80"`
	District    *string  `json:"district" binding:"omitempty,max=80"`
	City        string   `json:"city" binding:"required,max=80"`
	Province    string   `json:"province" binding:"required,max=80"`
	PostalCode  string   `json:"postal_code" binding:"required,max=10"`
	Latitude    *float64 `json:"latitude"`
	Longitude   *float64 `json:"longitude"`
	IsPrimary   bool     `json:"is_primary"`
}

type UpdateCustomerSiteRequest struct {
	SiteName    *string           `json:"site_name" binding:"omitempty,min=1,max=100"`
	PICName     *string           `json:"pic_name" binding:"omitempty,min=1,max=100"`
	PICPhone    *string           `json:"pic_phone" binding:"omitempty,min=1,max=30"`
	AddressLine *string           `json:"address_line" binding:"omitempty,min=1,max=500"`
	Subdistrict Optional[string]  `json:"subdistrict"`
	District    Optional[string]  `json:"district"`
	City        *string           `json:"city" binding:"omitempty,min=1,max=80"`
	Province    *string           `json:"province" binding:"omitempty,min=1,max=80"`
	PostalCode  *string           `json:"postal_code" binding:"omitempty,min=1,max=10"`
	Latitude    Optional[float64] `json:"latitude"`
	Longitude   Optional[float64] `json:"longitude"`
	IsPrimary   *bool             `json:"is_primary"`
	IsActive    *bool             `json:"is_active"`
}

type CustomerSiteResponse struct {
	ID          int64     `json:"id"`
	CustomerID  int64     `json:"customer_id"`
	SiteName    string    `json:"site_name"`
	PICName     string    `json:"pic_name"`
	PICPhone    string    `json:"pic_phone"`
	AddressLine string    `json:"address_line"`
	Subdistrict *string   `json:"subdistrict"`
	District    *string   `json:"district"`
	City        string    `json:"city"`
	Province    string    `json:"province"`
	PostalCode  string    `json:"postal_code"`
	Latitude    *float64  `json:"latitude"`
	Longitude   *float64  `json:"longitude"`
	IsPrimary   bool      `json:"is_primary"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func NewCustomerSiteResponse(s *domain.CustomerSite) CustomerSiteResponse {
	return CustomerSiteResponse{
		ID:          s.ID,
		CustomerID:  s.CustomerID,
		SiteName:    s.SiteName,
		PICName:     s.PICName,
		PICPhone:    s.PICPhone,
		AddressLine: s.AddressLine,
		Subdistrict: s.Subdistrict,
		District:    s.District,
		City:        s.City,
		Province:    s.Province,
		PostalCode:  s.PostalCode,
		Latitude:    s.Latitude,
		Longitude:   s.Longitude,
		IsPrimary:   s.IsPrimary,
		IsActive:    s.IsActive,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
}

// ---------------------------------------------------------------- customer assets

type ListCustomerAssetsQuery struct {
	SiteID *int64 `form:"site_id" binding:"omitempty,gte=1"`
}

type CustomerAssetResponse struct {
	SerialID       int64     `json:"serial_id"`
	SerialNumber   string    `json:"serial_number"`
	AssetTag       string    `json:"asset_tag"`
	ProductID      int64     `json:"product_id"`
	ProductSKU     string    `json:"product_sku"`
	ProductName    string    `json:"product_name"`
	Condition      string    `json:"condition"`
	SiteID         *int64    `json:"site_id"`
	SiteName       *string   `json:"site_name"`
	SiteCity       *string   `json:"site_city"`
	ContractID     *int64    `json:"contract_id"`
	ContractNumber *string   `json:"contract_number"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func NewCustomerAssetResponse(a *domain.CustomerAsset) CustomerAssetResponse {
	return CustomerAssetResponse{
		SerialID:       a.SerialID,
		SerialNumber:   a.SerialNumber,
		AssetTag:       a.AssetTag,
		ProductID:      a.ProductID,
		ProductSKU:     a.ProductSKU,
		ProductName:    a.ProductName,
		Condition:      a.Condition,
		SiteID:         a.SiteID,
		SiteName:       a.SiteName,
		SiteCity:       a.SiteCity,
		ContractID:     a.ContractID,
		ContractNumber: a.ContractNumber,
		UpdatedAt:      a.UpdatedAt,
	}
}
