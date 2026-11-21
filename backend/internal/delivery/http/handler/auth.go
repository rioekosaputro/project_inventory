// Package handler berisi handler Gin yang tipis: parse → usecase → response/httperr.
package handler

import (
	"errors"
	nethttp "net/http"
	"time"

	"github.com/gin-gonic/gin"

	"rental-inventory-api/internal/delivery/http/dto"
	"rental-inventory-api/internal/delivery/http/httperr"
	"rental-inventory-api/internal/delivery/http/middleware"
	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/usecase"
	"rental-inventory-api/pkg/response"
)

// Cookie refresh token (plan Bagian 3.4): HttpOnly, SameSite=Lax, Path dibatasi ke /api/v1/auth.
const (
	RefreshCookieName = "erp_rt"
	refreshCookiePath = "/api/v1/auth"

	RefreshModeCookie = "cookie"
	RefreshModeBody   = "body"
)

// AuthSettings mengatur cara refresh token dikirim ke klien.
type AuthSettings struct {
	RefreshMode  string // cookie | body
	CookieSecure bool
}

type AuthHandler struct {
	uc       *usecase.AuthUsecase
	settings AuthSettings
}

func NewAuthHandler(uc *usecase.AuthUsecase, s AuthSettings) *AuthHandler {
	if s.RefreshMode == "" {
		s.RefreshMode = RefreshModeCookie
	}
	return &AuthHandler{uc: uc, settings: s}
}

// Login — POST /auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	sess, err := h.uc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	h.writeSession(c, sess)
}

// Refresh — POST /auth/refresh. Token dari cookie (mode cookie) atau body (mode body).
func (h *AuthHandler) Refresh(c *gin.Context) {
	raw, ok := h.readRefreshToken(c)
	if !ok {
		return
	}
	sess, err := h.uc.Refresh(c.Request.Context(), raw)
	if err != nil {
		// Token tidak sah/dicabut: hapus cookie agar klien tidak terus mengirim token mati.
		if errors.Is(err, domain.ErrInvalidToken) || errors.Is(err, domain.ErrAccountDisabled) {
			h.clearRefreshCookie(c)
		}
		httperr.Write(c, err)
		return
	}
	h.writeSession(c, sess)
}

// Logout — POST /auth/logout. Berbasis refresh token (bukan access token) agar tetap bisa
// keluar walau access token sudah kedaluwarsa. Idempoten.
func (h *AuthHandler) Logout(c *gin.Context) {
	raw, ok := h.readRefreshToken(c)
	if !ok {
		return
	}
	if err := h.uc.Logout(c.Request.Context(), raw); err != nil {
		httperr.Write(c, err)
		return
	}
	h.clearRefreshCookie(c)
	response.OKMessage(c, "Berhasil keluar", nil)
}

// Me — GET /auth/me
func (h *AuthHandler) Me(c *gin.Context) {
	p, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	profile, err := h.uc.Me(c.Request.Context(), p)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	response.OK(c, dto.NewProfileResponse(profile))
}

// ChangePassword — POST /auth/change-password
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	p, ok := middleware.PrincipalFrom(c)
	if !ok {
		httperr.Write(c, domain.ErrUnauthorized)
		return
	}
	var req dto.ChangePasswordRequest
	if !httperr.BindJSON(c, &req) {
		return
	}
	sess, err := h.uc.ChangePassword(c.Request.Context(), p, req.CurrentPassword, req.NewPassword)
	if err != nil {
		httperr.Write(c, err)
		return
	}
	h.writeSession(c, sess)
}

// ---------------------------------------------------------------- helpers

func (h *AuthHandler) writeSession(c *gin.Context, s *usecase.Session) {
	resp := dto.SessionResponse{
		AccessToken:     s.AccessToken,
		TokenType:       "Bearer",
		ExpiresIn:       int64(s.AccessExpiresAt.Sub(s.IssuedAt) / time.Second), // dari clock usecase, bukan jam sistem
		ProfileResponse: dto.NewProfileResponse(&s.Profile),
	}
	if h.settings.RefreshMode == RefreshModeBody {
		resp.RefreshToken = s.RefreshToken
	} else {
		h.setRefreshCookie(c, s.RefreshToken, int(s.RefreshExpiresAt.Sub(s.IssuedAt)/time.Second))
	}
	response.OK(c, resp)
}

// readRefreshToken membaca token sesuai mode. Gagal baca body → respons error sudah ditulis (ok=false).
// Token kosong bukan error di sini: usecase menjawab ErrInvalidToken / logout idempoten.
func (h *AuthHandler) readRefreshToken(c *gin.Context) (string, bool) {
	if h.settings.RefreshMode == RefreshModeBody {
		var req dto.RefreshRequest
		if c.Request.ContentLength == 0 {
			return "", true
		}
		if !httperr.BindJSON(c, &req) {
			return "", false
		}
		return req.RefreshToken, true
	}
	raw, err := c.Cookie(RefreshCookieName)
	if err != nil {
		return "", true
	}
	return raw, true
}

func (h *AuthHandler) setRefreshCookie(c *gin.Context, value string, maxAge int) {
	c.SetSameSite(nethttp.SameSiteLaxMode)
	c.SetCookie(RefreshCookieName, value, maxAge, refreshCookiePath, "", h.settings.CookieSecure, true)
}

func (h *AuthHandler) clearRefreshCookie(c *gin.Context) {
	if h.settings.RefreshMode == RefreshModeBody {
		return
	}
	c.SetSameSite(nethttp.SameSiteLaxMode)
	c.SetCookie(RefreshCookieName, "", -1, refreshCookiePath, "", h.settings.CookieSecure, true)
}
