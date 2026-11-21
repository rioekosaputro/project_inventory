package dto

import (
	"bytes"
	"encoding/json"
	"time"

	"rental-inventory-api/internal/domain"
)

// Optional membedakan field yang TIDAK dikirim (Set=false) dari yang dikirim null
// (Set=true, Value=nil) — dibutuhkan PATCH untuk mengosongkan phone / warehouse_id.
type Optional[T any] struct {
	Set   bool
	Value *T
}

func (o *Optional[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		o.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

type CreateUserRequest struct {
	Name        string  `json:"name" binding:"required,max=100"`
	Email       string  `json:"email" binding:"required,email,max=120"`
	Phone       *string `json:"phone" binding:"omitempty,max=30"`
	Role        string  `json:"role" binding:"required"`
	WarehouseID *int64  `json:"warehouse_id" binding:"omitempty,gte=1"`
}

// UpdateUserRequest: semua field opsional (PATCH). phone & warehouse_id boleh null untuk mengosongkan.
type UpdateUserRequest struct {
	Name        *string          `json:"name" binding:"omitempty,min=1,max=100"`
	Phone       Optional[string] `json:"phone"`
	Role        *string          `json:"role"`
	WarehouseID Optional[int64]  `json:"warehouse_id"`
	IsActive    *bool            `json:"is_active"`
}

// ListUsersQuery: filter daftar user (page/limit/sort di-parse terpisah oleh pkg/pagination).
type ListUsersQuery struct {
	Q           string `form:"q" binding:"omitempty,max=100"`
	Role        string `form:"role"`
	IsActive    *bool  `form:"is_active"`
	WarehouseID *int64 `form:"warehouse_id" binding:"omitempty,gte=1"`
}

// AdminUserResponse: tampilan user untuk ADMIN. password_hash tidak pernah disertakan.
type AdminUserResponse struct {
	ID                 int64      `json:"id"`
	Name               string     `json:"name"`
	Email              string     `json:"email"`
	Phone              *string    `json:"phone"`
	Role               string     `json:"role"`
	WarehouseID        *int64     `json:"warehouse_id"`
	IsActive           bool       `json:"is_active"`
	MustChangePassword bool       `json:"must_change_password"`
	LastLoginAt        *time.Time `json:"last_login_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func NewAdminUserResponse(u *domain.User) AdminUserResponse {
	return AdminUserResponse{
		ID: u.ID, Name: u.Name, Email: u.Email, Phone: u.Phone, Role: string(u.Role),
		WarehouseID: u.WarehouseID, IsActive: u.IsActive, MustChangePassword: u.MustChangePassword,
		LastLoginAt: u.LastLoginAt, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
	}
}

// CreateUserResponse memuat password sementara yang hanya ditampilkan SEKALI.
type CreateUserResponse struct {
	AdminUserResponse
	TemporaryPassword string `json:"temporary_password"`
}

type ResetPasswordResponse struct {
	TemporaryPassword string `json:"temporary_password"`
}
