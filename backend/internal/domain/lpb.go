package domain

import (
	"context"
	"encoding/json"
	"time"

	"github.com/shopspring/decimal"
)

type LPBType string

const (
	LPBInbound  LPBType = "INBOUND"
	LPBOutbound LPBType = "OUTBOUND"
)

// LPBSubType: jenis LPB (CHECK chk_lpb_type_subtype). Efek stok per jenis: plan Bagian 5.2.
type LPBSubType string

const (
	// INBOUND
	LPBPurchase           LPBSubType = "PURCHASE"
	LPBInitialStock       LPBSubType = "INITIAL_STOCK"
	LPBRentalReturn       LPBSubType = "RENTAL_RETURN"
	LPBFailedDeliveryRet  LPBSubType = "FAILED_DELIVERY_RETURN"
	LPBTransferIn         LPBSubType = "TRANSFER_IN"
	LPBOpnameAdjustmentIn LPBSubType = "OPNAME_ADJUSTMENT_IN"
	// OUTBOUND
	LPBRentalDelivery      LPBSubType = "RENTAL_DELIVERY"
	LPBConsumableDelivery  LPBSubType = "CONSUMABLE_DELIVERY"
	LPBSparePartIssue      LPBSubType = "SPARE_PART_ISSUE"
	LPBTransferOut         LPBSubType = "TRANSFER_OUT"
	LPBOpnameAdjustmentOut LPBSubType = "OPNAME_ADJUSTMENT_OUT"
	LPBDisposal            LPBSubType = "DISPOSAL"
)

var inboundSubTypes = map[LPBSubType]bool{
	LPBPurchase: true, LPBInitialStock: true, LPBRentalReturn: true,
	LPBFailedDeliveryRet: true, LPBTransferIn: true, LPBOpnameAdjustmentIn: true,
}

var outboundSubTypes = map[LPBSubType]bool{
	LPBRentalDelivery: true, LPBConsumableDelivery: true, LPBSparePartIssue: true,
	LPBTransferOut: true, LPBOpnameAdjustmentOut: true, LPBDisposal: true,
}

// Valid true bila jenis dikenal.
func (s LPBSubType) Valid() bool { return inboundSubTypes[s] || outboundSubTypes[s] }

// Type: arah LPB yang ditentukan jenisnya.
func (s LPBSubType) Type() LPBType {
	if inboundSubTypes[s] {
		return LPBInbound
	}
	return LPBOutbound
}

// ManuallyCreatable: jenis yang boleh dibuat staf lewat API (POST /lpb). Jenis lain lahir dari
// proses lain (DO, tiket, opname, pasangan transfer) pada fase masing-masing.
func (s LPBSubType) ManuallyCreatable() bool {
	switch s {
	case LPBPurchase, LPBInitialStock, LPBDisposal:
		return true
	}
	return false
}

type LPBStatus string

const (
	LPBDraft           LPBStatus = "DRAFT"
	LPBPendingApproval LPBStatus = "PENDING_APPROVAL"
	LPBApproved        LPBStatus = "APPROVED"
	LPBRejected        LPBStatus = "REJECTED"
	LPBCancelled       LPBStatus = "CANCELLED"
)

func (s LPBStatus) Valid() bool {
	switch s {
	case LPBDraft, LPBPendingApproval, LPBApproved, LPBRejected, LPBCancelled:
		return true
	}
	return false
}

// SerialCondition: kondisi unit saat discan (CHECK condition_on_scan).
type SerialCondition string

const (
	ConditionGood        SerialCondition = "GOOD"
	ConditionMinorDefect SerialCondition = "MINOR_DEFECT"
	ConditionBroken      SerialCondition = "BROKEN"
)

func (c SerialCondition) Valid() bool {
	return c == ConditionGood || c == ConditionMinorDefect || c == ConditionBroken
}

// Aksi audit LPB (entity_type "lpb"); juga membentuk timeline dokumen.
const (
	EntityLPB       = "lpb"
	AuditLPBCreate  = "LPB_CREATE"
	AuditLPBUpdate  = "LPB_UPDATE"
	AuditLPBSubmit  = "LPB_SUBMIT"
	AuditLPBCancel  = "LPB_CANCEL"
	AuditLPBRevise  = "LPB_REVISE"
	AuditLPBApprove = "LPB_APPROVE"
	AuditLPBReject  = "LPB_REJECT"
)

