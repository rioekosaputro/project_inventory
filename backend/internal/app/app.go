// Package app adalah composition root: merakit repository → usecase → handler → router.
// Dipakai oleh cmd/api dan oleh test integrasi HTTP agar wiring-nya identik.
package app

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"rental-inventory-api/config"
	apihttp "rental-inventory-api/internal/delivery/http"
	"rental-inventory-api/internal/delivery/http/handler"
	"rental-inventory-api/internal/repository/postgres"
	"rental-inventory-api/internal/usecase"
	"rental-inventory-api/pkg/clock"
	"rental-inventory-api/pkg/database"
	"rental-inventory-api/pkg/docnum"
	"rental-inventory-api/pkg/jwt"
	"rental-inventory-api/pkg/pdf"
)

// NewRouter merakit seluruh aplikasi. Gagal bila konfigurasi keamanan tidak layak
// (mis. JWT_ACCESS_SECRET lemah) — lebih baik berhenti saat start daripada jalan tidak aman.
func NewRouter(cfg *config.Config, logger *slog.Logger, pool *pgxpool.Pool, clk clock.Clock) (*gin.Engine, error) {
	tokens, err := jwt.NewManager(cfg.JWTAccessSecret, cfg.JWTAccessTTL)
	if err != nil {
		return nil, err
	}

	txManager := database.NewTxManager(pool)

	userRepo := postgres.NewUserRepo(pool)
	refreshRepo := postgres.NewRefreshTokenRepo(pool)
	auditRepo := postgres.NewAuditRepo(pool)

	authUC := usecase.NewAuthUsecase(
		txManager,
		userRepo,
		refreshRepo,
		postgres.NewLoginAttemptRepo(pool),
		auditRepo,
		tokens,
		clk,
		usecase.DefaultAuthConfig(cfg.JWTRefreshTTL),
	)
	warehouseRepo := postgres.NewWarehouseRepo(pool)
	rackRepo := postgres.NewRackRepo(pool)
	categoryRepo := postgres.NewCategoryRepo(pool)
	productRepo := postgres.NewProductRepo(pool)
	supplierRepo := postgres.NewSupplierRepo(pool)
	customerRepo := postgres.NewCustomerRepo(pool)
	contractRepo := postgres.NewContractRepo(pool)
	soRepo := postgres.NewSalesOrderRepo(pool)
	doRepo := postgres.NewDeliveryOrderRepo(pool)
	invoiceRepo := postgres.NewInvoiceRepo(pool)
	taxRepo := postgres.NewTaxSettingRepo(pool)

	userUC := usecase.NewUserUsecase(txManager, userRepo, refreshRepo, warehouseRepo, auditRepo, clk)
	var pdfClient pdf.Renderer = pdf.NewGotenbergClient(cfg.GotenbergURL, cfg.GotenbergUsername, cfg.GotenbergPassword)
	if cfg.IsLocal() && cfg.GotenbergURL == "http://localhost:3000" {
		if chrome := pdf.NewLocalChromeRenderer(); chrome.Available() {
			pdfClient = pdf.NewFallbackRenderer(pdfClient, chrome)
		}
	}
	inventoryRepo := postgres.NewInventoryRepo(pool)
	lpbRepo := postgres.NewLPBRepo(pool)
	stockRepo := postgres.NewStockRepo(pool)
	docGen := docnum.New(pool)

	billingUC := usecase.NewBillingUsecase(txManager, invoiceRepo, taxRepo, auditRepo, docGen, clk)
	jobUC := usecase.NewJobUsecase(postgres.NewJobRepo(pool), billingUC, auditRepo, clk, logger)

	deps := apihttp.Deps{
		Users:      handler.NewUserHandler(userUC),
		Warehouses: handler.NewWarehouseHandler(usecase.NewWarehouseUsecase(txManager, warehouseRepo, auditRepo)),
		Racks:      handler.NewRackHandler(usecase.NewRackUsecase(txManager, rackRepo, warehouseRepo, auditRepo).WithPDF(pdfClient)),
		Categories: handler.NewCategoryHandler(usecase.NewCategoryUsecase(txManager, categoryRepo, auditRepo)),
		Products:   handler.NewProductHandler(usecase.NewProductUsecase(txManager, productRepo, categoryRepo, auditRepo)),
		Suppliers:  handler.NewSupplierHandler(usecase.NewSupplierUsecase(txManager, supplierRepo, auditRepo)),
		Inventory:  handler.NewInventoryHandler(usecase.NewInventoryUsecase(inventoryRepo).WithPDF(pdfClient)),
		Opname: handler.NewOpnameHandler(usecase.NewOpnameUsecase(
			txManager, postgres.NewOpnameRepo(pool), lpbRepo, warehouseRepo, rackRepo, productRepo,
			inventoryRepo, stockRepo, auditRepo, docGen, clk,
		)),
		Dashboard: handler.NewDashboardHandler(usecase.NewDashboardUsecase(postgres.NewDashboardRepo(pool), clk)),
		Customers: handler.NewCustomerHandler(usecase.NewCustomerUsecase(txManager, customerRepo, auditRepo)),
		Contracts: handler.NewContractHandler(usecase.NewContractUsecase(
			txManager, contractRepo, customerRepo, productRepo, auditRepo, docGen, clk,
		)),
		SalesOrders: handler.NewSalesOrderHandler(usecase.NewSalesOrderUsecase(
			txManager, soRepo, customerRepo, contractRepo, productRepo, warehouseRepo,
			inventoryRepo, taxRepo, auditRepo, docGen, clk,
		)),
		DeliveryOrders: handler.NewDeliveryOrderHandler(usecase.NewDeliveryOrderUsecase(
			txManager, doRepo, rackRepo, stockRepo, lpbRepo, auditRepo, docGen, clk,
		)),
		Invoices: handler.NewInvoiceHandler(
			billingUC,
			usecase.NewInvoiceUsecase(txManager, invoiceRepo, auditRepo, clk).WithPDF(pdfClient),
		),
		Jobs: handler.NewJobHandler(jobUC),
		LPB: handler.NewLPBHandler(usecase.NewLPBUsecase(
			txManager, lpbRepo, warehouseRepo, rackRepo, productRepo, supplierRepo,
			inventoryRepo, stockRepo, postgres.NewSettingsRepo(pool),
			auditRepo, docGen, clk,
		).WithPDF(pdfClient, cfg.AppBaseURL).WithPOD(postgres.NewDeliveryPODRepo(pool))),
		Auth: handler.NewAuthHandler(authUC, handler.AuthSettings{
			RefreshMode:  cfg.AuthRefreshMode,
			CookieSecure: cfg.CookieSecure,
		}),
		Tokens:      tokens,
		Idempotency: postgres.NewIdempotencyRepo(pool),
		Clock:       clk,
	}

	return apihttp.NewRouter(cfg, logger, deps, apihttp.ReadinessCheck{
		Name:  "database",
		Check: pool.Ping,
	}), nil
}

// NewJobUsecase merakit JobUsecase beserta dependensinya agar cmd/worker berbagi wiring
// yang sama dengan cmd/api (T2.10).
func NewJobUsecase(pool *pgxpool.Pool, clk clock.Clock, logger *slog.Logger) *usecase.JobUsecase {
	txManager := database.NewTxManager(pool)
	auditRepo := postgres.NewAuditRepo(pool)
	invoiceRepo := postgres.NewInvoiceRepo(pool)
	taxRepo := postgres.NewTaxSettingRepo(pool)
	docGen := docnum.New(pool)
	billingUC := usecase.NewBillingUsecase(txManager, invoiceRepo, taxRepo, auditRepo, docGen, clk)
	return usecase.NewJobUsecase(postgres.NewJobRepo(pool), billingUC, auditRepo, clk, logger)
}
