package domain

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// NewSerial: unit SN baru yang didaftarkan saat approve LPB masuk (PURCHASE/INITIAL_STOCK).
type NewSerial struct {
	ProductID     int64
	WarehouseID   int64
	RackID        int64
	AssetTag      string
	SerialNumber  string
	Status        SerialStatus
	Condition     SerialCondition
	SupplierID    *int64
	PurchaseDate  *string // YYYY-MM-DD
	PurchasePrice *decimal.Decimal
}

// SerialHistoryEntry: satu baris product_serial_histories (append-only).
type SerialHistoryEntry struct {
	SerialID      int64
	FromStatus    *string
	ToStatus      string
	FromRackID    *int64
	ToRackID      *int64
	EventType     string // mis. LPB_APPROVED
	ReferenceType *string
	ReferenceID   *int64
	UserID        *int64
	Notes         *string
}

// LedgerEntry: satu baris stock_ledgers (append-only). Direction "IN" atau "OUT".
type LedgerEntry struct {
	ProductID    int64
	WarehouseID  int64
	RackID       int64
	Direction    string
	Qty          int
	Before       int
	After        int
	MovementType LPBSubType
	LPBID        int64
	LPBItemID    int64
	SerialIDs    []int64
	UnitCost     *decimal.Decimal
	CreatedBy    int64
	Notes        *string
}

// StockRepository: SATU-SATUNYA jalur penulisan saldo, unit, dan ledger. Hanya boleh dipanggil dari
// usecase transaksi (approve LPB, DO, POD, opname) di dalam transaksi — tidak pernah dari handler
// dan tidak pernah saat dokumen masih DRAFT (CLAUDE.md invarian #1). Urutan lock: header dokumen →
// saldo (product_id, rack_id) → unit (id).
type StockRepository interface {
	// AddStock menambah qty_on_hand rak (upsert, baris saldo terkunci) dan mengembalikan saldo
	// sebelum/sesudah. Wajib di dalam transaksi.
	AddStock(ctx context.Context, productID, rackID int64, qty int) (before, after int, err error)
	// InsertSerial mendaftarkan unit baru; (produk, SN) sudah ada → ErrSerialAlreadyExists.
	InsertSerial(ctx context.Context, s *NewSerial) (int64, error)
	// RemoveStock mengurangi qty_on_hand rak dan mengembalikan saldo sebelum/sesudah. Gagal dengan
	// ErrInsufficientStock bila (on hand − reserved) < qty atau saldo belum ada. Wajib di dalam transaksi.
	RemoveStock(ctx context.Context, productID, rackID int64, qty int) (before, after int, err error)
	// ReserveStock menambah qty_reserved rak (menolak dengan ErrInsufficientStock bila tersedia < qty).
	ReserveStock(ctx context.Context, productID, rackID int64, qty int) error
	// ReleaseStock mengurangi qty_reserved rak (menolak dengan ErrReservationMismatch bila reserved < qty).
	ReleaseStock(ctx context.Context, productID, rackID int64, qty int) error
	// ConsumeReservedStock mengurangi qty_on_hand dan qty_reserved bersamaan dalam 1 UPDATE atomik (LPB dari DO).
	ConsumeReservedStock(ctx context.Context, productID, rackID int64, qty int) (before, after int, err error)
	// LockSerials mengunci unit (FOR UPDATE, urut id). Pemanggil wajib mengirim id yang sudah terurut.
	LockSerials(ctx context.Context, ids []int64) ([]SerialLock, error)
	// MoveSerial mengubah status dan lokasi unit (rak/gudang nil untuk IN_TRANSIT dan RETIRED).
	MoveSerial(ctx context.Context, id int64, status SerialStatus, warehouseID, rackID *int64) error
	// ReserveSerial mengubah unit AVAILABLE → RESERVED untuk doID.
	ReserveSerial(ctx context.Context, id, doID int64) error
	// ReleaseSerial mengembalikan unit RESERVED (milik doID) → AVAILABLE.
	ReleaseSerial(ctx context.Context, id, doID int64) error
	// DispatchReservedSerial mengubah unit RESERVED (milik doID) → IN_TRANSIT tujuan customer/site.
	DispatchReservedSerial(ctx context.Context, id, doID int64, customerID, siteID *int64) error
	// ReturnSerialToRack memindahkan unit IN_TRANSIT kembali ke gudang/rak (RENTAL_RETURN) dan mengosongkan customer/site/contract.
	ReturnSerialToRack(ctx context.Context, id int64, status SerialStatus, condition SerialCondition, warehouseID, rackID int64) error
	InsertSerialHistory(ctx context.Context, h SerialHistoryEntry) error
	InsertLedger(ctx context.Context, e LedgerEntry) error
}

// SerialLock: unit terdaftar yang dikunci (FOR UPDATE) beserta posisinya saat ini.
type SerialLock struct {
	ID              int64
	ProductID       int64
	AssetTag        string
	Number          string
	Status          SerialStatus
	RackID          *int64
	WarehouseID     *int64
	ReservedForDOID *int64
}

// SettingsRepository membaca app_settings.
type SettingsRepository interface {
	// Bool mengembalikan nilai boolean JSON; kunci tidak ada → def.
	Bool(ctx context.Context, key string, def bool) (bool, error)
}

// IdempotencyRecord: hasil permintaan sebelumnya untuk (key, user).
type IdempotencyRecord struct {
	Endpoint   string
	StatusCode *int // nil = permintaan pertama masih diproses
	Body       []byte
	CreatedAt  time.Time
}

// IdempotencyRepository: penyimpanan respons untuk endpoint kritis (plan Bagian 7.5).
type IdempotencyRepository interface {
	// Begin mencoba memesan (key, user). started=true → pemanggil yang memproses; false → rec berisi
	// catatan yang sudah ada. Catatan yang lebih tua dari ttl dibuang dan dipesan ulang.
	Begin(ctx context.Context, key string, userID int64, endpoint string, ttl time.Duration) (started bool, rec *IdempotencyRecord, err error)
	// Complete menyimpan respons final.
	Complete(ctx context.Context, key string, userID int64, status int, body []byte) error
	// Abort membebaskan kunci (respons 5xx / panik) agar permintaan yang sama dapat diulang.
	Abort(ctx context.Context, key string, userID int64) error
}
