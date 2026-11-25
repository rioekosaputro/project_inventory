package usecase

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/clock"
)

const (
	maxImportRows  = 2000
	maxImportBytes = 5 << 20 // 5 MB (plan Bagian 10)
)

type ImportInitialStockInput struct {
	WarehouseID int64
	DocDate     *string // YYYY-MM-DD (WIB); kosong = hari ini
	Notes       *string
	Filename    string
	FileBytes   []byte
}

// InitialStockTemplateXLSX membuat template Excel (.xlsx) untuk impor stok awal (T1.15).
func (u *LPBUsecase) InitialStockTemplateXLSX() ([]byte, error) {
	xl := excelize.NewFile()
	defer func() { _ = xl.Close() }()
	const dataSheet = "Stok Awal"
	_ = xl.SetSheetName("Sheet1", dataSheet)

	headers := []string{"sku", "rack_code", "qty", "serial_number", "condition", "unit_cost", "notes"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = xl.SetCellValue(dataSheet, cell, h)
	}
	examples := [][]any{
		{"TN-001", "A-01-01", 25, "", "", "150000.00", "Contoh barang non-SN (serial_number kosong)"},
		{"PRN-001", "A-01-01", 1, "SN-CONTOH-001", "GOOD", "2500000.00", "Contoh unit ber-SN (1 baris = 1 unit)"},
	}
	for r, row := range examples {
		for c, val := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			_ = xl.SetCellValue(dataSheet, cell, val)
		}
	}

	const guide = "Petunjuk"
	_, _ = xl.NewSheet(guide)
	notes := [][]any{
		{"Kolom", "Wajib", "Keterangan"},
		{"sku", "Ya", "Kode SKU produk yang sudah terdaftar dan aktif di master produk."},
		{"rack_code", "Ya", "Kode rak aktif milik gudang tujuan (tidak sedang dikunci opname)."},
		{"qty", "Ya (non-SN)", "Jumlah barang (>= 1). Untuk produk ber-SN boleh dikosongkan atau diisi 1."},
		{"serial_number", "Ya (produk SN)", "Nomor seri pabrikan. Kosongkan untuk barang non-SN. 1 baris = 1 unit SN."},
		{"condition", "Opsional", "GOOD (bawaan), MINOR_DEFECT, atau BROKEN (hanya di rak non-sellable)."},
		{"unit_cost", "Opsional", "Biaya satuan angka desimal (mis. 1500000 atau 1500000.50)."},
		{"notes", "Opsional", "Catatan tambahan untuk baris item."},
	}
	for r, row := range notes {
		for c, val := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+1)
			_ = xl.SetCellValue(guide, cell, val)
		}
	}

	var buf bytes.Buffer
	if err := xl.Write(&buf); err != nil {
		return nil, fmt.Errorf("buat template stok awal: %w", err)
	}
	return buf.Bytes(), nil
}

type rawImportRow struct {
	lineNum      int
	sku          string
	rackCode     string
	qtyStr       string
	serialNumber string
	condition    string
	unitCost     string
	notes        string
}

