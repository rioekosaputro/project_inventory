package handler

import (
	"fmt"
	nethttp "net/http"
	"time"

	"github.com/gin-gonic/gin"

	"rental-inventory-api/internal/delivery/http/dto"
	"rental-inventory-api/internal/delivery/http/httperr"
	"rental-inventory-api/internal/delivery/http/middleware"
	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/usecase"
	"rental-inventory-api/pkg/pagination"
	"rental-inventory-api/pkg/response"
)

var invoiceSort = pagination.SortSpec{
	Allowed: map[string]string{
		"invoice_number": "i.invoice_number",
		"invoice_date":   "i.invoice_date",
		"due_date":       "i.due_date",
		"grand_total":    "i.grand_total",
		"created_at":     "i.created_at",
	},
	Default: "-invoice_date",
}

// InvoiceHandler melayani /invoices (T2.8 generate billing, T2.9 CRUD/issue/void/pay/PDF/export).
type InvoiceHandler struct {
	billing *usecase.BillingUsecase
	uc      *usecase.InvoiceUsecase
}

func NewInvoiceHandler(billing *usecase.BillingUsecase, uc *usecase.InvoiceUsecase) *InvoiceHandler {
	return &InvoiceHandler{billing: billing, uc: uc}
}

func parseInvoiceFilter(q dto.ListInvoicesQuery) domain.InvoiceFilter {
	f := domain.InvoiceFilter{
		Query:       q.Q,
		CustomerID:  q.CustomerID,
		ContractID:  q.ContractID,
		OverdueOnly: q.Overdue,
	}
	if q.InvoiceType != "" {
		v := domain.InvoiceType(q.InvoiceType)
		f.InvoiceType = &v
	}
	if q.Status != "" {
		v := domain.InvoiceStatus(q.Status)
		f.Status = &v
	}
	if q.DateFrom != "" {
		if t, err := time.Parse("2006-01-02", q.DateFrom); err == nil {
			f.DateFrom = &t
		}
	}
	if q.DateTo != "" {
		if t, err := time.Parse("2006-01-02", q.DateTo); err == nil {
			f.DateTo = &t
		}
	}
	return f
}

// List — GET /api/v1/invoices
func (h *InvoiceHandler) List(c *gin.Context) {
	page, pp, ok := pageOf(c, invoiceSort)
	if !ok {
		return
	}
	var q dto.ListInvoicesQuery
	if !httperr.BindQuery(c, &q) {
		return
	}
	items, total, err := h.uc.List(c.Request.Context(), parseInvoiceFilter(q), pp)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	out := make([]dto.InvoiceResponse, 0, len(items))
	for i := range items {
		out = append(out, dto.NewInvoiceResponse(&items[i]))
	}
	response.Paginated(c, out, pagination.NewMeta(page, total))
}

// Get — GET /api/v1/invoices/:id
func (h *InvoiceHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	det, err := h.uc.Get(c.Request.Context(), id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewInvoiceDetailResponse(det))
}

// Generate — POST /api/v1/invoices/generate(?preview=1)
func (h *InvoiceHandler) Generate(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	var req dto.GenerateInvoicesRequest
	if c.Request.ContentLength > 0 {
		if !httperr.BindJSON(c, &req) {
			return
		}
	}
	preview := c.Query("preview") == "1" || c.Query("preview") == "true"

	res, err := h.billing.GenerateMonthlyInvoices(c.Request.Context(), &actor, usecase.GenerateBillingInput{
		RunDate:        req.RunDate,
		ContractID:     req.ContractID,
		Preview:        preview,
		GeneratedByJob: false,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewBillingRunResponse(res))
}

// Issue — POST /api/v1/invoices/:id/issue
func (h *InvoiceHandler) Issue(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.IssueInvoiceRequest
	if c.Request.ContentLength > 0 {
		if !httperr.BindJSON(c, &req) {
			return
		}
	}
	det, err := h.uc.Issue(c.Request.Context(), actor, id, usecase.IssueInvoiceInput{
		FakturPajakNumber: req.FakturPajakNumber,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewInvoiceDetailResponse(det))
}

// Void — POST /api/v1/invoices/:id/void
func (h *InvoiceHandler) Void(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.VoidInvoiceRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	det, err := h.uc.Void(c.Request.Context(), actor, id, usecase.VoidInvoiceInput{
		VoidReason: req.VoidReason,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewInvoiceDetailResponse(det))
}

// RecordPayment — POST /api/v1/invoices/:id/payments
func (h *InvoiceHandler) RecordPayment(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.RecordPaymentRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	det, err := h.uc.RecordPayment(c.Request.Context(), actor, id, usecase.RecordPaymentInput{
		PaymentDate:     req.PaymentDate,
		AmountReceived:  req.AmountReceived,
		PPh23Amount:     req.PPh23Amount,
		PPh23SlipNumber: req.PPh23SlipNumber,
		Method:          req.Method,
		ReferenceNumber: req.ReferenceNumber,
		ProofPath:       req.ProofPath,
		Notes:           req.Notes,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.Created(c, dto.NewInvoiceDetailResponse(det))
}

// PDF — GET /api/v1/invoices/:id/pdf(?download=1&fallback=html)
func (h *InvoiceHandler) PDF(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	data, filename, err := h.uc.RenderPDF(pdfContext(c), id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	writePDFOrHTML(c, filename, data)
}

// Export — GET /api/v1/invoices/export
func (h *InvoiceHandler) Export(c *gin.Context) {
	var q dto.ListInvoicesQuery
	if !httperr.BindQuery(c, &q) {
		return
	}
	data, filename, err := h.uc.ExportExcel(c.Request.Context(), parseInvoiceFilter(q))
	if err != nil {
		httperr.Write(c, err)
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(nethttp.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}
