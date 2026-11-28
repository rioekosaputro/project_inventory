package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/usecase"
	"rental-inventory-api/pkg/clock"
)

type fakeTicketRepo struct {
	tickets    map[int64]*domain.MaintenanceTicket
	activities []domain.TicketActivity
	nextID     int64
}

func newFakeTicketRepo() *fakeTicketRepo {
	return &fakeTicketRepo{tickets: map[int64]*domain.MaintenanceTicket{}, nextID: 1}
}

func (f *fakeTicketRepo) List(_ context.Context, _ domain.TicketFilter, _ domain.PageParams) ([]domain.MaintenanceTicket, int64, error) {
	out := make([]domain.MaintenanceTicket, 0, len(f.tickets))
	for _, t := range f.tickets {
		out = append(out, *t)
	}
	return out, int64(len(out)), nil
}
func (f *fakeTicketRepo) FindByID(_ context.Context, id int64) (*domain.MaintenanceTicket, error) {
	t, ok := f.tickets[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *t
	return &cp, nil
}
func (f *fakeTicketRepo) FindByIDForUpdate(ctx context.Context, id int64) (*domain.MaintenanceTicket, error) {
	return f.FindByID(ctx, id)
}
func (f *fakeTicketRepo) Create(_ context.Context, t *domain.MaintenanceTicket) (*domain.MaintenanceTicket, error) {
	cp := *t
	cp.ID = f.nextID
	f.nextID++
	f.tickets[cp.ID] = &cp
	return &cp, nil
}
func (f *fakeTicketRepo) Update(_ context.Context, t *domain.MaintenanceTicket) error {
	cp := *t
	f.tickets[cp.ID] = &cp
	return nil
}
func (f *fakeTicketRepo) Assign(_ context.Context, id int64, techID int64, st domain.TicketStatus) error {
	t := f.tickets[id]
	t.AssignedTo = &techID
	t.Status = st
	return nil
}
func (f *fakeTicketRepo) UpdateStatus(_ context.Context, id int64, st domain.TicketStatus, res *string, resAt, clsAt *time.Time) error {
	t := f.tickets[id]
	t.Status = st
	if res != nil {
		t.ResolutionNotes = res
	}
	if resAt != nil {
		t.ResolvedAt = resAt
	}
	if clsAt != nil {
		t.ClosedAt = clsAt
	}
	return nil
}
func (f *fakeTicketRepo) ListActivities(_ context.Context, ticketID int64) ([]domain.TicketActivity, error) {
	var out []domain.TicketActivity
	for _, a := range f.activities {
		if a.TicketID == ticketID {
			out = append(out, a)
		}
	}
	return out, nil
}
func (f *fakeTicketRepo) InsertActivity(_ context.Context, a *domain.TicketActivity) (*domain.TicketActivity, error) {
	cp := *a
	cp.ID = int64(len(f.activities) + 1)
	f.activities = append(f.activities, cp)
	return &cp, nil
}
func (f *fakeTicketRepo) ListSpareparts(_ context.Context, _ int64) ([]domain.TicketSparepart, error) {
	return nil, nil
}
func (f *fakeTicketRepo) InsertSparepart(_ context.Context, sp *domain.TicketSparepart) (*domain.TicketSparepart, error) {
	cp := *sp
	cp.ID = 1
	cp.Status = "REQUESTED"
	return &cp, nil
}
func (f *fakeTicketRepo) FindSparepartByIDForUpdate(_ context.Context, _ int64) (*domain.TicketSparepart, error) {
	return &domain.TicketSparepart{ID: 1, TicketID: 1, ProductID: 5, Qty: 2, Status: "REQUESTED", IsBillable: true}, nil
}
func (f *fakeTicketRepo) MarkSparepartLPBCreated(_ context.Context, _, _ int64) error { return nil }
func (f *fakeTicketRepo) MarkSparepartIssuedByLPB(_ context.Context, _ int64) ([]domain.TicketSparepart, error) {
	return nil, nil
}
func (f *fakeTicketRepo) SetSwapSO(_ context.Context, ticketID, soID int64) error {
	t := f.tickets[ticketID]
	t.SwapSOID = &soID
	num := "SO/GLOBAL/202611/0001"
	t.SwapSONumber = &num
	return nil
}

type fakeSwapSORepo struct {
	domain.SalesOrderRepository
	created *domain.SalesOrder
	items   []domain.SalesOrderItem
}

func (f *fakeSwapSORepo) Create(_ context.Context, so *domain.SalesOrder) (*domain.SalesOrder, error) {
	cp := *so
	cp.ID = 777
	f.created = &cp
	return &cp, nil
}
func (f *fakeSwapSORepo) ReplaceItems(_ context.Context, _ int64, items []domain.SalesOrderItem) ([]domain.SalesOrderItem, error) {
	f.items = items
	return items, nil
}

func TestTicketUsecase_LifecycleAndActivities(t *testing.T) {
	ctx := context.Background()
	repo := newFakeTicketRepo()
	audit := &fakeTrackingAuditRepo{}
	now := time.Date(2026, 11, 28, 10, 0, 0, 0, time.UTC)
	uc := usecase.NewTicketUsecase(stubTxManager{}, repo, audit, fakeDocGen{}, clock.NewFake(now))

	actor := domain.Principal{UserID: 1, Role: domain.RoleBusinessAdmin}
	tech := domain.Principal{UserID: 7, Role: domain.RoleTechnician}

	// 1. Create ticket → default OPEN & SLA 8h untuk HIGH.
	det, err := uc.Create(ctx, actor, usecase.CreateTicketInput{
		CustomerID: 2, CustomerSiteID: 3, ProductSerialID: 4,
		Category: "PAPER_JAM", Priority: "HIGH", Title: "Printer sering paper jam",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.TicketOpen, det.Status)
	require.NotNil(t, det.SLADueAt)
	assert.Equal(t, now.Add(8*time.Hour), *det.SLADueAt)

	// 2. Assign → OPEN → ASSIGNED.
	det, err = uc.Assign(ctx, actor, det.ID, tech.UserID, nil)
	require.NoError(t, err)
	assert.Equal(t, domain.TicketAssigned, det.Status)

	// 3. Status ASSIGNED → IN_PROGRESS → WAITING_PART → RESOLVED → CLOSED.
	det, err = uc.ChangeStatus(ctx, tech, det.ID, "IN_PROGRESS", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, domain.TicketInProgress, det.Status)

	// 4. Activity: check-in GPS & komentar.
	lat, lng := -6.9932, 110.4203
	_, err = uc.AddActivity(ctx, tech, det.ID, usecase.AddTicketActivityInput{
		ActivityType: "VISIT_CHECKIN", Latitude: &lat, Longitude: &lng,
	})
	require.NoError(t, err)

	resNote := "Roller dibersihkan dan diganti"
	det, err = uc.ChangeStatus(ctx, tech, det.ID, "RESOLVED", &resNote, nil)
	require.NoError(t, err)
	assert.Equal(t, domain.TicketResolved, det.Status)
	require.NotNil(t, det.ResolvedAt)

	det, err = uc.ChangeStatus(ctx, actor, det.ID, "CLOSED", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, domain.TicketClosed, det.Status)

	// 5. Transisi dari CLOSED ditolak.
	_, err = uc.ChangeStatus(ctx, actor, det.ID, "IN_PROGRESS", nil, nil)
	assert.ErrorIs(t, err, domain.ErrInvalidStateTransition)
}

func TestTicketUsecase_EscalateSwap(t *testing.T) {
	ctx := context.Background()
	repo := newFakeTicketRepo()
	audit := &fakeTrackingAuditRepo{}
	now := time.Date(2026, 11, 28, 11, 0, 0, 0, time.UTC)
	contractID := int64(88)
	soRepo := &fakeSwapSORepo{}
	invRepo := &fakeQCInventoryRepo{
		unit: &domain.Serial{ID: 4, ProductID: 12, AssetTag: "AST-0004", ContractID: &contractID},
	}
	uc := usecase.NewTicketUsecase(stubTxManager{}, repo, audit, fakeDocGen{}, clock.NewFake(now)).
		WithSwap(soRepo, invRepo)

	tech := domain.Principal{UserID: 7, Role: domain.RoleTechnician}
	det, err := uc.Create(ctx, tech, usecase.CreateTicketInput{
		CustomerID: 2, CustomerSiteID: 3, ProductSerialID: 4,
		Category: "HARDWARE", Priority: "CRITICAL", Title: "Mainboard mati total",
	})
	require.NoError(t, err)

	det, err = uc.EscalateSwap(ctx, tech, det.ID, usecase.EscalateSwapInput{})
	require.NoError(t, err)
	require.NotNil(t, det.SwapSOID)
	assert.Equal(t, int64(777), *det.SwapSOID)
	require.NotNil(t, soRepo.created)
	assert.Equal(t, domain.OrderTypeUnitSwap, soRepo.created.OrderType)
	require.Len(t, soRepo.items, 2)
	assert.Equal(t, domain.DirectionIn, soRepo.items[0].Direction)
	assert.Equal(t, domain.DirectionOut, soRepo.items[1].Direction)

	// Eskalasi ulang tiket yang sama ditolak 409 Conflict.
	_, err = uc.EscalateSwap(ctx, tech, det.ID, usecase.EscalateSwapInput{})
	assert.ErrorIs(t, err, domain.ErrConflict)
}
