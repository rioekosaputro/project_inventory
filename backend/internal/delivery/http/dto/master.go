package dto

import (
	"time"

	"github.com/shopspring/decimal"

	"rental-inventory-api/internal/domain"
)

// Catatan: binding hanya memeriksa bentuk dasar; aturan bisnis dan batas panjang ditegakkan lagi di usecase.
// Nilai uang dikirim/diterima sebagai string desimal ("550000.00") — tidak pernah float.

// ---------------------------------------------------------------- warehouse

type CreateWarehouseRequest struct {
	Code      string   `json:"code" binding:"required,max=20"`
	Name      string   `json:"name" binding:"required,max=100"`
	Address   *string  `json:"address" binding:"omitempty,max=500"`
	Phone     *string  `json:"phone" binding:"omitempty,max=30"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

// UpdateWarehouseRequest: semua field opsional (PATCH); kode tidak dapat diubah.
type UpdateWarehouseRequest struct {
	Name      *string           `json:"name" binding:"omitempty,min=1,max=100"`
	Address   Optional[string]  `json:"address"`
	Phone     Optional[string]  `json:"phone"`
	Latitude  Optional[float64] `json:"latitude"`
	Longitude Optional[float64] `json:"longitude"`
	IsActive  *bool             `json:"is_active"`
}

type ListWarehousesQuery struct {
	Q        string `form:"q" binding:"omitempty,max=100"`
	IsActive *bool  `form:"is_active"`
}

type WarehouseResponse struct {
	ID        int64     `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Address   *string   `json:"address"`
	Phone     *string   `json:"phone"`
	Latitude  *float64  `json:"latitude"`
	Longitude *float64  `json:"longitude"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewWarehouseResponse(w *domain.Warehouse) WarehouseResponse {
	return WarehouseResponse{
		ID: w.ID, Code: w.Code, Name: w.Name, Address: w.Address, Phone: w.Phone,
		Latitude: w.Latitude, Longitude: w.Longitude, IsActive: w.IsActive, CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt,
	}
}

// ---------------------------------------------------------------- rack

type CreateRackRequest struct {
	WarehouseID int64   `json:"warehouse_id" binding:"required,gte=1"`
	Code        string  `json:"code" binding:"required,max=30"`
	Zone        *string `json:"zone" binding:"omitempty,max=30"`
	IsSellable  *bool   `json:"is_sellable"`
	Description *string `json:"description" binding:"omitempty,max=1000"`
}

// UpdateRackRequest: kode dan gudang rak tidak dapat diubah.
type UpdateRackRequest struct {
	Zone        Optional[string] `json:"zone"`
	IsSellable  *bool            `json:"is_sellable"`
	Description Optional[string] `json:"description"`
	IsActive    *bool            `json:"is_active"`
}

type ListRacksQuery struct {
	Q           string `form:"q" binding:"omitempty,max=100"`
	WarehouseID *int64 `form:"warehouse_id" binding:"omitempty,gte=1"`
	Zone        string `form:"zone" binding:"omitempty,max=30"`
	IsSellable  *bool  `form:"is_sellable"`
	IsActive    *bool  `form:"is_active"`
}

type RackResponse struct {
	ID               int64     `json:"id"`
	WarehouseID      int64     `json:"warehouse_id"`
	WarehouseCode    string    `json:"warehouse_code"`
	Code             string    `json:"code"`
	Zone             *string   `json:"zone"`
	IsSellable       bool      `json:"is_sellable"`
	Description      *string   `json:"description"`
	IsActive         bool      `json:"is_active"`
	LockedByOpnameID *int64    `json:"locked_by_opname_id"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func NewRackResponse(r *domain.Rack) RackResponse {
	return RackResponse{
		ID: r.ID, WarehouseID: r.WarehouseID, WarehouseCode: r.WarehouseCode, Code: r.Code, Zone: r.Zone,
		IsSellable: r.IsSellable, Description: r.Description, IsActive: r.IsActive,
		LockedByOpnameID: r.LockedByOpnameID, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// ---------------------------------------------------------------- category

type CreateCategoryRequest struct {
	Name     string `json:"name" binding:"required,max=100"`
	ItemType string `json:"item_type" binding:"required,oneof=ASSET CONSUMABLE"`
}

type UpdateCategoryRequest struct {
	Name     *string `json:"name" binding:"omitempty,min=1,max=100"`
	ItemType *string `json:"item_type" binding:"omitempty,oneof=ASSET CONSUMABLE"`
	IsActive *bool   `json:"is_active"`
}

type ListCategoriesQuery struct {
	Q        string `form:"q" binding:"omitempty,max=100"`
	ItemType string `form:"item_type" binding:"omitempty,oneof=ASSET CONSUMABLE"`
	IsActive *bool  `form:"is_active"`
}

type CategoryResponse struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	ItemType string `json:"item_type"`
	IsActive bool   `json:"is_active"`
}

func NewCategoryResponse(c *domain.Category) CategoryResponse {
	return CategoryResponse{ID: c.ID, Name: c.Name, ItemType: string(c.ItemType), IsActive: c.IsActive}
}

// ---------------------------------------------------------------- product

type CreateProductRequest struct {
	SKU                string  `json:"sku" binding:"required,max=50"`
	Name               string  `json:"name" binding:"required,max=150"`
	CategoryID         int64   `json:"category_id" binding:"required,gte=1"`
	Brand              *string `json:"brand" binding:"omitempty,max=80"`
	Model              *string `json:"model" binding:"omitempty,max=80"`
	Unit               *string `json:"unit" binding:"omitempty,max=20"`
	HasSerialNumber    bool    `json:"has_serial_number"`
	MinStockAlert      int     `json:"min_stock_alert" binding:"gte=0"`
	DefaultMonthlyRate *string `json:"default_monthly_rate"`
	DefaultSellPrice   *string `json:"default_sell_price"`
	Description        *string `json:"description" binding:"omitempty,max=1000"`
}

type UpdateProductRequest struct {
	SKU                *string          `json:"sku" binding:"omitempty,min=1,max=50"`
	Name               *string          `json:"name" binding:"omitempty,min=1,max=150"`
	CategoryID         *int64           `json:"category_id" binding:"omitempty,gte=1"`
	Brand              Optional[string] `json:"brand"`
	Model              Optional[string] `json:"model"`
	Unit               *string          `json:"unit" binding:"omitempty,min=1,max=20"`
	HasSerialNumber    *bool            `json:"has_serial_number"`
	MinStockAlert      *int             `json:"min_stock_alert" binding:"omitempty,gte=0"`
	DefaultMonthlyRate Optional[string] `json:"default_monthly_rate"`
	DefaultSellPrice   Optional[string] `json:"default_sell_price"`
	Description        Optional[string] `json:"description"`
	IsActive           *bool            `json:"is_active"`
}

type ListProductsQuery struct {
	Q               string `form:"q" binding:"omitempty,max=100"`
	CategoryID      *int64 `form:"category_id" binding:"omitempty,gte=1"`
	ItemType        string `form:"item_type" binding:"omitempty,oneof=ASSET CONSUMABLE"`
	HasSerialNumber *bool  `form:"has_serial_number"`
	IsActive        *bool  `form:"is_active"`
}

type ProductResponse struct {
	ID                 int64     `json:"id"`
	SKU                string    `json:"sku"`
	Name               string    `json:"name"`
	CategoryID         int64     `json:"category_id"`
	CategoryName       string    `json:"category_name"`
	ItemType           string    `json:"item_type"`
	Brand              *string   `json:"brand"`
	Model              *string   `json:"model"`
	Unit               string    `json:"unit"`
	HasSerialNumber    bool      `json:"has_serial_number"`
	MinStockAlert      int       `json:"min_stock_alert"`
	DefaultMonthlyRate *string   `json:"default_monthly_rate"`
	DefaultSellPrice   *string   `json:"default_sell_price"`
	Description        *string   `json:"description"`
	IsActive           bool      `json:"is_active"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func money(d *decimal.Decimal) *string {
	if d == nil {
		return nil
	}
	s := d.StringFixed(2)
	return &s
}

func NewProductResponse(p *domain.Product) ProductResponse {
	return ProductResponse{
		ID: p.ID, SKU: p.SKU, Name: p.Name, CategoryID: p.CategoryID, CategoryName: p.CategoryName,
		ItemType: string(p.ItemType), Brand: p.Brand, Model: p.Model, Unit: p.Unit,
		HasSerialNumber: p.HasSerialNumber, MinStockAlert: p.MinStockAlert,
		DefaultMonthlyRate: money(p.DefaultMonthlyRate), DefaultSellPrice: money(p.DefaultSellPrice),
		Description: p.Description, IsActive: p.IsActive, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

// ---------------------------------------------------------------- supplier

type CreateSupplierRequest struct {
	Code    string  `json:"code" binding:"required,max=30"`
	Name    string  `json:"name" binding:"required,max=150"`
	PICName *string `json:"pic_name" binding:"omitempty,max=100"`
	Phone   *string `json:"phone" binding:"omitempty,max=30"`
	Email   *string `json:"email" binding:"omitempty,max=120"`
	NPWP    *string `json:"npwp" binding:"omitempty,max=30"`
	Address *string `json:"address" binding:"omitempty,max=500"`
}

// UpdateSupplierRequest: kode supplier tidak dapat diubah.
type UpdateSupplierRequest struct {
	Name     *string          `json:"name" binding:"omitempty,min=1,max=150"`
	PICName  Optional[string] `json:"pic_name"`
	Phone    Optional[string] `json:"phone"`
	Email    Optional[string] `json:"email"`
	NPWP     Optional[string] `json:"npwp"`
	Address  Optional[string] `json:"address"`
	IsActive *bool            `json:"is_active"`
}

type ListSuppliersQuery struct {
	Q        string `form:"q" binding:"omitempty,max=100"`
	IsActive *bool  `form:"is_active"`
}

type SupplierResponse struct {
	ID        int64     `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	PICName   *string   `json:"pic_name"`
	Phone     *string   `json:"phone"`
	Email     *string   `json:"email"`
	NPWP      *string   `json:"npwp"`
	Address   *string   `json:"address"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewSupplierResponse(s *domain.Supplier) SupplierResponse {
	return SupplierResponse{
		ID: s.ID, Code: s.Code, Name: s.Name, PICName: s.PICName, Phone: s.Phone, Email: s.Email,
		NPWP: s.NPWP, Address: s.Address, IsActive: s.IsActive, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
}
