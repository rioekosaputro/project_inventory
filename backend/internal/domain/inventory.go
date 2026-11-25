package domain

import (
	"context"
	"time"
)

// SerialStatus: status unit SN (CHECK di tabel product_serials).
type SerialStatus string

const (
	SerialAvailable   SerialStatus = "AVAILABLE"
	SerialReserved    SerialStatus = "RESERVED"
	SerialInTransit   SerialStatus = "IN_TRANSIT"
	SerialRented      SerialStatus = "RENTED"
	SerialMaintenance SerialStatus = "MAINTENANCE"
	SerialBroken      SerialStatus = "BROKEN"
	SerialLost        SerialStatus = "LOST"
	SerialRetired     SerialStatus = "RETIRED"
)

var allSerialStatuses = []SerialStatus{
	SerialAvailable, SerialReserved, SerialInTransit, SerialRented,
	SerialMaintenance, SerialBroken, SerialLost, SerialRetired,
}

func (s SerialStatus) Valid() bool {
	for _, x := range allSerialStatuses {
		if s == x {
			return true
		}
	}
	return false
}

// SerialStatuses mengembalikan salinan seluruh status (untuk pesan galat/dokumentasi).
func SerialStatuses() []SerialStatus { return append([]SerialStatus(nil), allSerialStatuses...) }

// StockBalance: satu baris saldo (produk × rak) beserta identitas produk/rak/gudang.
type StockBalance struct {
	ProductID       int64
	SKU             string
	ProductName     string
	Unit            string
	HasSerialNumber bool
	MinStockAlert   int

	RackID        int64
	RackCode      string
	Zone          *string
	IsSellable    bool
	WarehouseID   int64
	WarehouseCode string

	QtyOnHand    int
	QtyReserved  int
	QtyAvailable int  // qty_on_hand − qty_reserved bila rak dapat dijual, selain itu 0
	IsLowStock   bool // tingkat PRODUK (total tersedia di cakupan gudang) < min_stock_alert
	UpdatedAt    time.Time
}

// ProductStock: ringkasan stok per produk (semua rak dalam cakupan gudang).
type ProductStock struct {
	ProductID       int64
	SKU             string
	ProductName     string
	Unit            string
	CategoryID      int64
	CategoryName    string
	ItemType        ItemType
	HasSerialNumber bool
	MinStockAlert   int
	IsActive        bool

	QtyOnHand    int
	QtyReserved  int
	QtyAvailable int
	RackCount    int // jumlah rak yang menyimpan stok (qty_on_hand > 0)
	IsLowStock   bool
}

// Serial: satu unit SN beserta lokasi dan statusnya saat ini.
type Serial struct {
	ID              int64
	ProductID       int64
	SKU             string
	ProductName     string
	AssetTag        string
	SerialNumber    string
	Status          SerialStatus
	Condition       string
	RackID          *int64
	RackCode        *string
	WarehouseID     *int64
	WarehouseCode   *string
	CustomerID      *int64
	SiteID          *int64
	ContractID      *int64
	ReservedForDOID *int64
	WarrantyUntil   *string // YYYY-MM-DD
	Notes           *string
	UpdatedAt       time.Time
}

// SerialHistory: satu entri riwayat status/lokasi unit (append-only).
type SerialHistory struct {
	ID            int64
	FromStatus    *string
	ToStatus      string
	FromRackCode  *string
	ToRackCode    *string
	CustomerID    *int64
	EventType     string
	ReferenceType *string
	ReferenceID   *int64
	UserID        *int64
	UserName      *string
	Notes         *string
	CreatedAt     time.Time
}

// ---------------------------------------------------------------- filter

// BalanceFilter. ScopeWarehouseID membatasi hasil ke satu gudang (warehouse scoping pemanggil);
// WarehouseID adalah filter pilihan pengguna. Keduanya diterapkan juga pada penghitungan IsLowStock.
type BalanceFilter struct {
	Query            string // SKU atau nama produk
	ProductID        *int64
	RackID           *int64
	WarehouseID      *int64
	ScopeWarehouseID *int64
	LowStock         bool
	IncludeZero      bool // sertakan rak yang saldonya 0 (bawaan: disembunyikan)
}

