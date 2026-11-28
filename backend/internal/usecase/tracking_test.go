package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/usecase"
	"rental-inventory-api/pkg/clock"
)

type stubTxManager struct{}

func (stubTxManager) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type fakeTrackingRepo struct {
	tasks     []domain.DriverDeliveryTask
	byEventID map[string]*domain.ShipmentTrackingLog
	inserted  []*domain.ShipmentTrackingLog
	updatedSt map[int64]domain.TrackingStatus
	coords    *domain.SiteCoords
	nextID    int64
}

func newFakeTrackingRepo() *fakeTrackingRepo {
	return &fakeTrackingRepo{
		byEventID: map[string]*domain.ShipmentTrackingLog{},
		updatedSt: map[int64]domain.TrackingStatus{},
		nextID:    1,
	}
}

func (f *fakeTrackingRepo) ListMyDeliveries(_ context.Context, _ int64) ([]domain.DriverDeliveryTask, error) {
	return f.tasks, nil
}
func (f *fakeTrackingRepo) FindByClientEventID(_ context.Context, id string) (*domain.ShipmentTrackingLog, error) {
	return f.byEventID[id], nil
}
func (f *fakeTrackingRepo) ListByLPB(_ context.Context, _ int64) ([]domain.ShipmentTrackingLog, error) {
	out := make([]domain.ShipmentTrackingLog, 0, len(f.inserted))
	for _, l := range f.inserted {
		out = append(out, *l)
	}
	return out, nil
}
func (f *fakeTrackingRepo) Insert(_ context.Context, log *domain.ShipmentTrackingLog) error {
	log.ID = f.nextID
	f.nextID++
	cp := *log
	f.inserted = append(f.inserted, &cp)
	if log.ClientEventID != nil {
		f.byEventID[*log.ClientEventID] = &cp
	}
	return nil
}
func (f *fakeTrackingRepo) UpdateLPBDeliveryStatus(_ context.Context, lpbID int64, st domain.TrackingStatus) error {
	f.updatedSt[lpbID] = st
	return nil
}
func (f *fakeTrackingRepo) SiteCoordinates(_ context.Context, _ int64) (*domain.SiteCoords, error) {
	return f.coords, nil
}

type fakeTrackingLPBRepo struct {
	domain.LPBRepository
	lpb   *domain.LPB
	items []domain.LPBItem
}

func (f *fakeTrackingLPBRepo) FindByIDForUpdate(_ context.Context, _ int64) (*domain.LPB, error) {
	return f.lpb, nil
}
func (f *fakeTrackingLPBRepo) FindByID(_ context.Context, _ int64) (*domain.LPB, error) {
	return f.lpb, nil
}
func (f *fakeTrackingLPBRepo) Items(_ context.Context, _ int64) ([]domain.LPBItem, error) {
	return f.items, nil
}

type fakeTrackingStockRepo struct {
	domain.StockRepository
	serials   []domain.SerialLock
	histories []domain.SerialHistoryEntry
}

func (f *fakeTrackingStockRepo) LockSerials(_ context.Context, _ []int64) ([]domain.SerialLock, error) {
	return f.serials, nil
}
func (f *fakeTrackingStockRepo) InsertSerialHistory(_ context.Context, e domain.SerialHistoryEntry) error {
	f.histories = append(f.histories, e)
	return nil
}

type fakeTrackingPODRepo struct {
	doCtx            *domain.DOContext
	delivered        bool
	pickedUp         bool
	rentedSerials    []int64
	placements       int
	closedPlacements int
	completedDO      bool
}

