package domain

import (
	"context"
	"time"
)

// DeliveryOrder adalah dokumen penugasan gudang (OUTBOUND / INBOUND) yang lahir dari Sales Order.
type DeliveryOrder struct {
	ID             int64
	DONumber       string
	SOID           int64
	SONumber       string
	OrderType      OrderType
	CustomerID     int64
	CustomerCode   string
	CustomerName   string
	CustomerSiteID int64
	SiteName       string
	SiteCity       string
	ContractID     *int64
	WarehouseID    int64
	WarehouseCode  string
	WarehouseName  string
	Direction      DODirection
	Status         DOStatus
	ScheduledDate  *time.Time
	LPBID          *int64
	LPBNumber      *string
	Notes          *string
	CreatedBy      int64
	CreatedByName  string
	CreatedAt      time.Time
	UpdatedAt      time.Time

	ItemCount   int
	TotalQty    int
	ReservedQty int
}

type DeliveryOrderItem struct {
	ID               int64
	DOID             int64
	SOItemID         int64
	ProductID        int64
	ProductSKU       string
	ProductName      string
	HasSerialNumber  bool
	Qty              int
	ReservedQty      int
	SOProductSerial  *int64  // opsional: serial yang diminta pada SO WITHDRAWAL/UNIT_SWAP IN
	SOSerialNumber   *string // dari JOIN
	Reservations     []DOReservation
}

type DOReservation struct {
	ID              int64
	DOItemID        int64
	ProductID       int64
	RackID          int64
	RackCode        string
	ProductSerialID *int64
	SerialNumber    *string
	AssetTag        *string
	Qty             int
	ReservedBy      int64
	ReservedAt      time.Time
	ReleasedAt      *time.Time
}

type DeliveryOrderDetail struct {
	DO    *DeliveryOrder
	Items []DeliveryOrderItem
}

type DeliveryOrderFilter struct {
	Query       string
	WarehouseID *int64
	Direction   *DODirection
	Status      *DOStatus
	SOID        *int64
}

type DeliveryOrderRepository interface {
	FindByID(ctx context.Context, id int64) (*DeliveryOrder, error)
	FindByIDForUpdate(ctx context.Context, id int64) (*DeliveryOrder, error)
	List(ctx context.Context, f DeliveryOrderFilter, scopeWarehouseID *int64, p PageParams) ([]DeliveryOrder, int64, error)
	ListItems(ctx context.Context, doID int64) ([]DeliveryOrderItem, error)
	ListActiveReservations(ctx context.Context, doID int64) ([]DOReservation, error)
	InsertReservation(ctx context.Context, res *DOReservation) (*DOReservation, error)
	MarkReservationsReleased(ctx context.Context, doID int64, at time.Time) error
	UpdateStatus(ctx context.Context, doID int64, status DOStatus, lpbID *int64) error
}
