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
	Auth  *handler.AuthHandler
	Users *handler.UserHandler

	// Master data (T1.2); nil = grup route tidak didaftarkan.
	Warehouses *handler.WarehouseHandler
	Racks      *handler.RackHandler
	Categories *handler.CategoryHandler
	Products   *handler.ProductHandler
	Suppliers  *handler.SupplierHandler

	// Inventory baca (T1.4).
	Inventory *handler.InventoryHandler

	// LPB draft (T1.5).
	LPB *handler.LPBHandler

	// Stock Opname (T1.12).
	Opname *handler.OpnameHandler

	// Dashboard (T1.14).
	Dashboard *handler.DashboardHandler

	// Komersial: Pelanggan & site (T2.2), Kontrak sewa (T2.3), Sales Order (T2.5).
	Customers   *handler.CustomerHandler
	Contracts   *handler.ContractHandler
	SalesOrders *handler.SalesOrderHandler

	// Idempotency untuk aksi kritis (approve LPB, ...). nil = middleware tidak dipasang.
	Idempotency domain.IdempotencyRepository

	Tokens middleware.TokenParser
	Clock  clock.Clock
}

const readyzTimeout = 3 * time.Second

// idempotencyTTL: lama respons aksi kritis disimpan untuk diputar ulang.
const idempotencyTTL = 24 * time.Hour

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
	registerMasterRoutes(api, deps)
	registerInventoryRoutes(api, deps)
	registerOpnameRoutes(api, deps)
	registerLPBRoutes(api, deps)
	registerCustomerRoutes(api, deps)
	registerContractRoutes(api, deps)
	registerSalesOrderRoutes(api, deps)
	if deps.Dashboard != nil {
		dg := api.Group("/dashboard",
			middleware.Authenticate(deps.Tokens, deps.Clock),
			middleware.RequirePermission(domain.PermDashboardRead),
		)
		dg.GET("/summary", deps.Dashboard.Summary)
	}

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

// registerMasterRoutes — plan Bagian 8.2 (matriks 5.1): baca master.read (produk: product.read,
// agar TECHNICIAN ikut), tulis master.write (rak: rack.write agar STAFF_WAREHOUSE ikut).
// Warehouse scoping dan aturan bisnis ditegakkan di usecase.
func registerMasterRoutes(api *gin.RouterGroup, deps Deps) {
	authn := middleware.Authenticate(deps.Tokens, deps.Clock)
	masterRead := middleware.RequirePermission(domain.PermMasterRead)
	masterWrite := middleware.RequirePermission(domain.PermMasterWrite)

	if h := deps.Warehouses; h != nil {
		g := api.Group("/warehouses", authn)
		g.GET("", masterRead, h.List)
		g.GET("/:id", masterRead, h.Get)
		g.POST("", masterWrite, h.Create)
		g.PATCH("/:id", masterWrite, h.Update)
	}
	if h := deps.Racks; h != nil {
		rackWrite := middleware.RequirePermission(domain.PermRackWrite)
		g := api.Group("/racks", authn)
		g.GET("", masterRead, h.List)
		g.GET("/:id", masterRead, h.Get)
		g.POST("", rackWrite, h.Create)
		g.POST("/labels/pdf", masterRead, h.LabelsPDF)
		g.PATCH("/:id", rackWrite, h.Update)
	}
	if h := deps.Categories; h != nil {
		g := api.Group("/categories", authn)
		g.GET("", masterRead, h.List)
		g.GET("/:id", masterRead, h.Get)
		g.POST("", masterWrite, h.Create)
		g.PATCH("/:id", masterWrite, h.Update)
	}
	if h := deps.Products; h != nil {
		productRead := middleware.RequirePermission(domain.PermProductRead)
		g := api.Group("/products", authn)
		g.GET("", productRead, h.List)
		g.GET("/:id", productRead, h.Get)
		g.POST("", masterWrite, h.Create)
		g.PATCH("/:id", masterWrite, h.Update)
	}
	if h := deps.Suppliers; h != nil {
		g := api.Group("/suppliers", authn)
		g.GET("", masterRead, h.List)
		g.GET("/:id", masterRead, h.Get)
		g.POST("", masterWrite, h.Create)
		g.PATCH("/:id", masterWrite, h.Update)
	}
}

