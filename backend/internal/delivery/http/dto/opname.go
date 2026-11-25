package dto

import (
	"time"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/usecase"
)

type ListOpnamesQuery struct {
	Q           string `form:"q"`
	Status      string `form:"status"`
	WarehouseID *int64 `form:"warehouse_id"`
}

type StartOpnameRequest struct {
	WarehouseID  int64   `json:"warehouse_id" binding:"required,min=1"`
	RackIDs      []int64 `json:"rack_ids" binding:"required"`
	IsBlindCount *bool   `json:"is_blind_count"` // default true
	Notes        *string `json:"notes"`
}

type SaveOpnameCountsRequest struct {
	Items []OpnameCountItemRequest `json:"items" binding:"required"`
}

type OpnameCountItemRequest struct {
	ProductID  int64   `json:"product_id" binding:"required,min=1"`
	RackID     int64   `json:"rack_id" binding:"required,min=1"`
	CountedQty int     `json:"counted_qty" binding:"min=0"`
	Notes      *string `json:"notes"`
}

type ScanOpnameSerialRequest struct {
	RackID int64  `json:"rack_id" binding:"required,min=1"`
	Code   string `json:"code" binding:"required"`
}

type OpnameResponse struct {
	ID                     int64      `json:"id"`
	OpnameNumber           string     `json:"opname_number"`
	WarehouseID            int64      `json:"warehouse_id"`
	WarehouseCode          string     `json:"warehouse_code"`
	WarehouseName          string     `json:"warehouse_name"`
	Status                 string     `json:"status"`
	IsBlindCount           bool       `json:"is_blind_count"`
	StartedBy              int64      `json:"started_by"`
	StartedByName          string     `json:"started_by_name"`
	StartedAt              time.Time  `json:"started_at"`
	SubmittedBy            *int64     `json:"submitted_by"`
	SubmittedAt            *time.Time `json:"submitted_at"`
	FinalizedBy            *int64     `json:"finalized_by"`
	FinalizedAt            *time.Time `json:"finalized_at"`
	AdjustmentInLPBID      *int64     `json:"adjustment_in_lpb_id"`
	AdjustmentInLPBNumber  *string    `json:"adjustment_in_lpb_number"`
	AdjustmentOutLPBID     *int64     `json:"adjustment_out_lpb_id"`
	AdjustmentOutLPBNumber *string    `json:"adjustment_out_lpb_number"`
	Notes                  *string    `json:"notes"`
}

type OpnameListResponse struct {
	OpnameResponse
	RackCount    int `json:"rack_count"`
	ItemCount    int `json:"item_count"`
	CountedCount int `json:"counted_count"`
	SerialCount  int `json:"serial_count"`
	FoundCount   int `json:"found_count"`
}

type OpnameRackResponse struct {
	RackID     int64   `json:"rack_id"`
	RackCode   string  `json:"rack_code"`
	Zone       *string `json:"zone"`
	IsSellable bool    `json:"is_sellable"`
}

type OpnameItemResponse struct {
	ID          int64      `json:"id"`
	ProductID   int64      `json:"product_id"`
	SKU         string     `json:"sku"`
	ProductName string     `json:"product_name"`
	Unit        string     `json:"unit"`
	RackID      int64      `json:"rack_id"`
	RackCode    string     `json:"rack_code"`
	SystemQty   *int       `json:"system_qty"` // null saat blind count aktif utk staf
	CountedQty  *int       `json:"counted_qty"`
	DiffQty     *int       `json:"diff_qty"` // null saat blind count aktif utk staf
	CountedBy   *int64     `json:"counted_by"`
	CountedAt   *time.Time `json:"counted_at"`
	Notes       *string    `json:"notes"`
}

