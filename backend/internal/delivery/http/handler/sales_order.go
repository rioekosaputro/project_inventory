package handler

import (
	"github.com/gin-gonic/gin"

	"rental-inventory-api/internal/delivery/http/dto"
	"rental-inventory-api/internal/delivery/http/httperr"
	"rental-inventory-api/internal/delivery/http/middleware"
	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/usecase"
	"rental-inventory-api/pkg/pagination"
	"rental-inventory-api/pkg/response"
)

var salesOrderSort = pagination.SortSpec{
	Allowed: map[string]string{
		"so_number":   "so.so_number",
		"order_date":  "so.order_date",
		"grand_total": "so.grand_total",
		"created_at":  "so.created_at",
	},
	Default: "-created_at",
}

// SalesOrderHandler melayani /sales-orders (T2.5).
type SalesOrderHandler struct{ uc *usecase.SalesOrderUsecase }

func NewSalesOrderHandler(uc *usecase.SalesOrderUsecase) *SalesOrderHandler {
	return &SalesOrderHandler{uc: uc}
}

// List — GET /sales-orders
func (h *SalesOrderHandler) List(c *gin.Context) {
	page, pp, ok := pageOf(c, salesOrderSort)
	if !ok {
		return
	}
	var q dto.ListSalesOrdersQuery
	if !httperr.BindQuery(c, &q) {
		return
	}
	var ot *domain.OrderType
	if q.OrderType != "" {
		v := domain.OrderType(q.OrderType)
		ot = &v
	}
	var st *domain.SOStatus
	if q.Status != "" {
		v := domain.SOStatus(q.Status)
		st = &v
	}
	items, total, err := h.uc.List(c.Request.Context(), domain.SalesOrderFilter{
		Query: q.Q, OrderType: ot, Status: st, CustomerID: q.CustomerID, ContractID: q.ContractID,
	}, pp)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	out := make([]dto.SalesOrderResponse, 0, len(items))
	for i := range items {
		out = append(out, dto.NewSalesOrderResponse(&items[i]))
	}
	response.Paginated(c, out, pagination.NewMeta(page, total))
}

// Get — GET /sales-orders/:id
func (h *SalesOrderHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	detail, err := h.uc.Get(c.Request.Context(), id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewSalesOrderDetailResponse(detail))
}

// Create — POST /sales-orders
func (h *SalesOrderHandler) Create(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	var req dto.CreateSORequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	detail, err := h.uc.Create(c.Request.Context(), actor, usecase.CreateSOInput{
		OrderType:      req.OrderType,
		CustomerID:     req.CustomerID,
		CustomerSiteID: req.CustomerSiteID,
		ContractID:     req.ContractID,
		OrderDate:      req.OrderDate,
		RequestedDate:  req.RequestedDate,
		IsPPN:          req.IsPPN,
		DiscountAmount: req.DiscountAmount,
		Notes:          req.Notes,
		Items:          mapSOItems(req.Items),
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.Created(c, dto.NewSalesOrderDetailResponse(detail))
}

// Update — PATCH /sales-orders/:id
func (h *SalesOrderHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.UpdateSORequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	var itemsPtr *[]usecase.SOItemInput
	if req.Items != nil {
		mapped := mapSOItems(*req.Items)
		itemsPtr = &mapped
	}
	detail, err := h.uc.Update(c.Request.Context(), id, usecase.UpdateSOInput{
		CustomerSiteID: req.CustomerSiteID,
		OrderDate:      req.OrderDate,
		RequestedDate:  patch(req.RequestedDate),
		IsPPN:          req.IsPPN,
		DiscountAmount: req.DiscountAmount,
		Notes:          patch(req.Notes),
		Items:          itemsPtr,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewSalesOrderDetailResponse(detail))
}

// Confirm — POST /sales-orders/:id/confirm
func (h *SalesOrderHandler) Confirm(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	detail, err := h.uc.Confirm(c.Request.Context(), actor, id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewSalesOrderDetailResponse(detail))
}

// Cancel — POST /sales-orders/:id/cancel
func (h *SalesOrderHandler) Cancel(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.CancelSORequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	detail, err := h.uc.Cancel(c.Request.Context(), actor, id, usecase.CancelSOInput{
		CancelReason: req.CancelReason,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewSalesOrderDetailResponse(detail))
}

// IssueDeliveryOrders — POST /sales-orders/:id/delivery-orders
func (h *SalesOrderHandler) IssueDeliveryOrders(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.IssueDORequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	dos, err := h.uc.IssueDeliveryOrders(c.Request.Context(), actor, id, usecase.IssueDOInput{
		WarehouseID:   req.WarehouseID,
		ScheduledDate: req.ScheduledDate,
		Notes:         req.Notes,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	out := make([]dto.DeliveryOrderSummaryResponse, 0, len(dos))
	for i := range dos {
		out = append(out, dto.NewDeliveryOrderSummaryResponse(&dos[i]))
	}
	response.Created(c, out)
}

func mapSOItems(in []dto.SOItemRequest) []usecase.SOItemInput {
	out := make([]usecase.SOItemInput, 0, len(in))
	for _, it := range in {
		out = append(out, usecase.SOItemInput{
			ProductID:       it.ProductID,
			Direction:       it.Direction,
			Qty:             it.Qty,
			UnitPrice:       it.UnitPrice,
			ProductSerialID: it.ProductSerialID,
			Notes:           it.Notes,
		})
	}
	return out
}
