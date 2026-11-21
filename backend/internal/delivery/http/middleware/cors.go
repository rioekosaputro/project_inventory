package middleware

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORS mengizinkan hanya origin di CORS_ALLOWED_ORIGINS. Credentials diizinkan karena
// refresh token memakai cookie HttpOnly — itulah sebabnya wildcard "*" ditolak di config.
// Di production FE & API satu origin (rewrite Vercel/Caddy), sehingga ini terutama untuk dev lokal.
//
// Daftar kosong = tidak ada origin lintas-situs yang diizinkan (tanpa header CORS sama sekali).
func CORS(allowedOrigins []string) gin.HandlerFunc {
	if len(allowedOrigins) == 0 {
		return func(c *gin.Context) { c.Next() }
	}
	return cors.New(cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type", "Idempotency-Key", HeaderRequestID},
		ExposeHeaders:    []string{HeaderRequestID},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	})
}
