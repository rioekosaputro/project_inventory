// Package middleware berisi middleware Gin. Urutan pemasangan (plan Bagian 7.5):
// RequestID → SecurityHeaders → Recover → Logger → CORS → (RateLimit → Auth → RBAC → Idempotency).
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"

	"github.com/gin-gonic/gin"

	"rental-inventory-api/pkg/reqctx"
)

// HeaderRequestID dibaca dari klien/proxy (bila valid) dan selalu dikirim balik.
const HeaderRequestID = "X-Request-ID"

// Hanya karakter aman: mencegah log injection / header injection lewat id buatan klien.
var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{8,64}$`)

// RequestID memastikan setiap request punya id unik, tersedia di context & header respons.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderRequestID)
		if !validRequestID.MatchString(id) {
			id = newRequestID()
		}
		c.Request = c.Request.WithContext(reqctx.WithRequestID(c.Request.Context(), id))
		c.Writer.Header().Set(HeaderRequestID, id)
		c.Next()
	}
}

func newRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand tidak gagal di platform yang didukung Go
	return hex.EncodeToString(b)
}
