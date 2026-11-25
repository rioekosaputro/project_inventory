package usecase

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/xuri/excelize/v2"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/clock"
)

const (
	maxSerialHistoryLimit = 500
	maxExcelExportRows    = 10_000
)

// StockCard adalah kartu stok produk (opsional per rak/gudang/periode): saldo awal, daftar mutasi, saldo akhir.
type StockCard struct {
	ProductID      int64
	SKU            string
	ProductName    string
	Unit           string
	RackID         *int64
	RackCode       *string
	WarehouseID    *int64
	WarehouseCode  *string
	DateFrom       *string
	DateTo         *string
	OpeningBalance int
	TotalIn        int
	TotalOut       int
	ClosingBalance int
	Movements      []domain.LedgerRow
}

// SerialHistory mengembalikan detail satu unit SN beserta seluruh riwayat perubahan status/lokasinya.
func (u *InventoryUsecase) SerialHistory(ctx context.Context, actor domain.Principal, serialID int64) (*SerialScan, error) {
	s, err := u.repo.FindSerialByID(ctx, serialID)
	if err != nil {
		return nil, err
	}
	if s.WarehouseID != nil && !actor.CanAccessWarehouse(*s.WarehouseID) {
		return nil, domain.ErrForbiddenWarehouse
	}
	hist, err := u.repo.RecentSerialHistory(ctx, s.ID, maxSerialHistoryLimit)
	if err != nil {
		return nil, err
	}
	return &SerialScan{Serial: *s, History: hist}, nil
}

func validateOptionalDateRange(from, to *string) error {
	var errs []domain.FieldError
	var tFrom, tTo time.Time
	if from != nil && *from != "" {
		t, err := time.Parse(docDateLayout, *from)
		if err != nil {
			errs = append(errs, domain.FieldError{Field: "date_from", Message: "format tanggal harus YYYY-MM-DD"})
		} else {
			tFrom = t
		}
	}
	if to != nil && *to != "" {
		t, err := time.Parse(docDateLayout, *to)
		if err != nil {
			errs = append(errs, domain.FieldError{Field: "date_to", Message: "format tanggal harus YYYY-MM-DD"})
		} else {
			tTo = t
		}
	}
	if !tFrom.IsZero() && !tTo.IsZero() && tFrom.After(tTo) {
		errs = append(errs, domain.FieldError{Field: "date_to", Message: "tanggal akhir tidak boleh sebelum tanggal awal"})
	}
	if len(errs) > 0 {
		return &domain.ValidationError{Message: "Filter tanggal tidak valid", Errors: errs}
	}
	return nil
}

// Ledgers mengembalikan daftar buku besar mutasi stok (permission ledger.read).
func (u *InventoryUsecase) Ledgers(ctx context.Context, actor domain.Principal, f domain.LedgerFilter, p domain.PageParams) ([]domain.LedgerRow, int64, error) {
	if f.WarehouseID != nil && !actor.CanAccessWarehouse(*f.WarehouseID) {
		return nil, 0, domain.ErrForbiddenWarehouse
	}
	if err := validateOptionalDateRange(f.DateFrom, f.DateTo); err != nil {
		return nil, 0, err
	}
	f.ScopeWarehouseID = scopeOf(actor)
	return u.repo.ListLedgers(ctx, f, p)
}

// StockCard menghitung kartu stok untuk satu produk (saldo awal + mutasi kronologis + saldo akhir).
func (u *InventoryUsecase) StockCard(ctx context.Context, actor domain.Principal, f domain.StockCardFilter) (*StockCard, error) {
	if f.ProductID <= 0 {
		return nil, &domain.ValidationError{
			Message: "Parameter kartu stok tidak valid",
			Errors:  []domain.FieldError{{Field: "product_id", Message: "produk wajib dipilih"}},
		}
	}
	if f.WarehouseID != nil && !actor.CanAccessWarehouse(*f.WarehouseID) {
		return nil, domain.ErrForbiddenWarehouse
	}
	if err := validateOptionalDateRange(f.DateFrom, f.DateTo); err != nil {
		return nil, err
	}
	f.ScopeWarehouseID = scopeOf(actor)

	opening, err := u.repo.OpeningBalance(ctx, f)
	if err != nil {
		return nil, err
	}
	pid := f.ProductID
	rows, _, err := u.repo.ListLedgers(ctx, domain.LedgerFilter{
		ProductID:        &pid,
		RackID:           f.RackID,
		WarehouseID:      f.WarehouseID,
		ScopeWarehouseID: f.ScopeWarehouseID,
		DateFrom:         f.DateFrom,
		DateTo:           f.DateTo,
	}, domain.PageParams{Limit: maxExcelExportRows, Offset: 0, OrderBy: "l.occurred_at ASC, l.id ASC"})
	if err != nil {
		return nil, err
	}

	card := &StockCard{
		ProductID:      f.ProductID,
		RackID:         f.RackID,
		WarehouseID:    f.WarehouseID,
		DateFrom:       f.DateFrom,
		DateTo:         f.DateTo,
		OpeningBalance: opening,
		Movements:      rows,
	}
	for _, r := range rows {
		if card.SKU == "" {
			card.SKU = r.SKU
			card.ProductName = r.ProductName
			card.Unit = r.Unit
		}
		if f.RackID != nil && card.RackCode == nil {
			rc := r.RackCode
			card.RackCode = &rc
		}
		if f.WarehouseID != nil && card.WarehouseCode == nil {
			wc := r.WarehouseCode
			card.WarehouseCode = &wc
		}
		if r.Direction == "IN" {
			card.TotalIn += r.Qty
		} else {
			card.TotalOut += r.Qty
		}
	}
	card.ClosingBalance = card.OpeningBalance + card.TotalIn - card.TotalOut
	return card, nil
}

