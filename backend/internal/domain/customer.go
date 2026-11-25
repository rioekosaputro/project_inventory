package domain

import (
	"context"
	"time"
)

// Jenis entitas komersial untuk audit_logs.entity_type (T2.2).
const (
	EntityCustomer     = "customer"
	EntityCustomerSite = "customer_site"
)

// Customer adalah pelanggan B2B (perusahaan penyewa/pembeli).
type Customer struct {
	ID                int64
	CustomerCode      string // unik, tidak berubah setelah dibuat
	CompanyName       string
	PICName           string
	Phone             string
	Email             *string
	IsPKP             bool
	NPWP              *string
	TaxAddress        *string
	BillingAddress    string
	BillingPostalCode string
	BillingEmail      *string
	PaymentTermDays   int
	IsActive          bool
	CreatedBy         *int64
	CreatedAt         time.Time
	UpdatedAt         time.Time

	// Ringkasan baca (hanya diisi pada List/Get).
	SiteCount        int
	ActiveAssetCount int
}

// CustomerSite adalah lokasi cabang / penempatan unit milik pelanggan.
// Maksimal satu site aktif per pelanggan yang boleh berstatus is_primary = true
// (dijamin oleh partial unique index uq_customer_primary_site).
type CustomerSite struct {
	ID          int64
	CustomerID  int64
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
	IsActive    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CustomerAsset adalah unit berserial berstatus RENTED yang sedang terpasang di pelanggan.
type CustomerAsset struct {
	SerialID       int64
	SerialNumber   string
	AssetTag       string
	ProductID      int64
	ProductSKU     string
	ProductName    string
	Condition      string
	SiteID         *int64
	SiteName       *string
	SiteCity       *string
	ContractID     *int64
	ContractNumber *string
	UpdatedAt      time.Time
}

type CustomerFilter struct {
	Query    string // cocok sebagian pada customer_code, company_name, pic_name, atau npwp
	IsPKP    *bool
	IsActive *bool
}

type CustomerSiteFilter struct {
	IsActive *bool
}

type CustomerRepository interface {
	FindByID(ctx context.Context, id int64) (*Customer, error)
	// FindByIDForUpdate mengunci baris pelanggan (wajib di dalam transaksi).
	FindByIDForUpdate(ctx context.Context, id int64) (*Customer, error)
	List(ctx context.Context, f CustomerFilter, p PageParams) ([]Customer, int64, error)
	Create(ctx context.Context, c *Customer) (*Customer, error)
	Update(ctx context.Context, c *Customer) error
	HasActiveAssets(ctx context.Context, customerID int64) (bool, error)

	ListSites(ctx context.Context, customerID int64, f CustomerSiteFilter) ([]CustomerSite, error)
	FindSiteByID(ctx context.Context, siteID int64) (*CustomerSite, error)
	FindSiteByIDForUpdate(ctx context.Context, siteID int64) (*CustomerSite, error)
	CreateSite(ctx context.Context, s *CustomerSite) (*CustomerSite, error)
	UpdateSite(ctx context.Context, s *CustomerSite) error
	// ClearPrimarySite menonaktifkan flag is_primary pada semua site lain milik pelanggan (wajib dalam tx).
	ClearPrimarySite(ctx context.Context, customerID int64, exceptSiteID int64) error
	SiteHasActiveAssets(ctx context.Context, siteID int64) (bool, error)

	ListAssets(ctx context.Context, customerID int64, siteID *int64) ([]CustomerAsset, error)
}
