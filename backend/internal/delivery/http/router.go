// Package http berisi router dan lapisan delivery HTTP (Gin).
package http

import (
	"context"
	"log/slog"
	nethttp "net/http"
	"time"

	"github.com/gin-gonic/gin"

	"rental-inventory-api/config"
	"rental-inventory-api/internal/delivery/http/httperr"
	"rental-inventory-api/internal/delivery/http/middleware"
)

// ReadinessCheck memeriksa satu dependensi (mis. database). Error berarti belum siap.
type ReadinessCheck struct {
	Name  string
	Check func(ctx context.Context) error
}

const readyzTimeout = 3 * time.Second

// NewRouter membangun engine Gin dengan middleware dasar (plan Bagian 7.5):
// RequestID → SecurityHeaders → Recover → Logger → CORS.
// RateLimit, Auth, RBAC, dan Idempotency dipasang per grup route di task berikutnya.
func NewRouter(cfg *config.Config, logger *slog.Logger, checks ...ReadinessCheck) *gin.Engine {
	if cfg.AppEnv == config.EnvProduction {
		gin.SetMode(gin.ReleaseMode)
	}
	httperr.Setup()

	r := gin.New()
	// Default Gin mempercayai semua proxy → X-Forwarded-For bisa dipalsukan (merusak rate limit per IP).
	// SetTrustedProxies hanya gagal untuk input tidak valid, yang sudah ditolak oleh config.
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		logger.Error("TRUSTED_PROXIES tidak valid, tidak ada proxy yang dipercaya", "error", err)
		_ = r.SetTrustedProxies(nil)
	}
	r.HandleMethodNotAllowed = true
	r.NoRoute(httperr.NotFound)
	r.NoMethod(httperr.MethodNotAllowed)

	r.Use(
		middleware.RequestID(),
		middleware.SecurityHeaders(cfg.AppEnv != config.EnvLocal),
		middleware.Recover(logger),
		middleware.Logger(logger),
		middleware.CORS(cfg.CORSAllowedOrigins),
	)

	r.GET("/healthz", healthz)
	r.GET("/readyz", readyz(logger, checks))

	return r
}

// healthz: liveness — proses hidup. Tidak menyentuh dependensi.
func healthz(c *gin.Context) {
	c.JSON(nethttp.StatusOK, gin.H{"status": "ok"})
}

// readyz: readiness — semua dependensi (DB) siap melayani.
func readyz(logger *slog.Logger, checks []ReadinessCheck) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), readyzTimeout)
		defer cancel()

		results := make(map[string]string, len(checks))
		ready := true
		for _, chk := range checks {
			if err := chk.Check(ctx); err != nil {
				ready = false
				results[chk.Name] = "down"
				logger.Warn("readiness check gagal", "check", chk.Name, "error", err)
				continue
			}
			results[chk.Name] = "up"
		}

		status, label := nethttp.StatusOK, "ready"
		if !ready {
			status, label = nethttp.StatusServiceUnavailable, "not_ready"
		}
		c.JSON(status, gin.H{"status": label, "checks": results})
	}
}
