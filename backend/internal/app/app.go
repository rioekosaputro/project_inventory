// Package app adalah composition root: merakit repository → usecase → handler → router.
// Dipakai oleh cmd/api dan oleh test integrasi HTTP agar wiring-nya identik.
package app

import (
	"log/slog"
	"os"
	"path/filepath"

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
	"rental-inventory-api/pkg/storage"
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
	var localChrome pdf.Renderer
	if cfg.IsLocal() && cfg.GotenbergURL == "http://localhost:3000" {
		if chrome := pdf.NewLocalChromeRenderer(); chrome.Available() {
			localChrome = chrome
		}
	}
	pdfClient := pdf.NewFallbackRenderer(
		pdf.NewGotenbergClient(cfg.GotenbergURL, cfg.GotenbergUsername, cfg.GotenbergPassword),
		localChrome,
	)
	inventoryRepo := postgres.NewInventoryRepo(pool)
	lpbRepo := postgres.NewLPBRepo(pool)
	stockRepo := postgres.NewStockRepo(pool)
	docGen := docnum.New(pool)

	billingUC := usecase.NewBillingUsecase(txManager, invoiceRepo, taxRepo, auditRepo, docGen, clk)
	jobUC := usecase.NewJobUsecase(postgres.NewJobRepo(pool), billingUC, auditRepo, clk, logger)

	uploader, localStorage, err := buildStorage(cfg, logger)
	if err != nil {
		return nil, err
	}
	var fileHandler *handler.FileHandler
	if localStorage != nil {
		fileHandler = handler.NewFileHandler(localStorage)
	}

	deliveryPODRepo := postgres.NewDeliveryPODRepo(pool)
	trackingUC := usecase.NewTrackingUsecase(
		txManager, postgres.NewTrackingRepo(pool), lpbRepo, stockRepo, deliveryPODRepo, auditRepo, clk,
	).WithStorage(uploader).WithDocGen(docGen)
	ticketUC := usecase.NewTicketUsecase(
		txManager, postgres.NewTicketRepo(pool), auditRepo, docGen, clk,
	).WithStorage(uploader)

	deps := apihttp.Deps{
		Users:      handler.NewUserHandler(userUC),
		Warehouses: handler.NewWarehouseHandler(usecase.NewWarehouseUsecase(txManager, warehouseRepo, auditRepo)),
		Racks:      handler.NewRackHandler(usecase.NewRackUsecase(txManager, rackRepo, warehouseRepo, auditRepo).WithPDF(pdfClient)),
		Categories: handler.NewCategoryHandler(usecase.NewCategoryUsecase(txManager, categoryRepo, auditRepo)),
		Products:   handler.NewProductHandler(usecase.NewProductUsecase(txManager, productRepo, categoryRepo, auditRepo)),
		Suppliers:  handler.NewSupplierHandler(usecase.NewSupplierUsecase(txManager, supplierRepo, auditRepo)),
		Inventory:  handler.NewInventoryHandler(usecase.NewInventoryUsecase(inventoryRepo).WithPDF(pdfClient).WithQC(txManager, stockRepo, auditRepo)),
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
		Jobs:     handler.NewJobHandler(jobUC),
		Files:    fileHandler,
		Tracking: handler.NewTrackingHandler(trackingUC, uploader),
		Tickets:  handler.NewTicketHandler(ticketUC, uploader),
		LPB: handler.NewLPBHandler(usecase.NewLPBUsecase(
			txManager, lpbRepo, warehouseRepo, rackRepo, productRepo, supplierRepo,
			inventoryRepo, stockRepo, postgres.NewSettingsRepo(pool),
			auditRepo, docGen, clk,
		).WithPDF(pdfClient, cfg.AppBaseURL).WithPOD(deliveryPODRepo)).WithStorage(uploader),
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

func buildStorage(cfg *config.Config, logger *slog.Logger) (storage.Uploader, *storage.LocalStorage, error) {
	if cfg.StorageDriver == "s3" {
		s3st, err := storage.NewS3(storage.S3Config{
			Endpoint:       cfg.S3Endpoint,
			Region:         cfg.S3Region,
			Bucket:         cfg.S3Bucket,
			AccessKey:      cfg.S3AccessKey,
			SecretKey:      cfg.S3SecretKey,
			ForcePathStyle: cfg.S3ForcePathStyle,
		})
		if err != nil {
			return nil, nil, err
		}
		return s3st, nil, nil
	}

	localCfg := storage.LocalConfig{
		BaseDir:       cfg.StorageLocalDir,
		BaseURL:       "", // relatif terhadap origin API (/api/v1/files) agar aman di balik proxy
		SigningSecret: cfg.JWTAccessSecret,
	}
	loc, err := storage.NewLocal(localCfg)
	if err != nil {
		// Di lingkungan serverless read-only (mis. Vercel sebelum S3 dikonfigurasi), fallback ke /tmp.
		fallbackDir := filepath.Join(os.TempDir(), "erp-storage")
		logger.Warn("STORAGE_LOCAL_DIR tidak dapat ditulis, fallback ke temp dir",
			"dir", cfg.StorageLocalDir, "fallback", fallbackDir, "error", err)
		localCfg.BaseDir = fallbackDir
		loc, err = storage.NewLocal(localCfg)
		if err != nil {
			return nil, nil, err
		}
	}
	return loc, loc, nil
}
