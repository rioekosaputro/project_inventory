package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"rental-inventory-api/pkg/response"
)

// RequireCronSecret melindungi endpoint /internal/jobs/* dengan header
// Authorization: Bearer <CRON_SECRET> (Vercel Cron & ops manual, plan Bagian 8.6).
// Bila CRON_SECRET kosong, endpoint selalu menolak (503) agar tidak pernah terbuka tanpa autentikasi.
func RequireCronSecret(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(secret) == "" {
			response.Error(c, http.StatusServiceUnavailable, "CRON_NOT_CONFIGURED", "CRON_SECRET belum dikonfigurasi pada server", nil)
			c.Abort()
			return
		}
		authHeader := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(authHeader, prefix) {
			response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "Header Authorization Bearer CRON_SECRET wajib diisi", nil)
			c.Abort()
			return
		}
		token := strings.TrimPrefix(authHeader, prefix)
		if subtle.ConstantTimeCompare([]byte(token), []byte(secret)) != 1 {
			response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "CRON_SECRET tidak valid", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}
