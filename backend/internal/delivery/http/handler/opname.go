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

var opnameSort = pagination.SortSpec{
	Allowed: map[string]string{
		"opname_number": "o.opname_number", "status": "o.status", "started_at": "o.started_at", "warehouse": "w.code",
	},
	Default: "-started_at",
}

type OpnameHandler struct{ uc *usecase.OpnameUsecase }

func NewOpnameHandler(uc *usecase.OpnameUsecase) *OpnameHandler { return &OpnameHandler{uc: uc} }

// List — GET /inventory/opnames
func (h *OpnameHandler) List(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	page, pp, ok := pageOf(c, opnameSort)
	if !ok {
		return
	}
	var q dto.ListOpnamesQuery
	if !httperr.BindQuery(c, &q) {
		return
	}
	f := domain.OpnameFilter{Query: q.Q, WarehouseID: q.WarehouseID}
	if q.Status != "" {
		st := domain.OpnameStatus(q.Status)
		if !st.Valid() {
			httperr.BadQueryField(c, "status", "tidak valid")
			return
		}
		f.Status = &st
	}
	items, total, err := h.uc.List(c.Request.Context(), a, f, pp)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	out := make([]dto.OpnameListResponse, 0, len(items))
	for i := range items {
		out = append(out, dto.NewOpnameListResponse(&items[i]))
	}
	response.Paginated(c, out, pagination.NewMeta(page, total))
}

// Start — POST /inventory/opnames
func (h *OpnameHandler) Start(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	var req dto.StartOpnameRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	blind := true
	if req.IsBlindCount != nil {
		blind = *req.IsBlindCount
	}
	d, err := h.uc.Start(c.Request.Context(), a, usecase.StartOpnameInput{
		WarehouseID: req.WarehouseID, RackIDs: req.RackIDs, IsBlindCount: blind, Notes: req.Notes,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.Created(c, dto.NewOpnameDetailResponse(d))
}

// Get — GET /inventory/opnames/:id
func (h *OpnameHandler) Get(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	d, err := h.uc.Get(c.Request.Context(), a, id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewOpnameDetailResponse(d))
}

// SaveCounts — PUT /inventory/opnames/:id/counts
func (h *OpnameHandler) SaveCounts(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.SaveOpnameCountsRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	counts := make([]usecase.OpnameCountInput, 0, len(req.Items))
	for _, it := range req.Items {
		counts = append(counts, usecase.OpnameCountInput{
			ProductID: it.ProductID, RackID: it.RackID, CountedQty: it.CountedQty, Notes: it.Notes,
		})
	}
	d, err := h.uc.SaveCounts(c.Request.Context(), a, id, counts)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OKMessage(c, "Hitungan opname disimpan", dto.NewOpnameDetailResponse(d))
}

// ScanSerial — POST /inventory/opnames/:id/scans
func (h *OpnameHandler) ScanSerial(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.ScanOpnameSerialRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	s, err := h.uc.ScanSerial(c.Request.Context(), a, id, req.RackID, req.Code)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewOpnameSerialResponse(s))
}

// Submit — POST /inventory/opnames/:id/submit
func (h *OpnameHandler) Submit(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	d, err := h.uc.Submit(c.Request.Context(), a, id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OKMessage(c, "Stock opname diajukan untuk finalisasi", dto.NewOpnameDetailResponse(d))
}

// Finalize — POST /inventory/opnames/:id/finalize
func (h *OpnameHandler) Finalize(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	d, err := h.uc.Finalize(c.Request.Context(), a, id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OKMessage(c, "Stock opname difinalisasi; stok disesuaikan dan kunci rak dibuka", dto.NewOpnameDetailResponse(d))
}

// Cancel — POST /inventory/opnames/:id/cancel
func (h *OpnameHandler) Cancel(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	d, err := h.uc.Cancel(c.Request.Context(), a, id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OKMessage(c, "Stock opname dibatalkan; kunci rak dibuka", dto.NewOpnameDetailResponse(d))
}
