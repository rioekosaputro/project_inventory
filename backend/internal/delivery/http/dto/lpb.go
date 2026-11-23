package dto

import (
	"encoding/json"
	"time"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/usecase"
)

// ---------------------------------------------------------------- request

type LPBItemRequest struct {
	ProductID  int64   `json:"product_id" binding:"required,gte=1"`
	RackID     int64   `json:"rack_id" binding:"required,gte=1"`
	DestRackID *int64  `json:"dest_rack_id" binding:"omitempty,gte=1"` // rak tujuan (hanya TRANSFER_OUT)
	Qty        int     `json:"qty" binding:"required,gte=1,lte=1000000"`
	UnitCost   *string `json:"unit_cost"` // teks desimal, mis. "1500000.00"
	Notes      *string `json:"notes" binding:"omitempty,max=500"`
}

func (r LPBItemRequest) ToInput() usecase.LPBItemInput {
	return usecase.LPBItemInput{ProductID: r.ProductID, RackID: r.RackID, DestRackID: r.DestRackID, Qty: r.Qty, UnitCost: r.UnitCost, Notes: r.Notes}
}

func ToItemInputs(in []LPBItemRequest) []usecase.LPBItemInput {
	out := make([]usecase.LPBItemInput, 0, len(in))
	for _, it := range in {
		out = append(out, it.ToInput())
	}
	return out
}

type CreateLPBRequest struct {
	SubType         string           `json:"sub_type" binding:"required,max=30"`
	WarehouseID     int64            `json:"warehouse_id" binding:"required,gte=1"`
	DocDate         *string          `json:"doc_date" binding:"omitempty,datetime=2006-01-02"`
	SupplierID      *int64           `json:"supplier_id" binding:"omitempty,gte=1"`
	ReferenceNumber *string          `json:"reference_number" binding:"omitempty,max=60"`
	Notes           *string          `json:"notes" binding:"omitempty,max=1000"`
	Items           []LPBItemRequest `json:"items" binding:"omitempty,max=200,dive"`
}

// UpdateLPBRequest: PATCH parsial. Jenis dan gudang tidak dapat diubah. `items` (bila ada) menggantikan
// seluruh daftar item.
type UpdateLPBRequest struct {
	DocDate         *string           `json:"doc_date" binding:"omitempty,datetime=2006-01-02"`
	SupplierID      Optional[int64]   `json:"supplier_id"`
	ReferenceNumber Optional[string]  `json:"reference_number"`
	Notes           Optional[string]  `json:"notes"`
	Items           *[]LPBItemRequest `json:"items" binding:"omitempty,max=200,dive"`
}

type AddSerialRequest struct {
	SerialNumber string `json:"serial_number" binding:"required,max=100"`
	Condition    string `json:"condition" binding:"omitempty,oneof=GOOD MINOR_DEFECT BROKEN"`
}

type ListLPBQuery struct {
	Q           string `form:"q" binding:"omitempty,max=100"`
	Type        string `form:"type" binding:"omitempty,oneof=INBOUND OUTBOUND"`
	SubType     string `form:"sub_type" binding:"omitempty,max=30"`
	Status      string `form:"status" binding:"omitempty,max=20"`
	WarehouseID *int64 `form:"warehouse_id" binding:"omitempty,gte=1"`
	DateFrom    string `form:"date_from" binding:"omitempty,datetime=2006-01-02"`
	DateTo      string `form:"date_to" binding:"omitempty,datetime=2006-01-02"`
	Mine        bool   `form:"mine"`
}

// ---------------------------------------------------------------- response