func (f *fakeTrackingPODRepo) MarkDelivered(_ context.Context, _ int64, _ time.Time, _ string, _ *string) error {
	f.delivered = true
	return nil
}
func (f *fakeTrackingPODRepo) MarkPickedUp(_ context.Context, _ int64) error {
	f.pickedUp = true
	return nil
}
func (f *fakeTrackingPODRepo) LookupDOContext(_ context.Context, _ int64) (*domain.DOContext, error) {
	return f.doCtx, nil
}
func (f *fakeTrackingPODRepo) FindContractItemRate(_ context.Context, _, _, _ int64) (*int64, decimal.Decimal, error) {
	id := int64(55)
	return &id, decimal.RequireFromString("500000"), nil
}
func (f *fakeTrackingPODRepo) MarkSerialRented(_ context.Context, serialID, _, _ int64, _ *int64) error {
	f.rentedSerials = append(f.rentedSerials, serialID)
	return nil
}
func (f *fakeTrackingPODRepo) InsertRentalPlacement(_ context.Context, _ int64, _ *int64, _, _ int64, _ decimal.Decimal, _ string, _ int64) error {
	f.placements++
	return nil
}
func (f *fakeTrackingPODRepo) MarkSerialPickupInTransit(_ context.Context, _ int64) error { return nil }
func (f *fakeTrackingPODRepo) CloseActivePlacement(_ context.Context, _ int64, _, _ string, _ int64) error {
	f.closedPlacements++
	return nil
}
func (f *fakeTrackingPODRepo) CompleteDOAndMaybeSO(_ context.Context, _ int64) error {
	f.completedDO = true
	return nil
}

type fakeTrackingAuditRepo struct{ entries []domain.AuditEntry }

func (f *fakeTrackingAuditRepo) Insert(_ context.Context, e domain.AuditEntry) error {
	f.entries = append(f.entries, e)
	return nil
}

func TestTrackingUsecase_CheckpointIdempotentAndIDOR(t *testing.T) {
	ctx := context.Background()
	driverID := int64(10)
	otherDriver := domain.Principal{UserID: 99, Role: domain.RoleDriver}
	driver := domain.Principal{UserID: driverID, Role: domain.RoleDriver}

	trRepo := newFakeTrackingRepo()
	lpbRepo := &fakeTrackingLPBRepo{
		lpb: &domain.LPB{
			ID: 1, Number: "LPB/01", Status: domain.LPBApproved, SubType: domain.LPBRentalDelivery,
			RequiresDelivery: true, DriverID: &driverID,
		},
	}
	uc := usecase.NewTrackingUsecase(
		stubTxManager{}, trRepo, lpbRepo, &fakeTrackingStockRepo{}, &fakeTrackingPODRepo{}, &fakeTrackingAuditRepo{},
		clock.NewFake(time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC)),
	)

	// Driver lain (IDOR) harus ditolak 403.
	_, _, err := uc.RecordCheckpoint(ctx, otherDriver, 1, usecase.CheckpointInput{
		Status: "IN_TRANSIT", Latitude: -6.9932, Longitude: 110.4203,
	})
	assert.ErrorIs(t, err, domain.ErrForbidden)

	// Checkpoint pertama dengan client_event_id sukses.
	evID := "11111111-1111-4111-8111-111111111111"
	log1, idem1, err := uc.RecordCheckpoint(ctx, driver, 1, usecase.CheckpointInput{
		Status: "IN_TRANSIT", Latitude: -6.9932, Longitude: 110.4203, ClientEventID: &evID,
	})
	require.NoError(t, err)
	assert.False(t, idem1)
	assert.Equal(t, domain.TrackingInTransit, trRepo.updatedSt[1])
	assert.Len(t, trRepo.inserted, 1)

	// Retry dengan client_event_id sama → idempoten, tidak insert duplikat.
	log2, idem2, err := uc.RecordCheckpoint(ctx, driver, 1, usecase.CheckpointInput{
		Status: "IN_TRANSIT", Latitude: -6.9932, Longitude: 110.4203, ClientEventID: &evID,
	})
	require.NoError(t, err)
	assert.True(t, idem2)
	assert.Equal(t, log1.ID, log2.ID)
	assert.Len(t, trRepo.inserted, 1)
}

