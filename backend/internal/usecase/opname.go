package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/clock"
)

const (
	maxOpnameRacks = 100
	opnameRefType  = "STOCK_OPNAME"
)

type OpnameUsecase struct {
	tx         domain.TxManager
	repo       domain.OpnameRepository
	lpbRepo    domain.LPBRepository
	warehouses domain.WarehouseRepository
	racks      domain.RackRepository
	products   domain.ProductRepository
	inventory  domain.InventoryRepository
	stock      domain.StockRepository
	audit      domain.AuditRepository
	docs       domain.DocNumberGenerator
	clock      clock.Clock
}

func NewOpnameUsecase(
	tx domain.TxManager,
	repo domain.OpnameRepository,
	lpbRepo domain.LPBRepository,
	warehouses domain.WarehouseRepository,
	racks domain.RackRepository,
	products domain.ProductRepository,
	inventory domain.InventoryRepository,
	stock domain.StockRepository,
	audit domain.AuditRepository,
	docs domain.DocNumberGenerator,
	clk clock.Clock,
) *OpnameUsecase {
	return &OpnameUsecase{
		tx: tx, repo: repo, lpbRepo: lpbRepo, warehouses: warehouses, racks: racks,
		products: products, inventory: inventory, stock: stock, audit: audit, docs: docs, clock: clk,
	}
}

type StartOpnameInput struct {
	WarehouseID  int64
	RackIDs      []int64
	IsBlindCount bool
	Notes        *string
}

type OpnameCountInput struct {
	ProductID  int64
	RackID     int64
	CountedQty int
	Notes      *string
}

type OpnameDetail struct {
	Opname        domain.Opname
	Racks         []domain.OpnameRack
	Items         []domain.OpnameItem
	Serials       []domain.OpnameSerial
	HideSystemQty bool
}

func (u *OpnameUsecase) List(ctx context.Context, actor domain.Principal, f domain.OpnameFilter, p domain.PageParams) ([]domain.OpnameListItem, int64, error) {
	if f.WarehouseID != nil && !actor.CanAccessWarehouse(*f.WarehouseID) {
		return nil, 0, domain.ErrForbiddenWarehouse
	}
	f.ScopeWarehouseID = scopeOf(actor)
	return u.repo.List(ctx, f, p)
}

func (u *OpnameUsecase) Get(ctx context.Context, actor domain.Principal, id int64) (*OpnameDetail, error) {
	o, err := u.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !actor.CanAccessWarehouse(o.WarehouseID) {
		return nil, domain.ErrForbiddenWarehouse
	}
	return u.loadDetail(ctx, actor, o)
}

