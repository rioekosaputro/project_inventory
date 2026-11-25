package usecase

import (
	"context"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/clock"
)

type DashboardUsecase struct {
	repo domain.DashboardRepository
	clk  clock.Clock
}

func NewDashboardUsecase(repo domain.DashboardRepository, clk clock.Clock) *DashboardUsecase {
	return &DashboardUsecase{repo: repo, clk: clk}
}

func (u *DashboardUsecase) Summary(ctx context.Context, actor domain.Principal, warehouseID *int64) (*domain.WarehouseDashboardSummary, error) {
	if warehouseID != nil && !actor.CanAccessWarehouse(*warehouseID) {
		return nil, domain.ErrForbiddenWarehouse
	}
	effectiveWH := warehouseID
	if effectiveWH == nil {
		effectiveWH = scopeOf(actor)
	}
	summary, err := u.repo.WarehouseSummary(ctx, effectiveWH)
	if err != nil {
		return nil, err
	}

	// Metrik komersial (T2.11) hanya dimuat untuk role yang berhak membaca tagihan/kontrak
	// (MANAGEMENT, BUSINESS_ADMIN, ADMIN) — tidak membocorkan angka omzet ke staf gudang.
	if domain.Can(actor.Role, domain.PermInvoiceRead) || domain.Can(actor.Role, domain.PermContractRead) {
		comm, err := u.repo.CommercialSummary(ctx, todayDateWIB(u.clk.Now()))
		if err != nil {
			return nil, err
		}
		summary.Commercial = comm
	}
	return summary, nil
}
