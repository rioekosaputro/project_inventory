package dto

import (
	"time"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/usecase"
)

// ---------------------------------------------------------------- query

type ListBalancesQuery struct {
	Q           string `form:"q" binding:"omitempty,max=100"`
	ProductID   *int64 `form:"product_id" binding:"omitempty,gte=1"`
	RackID      *int64 `form:"rack_id" binding:"omitempty,gte=1"`
	WarehouseID *int64 `form:"warehouse_id" binding:"omitempty,gte=1"`
	LowStock    bool   `form:"low_stock"`
	IncludeZero bool   `form:"include_zero"`
}

type ListStockSummaryQuery struct {
	Q           string `form:"q" binding:"omitempty,max=100"`
	CategoryID  *int64 `form:"category_id" binding:"omitempty,gte=1"`
	ItemType    string `form:"item_type" binding:"omitempty,oneof=ASSET CONSUMABLE"`
	WarehouseID *int64 `form:"warehouse_id" binding:"omitempty,gte=1"`
	LowStock    bool   `form:"low_stock"`
	HasStock    bool   `form:"has_stock"`
	IsActive    *bool  `form:"is_active"`
}

type ListSerialsQuery struct {
	Q           string `form:"q" binding:"omitempty,max=100"`
	ProductID   *int64 `form:"product_id" binding:"omitempty,gte=1"`
	Status      string `form:"status" binding:"omitempty,max=20"`
	CustomerID  *int64 `form:"customer_id" binding:"omitempty,gte=1"`
	WarehouseID *int64 `form:"warehouse_id" binding:"omitempty,gte=1"`
	RackID      *int64 `form:"rack_id" binding:"omitempty,gte=1"`
}

// ---------------------------------------------------------------- response

