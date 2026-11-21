package middleware

import (
	"log/slog"
	nethttp "net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"rental-inventory-api/internal/delivery/http/httperr"
	"rental-inventory-api/pkg/reqctx"
	"rental-inventory-api/pkg/response"
)

// Recover menangkap panic handler, mencatat stack trace ke log (bukan ke klien),
// lalu menjawab 500 dalam format standar. Karena Logger berada di dalam Recover,
// panic dicatat di sini (dengan method, path, request_id).
func Recover(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == nethttp.ErrAbortHandler { // sinyal sah dari net/http: teruskan
				panic(rec)
			}
			logger.ErrorContext(c.Request.Context(), "panic pada handler",
				"panic", rec,
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"request_id", reqctx.RequestID(c.Request.Context()),
				"stack", string(debug.Stack()),
			)
			if c.Writer.Written() {
				c.Abort()
				return
			}
			response.Error(c, nethttp.StatusInternalServerError, httperr.CodeInternal, "Terjadi kesalahan pada server", nil)
		}()
		c.Next()
	}
}
