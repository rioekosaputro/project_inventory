package usecase

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"rental-inventory-api/internal/domain"
)

const (
	maxScanCodeLen = 100 // = panjang kolom serial_number
	scanHistoryLen = 5   // plan Bagian 8.2: 5 histori terakhir pada hasil scan
)

// InventoryUsecase: pembacaan stok & unit SN (permission inventory.read, dipasang di router).
// Hanya membaca — perubahan stok tidak pernah lewat sini (CLAUDE.md invarian #1).
//
// Warehouse scoping: akun yang dibatasi ke satu gudang hanya melihat saldo di gudangnya; unit SN
// yang sedang tidak berada di gudang mana pun (disewa, dalam perjalanan, hilang, pensiun) tetap terlihat.
type InventoryUsecase struct {
	repo domain.InventoryRepository
}

func NewInventoryUsecase(repo domain.InventoryRepository) *InventoryUsecase {
	return &InventoryUsecase{repo: repo}
}

// SerialScan: hasil pencarian unit lewat asset tag / SN beserta histori terbarunya.
type SerialScan struct {
	Serial  domain.Serial
	History []domain.SerialHistory
}

func (u *InventoryUsecase) Balances(ctx context.Context, actor domain.Principal, f domain.BalanceFilter, p domain.PageParams) ([]domain.StockBalance, int64, error) {
	if f.WarehouseID != nil && !actor.CanAccessWarehouse(*f.WarehouseID) {
		return nil, 0, domain.ErrForbiddenWarehouse
	}
	f.ScopeWarehouseID = scopeOf(actor)
	return u.repo.ListBalances(ctx, f, p)
}

func (u *InventoryUsecase) ProductStock(ctx context.Context, actor domain.Principal, f domain.StockSummaryFilter, p domain.PageParams) ([]domain.ProductStock, int64, error) {
	if f.WarehouseID != nil && !actor.CanAccessWarehouse(*f.WarehouseID) {
		return nil, 0, domain.ErrForbiddenWarehouse
	}
	f.ScopeWarehouseID = scopeOf(actor)
	return u.repo.ListProductStock(ctx, f, p)
}

func (u *InventoryUsecase) Serials(ctx context.Context, actor domain.Principal, f domain.SerialFilter, p domain.PageParams) ([]domain.Serial, int64, error) {
	if f.WarehouseID != nil && !actor.CanAccessWarehouse(*f.WarehouseID) {
		return nil, 0, domain.ErrForbiddenWarehouse
	}
	if f.Status != nil && !f.Status.Valid() {
		return nil, 0, &domain.ValidationError{Message: "Filter tidak valid", Errors: []domain.FieldError{
			{Field: "status", Message: "tidak dikenal"},
		}}
	}
	f.ScopeWarehouseID = scopeOf(actor)
	return u.repo.ListSerials(ctx, f, p)
}

// ScanSerial mencari satu unit lewat asset tag atau serial number (dipakai scanner POS).
//   - asset tag unik; serial number bisa sama pada produk berbeda → ErrSerialAmbiguous bila lebih dari satu.
//   - unit yang berada di gudang lain dari akun yang dibatasi → ErrForbiddenWarehouse.
func (u *InventoryUsecase) ScanSerial(ctx context.Context, actor domain.Principal, rawCode string) (*SerialScan, error) {
	code := strings.TrimSpace(rawCode)
	if n := utf8.RuneCountInString(code); n < 1 || n > maxScanCodeLen {
		return nil, &domain.ValidationError{Message: "Kode tidak valid", Errors: []domain.FieldError{
			{Field: "code", Message: fmt.Sprintf("wajib diisi, maksimal %d karakter", maxScanCodeLen)},
		}}
	}

	found, err := u.repo.FindSerialsByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	switch len(found) {
	case 0:
		return nil, fmt.Errorf("%w: unit dengan kode '%s' tidak ditemukan", domain.ErrNotFound, code)
	case 1:
	default:
		return nil, domain.ErrSerialAmbiguous
	}

	s := found[0]
	if s.WarehouseID != nil && !actor.CanAccessWarehouse(*s.WarehouseID) {
		return nil, domain.ErrForbiddenWarehouse
	}
	history, err := u.repo.RecentSerialHistory(ctx, s.ID, scanHistoryLen)
	if err != nil {
		return nil, err
	}
	return &SerialScan{Serial: s, History: history}, nil
}
