package domain

import (
	"context"
	"time"
)

type OpnameStatus string

const (
	OpnameInProgress OpnameStatus = "IN_PROGRESS"
	OpnameSubmitted  OpnameStatus = "SUBMITTED"
	OpnameFinalized  OpnameStatus = "FINALIZED"
	OpnameCancelled  OpnameStatus = "CANCELLED"
)

func (s OpnameStatus) Valid() bool {
	switch s {
	case OpnameInProgress, OpnameSubmitted, OpnameFinalized, OpnameCancelled:
		return true
	}
	return false
}

type OpnameSerialResult string

const (
	OpnameSerialFound      OpnameSerialResult = "FOUND"
	OpnameSerialMissing    OpnameSerialResult = "MISSING"
	OpnameSerialUnexpected OpnameSerialResult = "UNEXPECTED"
	OpnameSerialWrongRack  OpnameSerialResult = "WRONG_RACK"
)

const (
	EntityOpname        = "stock_opname"
	AuditOpnameStart    = "OPNAME_START"
	AuditOpnameSubmit   = "OPNAME_SUBMIT"
	AuditOpnameFinalize = "OPNAME_FINALIZE"
	AuditOpnameCancel   = "OPNAME_CANCEL"
)

type Opname struct {
	ID                     int64
	Number                 string
	WarehouseID            int64
	WarehouseCode          string
	WarehouseName          string
	Status                 OpnameStatus
	IsBlindCount           bool
	StartedBy              int64
	StartedByName          string
	StartedAt              time.Time
	SubmittedBy            *int64
	SubmittedAt            *time.Time
	FinalizedBy            *int64
	FinalizedAt            *time.Time
	AdjustmentInLPBID      *int64
	AdjustmentInLPBNumber  *string
	AdjustmentOutLPBID     *int64
	AdjustmentOutLPBNumber *string
	Notes                  *string
}

type OpnameListItem struct {
	Opname
	RackCount    int
	ItemCount    int
	CountedCount int
	SerialCount  int
	FoundCount   int
}

type OpnameRack struct {
	RackID     int64
	RackCode   string
	Zone       *string
	IsSellable bool
}

type OpnameItem struct {
	ID          int64
	OpnameID    int64
	ProductID   int64
	SKU         string
	ProductName string
	Unit        string
	RackID      int64
	RackCode    string
	SystemQty   int
	CountedQty  *int
	DiffQty     *int
	CountedBy   *int64
	CountedAt   *time.Time
	Notes       *string
}

type OpnameSerial struct {
	ID              int64
	OpnameID        int64
	RackID          int64
	RackCode        string
	ProductSerialID *int64
	AssetTag        *string
	SerialNumber    *string
	SKU             *string
	ProductName     *string
	ScannedCode     *string
	Result          OpnameSerialResult
	ScannedBy       *int64
	ScannedAt       *time.Time
}

type OpnameFilter struct {
	Query            string
	Status           *OpnameStatus
	WarehouseID      *int64
	ScopeWarehouseID *int64
}

type OpnameRepository interface {
	Create(ctx context.Context, o *Opname) (*Opname, error)
	FindByID(ctx context.Context, id int64) (*Opname, error)
	FindByIDForUpdate(ctx context.Context, id int64) (*Opname, error)
	List(ctx context.Context, f OpnameFilter, p PageParams) ([]OpnameListItem, int64, error)

	AddRacks(ctx context.Context, opnameID int64, rackIDs []int64) error
	Racks(ctx context.Context, opnameID int64) ([]OpnameRack, error)
	LockRacksForOpname(ctx context.Context, opnameID int64, rackIDs []int64) error
	UnlockRacksForOpname(ctx context.Context, opnameID int64) error

	// SnapshotNonSerialBalances menyalin saldo non-SN (>0) pada rak opname sebagai target hitung.
	SnapshotNonSerialBalances(ctx context.Context, opnameID int64, rackIDs []int64) error
	// SnapshotSerialUnits menyalin unit SN fisik pada rak opname dengan status awal MISSING.
	SnapshotSerialUnits(ctx context.Context, opnameID int64, rackIDs []int64) error

	Items(ctx context.Context, opnameID int64) ([]OpnameItem, error)
	UpsertItemCount(ctx context.Context, opnameID, productID, rackID int64, countedQty int, userID int64, at time.Time, notes *string) error

	Serials(ctx context.Context, opnameID int64) ([]OpnameSerial, error)
	UpsertSerialScan(ctx context.Context, opnameID, rackID int64, productSerialID *int64, scannedCode string, result OpnameSerialResult, userID int64, at time.Time) (*OpnameSerial, error)

	MarkSubmitted(ctx context.Context, id, userID int64, at time.Time) error
	MarkFinalized(ctx context.Context, id, userID int64, at time.Time, adjInLPBID, adjOutLPBID *int64) error
	MarkCancelled(ctx context.Context, id int64) error
}