func (u *OpnameUsecase) loadDetail(ctx context.Context, actor domain.Principal, o *domain.Opname) (*OpnameDetail, error) {
	racks, err := u.repo.Racks(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	items, err := u.repo.Items(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	serials, err := u.repo.Serials(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	hide := o.IsBlindCount && o.Status == domain.OpnameInProgress && !domain.Can(actor.Role, domain.PermOpnameFinalize)
	return &OpnameDetail{Opname: *o, Racks: racks, Items: items, Serials: serials, HideSystemQty: hide}, nil
}

// Start memulai stock opname: mengunci rak terpilih, mengambil snapshot saldo non-SN dan unit SN.
func (u *OpnameUsecase) Start(ctx context.Context, actor domain.Principal, in StartOpnameInput) (*OpnameDetail, error) {
	if !actor.CanAccessWarehouse(in.WarehouseID) {
		return nil, domain.ErrForbiddenWarehouse
	}
	rackIDs, err := dedupeSortedIDs(in.RackIDs, "rack_ids", maxOpnameRacks)
	if err != nil {
		return nil, err
	}

	var created *domain.Opname
	err = u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		wh, err := u.warehouses.FindByIDForShare(ctx, in.WarehouseID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return &domain.ValidationError{Message: "Data opname tidak valid", Errors: []domain.FieldError{{Field: "warehouse_id", Message: "gudang tidak ditemukan"}}}
			}
			return err
		}
		if !wh.IsActive {
			return &domain.ValidationError{Message: "Data opname tidak valid", Errors: []domain.FieldError{{Field: "warehouse_id", Message: "gudang tidak aktif"}}}
		}

		// Kunci rak urut ID agar tidak bentrok dengan opname/LPB lain.
		lockedRacks := make([]domain.Rack, 0, len(rackIDs))
		for _, rid := range rackIDs {
			rk, err := u.racks.FindByIDForUpdate(ctx, rid)
			if err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					return &domain.ValidationError{Message: "Data opname tidak valid", Errors: []domain.FieldError{{Field: "rack_ids", Message: "sebagian rak tidak ditemukan"}}}
				}
				return err
			}
			if rk.WarehouseID != in.WarehouseID {
				return &domain.ValidationError{Message: "Data opname tidak valid", Errors: []domain.FieldError{{Field: "rack_ids", Message: fmt.Sprintf("rak %s bukan milik gudang ini", rk.Code)}}}
			}
			if !rk.IsActive {
				return &domain.ValidationError{Message: "Data opname tidak valid", Errors: []domain.FieldError{{Field: "rack_ids", Message: fmt.Sprintf("rak %s tidak aktif", rk.Code)}}}
			}
			if rk.LockedByOpnameID != nil {
				return fmt.Errorf("%w: rak %s", domain.ErrRackLocked, rk.Code)
			}
			lockedRacks = append(lockedRacks, *rk)
		}

		now := u.clock.Now()
		num, err := u.docs.Next(ctx, "OPN", wh.Code, now)
		if err != nil {
			return err
		}
		created, err = u.repo.Create(ctx, &domain.Opname{
			Number:       num,
			WarehouseID:  in.WarehouseID,
			IsBlindCount: in.IsBlindCount,
			StartedBy:    actor.UserID,
			StartedAt:    now,
			Notes:        cleanOpt(in.Notes),
		})
		if err != nil {
			return err
		}
		if err := u.repo.AddRacks(ctx, created.ID, rackIDs); err != nil {
			return err
		}
		if err := u.repo.LockRacksForOpname(ctx, created.ID, rackIDs); err != nil {
			return err
		}
		if err := u.repo.SnapshotNonSerialBalances(ctx, created.ID, rackIDs); err != nil {
			return err
		}
		if err := u.repo.SnapshotSerialUnits(ctx, created.ID, rackIDs); err != nil {
			return err
		}

		rackCodes := make([]string, len(lockedRacks))
		for i, rk := range lockedRacks {
			rackCodes[i] = rk.Code
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			UserID: &actor.UserID, Action: domain.AuditOpnameStart, EntityType: domain.EntityOpname, EntityID: &created.ID,
			After: map[string]any{"opname_number": created.Number, "racks": rackCodes, "is_blind_count": created.IsBlindCount},
		})
	})
	if err != nil {
		return nil, err
	}
	return u.loadDetail(ctx, actor, created)
}

