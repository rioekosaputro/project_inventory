// Package dto berisi bentuk request/response HTTP. Entity domain tidak pernah diserialisasi
// langsung (mis. User membawa password_hash).
package dto

import (
	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/usecase"
)

// LoginRequest sengaja TIDAK memvalidasi format email: spasi di ujung (umum di keyboard HP)
// dinormalisasi usecase, dan format yang salah toh berakhir sebagai "kredensial salah".
type LoginRequest struct {
	Email    string `json:"email" binding:"required,max=120"`
	Password string `json:"password" binding:"required,max=72"`
}

// RefreshRequest hanya dipakai pada AUTH_REFRESH_MODE=body.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required,max=72"`
	NewPassword     string `json:"new_password" binding:"required,min=8,max=72"`
}

type UserResponse struct {
	ID                 int64   `json:"id"`
	Name               string  `json:"name"`
	Email              string  `json:"email"`
	Phone              *string `json:"phone"`
	Role               string  `json:"role"`
	WarehouseID        *int64  `json:"warehouse_id"`
	MustChangePassword bool    `json:"must_change_password"`
}

// ProfileResponse = GET /auth/me.
type ProfileResponse struct {
	User        UserResponse `json:"user"`
	Permissions []string     `json:"permissions"`
	DefaultHome string       `json:"default_home"`
}

// SessionResponse = login / refresh / change-password.
type SessionResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"` // detik
	// Batas sesi (detik, relatif agar tidak bergantung jam perangkat).
	SessionExpiresIn   int64 `json:"session_expires_in"`
	IdleTimeoutSeconds int64 `json:"idle_timeout_seconds"`
	// RefreshToken hanya terisi pada AUTH_REFRESH_MODE=body; mode cookie memakai cookie HttpOnly.
	RefreshToken string `json:"refresh_token,omitempty"`
	ProfileResponse
}

func NewUserResponse(u *domain.User) UserResponse {
	return UserResponse{
		ID: u.ID, Name: u.Name, Email: u.Email, Phone: u.Phone, Role: string(u.Role),
		WarehouseID: u.WarehouseID, MustChangePassword: u.MustChangePassword,
	}
}

func NewProfileResponse(p *usecase.Profile) ProfileResponse {
	perms := make([]string, 0, len(p.Permissions))
	for _, x := range p.Permissions {
		perms = append(perms, string(x))
	}
	return ProfileResponse{User: NewUserResponse(p.User), Permissions: perms, DefaultHome: p.DefaultHome}
}
