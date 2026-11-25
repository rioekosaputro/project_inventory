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

var contractSort = pagination.SortSpec{
	Allowed: map[string]string{
		"contract_number": "rc.contract_number",
		"start_date":      "rc.start_date",
		"end_date":        "rc.end_date",
		"created_at":      "rc.created_at",
	},
	Default: "-created_at",
}

// ContractHandler melayani /contracts (T2.3).
// Otorisasi contract.read / contract.write / contract.approve dipasang di router.
type ContractHandler struct{ uc *usecase.ContractUsecase }

func NewContractHandler(uc *usecase.ContractUsecase) *ContractHandler {
	return &ContractHandler{uc: uc}
}

// List — GET /contracts
func (h *ContractHandler) List(c *gin.Context) {
	page, pp, ok := pageOf(c, contractSort)
	if !ok {
		return
	}
	var q dto.ListContractsQuery
	if !httperr.BindQuery(c, &q) {
		return
	}
	var st *domain.ContractStatus
	if q.Status != "" {
		v := domain.ContractStatus(q.Status)
		st = &v
	}
	items, total, err := h.uc.List(c.Request.Context(), domain.ContractFilter{
		Query: q.Q, CustomerID: q.CustomerID, Status: st, ExpiringIn: q.ExpiringIn,
	}, pp)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	out := make([]dto.ContractResponse, 0, len(items))
	for i := range items {
		out = append(out, dto.NewContractResponse(&items[i]))
	}
	response.Paginated(c, out, pagination.NewMeta(page, total))
}

// Get — GET /contracts/:id
func (h *ContractHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	detail, err := h.uc.Get(c.Request.Context(), id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewContractDetailResponse(detail))
}

// Create — POST /contracts
func (h *ContractHandler) Create(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	var req dto.CreateContractRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	detail, err := h.uc.Create(c.Request.Context(), actor, usecase.CreateContractInput{
		CustomerID:         req.CustomerID,
		StartDate:          req.StartDate,
		EndDate:            req.EndDate,
		DurationMonths:     req.DurationMonths,
		BillingDayOfMonth:  req.BillingDayOfMonth,
		BillingMode:        req.BillingMode,
		BillingInAdvance:   req.BillingInAdvance,
		IsPPN:              req.IsPPN,
		PaymentTermDays:    req.PaymentTermDays,
		DepositAmount:      req.DepositAmount,
		SignedDocumentPath: req.SignedDocumentPath,
		Notes:              req.Notes,
		Items:              mapContractItems(req.Items),
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.Created(c, dto.NewContractDetailResponse(detail))
}

// Update — PATCH /contracts/:id
func (h *ContractHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.UpdateContractRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	var itemsPtr *[]usecase.ContractItemInput
	if req.Items != nil {
		mapped := mapContractItems(*req.Items)
		itemsPtr = &mapped
	}
	detail, err := h.uc.Update(c.Request.Context(), id, usecase.UpdateContractInput{
		StartDate:          req.StartDate,
		EndDate:            req.EndDate,
		DurationMonths:     req.DurationMonths,
		BillingDayOfMonth:  req.BillingDayOfMonth,
		BillingMode:        req.BillingMode,
		BillingInAdvance:   req.BillingInAdvance,
		IsPPN:              req.IsPPN,
		PaymentTermDays:    req.PaymentTermDays,
		DepositAmount:      req.DepositAmount,
		SignedDocumentPath: patch(req.SignedDocumentPath),
		Notes:              patch(req.Notes),
		Items:              itemsPtr,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewContractDetailResponse(detail))
}

// Activate — POST /contracts/:id/activate
func (h *ContractHandler) Activate(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	detail, err := h.uc.Activate(c.Request.Context(), actor, id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewContractDetailResponse(detail))
}

// Terminate — POST /contracts/:id/terminate
func (h *ContractHandler) Terminate(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.TerminateContractRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	detail, err := h.uc.Terminate(c.Request.Context(), actor, id, usecase.TerminateContractInput{
		TerminatedAt:      req.TerminatedAt,
		TerminationReason: req.TerminationReason,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewContractDetailResponse(detail))
}

// Renew — POST /contracts/:id/renew
func (h *ContractHandler) Renew(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.RenewContractRequest
	if c.Request.ContentLength > 0 {
		if !httperr.BindJSON(c, &req) {
			return
		}
	}
	detail, err := h.uc.Renew(c.Request.Context(), actor, id, usecase.RenewContractInput{
		StartDate:      req.StartDate,
		EndDate:        req.EndDate,
		DurationMonths: req.DurationMonths,
		Notes:          req.Notes,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.Created(c, dto.NewContractDetailResponse(detail))
}

func mapContractItems(in []dto.ContractItemRequest) []usecase.ContractItemInput {
	out := make([]usecase.ContractItemInput, 0, len(in))
	for _, it := range in {
		out = append(out, usecase.ContractItemInput{
			ProductID:      it.ProductID,
			CustomerSiteID: it.CustomerSiteID,
			Qty:            it.Qty,
			MonthlyRate:    it.MonthlyRate,
			Notes:          it.Notes,
		})
	}
	return out
}
