package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"rental-inventory-api/internal/delivery/http/httperr"
	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/clock"
	"rental-inventory-api/pkg/jwt"
)

const principalKey = "principal"

// TokenParser memverifikasi access token (diimplementasi *jwt.Manager).
type TokenParser interface {
	Parse(token string, now time.Time) (jwt.Claims, error)
}

// Authenticate mewajibkan header `Authorization: Bearer <access token>` yang valid, lalu
// menyimpan domain.Principal di context Gin. Akun yang masih wajib ganti password (klaim mcp)
// DITOLAK 403 di semua route kecuali yang memakai AuthenticateAllowPasswordChange.
func Authenticate(parser TokenParser, clk clock.Clock) gin.HandlerFunc {
	return authenticate(parser, clk, false)
}

// AuthenticateAllowPasswordChange sama seperti Authenticate tetapi mengizinkan akun yang
// wajib ganti password (untuk /auth/me dan /auth/change-password).
func AuthenticateAllowPasswordChange(parser TokenParser, clk clock.Clock) gin.HandlerFunc {
	return authenticate(parser, clk, true)
}

func authenticate(parser TokenParser, clk clock.Clock, allowMustChange bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok {
			unauthorized(c, domain.ErrUnauthorized)
			return
		}
		claims, err := parser.Parse(raw, clk.Now())
		if err != nil {
			unauthorized(c, domain.ErrInvalidToken)
			return
		}
		role := domain.Role(claims.Role)
		if !role.Valid() {
			unauthorized(c, domain.ErrInvalidToken)
			return
		}
		if claims.MustChangePassword && !allowMustChange {
			httperr.Write(c, domain.ErrPasswordChangeRequired)
			return
		}
		c.Set(principalKey, domain.Principal{
			UserID:             claims.UserID,
			Role:               role,
			WarehouseID:        claims.WarehouseID,
			MustChangePassword: claims.MustChangePassword,
		})
		c.Next()
	}
}

// PrincipalFrom mengambil identitas pemanggil yang dipasang Authenticate.
func PrincipalFrom(c *gin.Context) (domain.Principal, bool) {
	v, ok := c.Get(principalKey)
	if !ok {
		return domain.Principal{}, false
	}
	p, ok := v.(domain.Principal)
	return p, ok
}

func unauthorized(c *gin.Context, err error) {
	c.Header("WWW-Authenticate", `Bearer realm="api"`)
	httperr.Write(c, err)
}

// bearerToken mengambil token dari "Bearer <token>" (skema tidak peka huruf besar/kecil).
func bearerToken(header string) (string, bool) {
	scheme, token, found := strings.Cut(strings.TrimSpace(header), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}
