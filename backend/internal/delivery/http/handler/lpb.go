package handler

import (
	"fmt"
	"io"
	nethttp "net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"rental-inventory-api/internal/delivery/http/dto"
	"rental-inventory-api/internal/delivery/http/httperr"
	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/usecase"
	"rental-inventory-api/pkg/pagination"
	"rental-inventory-api/pkg/response"
)

var lpbSort = pagination.SortSpec{
	Allowed: map[string]string{
		"lpb_number": "l.lpb_number", "doc_date": "l.doc_date", "status": "l.status",
		"warehouse": "w.code", "created_at": "l.created_at",
	},
	Default: "-created_at",
}

// LPBHandler: /lpb (permission per route di router: lpb.read / lpb.create / lpb.submit).
type LPBHandler struct{ uc *usecase.LPBUsecase }

func NewLPBHandler(uc *usecase.LPBUsecase) *LPBHandler { return &LPBHandler{uc: uc} }

// List — GET /lpb
func (h *LPBHandler) List(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	page, pp, ok := pageOf(c, lpbSort)
	if !ok {
		return
	}
	var q dto.ListLPBQuery
	if !httperr.BindQuery(c, &q) {
		return
	}
	f := domain.LPBFilter{Query: q.Q, WarehouseID: q.WarehouseID}
	if q.Type != "" {
		t := domain.LPBType(q.Type)
		f.Type = &t
	}
	if q.SubType != "" {
		s := domain.LPBSubType(q.SubType)
		if !s.Valid() {
			httperr.BadQueryField(c, "sub_type", "tidak valid")
			return
		}
		f.SubType = &s
	}
	if q.Status != "" {
		s := domain.LPBStatus(q.Status)
		if !s.Valid() {
			httperr.BadQueryField(c, "status", "tidak valid")
			return
		}
		f.Status = &s
	}
	if q.DateFrom != "" {
		f.DateFrom = &q.DateFrom
	}
	if q.DateTo != "" {
		f.DateTo = &q.DateTo
	}
	if q.Mine {
		f.CreatedBy = &a.UserID
	}

	items, total, err := h.uc.List(c.Request.Context(), a, f, pp)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	out := make([]dto.LPBListResponse, 0, len(items))
	for i := range items {
		out = append(out, dto.NewLPBListResponse(&items[i]))
	}
	response.Paginated(c, out, pagination.NewMeta(page, total))
}

// Get — GET /lpb/:id
func (h *LPBHandler) Get(c *gin.Context) {
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
	response.OK(c, dto.NewLPBDetailResponse(d))
}