func parseImportRows(filename string, data []byte) ([]rawImportRow, error) {
	if len(data) == 0 {
		return nil, lpbInvalid([]domain.FieldError{{Field: "file", Message: "berkas kosong"}})
	}
	if len(data) > maxImportBytes {
		return nil, lpbInvalid([]domain.FieldError{{Field: "file", Message: "ukuran berkas maksimal 5 MB"}})
	}
	ext := strings.ToLower(filepath.Ext(filename))
	var table [][]string
	switch ext {
	case ".xlsx":
		xl, err := excelize.OpenReader(bytes.NewReader(data))
		if err != nil {
			return nil, lpbInvalid([]domain.FieldError{{Field: "file", Message: "berkas Excel (.xlsx) tidak dapat dibaca"}})
		}
		defer func() { _ = xl.Close() }()
		sheets := xl.GetSheetList()
		if len(sheets) == 0 {
			return nil, lpbInvalid([]domain.FieldError{{Field: "file", Message: "berkas Excel tidak memiliki sheet"}})
		}
		rows, err := xl.GetRows(sheets[0])
		if err != nil {
			return nil, lpbInvalid([]domain.FieldError{{Field: "file", Message: "gagal membaca baris Excel"}})
		}
		table = rows
	case ".csv":
		cleaned := bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}) // buang UTF-8 BOM
		r := csv.NewReader(bytes.NewReader(cleaned))
		r.FieldsPerRecord = -1
		// Dukung CSV dengan pemisah ';' (sering dihasilkan Excel locale Indonesia).
		if firstLine, _, _ := bytes.Cut(cleaned, []byte("\n")); bytes.Count(firstLine, []byte(";")) > bytes.Count(firstLine, []byte(",")) {
			r.Comma = ';'
		}
		for {
			rec, err := r.Read()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, lpbInvalid([]domain.FieldError{{Field: "file", Message: "format CSV tidak valid"}})
			}
			table = append(table, rec)
		}
	default:
		return nil, lpbInvalid([]domain.FieldError{{Field: "file", Message: "hanya mendukung format .xlsx atau .csv"}})
	}

	if len(table) < 2 {
		return nil, lpbInvalid([]domain.FieldError{{Field: "file", Message: "berkas minimal memiliki baris header dan 1 baris data"}})
	}
	colIdx := map[string]int{}
	for i, h := range table[0] {
		colIdx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	for _, reqCol := range []string{"sku", "rack_code"} {
		if _, ok := colIdx[reqCol]; !ok {
			return nil, lpbInvalid([]domain.FieldError{{Field: "file", Message: fmt.Sprintf("kolom wajib '%s' tidak ditemukan di header", reqCol)}})
		}
	}

	cell := func(row []string, name string) string {
		idx, ok := colIdx[name]
		if !ok || idx >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[idx])
	}

	out := make([]rawImportRow, 0, len(table)-1)
	for i := 1; i < len(table); i++ {
		row := table[i]
		r := rawImportRow{
			lineNum:      i + 1,
			sku:          strings.ToUpper(cell(row, "sku")),
			rackCode:     strings.ToUpper(cell(row, "rack_code")),
			qtyStr:       cell(row, "qty"),
			serialNumber: strings.ToUpper(cell(row, "serial_number")),
			condition:    strings.ToUpper(cell(row, "condition")),
			unitCost:     cell(row, "unit_cost"),
			notes:        cell(row, "notes"),
		}
		if r.sku == "" && r.rackCode == "" && r.qtyStr == "" && r.serialNumber == "" {
			continue // abaikan baris kosong
		}
		out = append(out, r)
		if len(out) > maxImportRows {
			return nil, lpbInvalid([]domain.FieldError{{Field: "file", Message: fmt.Sprintf("maksimal %d baris per impor", maxImportRows)}})
		}
	}
	if len(out) == 0 {
		return nil, lpbInvalid([]domain.FieldError{{Field: "file", Message: "tidak ada baris data yang dapat diimpor"}})
	}
	return out, nil
}

