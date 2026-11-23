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

	userUC := usecase.NewUserUsecase(txManager, userRepo, refreshRepo, warehouseRepo, auditRepo, clk)

	deps := apihttp.Deps{
		Users:      handler.NewUserHandler(userUC),
		Warehouses: handler.NewWarehouseHandler(usecase.NewWarehouseUsecase(txManager, warehouseRepo, auditRepo)),
		Racks:      handler.NewRackHandler(usecase.NewRackUsecase(txManager, rackRepo, warehouseRepo, auditRepo)),
		Categories: handler.NewCategoryHandler(usecase.NewCategoryUsecase(txManager, categoryRepo, auditRepo)),
		Products:   handler.NewProductHandler(usecase.NewProductUsecase(txManager, productRepo, categoryRepo, auditRepo)),
		Suppliers:  handler.NewSupplierHandler(usecase.NewSupplierUsecase(txManager, supplierRepo, auditRepo)),
		Inventory:  handler.NewInventoryHandler(usecase.NewInventoryUsecase(postgres.NewInventoryRepo(pool))),
		LPB: handler.NewLPBHandler(usecase.NewLPBUsecase(
			txManager, postgres.NewLPBRepo(pool), warehouseRepo, rackRepo, productRepo, supplierRepo,
			postgres.NewInventoryRepo(pool), auditRepo, docnum.New(pool), clk,
		)),
		Auth: handler.NewAuthHandler(authUC, handler.AuthSettings{
			RefreshMode:  cfg.AuthRefreshMode,
			CookieSecure: cfg.CookieSecure,
		}),
		Tokens: tokens,
		Clock:  clk,
	}

	return apihttp.NewRouter(cfg, logger, deps, apihttp.ReadinessCheck{
		Name:  "database",
		Check: pool.Ping,
	}), nil
}
