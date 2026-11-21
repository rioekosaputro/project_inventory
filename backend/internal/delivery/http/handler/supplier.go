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

var supplierSort = pagination.SortSpec{
	Allowed: map[string]string{"code": "s.code", "name": "s.name", "created_at": "s.created_at"},
	Default: "name",
}

// SupplierHandler: /suppliers (baca master.read, tulis master.write — dipasang di router).
type SupplierHandler struct{ uc *usecase.SupplierUsecase }

func NewSupplierHandler(uc *usecase.SupplierUsecase) *SupplierHandler {
	return &SupplierHandler{uc: uc}
}

// List — GET /suppliers
func (h *SupplierHandler) List(c *gin.Context) {
	page, pp, ok := pageOf(c, supplierSort)
	if !ok {
		return
	}
	var q dto.ListSuppliersQuery
	if !httperr.BindQuery(c, &q) {
		return
	}
	items, total, err := h.uc.List(c.Request.Context(), domain.SupplierFilter{Query: q.Q, IsActive: q.IsActive}, pp)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	out := make([]dto.SupplierResponse, 0, len(items))
	for i := range items {
		out = append(out, dto.NewSupplierResponse(&items[i]))
	}
	response.Paginated(c, out, pagination.NewMeta(page, total))
}

// Get — GET /suppliers/:id
func (h *SupplierHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	s, err := h.uc.Get(c.Request.Context(), id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewSupplierResponse(s))
}

// Create — POST /suppliers
func (h *SupplierHandler) Create(c *gin.Context) {
	var req dto.CreateSupplierRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	s, err := h.uc.Create(c.Request.Context(), usecase.CreateSupplierInput{
		Code: req.Code, Name: req.Name, PICName: req.PICName, Phone: req.Phone,
		Email: req.Email, NPWP: req.NPWP, Address: req.Address,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.Created(c, dto.NewSupplierResponse(s))
}

// Update — PATCH /suppliers/:id
func (h *SupplierHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.UpdateSupplierRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	s, err := h.uc.Update(c.Request.Context(), id, usecase.UpdateSupplierInput{
		Name: req.Name, PICName: patch(req.PICName), Phone: patch(req.Phone), Email: patch(req.Email),
		NPWP: patch(req.NPWP), Address: patch(req.Address), IsActive: req.IsActive,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewSupplierResponse(s))
}
