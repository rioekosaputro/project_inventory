package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"rental-inventory-api/internal/delivery/http/dto"
	"rental-inventory-api/internal/delivery/http/httperr"
	"rental-inventory-api/internal/delivery/http/middleware"
	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/usecase"
	"rental-inventory-api/pkg/pagination"
	"rental-inventory-api/pkg/response"
)

// userSort: nama yang boleh dipakai klien → ekspresi kolom SQL (whitelist).
var userSort = pagination.SortSpec{
	Allowed: map[string]string{"name": "name", "email": "email", "role": "role", "created_at": "created_at"},
	Default: "name",
}

// UserHandler: /users — hanya ADMIN (user.manage, dipasang di router).
type UserHandler struct{ uc *usecase.UserUsecase }

func NewUserHandler(uc *usecase.UserUsecase) *UserHandler { return &UserHandler{uc: uc} }

// List — GET /users
func (h *UserHandler) List(c *gin.Context) {
	page, err := pagination.FromQuery(c.Request.URL.Query(), userSort)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	var q dto.ListUsersQuery
	if !httperr.BindQuery(c, &q) {
		return
	}
	filter := domain.UserFilter{Query: q.Q, IsActive: q.IsActive, WarehouseID: q.WarehouseID}
	if q.Role != "" {
		role := domain.Role(q.Role)
		if !role.Valid() {
			httperr.BadQueryField(c, "role", "tidak valid")
			return
		}
		filter.Role = &role
	}

	users, total, err := h.uc.List(c.Request.Context(), filter,
		domain.PageParams{Limit: page.Limit, Offset: page.Offset(), OrderBy: page.OrderBy()})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	items := make([]dto.AdminUserResponse, 0, len(users))
	for i := range users {
		items = append(items, dto.NewAdminUserResponse(&users[i]))
	}
	response.Paginated(c, items, pagination.NewMeta(page, total))
}

// Get — GET /users/:id
func (h *UserHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	u, err := h.uc.Get(c.Request.Context(), id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewAdminUserResponse(u))
}

// Create — POST /users. Respons memuat temporary_password (sekali tampil).
func (h *UserHandler) Create(c *gin.Context) {
	var req dto.CreateUserRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	created, err := h.uc.Create(c.Request.Context(), usecase.CreateUserInput{
		Name: req.Name, Email: req.Email, Phone: req.Phone,
		Role: domain.Role(req.Role), WarehouseID: req.WarehouseID,
	})
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.Created(c, dto.CreateUserResponse{
		AdminUserResponse: dto.NewAdminUserResponse(created.User),
		TemporaryPassword: created.TemporaryPassword,
	})
}

// Update — PATCH /users/:id
func (h *UserHandler) Update(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.UpdateUserRequest
	if !httperr.BindJSON(c, &req) {
		return
	}

	in := usecase.UpdateUserInput{
		Name:           req.Name,
		SetPhone:       req.Phone.Set,
		Phone:          req.Phone.Value,
		SetWarehouseID: req.WarehouseID.Set,
		WarehouseID:    req.WarehouseID.Value,
		IsActive:       req.IsActive,
	}
	if req.Role != nil {
		role := domain.Role(*req.Role)
		in.Role = &role
	}

	u, err := h.uc.Update(c.Request.Context(), actor, id, in)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewAdminUserResponse(u))
}

// ResetPassword — POST /users/:id/reset-password. Respons memuat password sementara (sekali tampil).
func (h *UserHandler) ResetPassword(c *gin.Context) {
	actor, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	temp, err := h.uc.ResetPassword(c.Request.Context(), actor, id)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OKMessage(c, "Password direset; user wajib menggantinya saat login berikutnya", dto.ResetPasswordResponse{TemporaryPassword: temp})
}

func parseID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		httperr.BadQueryField(c, "id", "tidak valid")
		return 0, false
	}
	return id, true
}
