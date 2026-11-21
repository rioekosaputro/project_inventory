// Package http berisi router dan lapisan delivery HTTP (Gin).
package http

import (
	"context"
	"log/slog"
	nethttp "net/http"
	"time"

	"github.com/gin-gonic/gin"

	"rental-inventory-api/config"
)

// ReadinessCheck memeriksa satu dependensi (mis. database). Error berarti belum siap.
type ReadinessCheck struct {
	Name  string
	Check func(ctx context.Context) error
}

const readyzTimeout = 3 * time.Second

// NewRouter membangun engine Gin. Middleware lengkap (RequestID, Logger, CORS,
// security headers) ditambahkan di T0.5.
func NewRouter(cfg *config.Config, logger *slog.Logger, checks ...ReadinessCheck) *gin.Engine {
	if cfg.AppEnv == config.EnvProduction {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())

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
