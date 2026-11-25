package handler

import (
	"github.com/gin-gonic/gin"

	"rental-inventory-api/internal/delivery/http/dto"
	"rental-inventory-api/internal/delivery/http/httperr"
	"rental-inventory-api/internal/delivery/http/middleware"
	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/usecase"
	"rental-inventory-api/pkg/response"
)

// InvoiceHandler melayani /invoices (T2.8 generate billing, T2.9 CRUD/issue/void/pay/PDF/export).
type InvoiceHandler struct {
	billing *usecase.BillingUsecase
}

func NewInvoiceHandler(billing *usecase.BillingUsecase) *InvoiceHandler {
	return &InvoiceHandler{billing: billing}
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
