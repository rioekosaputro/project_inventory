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

var productSort = pagination.SortSpec{
	Allowed: map[string]string{"sku": "p.sku", "name": "p.name", "category": "c.name", "created_at": "p.created_at"},
	Default: "name",
}

// ProductHandler: /products (baca product.read, tulis master.write — dipasang di router).
type ProductHandler struct{ uc *usecase.ProductUsecase }

func NewProductHandler(uc *usecase.ProductUsecase) *ProductHandler { return &ProductHandler{uc: uc} }

// List — GET /products
func (h *ProductHandler) List(c *gin.Context) {
	page, pp, ok := pageOf(c, productSort)
	if !ok {
		return
	}
	var q dto.ListProductsQuery
	if !httperr.BindQuery(c, &q) {
		return
	}
	items, total, err := h.uc.List(c.Request.Context(), domain.ProductFilter{
		Query: q.Q, CategoryID: q.CategoryID, ItemType: itemTypePtr(q.ItemType),
		HasSerialNumber: q.HasSerialNumber, IsActive: q.IsActive,
	}, pp)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	out := make([]dto.ProductResponse, 0, len(items))
	for i := range items {
		out = append(out, dto.NewProductResponse(&items[i]))
	}
	response.Paginated(c, out, pagination.NewMeta(page, total))
}

// Get — GET /products/:id
func (h *ProductHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	p, err := h.uc.Get(c.Request.Context(), id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewProductResponse(p))
}

// Create — POST /products
func (h *ProductHandler) Create(c *gin.Context) {
	var req dto.CreateProductRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	p, err := h.uc.Create(c.Request.Context(), usecase.CreateProductInput{
		SKU: req.SKU, Name: req.Name, CategoryID: req.CategoryID, Brand: req.Brand, Model: req.Model,
		Unit: req.Unit, HasSerialNumber: req.HasSerialNumber, MinStockAlert: req.MinStockAlert,
		DefaultMonthlyRate: req.DefaultMonthlyRate, DefaultSellPrice: req.DefaultSellPrice, Description: req.Description,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.Created(c, dto.NewProductResponse(p))
}

// Update — PATCH /products/:id
func (h *ProductHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.UpdateProductRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	p, err := h.uc.Update(c.Request.Context(), id, usecase.UpdateProductInput{
		SKU: req.SKU, Name: req.Name, CategoryID: req.CategoryID,
		Brand: patch(req.Brand), Model: patch(req.Model), Unit: req.Unit,
		HasSerialNumber: req.HasSerialNumber, MinStockAlert: req.MinStockAlert,
		DefaultMonthlyRate: patch(req.DefaultMonthlyRate), DefaultSellPrice: patch(req.DefaultSellPrice),
		Description: patch(req.Description), IsActive: req.IsActive,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewProductResponse(p))
}
