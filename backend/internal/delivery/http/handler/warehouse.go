package handler

import (
	"github.com/gin-gonic/gin"

	"rental-inventory-api/internal/delivery/http/dto"
	"rental-inventory-api/internal/delivery/http/httperr"
	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/usecase"
	"rental-inventory-api/pkg/pagination"
	"rental-inventory-api/pkg/response"
)

var warehouseSort = pagination.SortSpec{
	Allowed: map[string]string{"code": "w.code", "name": "w.name", "created_at": "w.created_at"},
	Default: "code",
}

// WarehouseHandler: /warehouses (baca master.read, tulis master.write — dipasang di router).
type WarehouseHandler struct{ uc *usecase.WarehouseUsecase }

func NewWarehouseHandler(uc *usecase.WarehouseUsecase) *WarehouseHandler {
	return &WarehouseHandler{uc: uc}
}

// List — GET /warehouses
func (h *WarehouseHandler) List(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	page, pp, ok := pageOf(c, warehouseSort)
	if !ok {
		return
	}
	var q dto.ListWarehousesQuery
	if !httperr.BindQuery(c, &q) {
		return
	}
	items, total, err := h.uc.List(c.Request.Context(), a, domain.WarehouseFilter{Query: q.Q, IsActive: q.IsActive}, pp)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	out := make([]dto.WarehouseResponse, 0, len(items))
	for i := range items {
		out = append(out, dto.NewWarehouseResponse(&items[i]))
	}
	response.Paginated(c, out, pagination.NewMeta(page, total))
}

// Get — GET /warehouses/:id
func (h *WarehouseHandler) Get(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	w, err := h.uc.Get(c.Request.Context(), a, id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewWarehouseResponse(w))
}

// Create — POST /warehouses
func (h *WarehouseHandler) Create(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	var req dto.CreateWarehouseRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	w, err := h.uc.Create(c.Request.Context(), a, usecase.CreateWarehouseInput{
		Code: req.Code, Name: req.Name, Address: req.Address, Phone: req.Phone,
		Latitude: req.Latitude, Longitude: req.Longitude,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.Created(c, dto.NewWarehouseResponse(w))
}

// Update — PATCH /warehouses/:id
func (h *WarehouseHandler) Update(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.UpdateWarehouseRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	w, err := h.uc.Update(c.Request.Context(), a, id, usecase.UpdateWarehouseInput{
		Name: req.Name, Address: patch(req.Address), Phone: patch(req.Phone),
		Latitude: patch(req.Latitude), Longitude: patch(req.Longitude), IsActive: req.IsActive,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewWarehouseResponse(w))
}