type LPBResponse struct {
	ID                int64      `json:"id"`
	LPBNumber         string     `json:"lpb_number"`
	Type              string     `json:"type"`
	SubType           string     `json:"sub_type"`
	Status            string     `json:"status"`
	WarehouseID       int64      `json:"warehouse_id"`
	WarehouseCode     string     `json:"warehouse_code"`
	DocDate           string     `json:"doc_date"`
	ReferenceType     *string    `json:"reference_type"`
	ReferenceNumber   *string    `json:"reference_number"`
	SupplierID        *int64     `json:"supplier_id"`
	SupplierName      *string    `json:"supplier_name"`
	Notes             *string    `json:"notes"`
	CounterpartID     *int64     `json:"counterpart_id"`
	CounterpartNumber *string    `json:"counterpart_number"`
	CreatedBy         int64      `json:"created_by"`
	CreatedByName     string     `json:"created_by_name"`
	SubmittedBy       *int64     `json:"submitted_by"`
	SubmittedAt       *time.Time `json:"submitted_at"`
	ApprovedBy        *int64     `json:"approved_by"`
	ApprovedAt        *time.Time `json:"approved_at"`
	RejectedBy        *int64     `json:"rejected_by"`
	RejectedAt        *time.Time `json:"rejected_at"`
	RejectionReason   *string    `json:"rejection_reason"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func NewLPBResponse(l *domain.LPB) LPBResponse {
	return LPBResponse{
		ID: l.ID, LPBNumber: l.Number, Type: string(l.Type), SubType: string(l.SubType), Status: string(l.Status),
		WarehouseID: l.WarehouseID, WarehouseCode: l.WarehouseCode, DocDate: l.DocDate,
		ReferenceType: l.ReferenceType, ReferenceNumber: l.ReferenceNumber,
		SupplierID: l.SupplierID, SupplierName: l.SupplierName, Notes: l.Notes,
		CounterpartID: l.CounterpartID, CounterpartNumber: l.CounterpartNumber,
		CreatedBy: l.CreatedBy, CreatedByName: l.CreatedByName,
		SubmittedBy: l.SubmittedBy, SubmittedAt: l.SubmittedAt, ApprovedBy: l.ApprovedBy, ApprovedAt: l.ApprovedAt,
		RejectedBy: l.RejectedBy, RejectedAt: l.RejectedAt, RejectionReason: l.RejectionReason,
		CreatedAt: l.CreatedAt, UpdatedAt: l.UpdatedAt,
	}
}

type LPBListResponse struct {
	LPBResponse
	ItemCount int `json:"item_count"`
	TotalQty  int `json:"total_qty"`
}

func NewLPBListResponse(l *domain.LPBListItem) LPBListResponse {
	return LPBListResponse{LPBResponse: NewLPBResponse(&l.LPB), ItemCount: l.ItemCount, TotalQty: l.TotalQty}
}

type LPBItemSerialResponse struct {
	ID              int64     `json:"id"`
	ProductSerialID *int64    `json:"product_serial_id"`
	SerialNumber    string    `json:"serial_number"`
	AssetTag        *string   `json:"asset_tag"`
	Condition       string    `json:"condition"`
	ScannedBy       *int64    `json:"scanned_by"`
	ScannedAt       time.Time `json:"scanned_at"`
}

func NewLPBItemSerialResponse(s *domain.LPBItemSerial) LPBItemSerialResponse {
	return LPBItemSerialResponse{
		ID: s.ID, ProductSerialID: s.ProductSerialID, SerialNumber: s.SerialNumber, AssetTag: s.AssetTag,
		Condition: string(s.Condition), ScannedBy: s.ScannedBy, ScannedAt: s.ScannedAt,
	}
}

type LPBItemResponse struct {
	ID                int64                   `json:"id"`
	ProductID         int64                   `json:"product_id"`
	SKU               string                  `json:"sku"`
	ProductName       string                  `json:"product_name"`
	Unit              string                  `json:"unit"`
	HasSerialNumber   bool                    `json:"has_serial_number"`
	RackID            int64                   `json:"rack_id"`
	RackCode          string                  `json:"rack_code"`
	RackIsSellable    bool                    `json:"rack_is_sellable"`
	DestRackID        *int64                  `json:"dest_rack_id"`
	DestRackCode      *string                 `json:"dest_rack_code"`
	DestWarehouseID   *int64                  `json:"dest_warehouse_id"`
	DestWarehouseCode *string                 `json:"dest_warehouse_code"`
	Qty               int                     `json:"qty"`
	UnitCost          *string                 `json:"unit_cost"`
	Notes             *string                 `json:"notes"`
	ScannedCount      int                     `json:"scanned_count"`
	CurrentQty        int                     `json:"current_qty"`
	BalanceBefore     *int                    `json:"balance_before"`
	BalanceAfter      *int                    `json:"balance_after"`
	Serials           []LPBItemSerialResponse `json:"serials"`
}

type LPBEventResponse struct {
	Action    string          `json:"action"`
	UserID    *int64          `json:"user_id"`
	UserName  *string         `json:"user_name"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
}

type LPBDetailResponse struct {
	LPBResponse
	Items    []LPBItemResponse  `json:"items"`
	Timeline []LPBEventResponse `json:"timeline"`
}

func NewLPBDetailResponse(d *usecase.LPBDetail) LPBDetailResponse {
	items := make([]LPBItemResponse, 0, len(d.Items))
	for i := range d.Items {
		it := &d.Items[i]
		serials := make([]LPBItemSerialResponse, 0, len(it.Serials))
		for j := range it.Serials {
			serials = append(serials, NewLPBItemSerialResponse(&it.Serials[j]))
		}
		items = append(items, LPBItemResponse{
			ID: it.ID, ProductID: it.ProductID, SKU: it.SKU, ProductName: it.ProductName, Unit: it.Unit,
			HasSerialNumber: it.HasSerialNumber, RackID: it.RackID, RackCode: it.RackCode, RackIsSellable: it.RackIsSellable,
			DestRackID: it.DestRackID, DestRackCode: it.DestRackCode, DestWarehouseID: it.DestWarehouseID, DestWarehouseCode: it.DestWarehouseCode,
			Qty: it.Qty, UnitCost: money(it.UnitCost), Notes: it.Notes, ScannedCount: len(it.Serials), Serials: serials,
			CurrentQty: it.CurrentQty, BalanceBefore: it.BalanceBefore, BalanceAfter: it.BalanceAfter,
		})
	}
	events := make([]LPBEventResponse, 0, len(d.Timeline))
	for _, e := range d.Timeline {
		data := e.Data
		if len(data) == 0 {
			data = json.RawMessage("null")
		}
		events = append(events, LPBEventResponse{Action: e.Action, UserID: e.UserID, UserName: e.UserName, Data: data, CreatedAt: e.CreatedAt})
	}
	return LPBDetailResponse{LPBResponse: NewLPBResponse(&d.LPB), Items: items, Timeline: events}
}

// AddSerialResponse: SN yang tersimpan + progres scan item.
type AddSerialResponse struct {
	LPBItemSerialResponse
	Scanned int `json:"scanned_count"`
	Qty     int `json:"qty"`
}

type RejectLPBRequest struct {
	Reason string `json:"reason" binding:"required,min=3,max=500"`
}