type BalanceResponse struct {
	ProductID       int64     `json:"product_id"`
	SKU             string    `json:"sku"`
	ProductName     string    `json:"product_name"`
	Unit            string    `json:"unit"`
	HasSerialNumber bool      `json:"has_serial_number"`
	MinStockAlert   int       `json:"min_stock_alert"`
	RackID          int64     `json:"rack_id"`
	RackCode        string    `json:"rack_code"`
	Zone            *string   `json:"zone"`
	IsSellable      bool      `json:"is_sellable"`
	WarehouseID     int64     `json:"warehouse_id"`
	WarehouseCode   string    `json:"warehouse_code"`
	QtyOnHand       int       `json:"qty_on_hand"`
	QtyReserved     int       `json:"qty_reserved"`
	QtyAvailable    int       `json:"qty_available"`
	IsLowStock      bool      `json:"is_low_stock"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func NewBalanceResponse(s *domain.StockBalance) BalanceResponse {
	return BalanceResponse{
		ProductID: s.ProductID, SKU: s.SKU, ProductName: s.ProductName, Unit: s.Unit,
		HasSerialNumber: s.HasSerialNumber, MinStockAlert: s.MinStockAlert,
		RackID: s.RackID, RackCode: s.RackCode, Zone: s.Zone, IsSellable: s.IsSellable,
		WarehouseID: s.WarehouseID, WarehouseCode: s.WarehouseCode,
		QtyOnHand: s.QtyOnHand, QtyReserved: s.QtyReserved, QtyAvailable: s.QtyAvailable,
		IsLowStock: s.IsLowStock, UpdatedAt: s.UpdatedAt,
	}
}

type ProductStockResponse struct {
	ProductID       int64  `json:"product_id"`
	SKU             string `json:"sku"`
	ProductName     string `json:"product_name"`
	Unit            string `json:"unit"`
	CategoryID      int64  `json:"category_id"`
	CategoryName    string `json:"category_name"`
	ItemType        string `json:"item_type"`
	HasSerialNumber bool   `json:"has_serial_number"`
	MinStockAlert   int    `json:"min_stock_alert"`
	IsActive        bool   `json:"is_active"`
	QtyOnHand       int    `json:"qty_on_hand"`
	QtyReserved     int    `json:"qty_reserved"`
	QtyAvailable    int    `json:"qty_available"`
	RackCount       int    `json:"rack_count"`
	IsLowStock      bool   `json:"is_low_stock"`
}

func NewProductStockResponse(s *domain.ProductStock) ProductStockResponse {
	return ProductStockResponse{
		ProductID: s.ProductID, SKU: s.SKU, ProductName: s.ProductName, Unit: s.Unit,
		CategoryID: s.CategoryID, CategoryName: s.CategoryName, ItemType: string(s.ItemType),
		HasSerialNumber: s.HasSerialNumber, MinStockAlert: s.MinStockAlert, IsActive: s.IsActive,
		QtyOnHand: s.QtyOnHand, QtyReserved: s.QtyReserved, QtyAvailable: s.QtyAvailable,
		RackCount: s.RackCount, IsLowStock: s.IsLowStock,
	}
}

type SerialResponse struct {
	ID              int64     `json:"id"`
	ProductID       int64     `json:"product_id"`
	SKU             string    `json:"sku"`
	ProductName     string    `json:"product_name"`
	AssetTag        string    `json:"asset_tag"`
	SerialNumber    string    `json:"serial_number"`
	Status          string    `json:"status"`
	Condition       string    `json:"condition"`
	RackID          *int64    `json:"rack_id"`
	RackCode        *string   `json:"rack_code"`
	WarehouseID     *int64    `json:"warehouse_id"`
	WarehouseCode   *string   `json:"warehouse_code"`
	CustomerID      *int64    `json:"customer_id"`
	SiteID          *int64    `json:"site_id"`
	ContractID      *int64    `json:"contract_id"`
	ReservedForDOID *int64    `json:"reserved_for_do_id"`
	WarrantyUntil   *string   `json:"warranty_until"`
	Notes           *string   `json:"notes"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func NewSerialResponse(s *domain.Serial) SerialResponse {
	return SerialResponse{
		ID: s.ID, ProductID: s.ProductID, SKU: s.SKU, ProductName: s.ProductName,
		AssetTag: s.AssetTag, SerialNumber: s.SerialNumber, Status: string(s.Status), Condition: s.Condition,
		RackID: s.RackID, RackCode: s.RackCode, WarehouseID: s.WarehouseID, WarehouseCode: s.WarehouseCode,
		CustomerID: s.CustomerID, SiteID: s.SiteID, ContractID: s.ContractID, ReservedForDOID: s.ReservedForDOID,
		WarrantyUntil: s.WarrantyUntil, Notes: s.Notes, UpdatedAt: s.UpdatedAt,
	}
}

type SerialHistoryResponse struct {
	ID            int64     `json:"id"`
	FromStatus    *string   `json:"from_status"`
	ToStatus      string    `json:"to_status"`
	FromRackCode  *string   `json:"from_rack_code"`
	ToRackCode    *string   `json:"to_rack_code"`
	CustomerID    *int64    `json:"customer_id"`
	EventType     string    `json:"event_type"`
	ReferenceType *string   `json:"reference_type"`
	ReferenceID   *int64    `json:"reference_id"`
	UserID        *int64    `json:"user_id"`
	UserName      *string   `json:"user_name"`
	Notes         *string   `json:"notes"`
	CreatedAt     time.Time `json:"created_at"`
}

// SerialScanResponse: unit + 5 histori terakhir (terbaru dulu).
type SerialScanResponse struct {
	SerialResponse
	History []SerialHistoryResponse `json:"history"`
}

func NewSerialScanResponse(s *usecase.SerialScan) SerialScanResponse {
	hist := make([]SerialHistoryResponse, 0, len(s.History))
	for _, h := range s.History {
		hist = append(hist, SerialHistoryResponse{
			ID: h.ID, FromStatus: h.FromStatus, ToStatus: h.ToStatus,
			FromRackCode: h.FromRackCode, ToRackCode: h.ToRackCode, CustomerID: h.CustomerID,
			EventType: h.EventType, ReferenceType: h.ReferenceType, ReferenceID: h.ReferenceID,
			UserID: h.UserID, UserName: h.UserName, Notes: h.Notes, CreatedAt: h.CreatedAt,
		})
	}
	return SerialScanResponse{SerialResponse: NewSerialResponse(&s.Serial), History: hist}
}

// ---------------------------------------------------------------- ledger & stock card (T1.13)

type ListLedgersQuery struct {
	Q            string `form:"q" binding:"omitempty,max=100"`
	ProductID    *int64 `form:"product_id" binding:"omitempty,gte=1"`
	RackID       *int64 `form:"rack_id" binding:"omitempty,gte=1"`
	WarehouseID  *int64 `form:"warehouse_id" binding:"omitempty,gte=1"`
	Direction    string `form:"direction" binding:"omitempty,oneof=IN OUT"`
	MovementType string `form:"movement_type" binding:"omitempty,max=30"`
	DateFrom     string `form:"date_from" binding:"omitempty,len=10"`
	DateTo       string `form:"date_to" binding:"omitempty,len=10"`
}

type StockCardQuery struct {
	ProductID   int64  `form:"product_id" binding:"required,gte=1"`
	RackID      *int64 `form:"rack_id" binding:"omitempty,gte=1"`
	WarehouseID *int64 `form:"warehouse_id" binding:"omitempty,gte=1"`
	DateFrom    string `form:"date_from" binding:"omitempty,len=10"`
	DateTo      string `form:"date_to" binding:"omitempty,len=10"`
	Format      string `form:"format" binding:"omitempty,oneof=json xlsx"`
}

type LedgerRowResponse struct {
	ID            int64     `json:"id"`
	OccurredAt    time.Time `json:"occurred_at"`
	ProductID     int64     `json:"product_id"`
	SKU           string    `json:"sku"`
	ProductName   string    `json:"product_name"`
	Unit          string    `json:"unit"`
	WarehouseID   int64     `json:"warehouse_id"`
	WarehouseCode string    `json:"warehouse_code"`
	RackID        int64     `json:"rack_id"`
	RackCode      string    `json:"rack_code"`
	Direction     string    `json:"direction"`
	Qty           int       `json:"qty"`
	BalanceBefore int       `json:"balance_before"`
	BalanceAfter  int       `json:"balance_after"`
	MovementType  string    `json:"movement_type"`
	LPBID         int64     `json:"lpb_id"`
	LPBNumber     string    `json:"lpb_number"`
	SerialCount   int       `json:"serial_count"`
	CreatedBy     int64     `json:"created_by"`
	CreatedByName string    `json:"created_by_name"`
	Notes         *string   `json:"notes"`
}

func NewLedgerRowResponse(r *domain.LedgerRow) LedgerRowResponse {
	return LedgerRowResponse{
		ID: r.ID, OccurredAt: r.OccurredAt, ProductID: r.ProductID, SKU: r.SKU, ProductName: r.ProductName,
		Unit: r.Unit, WarehouseID: r.WarehouseID, WarehouseCode: r.WarehouseCode, RackID: r.RackID, RackCode: r.RackCode,
		Direction: r.Direction, Qty: r.Qty, BalanceBefore: r.BalanceBefore, BalanceAfter: r.BalanceAfter,
		MovementType: string(r.MovementType), LPBID: r.LPBID, LPBNumber: r.LPBNumber, SerialCount: r.SerialCount,
		CreatedBy: r.CreatedBy, CreatedByName: r.CreatedByName, Notes: r.Notes,
	}
}

type StockCardResponse struct {
	ProductID      int64               `json:"product_id"`
	SKU            string              `json:"sku"`
	ProductName    string              `json:"product_name"`
	Unit           string              `json:"unit"`
	RackID         *int64              `json:"rack_id"`
	RackCode       *string             `json:"rack_code"`
	WarehouseID    *int64              `json:"warehouse_id"`
	WarehouseCode  *string             `json:"warehouse_code"`
	DateFrom       *string             `json:"date_from"`
	DateTo         *string             `json:"date_to"`
	OpeningBalance int                 `json:"opening_balance"`
	TotalIn        int                 `json:"total_in"`
	TotalOut       int                 `json:"total_out"`
	ClosingBalance int                 `json:"closing_balance"`
	Movements      []LedgerRowResponse `json:"movements"`
}

func NewStockCardResponse(c *usecase.StockCard) StockCardResponse {
	moves := make([]LedgerRowResponse, 0, len(c.Movements))
	for i := range c.Movements {
		moves = append(moves, NewLedgerRowResponse(&c.Movements[i]))
	}
	return StockCardResponse{
		ProductID: c.ProductID, SKU: c.SKU, ProductName: c.ProductName, Unit: c.Unit,
		RackID: c.RackID, RackCode: c.RackCode, WarehouseID: c.WarehouseID, WarehouseCode: c.WarehouseCode,
		DateFrom: c.DateFrom, DateTo: c.DateTo, OpeningBalance: c.OpeningBalance,
		TotalIn: c.TotalIn, TotalOut: c.TotalOut, ClosingBalance: c.ClosingBalance, Movements: moves,
	}
}