type OpnameSerialResponse struct {
	ID              int64      `json:"id"`
	RackID          int64      `json:"rack_id"`
	RackCode        string     `json:"rack_code"`
	ProductSerialID *int64     `json:"product_serial_id"`
	AssetTag        *string    `json:"asset_tag"`
	SerialNumber    *string    `json:"serial_number"`
	SKU             *string    `json:"sku"`
	ProductName     *string    `json:"product_name"`
	ScannedCode     *string    `json:"scanned_code"`
	Result          string     `json:"result"`
	ScannedBy       *int64     `json:"scanned_by"`
	ScannedAt       *time.Time `json:"scanned_at"`
}

type OpnameDetailResponse struct {
	OpnameResponse
	Racks   []OpnameRackResponse   `json:"racks"`
	Items   []OpnameItemResponse   `json:"items"`
	Serials []OpnameSerialResponse `json:"serials"`
}

func NewOpnameResponse(o *domain.Opname) OpnameResponse {
	return OpnameResponse{
		ID: o.ID, OpnameNumber: o.Number, WarehouseID: o.WarehouseID,
		WarehouseCode: o.WarehouseCode, WarehouseName: o.WarehouseName,
		Status: string(o.Status), IsBlindCount: o.IsBlindCount,
		StartedBy: o.StartedBy, StartedByName: o.StartedByName, StartedAt: o.StartedAt,
		SubmittedBy: o.SubmittedBy, SubmittedAt: o.SubmittedAt,
		FinalizedBy: o.FinalizedBy, FinalizedAt: o.FinalizedAt,
		AdjustmentInLPBID: o.AdjustmentInLPBID, AdjustmentInLPBNumber: o.AdjustmentInLPBNumber,
		AdjustmentOutLPBID: o.AdjustmentOutLPBID, AdjustmentOutLPBNumber: o.AdjustmentOutLPBNumber,
		Notes: o.Notes,
	}
}

func NewOpnameListResponse(it *domain.OpnameListItem) OpnameListResponse {
	return OpnameListResponse{
		OpnameResponse: NewOpnameResponse(&it.Opname),
		RackCount:      it.RackCount, ItemCount: it.ItemCount, CountedCount: it.CountedCount,
		SerialCount: it.SerialCount, FoundCount: it.FoundCount,
	}
}

func NewOpnameSerialResponse(s *domain.OpnameSerial) OpnameSerialResponse {
	return OpnameSerialResponse{
		ID: s.ID, RackID: s.RackID, RackCode: s.RackCode, ProductSerialID: s.ProductSerialID,
		AssetTag: s.AssetTag, SerialNumber: s.SerialNumber, SKU: s.SKU, ProductName: s.ProductName,
		ScannedCode: s.ScannedCode, Result: string(s.Result), ScannedBy: s.ScannedBy, ScannedAt: s.ScannedAt,
	}
}

func NewOpnameDetailResponse(d *usecase.OpnameDetail) OpnameDetailResponse {
	racks := make([]OpnameRackResponse, 0, len(d.Racks))
	for _, r := range d.Racks {
		racks = append(racks, OpnameRackResponse{RackID: r.RackID, RackCode: r.RackCode, Zone: r.Zone, IsSellable: r.IsSellable})
	}
	items := make([]OpnameItemResponse, 0, len(d.Items))
	for i := range d.Items {
		it := &d.Items[i]
		var sysQty, diffQty *int
		if !d.HideSystemQty {
			v := it.SystemQty
			sysQty = &v
			diffQty = it.DiffQty
		}
		items = append(items, OpnameItemResponse{
			ID: it.ID, ProductID: it.ProductID, SKU: it.SKU, ProductName: it.ProductName, Unit: it.Unit,
			RackID: it.RackID, RackCode: it.RackCode, SystemQty: sysQty, CountedQty: it.CountedQty, DiffQty: diffQty,
			CountedBy: it.CountedBy, CountedAt: it.CountedAt, Notes: it.Notes,
		})
	}
	serials := make([]OpnameSerialResponse, 0, len(d.Serials))
	for i := range d.Serials {
		serials = append(serials, NewOpnameSerialResponse(&d.Serials[i]))
	}
	return OpnameDetailResponse{
		OpnameResponse: NewOpnameResponse(&d.Opname),
		Racks:          racks, Items: items, Serials: serials,
	}
}
