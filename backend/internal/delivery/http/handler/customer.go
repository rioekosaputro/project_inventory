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

var customerSort = pagination.SortSpec{
	Allowed: map[string]string{
		"customer_code": "c.customer_code",
		"company_name":  "c.company_name",
		"created_at":    "c.created_at",
	},
	Default: "company_name",
}

// CustomerHandler melayani /customers dan /customer-sites (T2.2).
// Otorisasi customer.read / customer.write dipasang di router.
type CustomerHandler struct{ uc *usecase.CustomerUsecase }

func NewCustomerHandler(uc *usecase.CustomerUsecase) *CustomerHandler {
	return &CustomerHandler{uc: uc}
}

// List — GET /customers
func (h *CustomerHandler) List(c *gin.Context) {
	page, pp, ok := pageOf(c, customerSort)
	if !ok {
		return
	}
	var q dto.ListCustomersQuery
	if !httperr.BindQuery(c, &q) {
		return
	}
	items, total, err := h.uc.List(c.Request.Context(), domain.CustomerFilter{
		Query: q.Q, IsPKP: q.IsPKP, IsActive: q.IsActive,
	}, pp)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	out := make([]dto.CustomerResponse, 0, len(items))
	for i := range items {
		out = append(out, dto.NewCustomerResponse(&items[i]))
	}
	response.Paginated(c, out, pagination.NewMeta(page, total))
}

// Get — GET /customers/:id
func (h *CustomerHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	detail, err := h.uc.Get(c.Request.Context(), id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	sites := make([]dto.CustomerSiteResponse, 0, len(detail.Sites))
	for i := range detail.Sites {
		sites = append(sites, dto.NewCustomerSiteResponse(&detail.Sites[i]))
	}
	response.OK(c, dto.CustomerDetailResponse{
		CustomerResponse: dto.NewCustomerResponse(detail.Customer),
		Sites:            sites,
	})
}

// Create — POST /customers
func (h *CustomerHandler) Create(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	var req dto.CreateCustomerRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	created, err := h.uc.Create(c.Request.Context(), actor, usecase.CreateCustomerInput{
		CustomerCode:      req.CustomerCode,
		CompanyName:       req.CompanyName,
		PICName:           req.PICName,
		Phone:             req.Phone,
		Email:             req.Email,
		IsPKP:             req.IsPKP,
		NPWP:              req.NPWP,
		TaxAddress:        req.TaxAddress,
		BillingAddress:    req.BillingAddress,
		BillingPostalCode: req.BillingPostalCode,
		BillingEmail:      req.BillingEmail,
		PaymentTermDays:   req.PaymentTermDays,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.Created(c, dto.NewCustomerResponse(created))
}

// Update — PATCH /customers/:id
func (h *CustomerHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.UpdateCustomerRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	updated, err := h.uc.Update(c.Request.Context(), id, usecase.UpdateCustomerInput{
		CompanyName:       req.CompanyName,
		PICName:           req.PICName,
		Phone:             req.Phone,
		Email:             patch(req.Email),
		IsPKP:             req.IsPKP,
		NPWP:              patch(req.NPWP),
		TaxAddress:        patch(req.TaxAddress),
		BillingAddress:    req.BillingAddress,
		BillingPostalCode: req.BillingPostalCode,
		BillingEmail:      patch(req.BillingEmail),
		PaymentTermDays:   req.PaymentTermDays,
		IsActive:          req.IsActive,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewCustomerResponse(updated))
}

// ListSites — GET /customers/:id/sites
func (h *CustomerHandler) ListSites(c *gin.Context) {
	customerID, ok := parseID(c)
	if !ok {
		return
	}
	var q dto.ListCustomerSitesQuery
	if !httperr.BindQuery(c, &q) {
		return
	}
	sites, err := h.uc.ListSites(c.Request.Context(), customerID, domain.CustomerSiteFilter{IsActive: q.IsActive})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	out := make([]dto.CustomerSiteResponse, 0, len(sites))
	for i := range sites {
		out = append(out, dto.NewCustomerSiteResponse(&sites[i]))
	}
	response.OK(c, out)
}

// CreateSite — POST /customers/:id/sites
func (h *CustomerHandler) CreateSite(c *gin.Context) {
	customerID, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.CreateCustomerSiteRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	created, err := h.uc.CreateSite(c.Request.Context(), customerID, usecase.CreateCustomerSiteInput{
		SiteName:    req.SiteName,
		PICName:     req.PICName,
		PICPhone:    req.PICPhone,
		AddressLine: req.AddressLine,
		Subdistrict: req.Subdistrict,
		District:    req.District,
		City:        req.City,
		Province:    req.Province,
		PostalCode:  req.PostalCode,
		Latitude:    req.Latitude,
		Longitude:   req.Longitude,
		IsPrimary:   req.IsPrimary,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.Created(c, dto.NewCustomerSiteResponse(created))
}

// UpdateSite — PATCH /customer-sites/:id
func (h *CustomerHandler) UpdateSite(c *gin.Context) {
	siteID, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.UpdateCustomerSiteRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	updated, err := h.uc.UpdateSite(c.Request.Context(), siteID, usecase.UpdateCustomerSiteInput{
		SiteName:    req.SiteName,
		PICName:     req.PICName,
		PICPhone:    req.PICPhone,
		AddressLine: req.AddressLine,
		Subdistrict: patch(req.Subdistrict),
		District:    patch(req.District),
		City:        req.City,
		Province:    req.Province,
		PostalCode:  req.PostalCode,
		Latitude:    patch(req.Latitude),
		Longitude:   patch(req.Longitude),
		IsPrimary:   req.IsPrimary,
		IsActive:    req.IsActive,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewCustomerSiteResponse(updated))
}

// ListAssets — GET /customers/:id/assets
func (h *CustomerHandler) ListAssets(c *gin.Context) {
	customerID, ok := parseID(c)
	if !ok {
		return
	}
	var q dto.ListCustomerAssetsQuery
	if !httperr.BindQuery(c, &q) {
		return
	}
	assets, err := h.uc.ListAssets(c.Request.Context(), customerID, q.SiteID)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	out := make([]dto.CustomerAssetResponse, 0, len(assets))
	for i := range assets {
		out = append(out, dto.NewCustomerAssetResponse(&assets[i]))
	}
	response.OK(c, out)
}
