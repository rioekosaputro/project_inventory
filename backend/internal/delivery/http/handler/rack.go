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

var rackSort = pagination.SortSpec{
	Allowed: map[string]string{"code": "r.code", "zone": "r.zone", "warehouse": "w.code", "created_at": "r.created_at"},
	Default: "warehouse,code",
}

// RackHandler: /racks (baca master.read, tulis rack.write — dipasang di router).
type RackHandler struct{ uc *usecase.RackUsecase }

func NewRackHandler(uc *usecase.RackUsecase) *RackHandler { return &RackHandler{uc: uc} }

// List — GET /racks
func (h *RackHandler) List(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	page, pp, ok := pageOf(c, rackSort)
	if !ok {
		return
	}
	var q dto.ListRacksQuery
	if !httperr.BindQuery(c, &q) {
		return
	}
	items, total, err := h.uc.List(c.Request.Context(), a, domain.RackFilter{
		Query: q.Q, WarehouseID: q.WarehouseID, Zone: q.Zone, IsSellable: q.IsSellable, IsActive: q.IsActive,
	}, pp)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	out := make([]dto.RackResponse, 0, len(items))
	for i := range items {
		out = append(out, dto.NewRackResponse(&items[i]))
	}
	response.Paginated(c, out, pagination.NewMeta(page, total))
}

// Get — GET /racks/:id
func (h *RackHandler) Get(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	r, err := h.uc.Get(c.Request.Context(), a, id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewRackResponse(r))
}

// Create — POST /racks
func (h *RackHandler) Create(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	var req dto.CreateRackRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	r, err := h.uc.Create(c.Request.Context(), a, usecase.CreateRackInput{
		WarehouseID: req.WarehouseID, Code: req.Code, Zone: req.Zone, IsSellable: req.IsSellable, Description: req.Description,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.Created(c, dto.NewRackResponse(r))
}

// Update — PATCH /racks/:id
func (h *RackHandler) Update(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.UpdateRackRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	r, err := h.uc.Update(c.Request.Context(), a, id, usecase.UpdateRackInput{
		Zone: patch(req.Zone), IsSellable: req.IsSellable, Description: patch(req.Description), IsActive: req.IsActive,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewRackResponse(r))
}

type rackLabelsRequest struct {
	RackIDs []int64 `json:"rack_ids" binding:"required"`
}

// LabelsPDF — POST /racks/labels/pdf (cetak label barcode kode rak)
func (h *RackHandler) LabelsPDF(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	var req rackLabelsRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	pdfBytes, err := h.uc.RenderLabelsPDF(pdfContext(c), a, req.RackIDs)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	writePDFOrHTML(c, "label-rak.pdf", pdfBytes)
}
