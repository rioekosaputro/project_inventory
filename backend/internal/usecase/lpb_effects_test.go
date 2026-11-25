package usecase

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/domain"
)

// fakeStock mencatat urutan penyentuhan saldo; tidak menyimpan apa pun.
type fakeStock struct {
	domain.StockRepository
	calls []string
	fail  bool
}

func (f *fakeStock) AddStock(_ context.Context, product, rack int64, qty int) (int, int, error) {
	f.calls = append(f.calls, fmt.Sprintf("IN p%d r%d", product, rack))
	return 0, qty, nil
}

func (f *fakeStock) RemoveStock(_ context.Context, product, rack int64, qty int) (int, int, error) {
	f.calls = append(f.calls, fmt.Sprintf("OUT p%d r%d", product, rack))
	if f.fail {
		return 0, 0, domain.ErrInsufficientStock
	}
	return qty, 0, nil
}

func (f *fakeStock) InsertLedger(context.Context, domain.LedgerEntry) error { return nil }
func (f *fakeStock) LockSerials(context.Context, []int64) ([]domain.SerialLock, error) {
	return nil, nil
}
func (f *fakeStock) MoveSerial(context.Context, int64, domain.SerialStatus, *int64, *int64) error {
	return nil
}
func (f *fakeStock) InsertSerial(context.Context, *domain.NewSerial) (int64, error) { return 0, nil }
func (f *fakeStock) InsertSerialHistory(context.Context, domain.SerialHistoryEntry) error {
	return nil
}

func move(lpb *domain.LPB, product, rack int64, dir string) stockMove {
	return stockMove{
		lpb:       lpb,
		item:      domain.LPBItem{ID: product*100 + rack, ProductID: product, SKU: fmt.Sprintf("P%d", product), RackCode: fmt.Sprintf("R%d", rack), Qty: 1},
		rackID:    rack,
		direction: dir,
	}
}

// Urutan saldo yang disentuh harus SELALU (produk, rak) naik, keluar sebelum masuk pada kunci yang sama —
// apa pun urutan item dan arah transfernya. Itulah yang mencegah deadlock antar dokumen (CLAUDE.md invarian #3).
func TestApplyMoves_TouchesBalancesInProductRackOrder(t *testing.T) {
	l := &domain.LPB{ID: 1, SubType: domain.LPBTransferOut}
	fs := &fakeStock{}
	u := &LPBUsecase{stock: fs}

	// Transfer rak 9 → rak 2 untuk produk 5, plus produk 3 rak 7 → rak 1: tersusun terbalik sengaja.
	moves := []stockMove{
		move(l, 5, 9, "OUT"), move(l, 5, 2, "IN"),
		move(l, 3, 7, "OUT"), move(l, 3, 1, "IN"),
		move(l, 5, 2, "OUT"),
	}
	_, err := u.applyMoves(context.Background(), domain.Principal{UserID: 1}, moves)
	require.NoError(t, err)

	assert.Equal(t, []string{
		"IN p3 r1", "OUT p3 r7",
		"OUT p5 r2", "IN p5 r2", "OUT p5 r9",
	}, fs.calls)
}

func TestApplyMoves_InsufficientStockNamesTheItem(t *testing.T) {
	l := &domain.LPB{ID: 1, SubType: domain.LPBDisposal}
	u := &LPBUsecase{stock: &fakeStock{fail: true}}

	_, err := u.applyMoves(context.Background(), domain.Principal{UserID: 1}, []stockMove{move(l, 4, 6, "OUT")})
	require.ErrorIs(t, err, domain.ErrInsufficientStock)
	assert.Contains(t, err.Error(), "P4")
	assert.Contains(t, err.Error(), "R6")
}

func TestUnitIDs_SortedAndRejectsUnregisteredScans(t *testing.T) {
	id := func(n int64) *int64 { return &n }
	ids, err := unitIDs(domain.LPBItem{Serials: []domain.LPBItemSerial{
		{SerialNumber: "A", ProductSerialID: id(9)}, {SerialNumber: "B", ProductSerialID: id(2)},
	}})
	require.NoError(t, err)
	assert.Equal(t, []int64{2, 9}, ids)

	_, err = unitIDs(domain.LPBItem{Serials: []domain.LPBItemSerial{{SerialNumber: "BARU"}}})
	require.ErrorIs(t, err, domain.ErrSerialNotValid)
}

func TestTransferDestination_SingleWarehouseOnly(t *testing.T) {
	i64 := func(n int64) *int64 { return &n }
	s := func(v string) *string { return &v }
	item := func(rack, wh int64, code string) domain.LPBItem {
		return domain.LPBItem{DestRackID: i64(rack), DestWarehouseID: i64(wh), DestWarehouseCode: s(code)}
	}

	wh, code, err := transferDestination([]domain.LPBItem{item(1, 7, "WH-B"), item(2, 7, "WH-B")})
	require.NoError(t, err)
	assert.Equal(t, int64(7), wh)
	assert.Equal(t, "WH-B", code)

	_, _, err = transferDestination([]domain.LPBItem{item(1, 7, "WH-B"), item(2, 8, "WH-C")})
	assert.Error(t, err, "dua gudang tujuan dalam satu dokumen")
	_, _, err = transferDestination([]domain.LPBItem{{}})
	assert.Error(t, err, "tanpa rak tujuan")
}