// LPB adalah header dokumen penerimaan/pengeluaran barang.
type LPB struct {
	ID              int64
	Number          string
	Type            LPBType
	SubType         LPBSubType
	Status          LPBStatus
	WarehouseID     int64
	WarehouseCode   string
	DocDate         string // YYYY-MM-DD (WIB)
	ReferenceType   *string
	ReferenceNumber *string
	SupplierID      *int64
	SupplierName    *string
	Notes           *string
	CreatedBy       int64
	CreatedByName   string
	SubmittedBy     *int64
	SubmittedAt     *time.Time
	ApprovedBy      *int64
	ApprovedAt      *time.Time
	RejectedBy      *int64
	RejectedAt      *time.Time
	RejectionReason *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// LPBListItem: baris daftar (header + agregat item).
type LPBListItem struct {
	LPB
	ItemCount int
	TotalQty  int
}

type LPBItem struct {
	ID              int64
	LPBID           int64
	ProductID       int64
	SKU             string
	ProductName     string
	Unit            string
	HasSerialNumber bool
	RackID          int64
	RackCode        string
	RackIsSellable  bool
	Qty             int
	UnitCost        *decimal.Decimal
	Notes           *string
	// CurrentQty: saldo rak saat ini (qty_on_hand) — untuk tampilan "sebelum → sesudah" sebelum approve.
	CurrentQty int
	// BalanceBefore/After: saldo rak sebelum/sesudah mutasi item ini (dari ledger; hanya setelah APPROVED).
	BalanceBefore *int
	BalanceAfter  *int
	Serials       []LPBItemSerial
}

type LPBItemSerial struct {
	ID              int64
	LPBItemID       int64
	ProductSerialID *int64 // terisi untuk unit yang sudah terdaftar (mis. DISPOSAL); NULL untuk SN baru
	SerialNumber    string
	AssetTag        *string // dari unit terdaftar (bila ada)
	Condition       SerialCondition
	ScannedBy       *int64
	ScannedAt       time.Time
}

// LPBEvent: satu entri timeline dokumen (dari audit_logs).
type LPBEvent struct {
	Action    string
	UserID    *int64
	UserName  *string
	Data      json.RawMessage // after_data
	CreatedAt time.Time
}

// SerialState: status/rak unit terdaftar saat ini (validasi ulang saat submit).
type SerialState struct {
	ID       int64
	Status   SerialStatus
	RackID   *int64
	AssetTag string
}

type LPBFilter struct {
	Query            string // nomor LPB, nomor referensi, atau nama supplier
	Type             *LPBType
	SubType          *LPBSubType
	Status           *LPBStatus
	WarehouseID      *int64
	ScopeWarehouseID *int64
	DateFrom         *string // YYYY-MM-DD
	DateTo           *string
	CreatedBy        *int64
}

// LPBRepository — SQL saja. Semua tulis wajib di dalam transaksi usecase (header di-lock FOR UPDATE).
type LPBRepository interface {
	Create(ctx context.Context, l *LPB) (*LPB, error)
	FindByID(ctx context.Context, id int64) (*LPB, error)
	// FindByIDForUpdate wajib di dalam transaksi; kunci header sebelum menyentuh item/saldo/unit.
	FindByIDForUpdate(ctx context.Context, id int64) (*LPB, error)
	List(ctx context.Context, f LPBFilter, p PageParams) ([]LPBListItem, int64, error)
	// UpdateHeader menyimpan doc_date, supplier_id, reference_type, reference_number, notes.
	UpdateHeader(ctx context.Context, l *LPB) error
	MarkSubmitted(ctx context.Context, id, userID int64, at time.Time) error
	MarkCancelled(ctx context.Context, id int64) error
	// MarkRevised: REJECTED → DRAFT (alasan penolakan tetap tersimpan sampai diajukan ulang).
	MarkRevised(ctx context.Context, id int64) error
	// MarkApproved: PENDING_APPROVAL → APPROVED beserta token verifikasi QR.
	MarkApproved(ctx context.Context, id, userID int64, at time.Time, verifyToken string) error
	// MarkRejected: PENDING_APPROVAL → REJECTED dengan alasan.
	MarkRejected(ctx context.Context, id, userID int64, at time.Time, reason string) error
	// SetItemSerialLink mengaitkan baris scan dengan unit yang baru didaftarkan saat approve.
	SetItemSerialLink(ctx context.Context, itemSerialID, productSerialID int64) error

	Items(ctx context.Context, lpbID int64) ([]LPBItem, error)
	FindItem(ctx context.Context, lpbID, itemID int64) (*LPBItem, error)
	InsertItem(ctx context.Context, lpbID int64, it *LPBItem) (int64, error)
	UpdateItem(ctx context.Context, it *LPBItem) error
	DeleteItem(ctx context.Context, itemID int64) error

	// InsertItemSerial: SN yang sama pada item yang sama → *DuplicateError.
	InsertItemSerial(ctx context.Context, s *LPBItemSerial) (*LPBItemSerial, error)
	DeleteItemSerial(ctx context.Context, itemID, serialRowID int64) error

	// SerialClaimedByOtherLPB: nomor LPB lain (DRAFT/PENDING_APPROVAL) yang sudah memuat SN ini
	// untuk produk yang sama; found=false bila bebas.
	SerialClaimedByOtherLPB(ctx context.Context, productID int64, serialNumber string, productSerialID *int64, excludeLPBID int64) (number string, found bool, err error)
	// LockSerialKey menyerialkan scan SN yang sama pada LPB berbeda (advisory lock tingkat TRANSAKSI per
	// produk+SN; aman untuk transaction pooler karena dilepas saat transaksi selesai). Wajib di dalam transaksi.
	LockSerialKey(ctx context.Context, productID int64, serialNumber string) error
	// ProductSerialExists: SN (tanpa peduli huruf) sudah terdaftar di product_serials untuk produk ini.
	ProductSerialExists(ctx context.Context, productID int64, serialNumber string) (bool, error)
	SerialStates(ctx context.Context, ids []int64) ([]SerialState, error)
	// AvailableQty: stok tersedia (on hand − reserved, rak dapat dijual) untuk peringatan dini saat submit.
	AvailableQty(ctx context.Context, productID, rackID int64) (int, error)

	Timeline(ctx context.Context, lpbID int64) ([]LPBEvent, error)
}