// SaveCounts menyimpan hitungan fisik barang non-SN (boleh parsial; bisa dipanggil berkali-kali saat IN_PROGRESS).
func (u *OpnameUsecase) SaveCounts(ctx context.Context, actor domain.Principal, id int64, counts []OpnameCountInput) (*OpnameDetail, error) {
	if len(counts) == 0 {
		return nil, &domain.ValidationError{Message: "Data hitungan tidak valid", Errors: []domain.FieldError{{Field: "items", Message: "minimal 1 baris hitungan"}}}
	}
	var op *domain.Opname
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		var err error
		op, err = u.repo.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if !actor.CanAccessWarehouse(op.WarehouseID) {
			return domain.ErrForbiddenWarehouse
		}
		if op.Status != domain.OpnameInProgress {
			return domain.ErrInvalidStateTransition
		}
		opRacks, err := u.repo.Racks(ctx, op.ID)
		if err != nil {
			return err
		}
		allowedRack := make(map[int64]bool, len(opRacks))
		for _, r := range opRacks {
			allowedRack[r.RackID] = true
		}

		now := u.clock.Now()
		for i, c := range counts {
			if c.CountedQty < 0 || c.CountedQty > maxItemQty {
				return &domain.ValidationError{Message: "Data hitungan tidak valid", Errors: []domain.FieldError{
					{Field: fmt.Sprintf("items[%d].counted_qty", i), Message: "jumlah harus >= 0"},
				}}
			}
			if !allowedRack[c.RackID] {
				return &domain.ValidationError{Message: "Data hitungan tidak valid", Errors: []domain.FieldError{
					{Field: fmt.Sprintf("items[%d].rack_id", i), Message: "rak bukan bagian dari opname ini"},
				}}
			}
			prod, err := u.products.FindByID(ctx, c.ProductID)
			if err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					return &domain.ValidationError{Message: "Data hitungan tidak valid", Errors: []domain.FieldError{
						{Field: fmt.Sprintf("items[%d].product_id", i), Message: "produk tidak ditemukan"},
					}}
				}
				return err
			}
			if prod.HasSerialNumber {
				return &domain.ValidationError{Message: "Data hitungan tidak valid", Errors: []domain.FieldError{
					{Field: fmt.Sprintf("items[%d].product_id", i), Message: "produk berserial harus dihitung lewat scan SN"},
				}}
			}
			if err := u.repo.UpsertItemCount(ctx, op.ID, c.ProductID, c.RackID, c.CountedQty, actor.UserID, now, cleanOpt(c.Notes)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return u.loadDetail(ctx, actor, op)
}

// ScanSerial memindai satu unit SN/asset tag pada rak opname dan mengklasifikasikan FOUND / WRONG_RACK / UNEXPECTED.
func (u *OpnameUsecase) ScanSerial(ctx context.Context, actor domain.Principal, id, rackID int64, rawCode string) (*domain.OpnameSerial, error) {
	code := strings.TrimSpace(rawCode)
	if code == "" || len(code) > maxScanCodeLen {
		return nil, &domain.ValidationError{Message: "Kode scan tidak valid", Errors: []domain.FieldError{{Field: "code", Message: "kode wajib diisi"}}}
	}

	var scanned *domain.OpnameSerial
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		op, err := u.repo.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if !actor.CanAccessWarehouse(op.WarehouseID) {
			return domain.ErrForbiddenWarehouse
		}
		if op.Status != domain.OpnameInProgress {
			return domain.ErrInvalidStateTransition
		}
		opRacks, err := u.repo.Racks(ctx, op.ID)
		if err != nil {
			return err
		}
		inOpname := false
		for _, r := range opRacks {
			if r.RackID == rackID {
				inOpname = true
				break
			}
		}
		if !inOpname {
			return &domain.ValidationError{Message: "Rak tidak valid", Errors: []domain.FieldError{{Field: "rack_id", Message: "rak bukan bagian dari opname ini"}}}
		}

		found, err := u.inventory.FindSerialsByCode(ctx, code)
		if err != nil {
			return err
		}
		if len(found) > 1 {
			return domain.ErrSerialAmbiguous
		}

		var (
			serialID *int64
			result   domain.OpnameSerialResult
		)
		if len(found) == 0 {
			result = domain.OpnameSerialUnexpected
		} else {
			u0 := found[0]
			serialID = &u0.ID
			switch {
			case u0.RackID != nil && *u0.RackID == rackID && isPhysicalInWarehouse(u0.Status):
				result = domain.OpnameSerialFound
			case u0.Status == domain.SerialLost:
				result = domain.OpnameSerialUnexpected
			default:
				result = domain.OpnameSerialWrongRack
			}
		}

		scanned, err = u.repo.UpsertSerialScan(ctx, op.ID, rackID, serialID, strings.ToUpper(code), result, actor.UserID, u.clock.Now())
		return err
	})
	return scanned, err
}

