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

var categorySort = pagination.SortSpec{
	Allowed: map[string]string{"name": "c.name", "item_type": "c.item_type"},
	Default: "name",
}

// CategoryHandler: /categories (baca master.read, tulis master.write — dipasang di router).
type CategoryHandler struct{ uc *usecase.CategoryUsecase }

func NewCategoryHandler(uc *usecase.CategoryUsecase) *CategoryHandler {
	return &CategoryHandler{uc: uc}
}

// List — GET /categories
func (h *CategoryHandler) List(c *gin.Context) {
	page, pp, ok := pageOf(c, categorySort)
	if !ok {
		return
	}
	var q dto.ListCategoriesQuery
	if !httperr.BindQuery(c, &q) {
		return
	}
	items, total, err := h.uc.List(c.Request.Context(),
		domain.CategoryFilter{Query: q.Q, ItemType: itemTypePtr(q.ItemType), IsActive: q.IsActive}, pp)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	out := make([]dto.CategoryResponse, 0, len(items))
	for i := range items {
		out = append(out, dto.NewCategoryResponse(&items[i]))
	}
	response.Paginated(c, out, pagination.NewMeta(page, total))
}

// Get — GET /categories/:id
func (h *CategoryHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	cat, err := h.uc.Get(c.Request.Context(), id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewCategoryResponse(cat))
}

// Create — POST /categories
func (h *CategoryHandler) Create(c *gin.Context) {
	var req dto.CreateCategoryRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	cat, err := h.uc.Create(c.Request.Context(), usecase.CreateCategoryInput{
		Name: req.Name, ItemType: domain.ItemType(req.ItemType),
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.Created(c, dto.NewCategoryResponse(cat))
}

// Update — PATCH /categories/:id
func (h *CategoryHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.UpdateCategoryRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	cat, err := h.uc.Update(c.Request.Context(), id, usecase.UpdateCategoryInput{
		Name: req.Name, ItemType: itemTypePtr(deref(req.ItemType)), IsActive: req.IsActive,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewCategoryResponse(cat))
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
