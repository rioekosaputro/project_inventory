package dto

import (
	"time"

	"rental-inventory-api/internal/domain"
)

type ListSalesOrdersQuery struct {
	Q          string `form:"q" binding:"omitempty,max=100"`
	OrderType  string `form:"order_type" binding:"omitempty,oneof=NEW_DELIVERY WITHDRAWAL UNIT_SWAP CONSUMABLE"`
	Status     string `form:"status" binding:"omitempty,oneof=DRAFT CONFIRMED IN_PROGRESS COMPLETED CANCELLED"`
	CustomerID *int64 `form:"customer_id" binding:"omitempty,gte=1"`
	ContractID *int64 `form:"contract_id" binding:"omitempty,gte=1"`
}

type SOItemRequest struct {
	ProductID       int64   `json:"product_id" binding:"required,gte=1"`
	Direction       *string `json:"direction" binding:"omitempty,oneof=OUT IN"`
	Qty             int     `json:"qty" binding:"required,gte=1"`
	UnitPrice       *string `json:"unit_price"`
	ProductSerialID *int64  `json:"product_serial_id" binding:"omitempty,gte=1"`
	Notes           *string `json:"notes" binding:"omitempty,max=1000"`
}

type CreateSORequest struct {
	OrderType      string          `json:"order_type" binding:"required,oneof=NEW_DELIVERY WITHDRAWAL UNIT_SWAP CONSUMABLE"`
	CustomerID     int64           `json:"customer_id" binding:"required,gte=1"`
	CustomerSiteID int64           `json:"customer_site_id" binding:"required,gte=1"`
	ContractID     *int64          `json:"contract_id" binding:"omitempty,gte=1"`
	OrderDate      *string         `json:"order_date"`
	RequestedDate  *string         `json:"requested_date"`
	IsPPN          *bool           `json:"is_ppn"`
	DiscountAmount *string         `json:"discount_amount"`
	Notes          *string         `json:"notes" binding:"omitempty,max=1000"`
	Items          []SOItemRequest `json:"items" binding:"required,min=1,dive"`
}

type UpdateSORequest struct {
	CustomerSiteID *int64           `json:"customer_site_id" binding:"omitempty,gte=1"`
	OrderDate      *string          `json:"order_date"`
	RequestedDate  Optional[string] `json:"requested_date"`
	IsPPN          *bool            `json:"is_ppn"`
	DiscountAmount *string          `json:"discount_amount"`
	Notes          Optional[string] `json:"notes"`
	Items          *[]SOItemRequest `json:"items" binding:"omitempty,min=1,dive"`
}

type CancelSORequest struct {
	CancelReason string `json:"cancel_reason" binding:"required,min=5,max=500"`
}

type IssueDORequest struct {
	WarehouseID   int64   `json:"warehouse_id" binding:"required,gte=1"`
	ScheduledDate *string `json:"scheduled_date"`
	Notes         *string `json:"notes" binding:"omitempty,max=1000"`
}

type SOItemResponse struct {
	ID              int64   `json:"id"`
	SOID            int64   `json:"so_id"`
	ProductID       int64   `json:"product_id"`
	ProductSKU      string  `json:"product_sku"`
	ProductName     string  `json:"product_name"`
	Direction       string  `json:"direction"`
	Qty             int     `json:"qty"`
	UnitPrice       string  `json:"unit_price"`
	LineTotal       string  `json:"line_total"`
	ProductSerialID *int64  `json:"product_serial_id"`
	SerialNumber    *string `json:"serial_number"`
	AssetTag        *string `json:"asset_tag"`
	Notes           *string `json:"notes"`
}

type DeliveryOrderSummaryResponse struct {
	ID            int64     `json:"id"`
	DONumber      string    `json:"do_number"`
	SOID          int64     `json:"so_id"`
	WarehouseID   int64     `json:"warehouse_id"`
	WarehouseCode string    `json:"warehouse_code"`
	WarehouseName string    `json:"warehouse_name"`
	Direction     string    `json:"direction"`
	Status        string    `json:"status"`
	ScheduledDate *string   `json:"scheduled_date"`
	LPBID         *int64    `json:"lpb_id"`
	Notes         *string   `json:"notes"`
	ItemCount     int       `json:"item_count"`
	TotalQty      int       `json:"total_qty"`
	CreatedAt     time.Time `json:"created_at"`
}