func isPhysicalInWarehouse(st domain.SerialStatus) bool {
	return st == domain.SerialAvailable || st == domain.SerialReserved || st == domain.SerialMaintenance || st == domain.SerialBroken
}

// Submit mengubah opname dari IN_PROGRESS → SUBMITTED setelah seluruh item non-SN dihitung.
func (u *OpnameUsecase) Submit(ctx context.Context, actor domain.Principal, id int64) (*OpnameDetail, error) {
	var op *domain.Opname
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		var err error
		op, err = u.repo.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if !actor.CanAccessWarehouse(op.WarehouseID) {
			return domain.ErrForbiddenWarehouse
		}
		if op.Status != domain.OpnameInProgress {
			return domain.ErrInvalidStateTransition
		}
		items, err := u.repo.Items(ctx, op.ID)
		if err != nil {
			return err
		}
		uncounted := 0
		for _, it := range items {
			if it.CountedQty == nil {
				uncounted++
			}
		}
		if uncounted > 0 {
			return &domain.ValidationError{
				Message: "Hitungan belum lengkap",
				Errors:  []domain.FieldError{{Field: "items", Message: fmt.Sprintf("masih ada %d baris non-SN yang belum diisi jumlah hitungannya", uncounted)}},
			}
		}
		now := u.clock.Now()
		if err := u.repo.MarkSubmitted(ctx, op.ID, actor.UserID, now); err != nil {
			return err
		}
		op.Status = domain.OpnameSubmitted
		op.SubmittedBy = &actor.UserID
		op.SubmittedAt = &now
		return u.audit.Insert(ctx, domain.AuditEntry{
			UserID: &actor.UserID, Action: domain.AuditOpnameSubmit, EntityType: domain.EntityOpname, EntityID: &op.ID,
			After: map[string]any{"opname_number": op.Number, "status": string(op.Status)},
		})
	})
	if err != nil {
		return nil, err
	}
	return u.loadDetail(ctx, actor, op)
}

// Cancel membatalkan opname (IN_PROGRESS / SUBMITTED → CANCELLED) dan membuka kunci rak tanpa mengubah stok.
func (u *OpnameUsecase) Cancel(ctx context.Context, actor domain.Principal, id int64) (*OpnameDetail, error) {
	var op *domain.Opname
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		var err error
		op, err = u.repo.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if !actor.CanAccessWarehouse(op.WarehouseID) {
			return domain.ErrForbiddenWarehouse
		}
		if op.Status != domain.OpnameInProgress && op.Status != domain.OpnameSubmitted {
			return domain.ErrInvalidStateTransition
		}
		if err := u.repo.MarkCancelled(ctx, op.ID); err != nil {
			return err
		}
		if err := u.repo.UnlockRacksForOpname(ctx, op.ID); err != nil {
			return err
		}
		op.Status = domain.OpnameCancelled
		return u.audit.Insert(ctx, domain.AuditEntry{
			UserID: &actor.UserID, Action: domain.AuditOpnameCancel, EntityType: domain.EntityOpname, EntityID: &op.ID,
			After: map[string]any{"opname_number": op.Number, "status": string(op.Status)},
		})
	})
	if err != nil {
		return nil, err
	}
	return u.loadDetail(ctx, actor, op)
}

type opnameAdjLine struct {
	productID int64
	rackID    int64
	qty       int
	serials   []domain.SerialLock
}

