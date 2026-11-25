package dto

import (
	"time"

	"rental-inventory-api/internal/domain"
)

type ListDeliveryOrdersQuery struct {
	Q           string `form:"q" binding:"omitempty,max=100"`
	WarehouseID *int64 `form:"warehouse_id" binding:"omitempty,gte=1"`
	Direction   string `form:"direction" binding:"omitempty,oneof=OUTBOUND INBOUND"`
	Status      string `form:"status" binding:"omitempty,oneof=OPEN RESERVED LPB_CREATED COMPLETED CANCELLED"`
	SOID        *int64 `form:"so_id" binding:"omitempty,gte=1"`
}

type ReserveDOItemRequest struct {
	DOItemID  int64   `json:"do_item_id" binding:"required,gte=1"`
	RackID    int64   `json:"rack_id" binding:"required,gte=1"`
	Qty       int     `json:"qty" binding:"required,gte=1"`
	SerialIDs []int64 `json:"serial_ids"`
}

type ReserveDORequest struct {
	Items []ReserveDOItemRequest `json:"items" binding:"required,min=1,dive"`
}

type CreateLPBFromDORequest struct {
	RackID *int64  `json:"rack_id" binding:"omitempty,gte=1"`
	Notes  *string `json:"notes" binding:"omitempty,max=1000"`
}

type DOReservationResponse struct {
	ID              int64     `json:"id"`
	DOItemID        int64     `json:"do_item_id"`
	RackID          int64     `json:"rack_id"`
	RackCode        string    `json:"rack_code"`
	ProductSerialID *int64    `json:"product_serial_id"`
	SerialNumber    *string   `json:"serial_number"`
	AssetTag        *string   `json:"asset_tag"`
	Qty             int       `json:"qty"`
	ReservedAt      time.Time `json:"reserved_at"`
}

type DOItemResponse struct {
	ID              int64                   `json:"id"`
	DOID            int64                   `json:"do_id"`
	SOItemID        int64                   `json:"so_item_id"`
	ProductID       int64                   `json:"product_id"`
	ProductSKU      string                  `json:"product_sku"`
	ProductName     string                  `json:"product_name"`
	HasSerialNumber bool                    `json:"has_serial_number"`
	Qty             int                     `json:"qty"`
	ReservedQty     int                     `json:"reserved_qty"`
	RemainingQty    int                     `json:"remaining_qty"`
	SOProductSerial *int64                  `json:"so_product_serial_id"`
	SOSerialNumber  *string                 `json:"so_serial_number"`
	Reservations    []DOReservationResponse `json:"reservations"`
}

type DeliveryOrderResponse struct {
	ID             int64     `json:"id"`
	DONumber       string    `json:"do_number"`
	SOID           int64     `json:"so_id"`
	SONumber       string    `json:"so_number"`
	OrderType      string    `json:"order_type"`
	CustomerID     int64     `json:"customer_id"`
	CustomerCode   string    `json:"customer_code"`
	CustomerName   string    `json:"customer_name"`
	CustomerSiteID int64     `json:"customer_site_id"`
	SiteName       string    `json:"site_name"`
	SiteCity       string    `json:"site_city"`
	ContractID     *int64    `json:"contract_id"`
	WarehouseID    int64     `json:"warehouse_id"`
	WarehouseCode  string    `json:"warehouse_code"`
	WarehouseName  string    `json:"warehouse_name"`
	Direction      string    `json:"direction"`
	Status         string    `json:"status"`
	ScheduledDate  *string   `json:"scheduled_date"`
	LPBID          *int64    `json:"lpb_id"`
	LPBNumber      *string   `json:"lpb_number"`
	Notes          *string   `json:"notes"`
	CreatedByName  string    `json:"created_by_name"`
	ItemCount      int       `json:"item_count"`
	TotalQty       int       `json:"total_qty"`
	ReservedQty    int       `json:"reserved_qty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type DeliveryOrderDetailResponse struct {
	DeliveryOrderResponse
	Items []DOItemResponse `json:"items"`
}

func NewDeliveryOrderResponse(d *domain.DeliveryOrder) DeliveryOrderResponse {
	var sched *string
	if d.ScheduledDate != nil {
		s := d.ScheduledDate.Format("2006-01-02")
		sched = &s
	}
	return DeliveryOrderResponse{
		ID:             d.ID,
		DONumber:       d.DONumber,
		SOID:           d.SOID,
		SONumber:       d.SONumber,
		OrderType:      string(d.OrderType),
		CustomerID:     d.CustomerID,
		CustomerCode:   d.CustomerCode,
		CustomerName:   d.CustomerName,
		CustomerSiteID: d.CustomerSiteID,
		SiteName:       d.SiteName,
		SiteCity:       d.SiteCity,
		ContractID:     d.ContractID,
		WarehouseID:    d.WarehouseID,
		WarehouseCode:  d.WarehouseCode,
		WarehouseName:  d.WarehouseName,
		Direction:      string(d.Direction),
		Status:         string(d.Status),
		ScheduledDate:  sched,
		LPBID:          d.LPBID,
		LPBNumber:      d.LPBNumber,
		Notes:          d.Notes,
		CreatedByName:  d.CreatedByName,
		ItemCount:      d.ItemCount,
		TotalQty:       d.TotalQty,
		ReservedQty:    d.ReservedQty,
		CreatedAt:      d.CreatedAt,
		UpdatedAt:      d.UpdatedAt,
	}
}

func NewDeliveryOrderDetailResponse(det *domain.DeliveryOrderDetail) DeliveryOrderDetailResponse {
	items := make([]DOItemResponse, 0, len(det.Items))
	for i := range det.Items {
		it := &det.Items[i]
		resList := make([]DOReservationResponse, 0, len(it.Reservations))
		for j := range it.Reservations {
			rs := &it.Reservations[j]
			resList = append(resList, DOReservationResponse{
				ID:              rs.ID,
				DOItemID:        rs.DOItemID,
				RackID:          rs.RackID,
				RackCode:        rs.RackCode,
				ProductSerialID: rs.ProductSerialID,
				SerialNumber:    rs.SerialNumber,
				AssetTag:        rs.AssetTag,
				Qty:             rs.Qty,
				ReservedAt:      rs.ReservedAt,
			})
		}
		rem := it.Qty - it.ReservedQty
		if rem < 0 {
			rem = 0
		}
		items = append(items, DOItemResponse{
			ID:              it.ID,
			DOID:            it.DOID,
			SOItemID:        it.SOItemID,
			ProductID:       it.ProductID,
			ProductSKU:      it.ProductSKU,
			ProductName:     it.ProductName,
			HasSerialNumber: it.HasSerialNumber,
			Qty:             it.Qty,
			ReservedQty:     it.ReservedQty,
			RemainingQty:    rem,
			SOProductSerial: it.SOProductSerial,
			SOSerialNumber:  it.SOSerialNumber,
			Reservations:    resList,
		})
	}
	return DeliveryOrderDetailResponse{
		DeliveryOrderResponse: NewDeliveryOrderResponse(det.DO),
		Items:                 items,
	}
}