type SalesOrderResponse struct {
	ID              int64      `json:"id"`
	SONumber        string     `json:"so_number"`
	OrderType       string     `json:"order_type"`
	CustomerID      int64      `json:"customer_id"`
	CustomerCode    string     `json:"customer_code"`
	CustomerName    string     `json:"customer_name"`
	CustomerSiteID  int64      `json:"customer_site_id"`
	SiteName        string     `json:"site_name"`
	SiteCity        string     `json:"site_city"`
	ContractID      *int64     `json:"contract_id"`
	ContractNumber  *string    `json:"contract_number"`
	Status          string     `json:"status"`
	OrderDate       string     `json:"order_date"`
	RequestedDate   *string    `json:"requested_date"`
	IsPPN           bool       `json:"is_ppn"`
	Subtotal        string     `json:"subtotal"`
	DiscountAmount  string     `json:"discount_amount"`
	DPP             string     `json:"dpp"`
	DPPOtherValue   string     `json:"dpp_other_value"`
	TaxRate         string     `json:"tax_rate"`
	TaxAmount       string     `json:"tax_amount"`
	GrandTotal      string     `json:"grand_total"`
	Notes           *string    `json:"notes"`
	CreatedBy       int64      `json:"created_by"`
	CreatedByName   string     `json:"created_by_name"`
	ConfirmedBy     *int64     `json:"confirmed_by"`
	ConfirmedByName *string    `json:"confirmed_by_name"`
	ConfirmedAt     *time.Time `json:"confirmed_at"`
	CancelledBy     *int64     `json:"cancelled_by"`
	CancelledByName *string    `json:"cancelled_by_name"`
	CancelledAt     *time.Time `json:"cancelled_at"`
	CancelReason    *string    `json:"cancel_reason"`
	ItemCount       int        `json:"item_count"`
	TotalQty        int        `json:"total_qty"`
	DOCount         int        `json:"do_count"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type SalesOrderDetailResponse struct {
	SalesOrderResponse
	Items          []SOItemResponse               `json:"items"`
	DeliveryOrders []DeliveryOrderSummaryResponse `json:"delivery_orders"`
}

func NewSalesOrderResponse(so *domain.SalesOrder) SalesOrderResponse {
	var reqDate *string
	if so.RequestedDate != nil {
		s := so.RequestedDate.Format("2006-01-02")
		reqDate = &s
	}
	return SalesOrderResponse{
		ID:              so.ID,
		SONumber:        so.SONumber,
		OrderType:       string(so.OrderType),
		CustomerID:      so.CustomerID,
		CustomerCode:    so.CustomerCode,
		CustomerName:    so.CustomerName,
		CustomerSiteID:  so.CustomerSiteID,
		SiteName:        so.SiteName,
		SiteCity:        so.SiteCity,
		ContractID:      so.ContractID,
		ContractNumber:  so.ContractNumber,
		Status:          string(so.Status),
		OrderDate:       so.OrderDate.Format("2006-01-02"),
		RequestedDate:   reqDate,
		IsPPN:           so.IsPPN,
		Subtotal:        so.Subtotal.StringFixed(2),
		DiscountAmount:  so.DiscountAmount.StringFixed(2),
		DPP:             so.DPP.StringFixed(2),
		DPPOtherValue:   so.DPPOtherValue.StringFixed(2),
		TaxRate:         so.TaxRate.StringFixed(2),
		TaxAmount:       so.TaxAmount.StringFixed(2),
		GrandTotal:      so.GrandTotal.StringFixed(2),
		Notes:           so.Notes,
		CreatedBy:       so.CreatedBy,
		CreatedByName:   so.CreatedByName,
		ConfirmedBy:     so.ConfirmedBy,
		ConfirmedByName: so.ConfirmedByName,
		ConfirmedAt:     so.ConfirmedAt,
		CancelledBy:     so.CancelledBy,
		CancelledByName: so.CancelledByName,
		CancelledAt:     so.CancelledAt,
		CancelReason:    so.CancelReason,
		ItemCount:       so.ItemCount,
		TotalQty:        so.TotalQty,
		DOCount:         so.DOCount,
		CreatedAt:       so.CreatedAt,
		UpdatedAt:       so.UpdatedAt,
	}
}

func NewDeliveryOrderSummaryResponse(d *domain.DeliveryOrderSummary) DeliveryOrderSummaryResponse {
	var sched *string
	if d.ScheduledDate != nil {
		s := d.ScheduledDate.Format("2006-01-02")
		sched = &s
	}
	return DeliveryOrderSummaryResponse{
		ID:            d.ID,
		DONumber:      d.DONumber,
		SOID:          d.SOID,
		WarehouseID:   d.WarehouseID,
		WarehouseCode: d.WarehouseCode,
		WarehouseName: d.WarehouseName,
		Direction:     string(d.Direction),
		Status:        string(d.Status),
		ScheduledDate: sched,
		LPBID:         d.LPBID,
		Notes:         d.Notes,
		ItemCount:     d.ItemCount,
		TotalQty:      d.TotalQty,
		CreatedAt:     d.CreatedAt,
	}
}

func NewSalesOrderDetailResponse(d *domain.SalesOrderDetail) SalesOrderDetailResponse {
	items := make([]SOItemResponse, 0, len(d.Items))
	for i := range d.Items {
		it := &d.Items[i]
		items = append(items, SOItemResponse{
			ID:              it.ID,
			SOID:            it.SOID,
			ProductID:       it.ProductID,
			ProductSKU:      it.ProductSKU,
			ProductName:     it.ProductName,
			Direction:       string(it.Direction),
			Qty:             it.Qty,
			UnitPrice:       it.UnitPrice.StringFixed(2),
			LineTotal:       it.LineTotal.StringFixed(2),
			ProductSerialID: it.ProductSerialID,
			SerialNumber:    it.SerialNumber,
			AssetTag:        it.AssetTag,
			Notes:           it.Notes,
		})
	}
	dos := make([]DeliveryOrderSummaryResponse, 0, len(d.DeliveryOrders))
	for i := range d.DeliveryOrders {
		dos = append(dos, NewDeliveryOrderSummaryResponse(&d.DeliveryOrders[i]))
	}
	return SalesOrderDetailResponse{
		SalesOrderResponse: NewSalesOrderResponse(d.SO),
		Items:              items,
		DeliveryOrders:     dos,
	}
}
