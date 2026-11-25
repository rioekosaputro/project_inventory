package domain

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// ItemType: jenis barang pada kategori (CHECK di tabel categories).
type ItemType string

const (
	ItemTypeAsset      ItemType = "ASSET"
	ItemTypeConsumable ItemType = "CONSUMABLE"
)

func (t ItemType) Valid() bool { return t == ItemTypeAsset || t == ItemTypeConsumable }

// Jenis entitas master untuk audit_logs.entity_type.
const (
	EntityWarehouse = "warehouse"
	EntityRack      = "rack"
	EntityCategory  = "category"
	EntityProduct   = "product"
	EntitySupplier  = "supplier"
)

// Verba aksi audit master: aksi lengkap = ToUpper(entity)+"_"+verba (mis. WAREHOUSE_CREATE).
const (
	AuditVerbCreate     = "CREATE"
	AuditVerbUpdate     = "UPDATE"
	AuditVerbActivate   = "ACTIVATE"
	AuditVerbDeactivate = "DEACTIVATE"
)

// ---------------------------------------------------------------- entity

type Warehouse struct {
	ID        int64
	Code      string // tidak berubah setelah dibuat: dipakai sebagai scope nomor dokumen
	Name      string
	Address   *string
	Phone     *string
	Latitude  *float64
	Longitude *float64
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Rack struct {
	ID               int64
	WarehouseID      int64
	WarehouseCode    string // dari JOIN, hanya baca
	Code             string // tidak berubah setelah dibuat (tercetak di label rak)
	Zone             *string
	IsSellable       bool
	Description      *string
	IsActive         bool
	LockedByOpnameID *int64 // diisi/dikosongkan oleh stock opname, bukan lewat CRUD master
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type Category struct {
	ID       int64
	Name     string
	ItemType ItemType
	IsActive bool
}

type Product struct {
	ID                 int64
	SKU                string
	Name               string
	CategoryID         int64
	CategoryName       string   // dari JOIN, hanya baca
	ItemType           ItemType // dari JOIN, hanya baca
	Brand              *string
	Model              *string
	Unit               string
	HasSerialNumber    bool
	MinStockAlert      int
	DefaultMonthlyRate *decimal.Decimal
	DefaultSellPrice   *decimal.Decimal
	Description        *string
	IsActive           bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type Supplier struct {
	ID        int64
	Code      string
	Name      string
	PICName   *string
	Phone     *string
	Email     *string
	NPWP      *string
	Address   *string
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ---------------------------------------------------------------- filter

type WarehouseFilter struct {
	Query    string // cocok sebagian pada kode atau nama
	IsActive *bool
	OnlyID   *int64 // batasan scope gudang pemanggil
}

type RackFilter struct {
	Query       string // kode atau zona
	WarehouseID *int64
	Zone        string
	IsSellable  *bool
	IsActive    *bool
}

type CategoryFilter struct {
	Query    string
	ItemType *ItemType
	IsActive *bool
}

type ProductFilter struct {
	Query           string // SKU, nama, merek, atau model
	CategoryID      *int64
	ItemType        *ItemType
	HasSerialNumber *bool
	IsActive        *bool
}

type SupplierFilter struct {
	Query    string // kode, nama, atau PIC
	IsActive *bool
}

// ---------------------------------------------------------------- repository

// WarehouseRepository. Create yang bentrok kode → *DuplicateError.
type WarehouseRepository interface {
	// IsActive: ErrNotFound bila gudang tidak ada.
	IsActive(ctx context.Context, id int64) (bool, error)
	FindByID(ctx context.Context, id int64) (*Warehouse, error)
	// FindByIDForUpdate wajib di dalam transaksi.
	FindByIDForUpdate(ctx context.Context, id int64) (*Warehouse, error)
	// FindByIDForShare: FOR SHARE (wajib di dalam transaksi) — menahan perubahan gudang selama approve.
	FindByIDForShare(ctx context.Context, id int64) (*Warehouse, error)
	List(ctx context.Context, f WarehouseFilter, p PageParams) ([]Warehouse, int64, error)
	Create(ctx context.Context, w *Warehouse) (*Warehouse, error)
	// Update menyimpan name, address, phone, latitude, longitude, is_active (code tidak berubah).
	Update(ctx context.Context, w *Warehouse) error
	// HasStock true bila ada rak di gudang ini yang masih menyimpan stok (qty_on_hand > 0) atau unit SN.
	HasStock(ctx context.Context, id int64) (bool, error)
}

type RackRepository interface {
	// LockForShare mengunci baris rak (FOR SHARE, urut id) agar rak tidak dapat dikunci opname / dinonaktifkan
	// selama transaksi approve berjalan. Wajib di dalam transaksi.
	LockForShare(ctx context.Context, ids []int64) ([]Rack, error)
	FindByID(ctx context.Context, id int64) (*Rack, error)
	FindByIDs(ctx context.Context, ids []int64) ([]Rack, error)
	FindByIDForUpdate(ctx context.Context, id int64) (*Rack, error)
	List(ctx context.Context, f RackFilter, scopeWarehouseID *int64, p PageParams) ([]Rack, int64, error)
	// Create: kode bentrok pada gudang yang sama → *DuplicateError.
	Create(ctx context.Context, r *Rack) (*Rack, error)
	// Update menyimpan zone, is_sellable, description, is_active (warehouse & code tidak berubah).
	Update(ctx context.Context, r *Rack) error
	// HasStock true bila rak masih menyimpan stok (qty_on_hand > 0) atau unit SN.
	HasStock(ctx context.Context, id int64) (bool, error)
}

type CategoryRepository interface {
	FindByID(ctx context.Context, id int64) (*Category, error)
	FindByIDForUpdate(ctx context.Context, id int64) (*Category, error)
	List(ctx context.Context, f CategoryFilter, p PageParams) ([]Category, int64, error)
	Create(ctx context.Context, c *Category) (*Category, error)
	// Update menyimpan name, item_type, is_active.
	Update(ctx context.Context, c *Category) error
	// HasProducts true bila ada produk (aktif maupun tidak) di kategori ini.
	HasProducts(ctx context.Context, id int64) (bool, error)
}

type ProductRepository interface {
	FindByID(ctx context.Context, id int64) (*Product, error)
	FindByIDForUpdate(ctx context.Context, id int64) (*Product, error)
	List(ctx context.Context, f ProductFilter, p PageParams) ([]Product, int64, error)
	Create(ctx context.Context, p *Product) (*Product, error)
	// Update menyimpan semua kolom yang dapat diubah (sku, name, category, brand, model, unit,
	// has_serial_number, min_stock_alert, tarif/harga, description, is_active).
	Update(ctx context.Context, p *Product) error
	// HasMovements true bila produk sudah dipakai stok/SN/LPB, sehingga SKU & mode SN dikunci.
	HasMovements(ctx context.Context, id int64) (bool, error)
}

type SupplierRepository interface {
	FindByID(ctx context.Context, id int64) (*Supplier, error)
	FindByIDForUpdate(ctx context.Context, id int64) (*Supplier, error)
	List(ctx context.Context, f SupplierFilter, p PageParams) ([]Supplier, int64, error)
	Create(ctx context.Context, s *Supplier) (*Supplier, error)
	// Update menyimpan semua kolom selain code (name, pic, phone, email, npwp, address, is_active).
	Update(ctx context.Context, s *Supplier) error
}
