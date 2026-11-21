package middleware

import "github.com/gin-gonic/gin"

// SecurityHeaders memasang header keamanan standar untuk API JSON.
//   - CSP "default-src 'none'" + frame-ancestors: API tidak pernah dirender sebagai halaman.
//   - Cache-Control: no-store sebagai default (data bisnis/PII); handler (mis. PDF/gambar) boleh menimpa.
//   - HSTS hanya bila hsts=true (staging/production di belakang HTTPS), agar dev lokal via http tidak terkunci.
func SecurityHeaders(hsts bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("Cross-Origin-Resource-Policy", "same-site")
		h.Set("Cache-Control", "no-store")
		if hsts {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}
