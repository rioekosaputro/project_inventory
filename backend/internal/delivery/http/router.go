// Package http berisi router dan lapisan delivery HTTP (Gin).
package http

import (
	"context"
	"log/slog"
	nethttp "net/http"
	"time"

	"github.com/gin-gonic/gin"

	"rental-inventory-api/config"
	"rental-inventory-api/internal/delivery/http/handler"
	"rental-inventory-api/internal/delivery/http/httperr"
	"rental-inventory-api/internal/delivery/http/middleware"
	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/clock"
)

// ReadinessCheck memeriksa satu dependensi (mis. database). Error berarti belum siap.
type ReadinessCheck struct {
	Name  string
	Check func(ctx context.Context) error
}

// Deps adalah dependensi handler. Field nil = grup route-nya tidak didaftarkan
// (berguna untuk test yang hanya menguji fondasi).
type Deps struct {
	Auth   *handler.AuthHandler
	Users  *handler.UserHandler
	Tokens middleware.TokenParser
	Clock  clock.Clock
}

const readyzTimeout = 3 * time.Second

// NewRouter membangun engine Gin dengan middleware dasar (plan Bagian 7.5):
// RequestID → SecurityHeaders → Recover → Logger → CORS, lalu per route:
// Authenticate → RequirePermission (→ Idempotency pada endpoint kritis).
func NewRouter(cfg *config.Config, logger *slog.Logger, deps Deps, checks ...ReadinessCheck) *gin.Engine {
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

	api := r.Group("/api/v1")
	registerAuthRoutes(api, deps)
	registerUserRoutes(api, deps)

	return r
}

// registerUserRoutes — plan Bagian 8.1 (hanya ADMIN: permission user.manage).
func registerUserRoutes(api *gin.RouterGroup, deps Deps) {
	if deps.Users == nil {
		return
	}
	users := api.Group("/users",
		middleware.Authenticate(deps.Tokens, deps.Clock),
		middleware.RequirePermission(domain.PermUserManage),
	)
	users.GET("", deps.Users.List)
	users.POST("", deps.Users.Create)
	users.GET("/:id", deps.Users.Get)
	users.PATCH("/:id", deps.Users.Update)
	users.POST("/:id/reset-password", deps.Users.ResetPassword)
}

// registerAuthRoutes — plan Bagian 8.1.
func registerAuthRoutes(api *gin.RouterGroup, deps Deps) {
	if deps.Auth == nil {
		return
	}
	auth := api.Group("/auth")
	auth.POST("/login", deps.Auth.Login)
	auth.POST("/refresh", deps.Auth.Refresh)
	auth.POST("/logout", deps.Auth.Logout)

	// Akun yang wajib ganti password tetap boleh melihat profil & mengganti password.
	loggedIn := auth.Group("", middleware.AuthenticateAllowPasswordChange(deps.Tokens, deps.Clock))
	loggedIn.GET("/me", deps.Auth.Me)
	loggedIn.POST("/change-password", deps.Auth.ChangePassword)
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