// Create — POST /lpb
func (h *LPBHandler) Create(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	var req dto.CreateLPBRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	d, err := h.uc.Create(c.Request.Context(), a, usecase.CreateLPBInput{
		SubType: domain.LPBSubType(req.SubType), WarehouseID: req.WarehouseID, DocDate: req.DocDate,
		SupplierID: req.SupplierID, ReferenceNumber: req.ReferenceNumber, Notes: req.Notes,
		Items: dto.ToItemInputs(req.Items),
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.Created(c, dto.NewLPBDetailResponse(d))
}

// Update — PATCH /lpb/:id
func (h *LPBHandler) Update(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.UpdateLPBRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	in := usecase.UpdateLPBInput{
		DocDate: req.DocDate, SupplierID: patch(req.SupplierID),
		ReferenceNumber: patch(req.ReferenceNumber), Notes: patch(req.Notes),
	}
	if req.Items != nil {
		items := dto.ToItemInputs(*req.Items)
		in.Items = &items
	}
	d, err := h.uc.Update(c.Request.Context(), a, id, in)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewLPBDetailResponse(d))
}

// transition menjalankan Submit/Cancel/Revise yang bentuknya sama.
func (h *LPBHandler) transition(c *gin.Context, fn func(ctxA domain.Principal, id int64) (*usecase.LPBDetail, error), message string) {
	a, ok := actor(c)
	if !ok {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	d, err := fn(a, id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OKMessage(c, message, dto.NewLPBDetailResponse(d))
}

// Submit — POST /lpb/:id/submit
func (h *LPBHandler) Submit(c *gin.Context) {
	h.transition(c, func(a domain.Principal, id int64) (*usecase.LPBDetail, error) {
		return h.uc.Submit(c.Request.Context(), a, id)
	}, "LPB diajukan untuk persetujuan")
}

// Cancel — POST /lpb/:id/cancel
func (h *LPBHandler) Cancel(c *gin.Context) {
	h.transition(c, func(a domain.Principal, id int64) (*usecase.LPBDetail, error) {
		return h.uc.Cancel(c.Request.Context(), a, id)
	}, "LPB dibatalkan")
}

// Revise — POST /lpb/:id/revise
func (h *LPBHandler) Revise(c *gin.Context) {
	h.transition(c, func(a domain.Principal, id int64) (*usecase.LPBDetail, error) {
		return h.uc.Revise(c.Request.Context(), a, id)
	}, "LPB dikembalikan ke draft untuk direvisi")
}

// AddSerial — POST /lpb/:id/items/:itemId/serials
func (h *LPBHandler) AddSerial(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	itemID, ok := parseIDParam(c, "itemId")
	if !ok {
		return
	}
	var req dto.AddSerialRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	res, err := h.uc.AddSerial(c.Request.Context(), a, id, itemID, usecase.AddSerialInput{
		SerialNumber: req.SerialNumber, Condition: req.Condition,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.Created(c, dto.AddSerialResponse{
		LPBItemSerialResponse: dto.NewLPBItemSerialResponse(&res.Serial), Scanned: res.Scanned, Qty: res.Qty,
	})
}

// RemoveSerial — DELETE /lpb/:id/items/:itemId/serials/:serialId
func (h *LPBHandler) RemoveSerial(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	itemID, ok := parseIDParam(c, "itemId")
	if !ok {
		return
	}
	serialID, ok := parseIDParam(c, "serialId")
	if !ok {
		return
	}
	if err := h.uc.RemoveSerial(c.Request.Context(), a, id, itemID, serialID); err != nil {
		httperr.Write(c, err)
		return
	}
	response.OKMessage(c, "Serial number dihapus dari item", nil)
}

func parseIDParam(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id < 1 {
		httperr.BadQueryField(c, name, "tidak valid")
		return 0, false
	}
	return id, true
}

var approvalSort = pagination.SortSpec{
	Allowed: map[string]string{
		"submitted_at": "l.submitted_at", "lpb_number": "l.lpb_number", "doc_date": "l.doc_date", "warehouse": "w.code",
	},
	Default: "submitted_at",
}

// Approvals — GET /lpb/approvals (antrean menunggu persetujuan, tertua dulu)
func (h *LPBHandler) Approvals(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	page, pp, ok := pageOf(c, approvalSort)
	if !ok {
		return
	}
	var q dto.ListLPBQuery
	if !httperr.BindQuery(c, &q) {
		return
	}
	f := domain.LPBFilter{Query: q.Q, WarehouseID: q.WarehouseID}
	if q.SubType != "" {
		s := domain.LPBSubType(q.SubType)
		if !s.Valid() {
			httperr.BadQueryField(c, "sub_type", "tidak valid")
			return
		}
		f.SubType = &s
	}
	items, total, err := h.uc.Approvals(c.Request.Context(), a, f, pp)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	out := make([]dto.LPBListResponse, 0, len(items))
	for i := range items {
		out = append(out, dto.NewLPBListResponse(&items[i]))
	}
	response.Paginated(c, out, pagination.NewMeta(page, total))
}

// Approve — POST /lpb/:id/approve (wajib Idempotency-Key; dipasang di router)
func (h *LPBHandler) Approve(c *gin.Context) {
	h.transition(c, func(a domain.Principal, id int64) (*usecase.LPBDetail, error) {
		return h.uc.Approve(c.Request.Context(), a, id)
	}, "LPB disetujui; stok diperbarui")
}

// Reject — POST /lpb/:id/reject  {reason}
func (h *LPBHandler) Reject(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.RejectLPBRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	d, err := h.uc.Reject(c.Request.Context(), a, id, req.Reason)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OKMessage(c, "LPB ditolak", dto.NewLPBDetailResponse(d))
}

// PDF — GET /lpb/:id/pdf (hanya APPROVED; ?download=1 untuk attachment)
func (h *LPBHandler) PDF(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	filename, pdfBytes, err := h.uc.RenderPDF(c.Request.Context(), a, id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	disposition := "inline"
	if c.Query("download") == "1" || c.Query("download") == "true" {
		disposition = "attachment"
	}
	c.Header("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, filename))
	c.Data(nethttp.StatusOK, "application/pdf", pdfBytes)
}

// VerifyPublic — GET /public/lpb/verify/:token (publik, tanpa login; ringkas tanpa harga/SN)
func (h *LPBHandler) VerifyPublic(c *gin.Context) {
	summary, err := h.uc.VerifyByToken(c.Request.Context(), c.Param("token"))
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewLPBVerifyResponse(summary))
}

// InitialStockTemplate — GET /lpb/import-initial-stock/template (.xlsx)
func (h *LPBHandler) InitialStockTemplate(c *gin.Context) {
	data, err := h.uc.InitialStockTemplateXLSX()
	if err != nil {
		httperr.Write(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="template-stok-awal.xlsx"`)
	c.Data(nethttp.StatusOK, xlsxMIME, data)
}

// ImportInitialStock — POST /lpb/import-initial-stock (multipart: warehouse_id, doc_date, notes, file)
func (h *LPBHandler) ImportInitialStock(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	whID, err := strconv.ParseInt(c.PostForm("warehouse_id"), 10, 64)
	if err != nil || whID < 1 {
		httperr.Write(c, &domain.ValidationError{
			Message: "Data impor tidak valid",
			Errors:  []domain.FieldError{{Field: "warehouse_id", Message: "gudang wajib dipilih"}},
		})
		return
	}
	fh, err := c.FormFile("file")
	if err != nil {
		httperr.Write(c, &domain.ValidationError{
			Message: "Data impor tidak valid",
			Errors:  []domain.FieldError{{Field: "file", Message: "berkas Excel (.xlsx) atau CSV wajib diunggah"}},
		})
		return
	}
	f, err := fh.Open()
	if err != nil {
		httperr.Write(c, err)
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (5<<20)+1))
	if err != nil {
		httperr.Write(c, err)
		return
	}
	var docDate, notes *string
	if v := c.PostForm("doc_date"); v != "" {
		docDate = &v
	}
	if v := c.PostForm("notes"); v != "" {
		notes = &v
	}
	d, err := h.uc.ImportInitialStock(c.Request.Context(), a, usecase.ImportInitialStockInput{
		WarehouseID: whID, DocDate: docDate, Notes: notes, Filename: fh.Filename, FileBytes: data,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.Created(c, dto.NewLPBDetailResponse(d))
}
