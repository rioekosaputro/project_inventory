package usecase_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/usecase"
)

type fakeQCInventoryRepo struct {
	domain.InventoryRepository
	unit    *domain.Serial
	history []domain.SerialHistory
}

func (f *fakeQCInventoryRepo) FindSerialByID(_ context.Context, _ int64) (*domain.Serial, error) {
	return f.unit, nil
}
func (f *fakeQCInventoryRepo) RecentSerialHistory(_ context.Context, _ int64, _ int) ([]domain.SerialHistory, error) {
	return f.history, nil
}

type fakeQCStockRepo struct {
	domain.StockRepository
	serials    []domain.SerialLock
	updatedSt  domain.SerialStatus
	updatedCnd domain.SerialCondition
	histories  []domain.SerialHistoryEntry
}

func (f *fakeQCStockRepo) LockSerials(_ context.Context, _ []int64) ([]domain.SerialLock, error) {
	return f.serials, nil
}
func (f *fakeQCStockRepo) UpdateSerialQC(_ context.Context, _ int64, st domain.SerialStatus, cnd domain.SerialCondition) error {
	f.updatedSt = st
	f.updatedCnd = cnd
	return nil
}
func (f *fakeQCStockRepo) InsertSerialHistory(_ context.Context, h domain.SerialHistoryEntry) error {
	f.histories = append(f.histories, h)
	return nil
}

func TestInventoryUsecase_QCSerial(t *testing.T) {
	ctx := context.Background()
	whID := int64(1)
	rackID := int64(10)
	otherWH := int64(2)
	headWH := domain.Principal{UserID: 5, Role: domain.RoleHeadWarehouse, WarehouseID: &whID}
	headOther := domain.Principal{UserID: 6, Role: domain.RoleHeadWarehouse, WarehouseID: &otherWH}

	t.Run("MAINTENANCE → AVAILABLE lulus QC", func(t *testing.T) {
		invRepo := &fakeQCInventoryRepo{unit: &domain.Serial{ID: 1, Status: domain.SerialAvailable}}
		stRepo := &fakeQCStockRepo{
			serials: []domain.SerialLock{{ID: 1, Status: domain.SerialMaintenance, WarehouseID: &whID, RackID: &rackID}},
		}
		audit := &fakeTrackingAuditRepo{}
		uc := usecase.NewInventoryUsecase(invRepo).WithQC(stubTxManager{}, stRepo, audit)

		unit, _, err := uc.QCSerial(ctx, headWH, 1, usecase.QCSerialInput{ToStatus: "AVAILABLE"})
		require.NoError(t, err)
		assert.Equal(t, domain.SerialAvailable, unit.Status)
		assert.Equal(t, domain.SerialAvailable, stRepo.updatedSt)
		assert.Equal(t, domain.ConditionGood, stRepo.updatedCnd)
		require.Len(t, stRepo.histories, 1)
		assert.Equal(t, "QC_PASSED", stRepo.histories[0].EventType)
		assert.Len(t, audit.entries, 1)
	})

	t.Run("MAINTENANCE → BROKEN gagal QC", func(t *testing.T) {
		invRepo := &fakeQCInventoryRepo{unit: &domain.Serial{ID: 1, Status: domain.SerialBroken}}
		stRepo := &fakeQCStockRepo{
			serials: []domain.SerialLock{{ID: 1, Status: domain.SerialMaintenance, WarehouseID: &whID, RackID: &rackID}},
		}
		uc := usecase.NewInventoryUsecase(invRepo).WithQC(stubTxManager{}, stRepo, &fakeTrackingAuditRepo{})

		_, _, err := uc.QCSerial(ctx, headWH, 1, usecase.QCSerialInput{ToStatus: "BROKEN"})
		require.NoError(t, err)
		assert.Equal(t, domain.SerialBroken, stRepo.updatedSt)
		assert.Equal(t, "QC_FAILED", stRepo.histories[0].EventType)
	})

	t.Run("transisi dari RENTED ditolak", func(t *testing.T) {
		stRepo := &fakeQCStockRepo{
			serials: []domain.SerialLock{{ID: 1, Status: domain.SerialRented}},
		}
		uc := usecase.NewInventoryUsecase(&fakeQCInventoryRepo{}).WithQC(stubTxManager{}, stRepo, &fakeTrackingAuditRepo{})
		_, _, err := uc.QCSerial(ctx, headWH, 1, usecase.QCSerialInput{ToStatus: "AVAILABLE"})
		assert.ErrorIs(t, err, domain.ErrInvalidStateTransition)
	})

	t.Run("gudang lain ditolak 403", func(t *testing.T) {
		stRepo := &fakeQCStockRepo{
			serials: []domain.SerialLock{{ID: 1, Status: domain.SerialMaintenance, WarehouseID: &whID, RackID: &rackID}},
		}
		uc := usecase.NewInventoryUsecase(&fakeQCInventoryRepo{}).WithQC(stubTxManager{}, stRepo, &fakeTrackingAuditRepo{})
		_, _, err := uc.QCSerial(ctx, headOther, 1, usecase.QCSerialInput{ToStatus: "AVAILABLE"})
		assert.ErrorIs(t, err, domain.ErrForbiddenWarehouse)
	})
}