// ExportLedgersXLSX mengekspor buku besar stok terfilter ke file Excel (.xlsx).
func (u *InventoryUsecase) ExportLedgersXLSX(ctx context.Context, actor domain.Principal, f domain.LedgerFilter) ([]byte, error) {
	rows, _, err := u.Ledgers(ctx, actor, f, domain.PageParams{
		Limit: maxExcelExportRows, Offset: 0, OrderBy: "l.occurred_at DESC, l.id DESC",
	})
	if err != nil {
		return nil, err
	}
	xl := excelize.NewFile()
	defer func() { _ = xl.Close() }()
	const sheet = "Buku Besar Stok"
	_ = xl.SetSheetName("Sheet1", sheet)

	headers := []string{
		"Waktu (WIB)", "No. LPB", "Jenis Mutasi", "Arah",
		"Gudang", "Rak", "SKU", "Nama Produk",
		"Qty", "Saldo Sebelum", "Saldo Sesudah", "Petugas", "Catatan",
	}
	for col, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		_ = xl.SetCellValue(sheet, cell, h)
	}
	for i, r := range rows {
		rowNum := i + 2
		vals := []any{
			r.OccurredAt.In(clock.WIB).Format("2006-01-02 15:04:05"),
			r.LPBNumber,
			subTypeLabelID(r.MovementType),
			r.Direction,
			r.WarehouseCode,
			r.RackCode,
			r.SKU,
			r.ProductName,
			r.Qty,
			r.BalanceBefore,
			r.BalanceAfter,
			r.CreatedByName,
			derefStr(r.Notes),
		}
		for col, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(col+1, rowNum)
			_ = xl.SetCellValue(sheet, cell, v)
		}
	}
	var buf bytes.Buffer
	if err := xl.Write(&buf); err != nil {
		return nil, fmt.Errorf("export ledger xlsx: %w", err)
	}
	return buf.Bytes(), nil
}

// ExportStockCardXLSX mengekspor kartu stok produk ke file Excel (.xlsx).
func (u *InventoryUsecase) ExportStockCardXLSX(ctx context.Context, actor domain.Principal, f domain.StockCardFilter) ([]byte, error) {
	card, err := u.StockCard(ctx, actor, f)
	if err != nil {
		return nil, err
	}
	xl := excelize.NewFile()
	defer func() { _ = xl.Close() }()
	const sheet = "Kartu Stok"
	_ = xl.SetSheetName("Sheet1", sheet)

	_ = xl.SetCellValue(sheet, "A1", "Kartu Stok Produk")
	_ = xl.SetCellValue(sheet, "A2", "Produk")
	_ = xl.SetCellValue(sheet, "B2", fmt.Sprintf("%s — %s", card.SKU, card.ProductName))
	_ = xl.SetCellValue(sheet, "A3", "Saldo Awal")
	_ = xl.SetCellValue(sheet, "B3", card.OpeningBalance)
	_ = xl.SetCellValue(sheet, "C3", "Total Masuk")
	_ = xl.SetCellValue(sheet, "D3", card.TotalIn)
	_ = xl.SetCellValue(sheet, "E3", "Total Keluar")
	_ = xl.SetCellValue(sheet, "F3", card.TotalOut)
	_ = xl.SetCellValue(sheet, "G3", "Saldo Akhir")
	_ = xl.SetCellValue(sheet, "H3", card.ClosingBalance)

	headers := []string{"Waktu (WIB)", "No. LPB", "Jenis Mutasi", "Gudang", "Rak", "Masuk", "Keluar", "Saldo Rak", "Petugas", "Catatan"}
	for col, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 5)
		_ = xl.SetCellValue(sheet, cell, h)
	}
	for i, r := range card.Movements {
		rowNum := i + 6
		inQty, outQty := 0, 0
		if r.Direction == "IN" {
			inQty = r.Qty
		} else {
			outQty = r.Qty
		}
		vals := []any{
			r.OccurredAt.In(clock.WIB).Format("2006-01-02 15:04:05"),
			r.LPBNumber,
			subTypeLabelID(r.MovementType),
			r.WarehouseCode,
			r.RackCode,
			inQty,
			outQty,
			r.BalanceAfter,
			r.CreatedByName,
			derefStr(r.Notes),
		}
		for col, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(col+1, rowNum)
			_ = xl.SetCellValue(sheet, cell, v)
		}
	}
	var buf bytes.Buffer
	if err := xl.Write(&buf); err != nil {
		return nil, fmt.Errorf("export stock card xlsx: %w", err)
	}
	return buf.Bytes(), nil
}