type StockSummaryFilter struct {
	Query            string
	CategoryID       *int64
	ItemType         *ItemType
	WarehouseID      *int64
	ScopeWarehouseID *int64
	LowStock         bool
	HasStock         bool // hanya produk dengan qty_on_hand > 0
	IsActive         *bool
}

// SerialFilter. Unit yang sedang tidak berada di gudang (RENTED, IN_TRANSIT, LOST, RETIRED)
// tetap terlihat oleh akun yang dibatasi ke satu gudang.
type SerialFilter struct {
	Query            string // asset tag, SN, SKU, atau nama produk
	ProductID        *int64
	Status           *SerialStatus
	CustomerID       *int64
	WarehouseID      *int64
	RackID           *int64
	ScopeWarehouseID *int64
}

// ---------------------------------------------------------------- repository

// InventoryRepository: pembacaan stok & unit (tanpa mutasi; mutasi stok hanya di usecase transaksi).
type InventoryRepository interface {
	ListBalances(ctx context.Context, f BalanceFilter, p PageParams) ([]StockBalance, int64, error)
	ListProductStock(ctx context.Context, f StockSummaryFilter, p PageParams) ([]ProductStock, int64, error)
	ListSerials(ctx context.Context, f SerialFilter, p PageParams) ([]Serial, int64, error)
	// FindSerialsByCode mencari unit lewat asset tag (unik) atau, bila tidak ada, serial number
	// (bisa lebih dari satu produk). Hasil kosong = tidak ditemukan.
	FindSerialsByCode(ctx context.Context, code string) ([]Serial, error)
	// FindSerialsByIDs mengambil daftar unit SN untuk cetak label batch.
	FindSerialsByIDs(ctx context.Context, ids []int64) ([]Serial, error)
	FindSerialByID(ctx context.Context, id int64) (*Serial, error)
	// RecentSerialHistory: riwayat terbaru (terbaru dulu), maksimal limit entri.
	RecentSerialHistory(ctx context.Context, serialID int64, limit int) ([]SerialHistory, error)

	// Buku besar & kartu stok (T1.13).
	ListLedgers(ctx context.Context, f LedgerFilter, p PageParams) ([]LedgerRow, int64, error)
	// OpeningBalance menghitung saldo (Σ IN − Σ OUT) sebelum tanggal DateFrom (00:00 WIB).
	OpeningBalance(ctx context.Context, f StockCardFilter) (int, error)
}

// LedgerRow: satu baris buku besar mutasi stok beserta relasi produk/rak/LPB/approver.
type LedgerRow struct {
	ID            int64
	OccurredAt    time.Time
	ProductID     int64
	SKU           string
	ProductName   string
	Unit          string
	WarehouseID   int64
	WarehouseCode string
	RackID        int64
	RackCode      string
	Direction     string
	Qty           int
	BalanceBefore int
	BalanceAfter  int
	MovementType  LPBSubType
	LPBID         int64
	LPBNumber     string
	SerialCount   int
	CreatedBy     int64
	CreatedByName string
	Notes         *string
}

type LedgerFilter struct {
	Query            string // SKU, nama produk, atau nomor LPB
	ProductID        *int64
	RackID           *int64
	WarehouseID      *int64
	ScopeWarehouseID *int64
	Direction        *string // IN | OUT
	MovementType     *LPBSubType
	DateFrom         *string // YYYY-MM-DD (WIB)
	DateTo           *string // YYYY-MM-DD (WIB)
}

type StockCardFilter struct {
	ProductID        int64
	RackID           *int64
	WarehouseID      *int64
	ScopeWarehouseID *int64
	DateFrom         *string // YYYY-MM-DD (WIB)
	DateTo           *string // YYYY-MM-DD (WIB)
}
