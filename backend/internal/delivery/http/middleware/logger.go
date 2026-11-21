package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"rental-inventory-api/pkg/reqctx"
)

// Logger mencatat satu baris access log (slog) per request.
// Query string TIDAK dicatat karena bisa memuat token/PII; path memakai pola route
// (mis. /lpb/:id) agar kardinalitas log rendah, plus path asli untuk investigasi.
// Level: 5xx=Error, 4xx=Warn, lainnya=Info (health check = Debug agar tidak berisik).
func Logger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		status := c.Writer.Status()
		level := slog.LevelInfo
		switch {
		case status >= 500:
			level = slog.LevelError
		case status >= 400:
			level = slog.LevelWarn
		case isHealthPath(c.Request.URL.Path):
			level = slog.LevelDebug
		}

		logger.Log(c.Request.Context(), level, "request",
			"request_id", reqctx.RequestID(c.Request.Context()),
			"method", c.Request.Method,
			"route", c.FullPath(),
			"path", c.Request.URL.Path,
			"status", status,
			"latency_ms", time.Since(start).Milliseconds(),
			"bytes", c.Writer.Size(),
			"ip", c.ClientIP(),
			"user_agent", c.Request.UserAgent(),
		)
	}
}

func isHealthPath(p string) bool { return p == "/healthz" || p == "/readyz" }