// registerInventoryRoutes — plan Bagian 8.2: pembacaan saldo & unit (permission inventory.read).
// Warehouse scoping ditegakkan di usecase.
func registerInventoryRoutes(api *gin.RouterGroup, deps Deps) {
	h := deps.Inventory
	if h == nil {
		return
	}
	g := api.Group("/inventory",
		middleware.Authenticate(deps.Tokens, deps.Clock),
		middleware.RequirePermission(domain.PermInventoryRead),
	)
	g.GET("/balances", h.Balances)
	g.GET("/balances/summary", h.ProductStock)
	g.GET("/serials", h.Serials)
	g.GET("/serials/scan/:code", h.ScanSerial)
	g.GET("/serials/:id/history", h.SerialHistory)
	g.POST("/serials/labels/pdf", h.SerialLabelsPDF)

	// Buku besar & kartu stok (permission ledger.read: MGMT, STAFF, HEAD, ADMIN).
	lg := api.Group("/inventory",
		middleware.Authenticate(deps.Tokens, deps.Clock),
		middleware.RequirePermission(domain.PermLedgerRead),
	)
	lg.GET("/ledgers", h.Ledgers)
	lg.GET("/ledgers/export", h.ExportLedgers)
	lg.GET("/stock-card", h.StockCard)
}

// registerOpnameRoutes — plan Bagian 8.3: stock opname per rak.
func registerOpnameRoutes(api *gin.RouterGroup, deps Deps) {
	h := deps.Opname
	if h == nil {
		return
	}
	read := middleware.RequirePermission(domain.PermOpnameRead)
	create := middleware.RequirePermission(domain.PermOpnameCreate)
	finalize := middleware.RequirePermission(domain.PermOpnameFinalize)

	g := api.Group("/inventory/opnames", middleware.Authenticate(deps.Tokens, deps.Clock))
	g.GET("", read, h.List)
	g.POST("", create, h.Start)
	g.GET("/:id", read, h.Get)
	g.PUT("/:id/counts", create, h.SaveCounts)
	g.POST("/:id/scans", create, h.ScanSerial)
	g.POST("/:id/submit", create, h.Submit)
	g.POST("/:id/finalize", finalize, h.Finalize)
	g.POST("/:id/cancel", finalize, h.Cancel)
}

// registerLPBRoutes — plan Bagian 8.3 (tahap draft): baca lpb.read, ubah/scan/batal/revisi lpb.create,
// ajukan lpb.submit. Approve/reject menyusul (T1.6). Akses gudang dan kepemilikan draft di usecase.
func registerLPBRoutes(api *gin.RouterGroup, deps Deps) {
	h := deps.LPB
	if h == nil {
		return
	}
	read := middleware.RequirePermission(domain.PermLPBRead)
	create := middleware.RequirePermission(domain.PermLPBCreate)
	submit := middleware.RequirePermission(domain.PermLPBSubmit)
	approve := middleware.RequirePermission(domain.PermLPBApprove)
	printPerm := middleware.RequirePermission(domain.PermLPBPrint)

	g := api.Group("/lpb", middleware.Authenticate(deps.Tokens, deps.Clock))
	g.GET("", read, h.List)
	g.POST("", create, h.Create)
	g.GET("/approvals", approve, h.Approvals)
	g.GET("/import-initial-stock/template", create, h.InitialStockTemplate)
	g.POST("/import-initial-stock", create, h.ImportInitialStock)
	g.GET("/:id", read, h.Get)
	g.GET("/:id/pdf", printPerm, h.PDF)
	g.PATCH("/:id", create, h.Update)
	g.POST("/:id/items/:itemId/serials", create, h.AddSerial)
	g.DELETE("/:id/items/:itemId/serials/:serialId", create, h.RemoveSerial)
	g.POST("/:id/submit", submit, h.Submit)
	g.POST("/:id/cancel", create, h.Cancel)
	g.POST("/:id/revise", create, h.Revise)
	if deps.Idempotency != nil {
		g.POST("/:id/approve", approve, middleware.Idempotency(deps.Idempotency, idempotencyTTL), h.Approve)
	}
	g.POST("/:id/reject", approve, h.Reject)

	// Verifikasi QR publik (plan Bagian 7.6 & 8.3): tanpa autentikasi.
	api.GET("/public/lpb/verify/:token", h.VerifyPublic)
}