// Finalize menutup stock opname (SUBMITTED → FINALIZED): membuat & menyetujui LPB adjustment
// (OPNAME_ADJUSTMENT_IN dan/atau OPNAME_ADJUSTMENT_OUT), memperbarui saldo/SN/ledger/riwayat secara atomik,
// lalu membuka kunci rak.
func (u *OpnameUsecase) Finalize(ctx context.Context, actor domain.Principal, id int64) (*OpnameDetail, error) {
	var op *domain.Opname
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		var err error
		op, err = u.repo.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if !actor.CanAccessWarehouse(op.WarehouseID) {
			return domain.ErrForbiddenWarehouse
		}
		if op.Status != domain.OpnameSubmitted {
			return domain.ErrInvalidStateTransition
		}

		items, err := u.repo.Items(ctx, op.ID)
		if err != nil {
			return err
		}
		serials, err := u.repo.Serials(ctx, op.ID)
		if err != nil {
			return err
		}

		// Kumpulkan unit SN yang perlu dikunci: MISSING (akan jadi LOST) dan UNEXPECTED yang sebelumnya LOST (akan jadi AVAILABLE).
		var missingIDs, foundLostIDs []int64
		rackOfFoundLost := map[int64]int64{}
		for _, s := range serials {
			if s.ProductSerialID == nil {
				continue
			}
			switch s.Result {
			case domain.OpnameSerialMissing:
				missingIDs = append(missingIDs, *s.ProductSerialID)
			case domain.OpnameSerialUnexpected:
				foundLostIDs = append(foundLostIDs, *s.ProductSerialID)
				rackOfFoundLost[*s.ProductSerialID] = s.RackID
			}
		}

		allSerialIDs := append(append([]int64{}, missingIDs...), foundLostIDs...)
		sort.Slice(allSerialIDs, func(i, j int) bool { return allSerialIDs[i] < allSerialIDs[j] })
		lockedSerials, err := u.stock.LockSerials(ctx, allSerialIDs)
		if err != nil {
			return err
		}
		bySerialID := make(map[int64]domain.SerialLock, len(lockedSerials))
		for _, ls := range lockedSerials {
			bySerialID[ls.ID] = ls
		}

		// Susun baris penyesuaian IN dan OUT (key: product_id + rack_id).
		type prKey struct{ p, r int64 }
		inMap := map[prKey]*opnameAdjLine{}
		outMap := map[prKey]*opnameAdjLine{}
		addLine := func(m map[prKey]*opnameAdjLine, p, r int64, qty int, serial *domain.SerialLock) {
			k := prKey{p, r}
			ln := m[k]
			if ln == nil {
				ln = &opnameAdjLine{productID: p, rackID: r}
				m[k] = ln
			}
			ln.qty += qty
			if serial != nil {
				ln.serials = append(ln.serials, *serial)
			}
		}

		for _, it := range items {
			if it.DiffQty == nil || *it.DiffQty == 0 {
				continue
			}
			if *it.DiffQty > 0 {
				addLine(inMap, it.ProductID, it.RackID, *it.DiffQty, nil)
			} else {
				addLine(outMap, it.ProductID, it.RackID, -*it.DiffQty, nil)
			}
		}

		for _, sid := range missingIDs {
			ls, ok := bySerialID[sid]
			if !ok || ls.RackID == nil {
				continue
			}
			if ls.Status == domain.SerialReserved {
				return fmt.Errorf("%w: unit %s sedang direservasi DO", domain.ErrReservationMismatch, ls.AssetTag)
			}
			if !isPhysicalInWarehouse(ls.Status) {
				continue
			}
			addLine(outMap, ls.ProductID, *ls.RackID, 1, &ls)
		}
		for _, sid := range foundLostIDs {
			ls, ok := bySerialID[sid]
			if !ok || ls.Status != domain.SerialLost {
				continue
			}
			targetRack := rackOfFoundLost[sid]
			addLine(inMap, ls.ProductID, targetRack, 1, &ls)
		}

		toSorted := func(m map[prKey]*opnameAdjLine) []*opnameAdjLine {
			out := make([]*opnameAdjLine, 0, len(m))
			for _, ln := range m {
				out = append(out, ln)
			}
			sort.Slice(out, func(i, j int) bool {
				if out[i].productID != out[j].productID {
					return out[i].productID < out[j].productID
				}
				return out[i].rackID < out[j].rackID
			})
			return out
		}

		now := u.clock.Now()
		var adjInID, adjOutID *int64

		if inLines := toSorted(inMap); len(inLines) > 0 {
			lpbID, err := u.applyOpnameAdjustmentLPB(ctx, op, actor.UserID, now, domain.LPBInbound, domain.LPBOpnameAdjustmentIn, inLines)
			if err != nil {
				return err
			}
			adjInID = &lpbID
		}
		if outLines := toSorted(outMap); len(outLines) > 0 {
			lpbID, err := u.applyOpnameAdjustmentLPB(ctx, op, actor.UserID, now, domain.LPBOutbound, domain.LPBOpnameAdjustmentOut, outLines)
			if err != nil {
				return err
			}
			adjOutID = &lpbID
		}

		if err := u.repo.MarkFinalized(ctx, op.ID, actor.UserID, now, adjInID, adjOutID); err != nil {
			return err
		}
		if err := u.repo.UnlockRacksForOpname(ctx, op.ID); err != nil {
			return err
		}

		if err := u.audit.Insert(ctx, domain.AuditEntry{
			UserID: &actor.UserID, Action: domain.AuditOpnameFinalize, EntityType: domain.EntityOpname, EntityID: &op.ID,
			After: map[string]any{
				"opname_number":         op.Number,
				"adjustment_in_lpb_id":  adjInID,
				"adjustment_out_lpb_id": adjOutID,
			},
		}); err != nil {
			return err
		}
		op, err = u.repo.FindByID(ctx, op.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return u.loadDetail(ctx, actor, op)
}

func (u *OpnameUsecase) applyOpnameAdjustmentLPB(
	ctx context.Context,
	op *domain.Opname,
	approverID int64,
	now time.Time,
	typ domain.LPBType,
	subType domain.LPBSubType,
	lines []*opnameAdjLine,
) (int64, error) {
	prefix := "LPBI"
	dir := "IN"
	if typ == domain.LPBOutbound {
		prefix = "LPBO"
		dir = "OUT"
	}
	num, err := u.docs.Next(ctx, prefix, op.WarehouseCode, now)
	if err != nil {
		return 0, err
	}
	refType := opnameRefType
	refNum := op.Number
	note := fmt.Sprintf("Penyesuaian otomatis dari Stock Opname %s", op.Number)
	doc, err := u.lpbRepo.Create(ctx, &domain.LPB{
		Number:          num,
		Type:            typ,
		SubType:         subType,
		Status:          domain.LPBPendingApproval,
		WarehouseID:     op.WarehouseID,
		DocDate:         now.In(clock.WIB).Format(docDateLayout),
		ReferenceType:   &refType,
		ReferenceNumber: &refNum,
		Notes:           &note,
		CreatedBy:       approverID,
		SubmittedBy:     &approverID,
		SubmittedAt:     &now,
	})
	if err != nil {
		return 0, err
	}

	for _, ln := range lines {
		itemID, err := u.lpbRepo.InsertItem(ctx, doc.ID, &domain.LPBItem{
			ProductID: ln.productID,
			RackID:    ln.rackID,
			Qty:       ln.qty,
		})
		if err != nil {
			return 0, err
		}

		var before, after int
		if dir == "IN" {
			before, after, err = u.stock.AddStock(ctx, ln.productID, ln.rackID, ln.qty)
		} else {
			before, after, err = u.stock.RemoveStock(ctx, ln.productID, ln.rackID, ln.qty)
		}
		if err != nil {
			return 0, err
		}

		serialIDs := make([]int64, 0, len(ln.serials))
		lpbRef := "LPB"
		for _, ls := range ln.serials {
			serialIDs = append(serialIDs, ls.ID)
			if _, err := u.lpbRepo.InsertItemSerial(ctx, &domain.LPBItemSerial{
				LPBItemID:       itemID,
				ProductSerialID: &ls.ID,
				SerialNumber:    ls.Number,
				Condition:       domain.ConditionGood,
				ScannedBy:       &approverID,
				ScannedAt:       now,
			}); err != nil {
				return 0, err
			}
			fromStatus := string(ls.Status)
			if dir == "OUT" {
				if err := u.stock.MoveSerial(ctx, ls.ID, domain.SerialLost, nil, nil); err != nil {
					return 0, err
				}
				if err := u.stock.InsertSerialHistory(ctx, domain.SerialHistoryEntry{
					SerialID: ls.ID, FromStatus: &fromStatus, ToStatus: string(domain.SerialLost),
					FromRackID: ls.RackID, ToRackID: nil, EventType: "OPNAME_FINALIZED",
					ReferenceType: &lpbRef, ReferenceID: &doc.ID, UserID: &approverID, Notes: &note,
				}); err != nil {
					return 0, err
				}
			} else {
				whID := op.WarehouseID
				rkID := ln.rackID
				if err := u.stock.MoveSerial(ctx, ls.ID, domain.SerialAvailable, &whID, &rkID); err != nil {
					return 0, err
				}
				if err := u.stock.InsertSerialHistory(ctx, domain.SerialHistoryEntry{
					SerialID: ls.ID, FromStatus: &fromStatus, ToStatus: string(domain.SerialAvailable),
					FromRackID: nil, ToRackID: &rkID, EventType: "OPNAME_FINALIZED",
					ReferenceType: &lpbRef, ReferenceID: &doc.ID, UserID: &approverID, Notes: &note,
				}); err != nil {
					return 0, err
				}
			}
		}

		if err := u.stock.InsertLedger(ctx, domain.LedgerEntry{
			ProductID: ln.productID, WarehouseID: op.WarehouseID, RackID: ln.rackID,
			Direction: dir, Qty: ln.qty, Before: before, After: after,
			MovementType: subType, LPBID: doc.ID, LPBItemID: itemID,
			SerialIDs: serialIDs, CreatedBy: approverID, Notes: &note,
		}); err != nil {
			return 0, err
		}
	}

	tokBuf := make([]byte, 32)
	if _, err := rand.Read(tokBuf); err != nil {
		return 0, err
	}
	verifyToken := base64.RawURLEncoding.EncodeToString(tokBuf)
	if err := u.lpbRepo.MarkApproved(ctx, doc.ID, approverID, now, verifyToken); err != nil {
		return 0, err
	}
	if err := u.audit.Insert(ctx, domain.AuditEntry{
		UserID: &approverID, Action: domain.AuditLPBApprove, EntityType: domain.EntityLPB, EntityID: &doc.ID,
		After: map[string]any{"lpb_number": doc.Number, "sub_type": string(subType), "opname_number": op.Number},
	}); err != nil {
		return 0, err
	}
	return doc.ID, nil
}

func dedupeSortedIDs(raw []int64, field string, maxLen int) ([]int64, error) {
	if len(raw) == 0 {
		return nil, &domain.ValidationError{
			Message: "Data opname tidak valid",
			Errors:  []domain.FieldError{{Field: field, Message: "pilih minimal 1 rak"}},
		}
	}
	seen := make(map[int64]bool, len(raw))
	out := make([]int64, 0, len(raw))
	for _, id := range raw {
		if id <= 0 {
			return nil, &domain.ValidationError{
				Message: "Data opname tidak valid",
				Errors:  []domain.FieldError{{Field: field, Message: "ID rak tidak valid"}},
			}
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if len(out) > maxLen {
		return nil, &domain.ValidationError{
			Message: "Data opname tidak valid",
			Errors:  []domain.FieldError{{Field: field, Message: fmt.Sprintf("maksimal %d rak per opname", maxLen)}},
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}
