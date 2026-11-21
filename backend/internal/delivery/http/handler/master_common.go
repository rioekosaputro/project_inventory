package handler

import (
	"github.com/gin-gonic/gin"

	"rental-inventory-api/internal/delivery/http/dto"
	"rental-inventory-api/internal/delivery/http/httperr"
	"rental-inventory-api/internal/delivery/http/middleware"
	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/usecase"
	"rental-inventory-api/pkg/pagination"
)

// patch mengubah field PATCH dari DTO ke bentuk usecase (Set=false → tidak diubah).
func patch[T any](o dto.Optional[T]) usecase.Patch[T] {
	return usecase.Patch[T]{Set: o.Set, Value: o.Value}
}

// actor mengambil identitas pemanggil dari context; gagal → 401.
func actor(c *gin.Context) (domain.Principal, bool) {
	p, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
	}
	return p, ok
}

// pageOf mem-parsing ?page, ?limit, ?sort (whitelist) menjadi parameter paginasi repository.
func pageOf(c *gin.Context, spec pagination.SortSpec) (pagination.Params, domain.PageParams, bool) {
	page, err := pagination.FromQuery(c.Request.URL.Query(), spec)
	if err != nil {
		httperr.Write(c, err)
		return page, domain.PageParams{}, false
	}
	return page, domain.PageParams{Limit: page.Limit, Offset: page.Offset(), OrderBy: page.OrderBy()}, true
}

// itemTypePtr: "" → nil (tidak difilter).
func itemTypePtr(s string) *domain.ItemType {
	if s == "" {
		return nil
	}
	t := domain.ItemType(s)
	return &t
}