// ImportInitialStock mengimpor berkas Excel/CSV menjadi satu dokumen DRAFT LPB INITIAL_STOCK (T1.15).
// Tidak mengubah stok secara langsung — tetap mengikuti alur ajukan → persetujuan Kepala Gudang (invarian #1).
func (u *LPBUsecase) ImportInitialStock(ctx context.Context, actor domain.Principal, in ImportInitialStockInput) (*LPBDetail, error) {
	if !actor.CanAccessWarehouse(in.WarehouseID) {
		return nil, domain.ErrForbiddenWarehouse
	}
	rows, err := parseImportRows(in.Filename, in.FileBytes)
	if err != nil {
		return nil, err
	}

	today := u.clock.Now().In(clock.WIB).Format(docDateLayout)
	docDate := today
	if in.DocDate != nil && strings.TrimSpace(*in.DocDate) != "" {
		docDate = strings.TrimSpace(*in.DocDate)
		if _, err := timeParseDate(docDate); err != nil {
			return nil, lpbInvalid([]domain.FieldError{{Field: "doc_date", Message: "format tanggal harus YYYY-MM-DD"}})
		}
		if docDate > today {
			return nil, lpbInvalid([]domain.FieldError{{Field: "doc_date", Message: "tanggal dokumen tidak boleh di masa depan"}})
		}
	}

	// Muat master produk & rak gudang untuk validasi cepat.
	prods, _, err := u.products.List(ctx, domain.ProductFilter{}, domain.PageParams{Limit: 10000, Offset: 0, OrderBy: "p.id ASC"})
	if err != nil {
		return nil, err
	}
	prodBySKU := make(map[string]domain.Product, len(prods))
	for _, p := range prods {
		prodBySKU[strings.ToUpper(p.SKU)] = p
	}
	whID := in.WarehouseID
	racks, _, err := u.racks.List(ctx, domain.RackFilter{WarehouseID: &whID}, nil, domain.PageParams{Limit: 10000, Offset: 0, OrderBy: "r.id ASC"})
	if err != nil {
		return nil, err
	}
	rackByCode := make(map[string]domain.Rack, len(racks))
	for _, rk := range racks {
		rackByCode[strings.ToUpper(rk.Code)] = rk
	}

	type prKey struct{ prodID, rackID int64 }
	type groupedSerial struct {
		sn   string
		cond domain.SerialCondition
	}
	type groupedItem struct {
		product  domain.Product
		rack     domain.Rack
		qty      int
		unitCost *decimal.Decimal
		notes    *string
		serials  []groupedSerial
	}

	groups := map[prKey]*groupedItem{}
	seenSNInFile := map[string]int{} // "prodID:SN" -> lineNum
	var errs []domain.FieldError
	addErr := func(line int, col, msg string) {
		if len(errs) < 50 {
			errs = append(errs, domain.FieldError{Field: fmt.Sprintf("row[%d].%s", line, col), Message: msg})
		}
	}

	for _, r := range rows {
		prod, ok := prodBySKU[r.sku]
		if !ok {
			addErr(r.lineNum, "sku", fmt.Sprintf("SKU '%s' tidak ditemukan", r.sku))
			continue
		}
		if !prod.IsActive {
			addErr(r.lineNum, "sku", fmt.Sprintf("produk '%s' tidak aktif", r.sku))
			continue
		}
		rk, ok := rackByCode[r.rackCode]
		if !ok {
			addErr(r.lineNum, "rack_code", fmt.Sprintf("rak '%s' tidak ditemukan di gudang ini", r.rackCode))
			continue
		}
		if !rk.IsActive {
			addErr(r.lineNum, "rack_code", fmt.Sprintf("rak '%s' tidak aktif", r.rackCode))
			continue
		}
		if rk.LockedByOpnameID != nil {
			addErr(r.lineNum, "rack_code", fmt.Sprintf("rak '%s' sedang dikunci stock opname", r.rackCode))
			continue
		}

		var unitCost *decimal.Decimal
		if r.unitCost != "" {
			d, err := decimal.NewFromString(r.unitCost)
			if err != nil || d.IsNegative() || d.Exponent() < -2 {
				addErr(r.lineNum, "unit_cost", "biaya satuan harus angka desimal >= 0 (maks 2 desimal)")
				continue
			}
			unitCost = &d
		}

		k := prKey{prod.ID, rk.ID}
		g := groups[k]
		if g == nil {
			g = &groupedItem{product: prod, rack: rk, unitCost: unitCost, notes: cleanOptStr(r.notes)}
			groups[k] = g
		}

		if prod.HasSerialNumber {
			if r.serialNumber == "" {
				addErr(r.lineNum, "serial_number", "produk berserial wajib mengisi serial_number")
				continue
			}
			cond := domain.ConditionGood
			if r.condition != "" {
				cond = domain.SerialCondition(r.condition)
				if !cond.Valid() {
					addErr(r.lineNum, "condition", "kondisi harus GOOD, MINOR_DEFECT, atau BROKEN")
					continue
				}
			}
			if cond == domain.ConditionBroken && rk.IsSellable {
				addErr(r.lineNum, "condition", "unit BROKEN harus ditempatkan di rak non-sellable (karantina/workshop)")
				continue
			}
			snKey := fmt.Sprintf("%d:%s", prod.ID, r.serialNumber)
			if prevLine, dup := seenSNInFile[snKey]; dup {
				addErr(r.lineNum, "serial_number", fmt.Sprintf("serial number '%s' duplikat dengan baris %d", r.serialNumber, prevLine))
				continue
			}
			seenSNInFile[snKey] = r.lineNum

			// Cek apakah SN sudah terdaftar untuk produk ini atau dipakai LPB aktif lain.
			existing, err := u.inventory.FindSerialsByCode(ctx, r.serialNumber)
			if err != nil {
				return nil, err
			}
			already := false
			for _, ex := range existing {
				if ex.ProductID == prod.ID && strings.EqualFold(ex.SerialNumber, r.serialNumber) {
					already = true
					break
				}
			}
			if already {
				addErr(r.lineNum, "serial_number", fmt.Sprintf("serial number '%s' sudah terdaftar di sistem", r.serialNumber))
				continue
			}
			if otherNum, claimed, err := u.repo.SerialClaimedByOtherLPB(ctx, prod.ID, r.serialNumber, nil, 0); err != nil {
				return nil, err
			} else if claimed {
				addErr(r.lineNum, "serial_number", fmt.Sprintf("serial number '%s' sudah dipakai di LPB %s", r.serialNumber, otherNum))
				continue
			}

			g.serials = append(g.serials, groupedSerial{sn: r.serialNumber, cond: cond})
			g.qty = len(g.serials)
		} else {
			if r.serialNumber != "" {
				addErr(r.lineNum, "serial_number", "produk non-SN tidak boleh mengisi serial_number")
				continue
			}
			qty, err := strconv.Atoi(r.qtyStr)
			if err != nil || qty < 1 || qty > maxItemQty {
				addErr(r.lineNum, "qty", "jumlah wajib bilangan bulat >= 1")
				continue
			}
			g.qty += qty
		}
	}

	if len(errs) > 0 {
		return nil, &domain.ValidationError{Message: "Sebagian baris impor tidak valid", Errors: errs}
	}
	if len(groups) > maxLPBItems {
		return nil, lpbInvalid([]domain.FieldError{{Field: "file", Message: fmt.Sprintf("maksimal %d kombinasi (produk, rak) per dokumen LPB", maxLPBItems)}})
	}

	// Urutkan item agar deterministik.
	keys := make([]prKey, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].prodID != keys[j].prodID {
			return keys[i].prodID < keys[j].prodID
		}
		return keys[i].rackID < keys[j].rackID
	})

	var createdID int64
	err = u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		wh, err := u.warehouses.FindByIDForShare(ctx, in.WarehouseID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return lpbInvalid([]domain.FieldError{{Field: "warehouse_id", Message: "gudang tidak ditemukan"}})
			}
			return err
		}
		if !wh.IsActive {
			return lpbInvalid([]domain.FieldError{{Field: "warehouse_id", Message: "gudang tidak aktif"}})
		}

		now := u.clock.Now()
		num, err := u.docs.Next(ctx, "LPBI", wh.Code, now)
		if err != nil {
			return err
		}
		note := cleanOpt(in.Notes)
		if note == nil {
			s := fmt.Sprintf("Impor stok awal dari berkas %s", filepath.Base(in.Filename))
			note = &s
		}
		doc, err := u.repo.Create(ctx, &domain.LPB{
			Number:      num,
			Type:        domain.LPBInbound,
			SubType:     domain.LPBInitialStock,
			Status:      domain.LPBDraft,
			WarehouseID: in.WarehouseID,
			DocDate:     docDate,
			Notes:       note,
			CreatedBy:   actor.UserID,
		})
		if err != nil {
			return err
		}
		createdID = doc.ID

		for _, k := range keys {
			g := groups[k]
			itemID, err := u.repo.InsertItem(ctx, doc.ID, &domain.LPBItem{
				ProductID: g.product.ID,
				RackID:    g.rack.ID,
				Qty:       g.qty,
				UnitCost:  g.unitCost,
				Notes:     g.notes,
			})
			if err != nil {
				return err
			}
			for _, s := range g.serials {
				if _, err := u.repo.InsertItemSerial(ctx, &domain.LPBItemSerial{
					LPBItemID:    itemID,
					SerialNumber: s.sn,
					Condition:    s.cond,
					ScannedBy:    &actor.UserID,
					ScannedAt:    now,
				}); err != nil {
					return err
				}
			}
		}

		return u.audit.Insert(ctx, domain.AuditEntry{
			UserID: &actor.UserID, Action: domain.AuditLPBCreate, EntityType: domain.EntityLPB, EntityID: &doc.ID,
			After: map[string]any{"lpb_number": doc.Number, "sub_type": string(domain.LPBInitialStock), "source": "import", "rows": len(rows)},
		})
	})
	if err != nil {
		return nil, err
	}
	return u.Get(ctx, actor, createdID)
}

func cleanOptStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func timeParseDate(s string) (string, error) {
	if len(s) != 10 {
		return "", errors.New("invalid")
	}
	if _, err := time.Parse(docDateLayout, s); err != nil {
		return "", err
	}
	return s, nil
}
