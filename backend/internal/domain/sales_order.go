package domain

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

const (
	EntitySalesOrder    = "sales_order"
	EntityDeliveryOrder = "delivery_order"
)

type OrderType string

const (
	OrderTypeNewDelivery OrderType = "NEW_DELIVERY"
	OrderTypeWithdrawal  OrderType = "WITHDRAWAL"
	OrderTypeUnitSwap    OrderType = "UNIT_SWAP"
	OrderTypeConsumable  OrderType = "CONSUMABLE"
)

func (t OrderType) Valid() bool {
	return t == OrderTypeNewDelivery || t == OrderTypeWithdrawal || t == OrderTypeUnitSwap || t == OrderTypeConsumable
}

type SOStatus string

const (
	SODraft      SOStatus = "DRAFT"
	SOConfirmed  SOStatus = "CONFIRMED"
	SOInProgress SOStatus = "IN_PROGRESS"
	SOCompleted  SOStatus = "COMPLETED"
	SOCancelled  SOStatus = "CANCELLED"
)

func (s SOStatus) Valid() bool {
	return s == SODraft || s == SOConfirmed || s == SOInProgress || s == SOCompleted || s == SOCancelled
}

type ItemDirection string

const (
	DirectionOut ItemDirection = "OUT"
	DirectionIn  ItemDirection = "IN"
)

func (d ItemDirection) Valid() bool { return d == DirectionOut || d == DirectionIn }

type DODirection string

const (
	DODirectionOutbound DODirection = "OUTBOUND"
	DODirectionInbound  DODirection = "INBOUND"
)

type DOStatus string

const (
	DOStatusOpen       DOStatus = "OPEN"
	DOStatusReserved   DOStatus = "RESERVED"
	DOStatusLPBCreated DOStatus = "LPB_CREATED"
	DOStatusCompleted  DOStatus = "COMPLETED"
	DOStatusCancelled  DOStatus = "CANCELLED"
)

// SalesOrder adalah header pesanan penjualan / penugasan operasional (4 tipe).
type SalesOrder struct {
	ID              int64
	SONumber        string
	OrderType       OrderType
	CustomerID      int64
	CustomerCode    string // dari JOIN
	CustomerName    string // dari JOIN
	CustomerSiteID  int64
	SiteName        string // dari JOIN
	SiteCity        string // dari JOIN
	ContractID      *int64
	ContractNumber  *string // dari JOIN
	Status          SOStatus
	OrderDate       time.Time
	RequestedDate   *time.Time
	IsPPN           bool
	Subtotal        decimal.Decimal
	DiscountAmount  decimal.Decimal
	DPP             decimal.Decimal
	DPPOtherValue   decimal.Decimal
	TaxRate         decimal.Decimal
	TaxAmount       decimal.Decimal
	GrandTotal      decimal.Decimal
	RelatedTicketID *int64
	Notes           *string
	CreatedBy       int64
	CreatedByName   string // dari JOIN
	ConfirmedBy     *int64
	ConfirmedByName *string // dari JOIN
	ConfirmedAt     *time.Time
	CancelledBy     *int64
	CancelledByName *string // dari JOIN
	CancelledAt     *time.Time
	CancelReason    *string
	CreatedAt       time.Time
	UpdatedAt       time.Time

	ItemCount int
	TotalQty  int
	DOCount   int
}

type SalesOrderItem struct {
	ID              int64
	SOID            int64
	ProductID       int64
	ProductSKU      string // dari JOIN
	ProductName     string // dari JOIN
	Direction       ItemDirection
	Qty             int
	UnitPrice       decimal.Decimal
	LineTotal       decimal.Decimal
	ProductSerialID *int64
	SerialNumber    *string // dari JOIN
	AssetTag        *string // dari JOIN
	Notes           *string
}

type DeliveryOrderSummary struct {
	ID            int64
	DONumber      string
	SOID          int64
	WarehouseID   int64
	WarehouseCode string
	WarehouseName string
	Direction     DODirection
	Status        DOStatus
	ScheduledDate *time.Time
	LPBID         *int64
	Notes         *string
	ItemCount     int
	TotalQty      int
	CreatedAt     time.Time
}

type SalesOrderDetail struct {
	SO             *SalesOrder
	Items          []SalesOrderItem
	DeliveryOrders []DeliveryOrderSummary
}

type SalesOrderFilter struct {
	Query      string
	OrderType  *OrderType
	Status     *SOStatus
	CustomerID *int64
	ContractID *int64
}

type SalesOrderRepository interface {
	FindByID(ctx context.Context, id int64) (*SalesOrder, error)
	FindByIDForUpdate(ctx context.Context, id int64) (*SalesOrder, error)
	List(ctx context.Context, f SalesOrderFilter, p PageParams) ([]SalesOrder, int64, error)
	Create(ctx context.Context, so *SalesOrder) (*SalesOrder, error)
	Update(ctx context.Context, so *SalesOrder) error
	UpdateStatus(ctx context.Context, so *SalesOrder) error

	ListItems(ctx context.Context, soID int64) ([]SalesOrderItem, error)
	ReplaceItems(ctx context.Context, soID int64, items []SalesOrderItem) ([]SalesOrderItem, error)

	ListDeliveryOrdersBySO(ctx context.Context, soID int64) ([]DeliveryOrderSummary, error)
	HasActiveDeliveryOrders(ctx context.Context, soID int64) (bool, error)
	CreateDeliveryOrder(ctx context.Context, do *DeliveryOrderSummary, items []SalesOrderItem, createdBy int64) (*DeliveryOrderSummary, error)
}
