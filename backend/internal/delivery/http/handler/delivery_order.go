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

var doSort = pagination.SortSpec{
	Allowed: map[string]string{
		"do_number":      "d.do_number",
		"scheduled_date": "d.scheduled_date",
		"created_at":     "d.created_at",
	},
	Default: "-created_at",
}

// DeliveryOrderHandler melayani /delivery-orders (T2.6).
type DeliveryOrderHandler struct{ uc *usecase.DeliveryOrderUsecase }

func NewDeliveryOrderHandler(uc *usecase.DeliveryOrderUsecase) *DeliveryOrderHandler {
	return &DeliveryOrderHandler{uc: uc}
}

// List — GET /delivery-orders
func (h *DeliveryOrderHandler) List(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	page, pp, ok := pageOf(c, doSort)
	if !ok {
		return
	}
	var q dto.ListDeliveryOrdersQuery
	if !httperr.BindQuery(c, &q) {
		return
	}
	var dir *domain.DODirection
	if q.Direction != "" {
		v := domain.DODirection(q.Direction)
		dir = &v
	}
	var st *domain.DOStatus
	if q.Status != "" {
		v := domain.DOStatus(q.Status)
		st = &v
	}
	items, total, err := h.uc.List(c.Request.Context(), actor, domain.DeliveryOrderFilter{
		Query: q.Q, WarehouseID: q.WarehouseID, Direction: dir, Status: st, SOID: q.SOID,
	}, pp)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	out := make([]dto.DeliveryOrderResponse, 0, len(items))
	for i := range items {
		out = append(out, dto.NewDeliveryOrderResponse(&items[i]))
	}
	response.Paginated(c, out, pagination.NewMeta(page, total))
}

// Get — GET /delivery-orders/:id
func (h *DeliveryOrderHandler) Get(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	detail, err := h.uc.Get(c.Request.Context(), actor, id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewDeliveryOrderDetailResponse(detail))
}

// Reserve — POST /delivery-orders/:id/reserve
func (h *DeliveryOrderHandler) Reserve(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.ReserveDORequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	mapped := make([]usecase.ReserveDOItemInput, 0, len(req.Items))
	for _, it := range req.Items {
		mapped = append(mapped, usecase.ReserveDOItemInput{
			DOItemID:  it.DOItemID,
			RackID:    it.RackID,
			Qty:       it.Qty,
			SerialIDs: it.SerialIDs,
		})
	}
	detail, err := h.uc.Reserve(c.Request.Context(), actor, id, usecase.ReserveDOInput{Items: mapped})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewDeliveryOrderDetailResponse(detail))
}

// Release — POST /delivery-orders/:id/release
func (h *DeliveryOrderHandler) Release(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	detail, err := h.uc.Release(c.Request.Context(), actor, id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewDeliveryOrderDetailResponse(detail))
}

// Cancel — POST /delivery-orders/:id/cancel
func (h *DeliveryOrderHandler) Cancel(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	detail, err := h.uc.Cancel(c.Request.Context(), actor, id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewDeliveryOrderDetailResponse(detail))
}

// CreateLPB — POST /delivery-orders/:id/lpb
func (h *DeliveryOrderHandler) CreateLPB(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.CreateLPBFromDORequest
	if c.Request.ContentLength > 0 {
		if !httperr.BindJSON(c, &req) {
			return
		}
	}
	lpbDoc, err := h.uc.CreateLPB(c.Request.Context(), actor, id, usecase.CreateLPBFromDOInput{
		RackID: req.RackID,
		Notes:  req.Notes,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.Created(c, gin.H{
		"id":         lpbDoc.ID,
		"lpb_number": lpbDoc.Number,
		"type":       string(lpbDoc.Type),
		"sub_type":   string(lpbDoc.SubType),
		"status":     string(lpbDoc.Status),
	})
}
