package usecase

import (
	"context"

	"rental-inventory-api/internal/domain"
)

type DashboardUsecase struct {
	repo domain.DashboardRepository
}

func NewDashboardUsecase(repo domain.DashboardRepository) *DashboardUsecase {
	return &DashboardUsecase{repo: repo}
}

func (u *DashboardUsecase) Summary(ctx context.Context, actor domain.Principal, warehouseID *int64) (*domain.WarehouseDashboardSummary, error) {
	if warehouseID != nil && !actor.CanAccessWarehouse(*warehouseID) {
		return nil, domain.ErrForbiddenWarehouse
	}
	effectiveWH := warehouseID
	if effectiveWH == nil {
		effectiveWH = scopeOf(actor)
	}
	return u.repo.WarehouseSummary(ctx, effectiveWH)
}