// registerCustomerRoutes — plan Bagian 8.4 (T2.2): baca customer.read, tulis customer.write.
func registerCustomerRoutes(api *gin.RouterGroup, deps Deps) {
	h := deps.Customers
	if h == nil {
		return
	}
	authn := middleware.Authenticate(deps.Tokens, deps.Clock)
	read := middleware.RequirePermission(domain.PermCustomerRead)
	write := middleware.RequirePermission(domain.PermCustomerWrite)

	g := api.Group("/customers", authn)
	g.GET("", read, h.List)
	g.POST("", write, h.Create)
	g.GET("/:id", read, h.Get)
	g.PATCH("/:id", write, h.Update)
	g.GET("/:id/sites", read, h.ListSites)
	g.POST("/:id/sites", write, h.CreateSite)
	g.GET("/:id/assets", read, h.ListAssets)

	sites := api.Group("/customer-sites", authn)
	sites.PATCH("/:id", write, h.UpdateSite)
}

// registerContractRoutes — plan Bagian 8.4 (T2.3): baca contract.read, tulis contract.write,
// aktivasi/terminasi/perpanjang contract.approve.
func registerContractRoutes(api *gin.RouterGroup, deps Deps) {
	h := deps.Contracts
	if h == nil {
		return
	}
	authn := middleware.Authenticate(deps.Tokens, deps.Clock)
	read := middleware.RequirePermission(domain.PermContractRead)
	write := middleware.RequirePermission(domain.PermContractWrite)
	approve := middleware.RequirePermission(domain.PermContractApprove)

	g := api.Group("/contracts", authn)
	g.GET("", read, h.List)
	g.POST("", write, h.Create)
	g.GET("/:id", read, h.Get)
	g.PATCH("/:id", write, h.Update)
	g.POST("/:id/activate", approve, h.Activate)
	g.POST("/:id/terminate", approve, h.Terminate)
	g.POST("/:id/renew", approve, h.Renew)
}

// registerSalesOrderRoutes — plan Bagian 8.4 (T2.5): so.read, so.write, so.approve, do.create.
func registerSalesOrderRoutes(api *gin.RouterGroup, deps Deps) {
	h := deps.SalesOrders
	if h == nil {
		return
	}
	authn := middleware.Authenticate(deps.Tokens, deps.Clock)
	read := middleware.RequirePermission(domain.PermSORead)
	write := middleware.RequirePermission(domain.PermSOWrite)
	approve := middleware.RequirePermission(domain.PermSOApprove)
	doCreate := middleware.RequirePermission(domain.PermDOCreate)

	g := api.Group("/sales-orders", authn)
	g.GET("", read, h.List)
	g.POST("", write, h.Create)
	g.GET("/:id", read, h.Get)
	g.PATCH("/:id", write, h.Update)
	g.POST("/:id/confirm", approve, h.Confirm)
	g.POST("/:id/cancel", approve, h.Cancel)
	g.POST("/:id/delivery-orders", doCreate, h.IssueDeliveryOrders)
}