func TestTrackingUsecase_DeliverWithDistanceWarningAndFail(t *testing.T) {
	ctx := context.Background()
	driverID := int64(10)
	driver := domain.Principal{UserID: driverID, Role: domain.RoleDriver}
	siteID := int64(5)
	contractID := int64(7)
	refType := "DELIVERY_ORDER"
	refID := int64(20)
	serialID := int64(101)

	siteLat, siteLng := -6.9932, 110.4203
	trRepo := newFakeTrackingRepo()
	trRepo.coords = &domain.SiteCoords{Latitude: &siteLat, Longitude: &siteLng}

	lpbRepo := &fakeTrackingLPBRepo{
		lpb: &domain.LPB{
			ID: 2, Number: "LPB/02", Status: domain.LPBApproved, SubType: domain.LPBRentalDelivery,
			RequiresDelivery: true, DriverID: &driverID, CustomerSiteID: &siteID,
			ReferenceType: &refType, ReferenceID: &refID,
		},
		items: []domain.LPBItem{
			{ID: 1, ProductID: 3, HasSerialNumber: true, Serials: []domain.LPBItemSerial{{ProductSerialID: &serialID}}},
		},
	}
	stockRepo := &fakeTrackingStockRepo{
		serials: []domain.SerialLock{{ID: serialID, Number: "SN-01", Status: domain.SerialInTransit}},
	}
	podRepo := &fakeTrackingPODRepo{
		doCtx: &domain.DOContext{DOID: refID, SOID: 30, OrderType: domain.OrderTypeNewDelivery, ContractID: &contractID, CustomerID: 4, SiteID: siteID},
	}
	auditRepo := &fakeTrackingAuditRepo{}
	uc := usecase.NewTrackingUsecase(
		stubTxManager{}, trRepo, lpbRepo, stockRepo, podRepo, auditRepo,
		clock.NewFake(time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC)),
	)

	// Koordinat ~1.1 km dari site → tetap sukses, tapi memunculkan DistanceWarning (> 500 m).
	evID := "22222222-2222-4222-8222-222222222222"
	res, err := uc.Deliver(ctx, driver, 2, usecase.DriverDeliverInput{
		ReceiverName:  "Budi Santoso",
		PODPhotoPath:  "pod/abc.jpg",
		Latitude:      -7.0032,
		Longitude:     110.4203,
		ClientEventID: &evID,
	})
	require.NoError(t, err)
	assert.False(t, res.Idempotent)
	assert.True(t, podRepo.delivered)
	assert.True(t, podRepo.completedDO)
	assert.Equal(t, []int64{serialID}, podRepo.rentedSerials)
	assert.Equal(t, 1, podRepo.placements)
	require.NotNil(t, res.DistanceWarning)
	assert.Contains(t, *res.DistanceWarning, "> 500 m")

	// Uji Fail pada LPB lain.
	lpbRepo.lpb = &domain.LPB{
		ID: 3, Number: "LPB/03", Status: domain.LPBApproved, SubType: domain.LPBRentalDelivery,
		RequiresDelivery: true, DriverID: &driverID,
	}
	failLog, idem, err := uc.Fail(ctx, driver, 3, usecase.DriverFailInput{
		Reason: "Kantor pelanggan tutup", Latitude: -6.9932, Longitude: 110.4203,
	})
	require.NoError(t, err)
	assert.False(t, idem)
	assert.Equal(t, domain.TrackingFailed, failLog.Status)
	assert.Equal(t, domain.TrackingFailed, trRepo.updatedSt[3])
}

func TestTrackingUsecase_PickupReturn(t *testing.T) {
	ctx := context.Background()
	driverID := int64(10)
	driver := domain.Principal{UserID: driverID, Role: domain.RoleDriver}
	serialID := int64(202)

	trRepo := newFakeTrackingRepo()
	lpbRepo := &fakeTrackingLPBRepo{
		lpb: &domain.LPB{
			ID: 4, Number: "LPB/RET/01", Status: domain.LPBDraft, SubType: domain.LPBRentalReturn,
			DriverID: &driverID,
		},
		items: []domain.LPBItem{
			{ID: 1, ProductID: 3, HasSerialNumber: true, Serials: []domain.LPBItemSerial{{ProductSerialID: &serialID}}},
		},
	}
	stockRepo := &fakeTrackingStockRepo{
		serials: []domain.SerialLock{{ID: serialID, Number: "SN-02", Status: domain.SerialRented}},
	}
	podRepo := &fakeTrackingPODRepo{}
	auditRepo := &fakeTrackingAuditRepo{}
	uc := usecase.NewTrackingUsecase(
		stubTxManager{}, trRepo, lpbRepo, stockRepo, podRepo, auditRepo,
		clock.NewFake(time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC)),
	)

	logEntry, idem, err := uc.Pickup(ctx, driver, 4, usecase.DriverPickupInput{
		Latitude: -6.9932, Longitude: 110.4203,
	})
	require.NoError(t, err)
	assert.False(t, idem)
	assert.Equal(t, domain.TrackingPickedUp, logEntry.Status)
	assert.True(t, podRepo.pickedUp)
	assert.Equal(t, 1, podRepo.closedPlacements)
	assert.Len(t, stockRepo.histories, 1)
}
