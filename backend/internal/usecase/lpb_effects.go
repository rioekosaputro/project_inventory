package usecase

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/clock"
)

// Efek stok per jenis LPB saat disetujui (plan Bagian 5.2 dan 6.2). Semua fungsi di sini dipanggil dari
// Approve di dalam SATU transaksi, setelah header dokumen dikunci dan gudang/rak ditahan. Urutan lock yang
// dijaga: saldo (urut produk, rak) lebih dulu, baru unit (urut id) — tidak pernah sebaliknya.

// stockMove: satu perubahan saldo pada satu (produk, rak) untuk satu item dokumen.
type stockMove struct {
	lpb             *domain.LPB
	item            domain.LPBItem
	rackID          int64
	direction       string // "IN" | "OUT"
	consumeReserved bool   // true untuk LPB OUT dari DO (mengurangi qty_on_hand & qty_reserved bersamaan)
	serialIDs       []int64
}

type moveResult struct {
	lpbID int64
	entry map[string]any
}

// applyMoves menjalankan semua perubahan saldo SECARA BERURUTAN (produk, rak; keluar sebelum masuk pada kunci
// yang sama) dan menulis satu baris ledger per perubahan. Urutan ini yang mencegah deadlock antar dokumen
// yang memindahkan produk yang sama ke arah berlawanan.
func (u *LPBUsecase) applyMoves(ctx context.Context, actor domain.Principal, moves []stockMove) ([]moveResult, error) {
	sort.SliceStable(moves, func(i, j int) bool {
		a, b := moves[i], moves[j]
		if a.item.ProductID != b.item.ProductID {
			return a.item.ProductID < b.item.ProductID
		}
		if a.rackID != b.rackID {
			return a.rackID < b.rackID
		}
		return a.direction == "OUT" && b.direction == "IN"
	})

	out := make([]moveResult, 0, len(moves))
	for _, m := range moves {
		var before, after int
		var err error
		if m.direction == "OUT" {
			if m.consumeReserved {
				before, after, err = u.stock.ConsumeReservedStock(ctx, m.item.ProductID, m.rackID, m.item.Qty)
				if errors.Is(err, domain.ErrReservationMismatch) {
					return nil, fmt.Errorf("%w: %s di rak %s (diminta %d)", domain.ErrReservationMismatch, m.item.SKU, m.item.RackCode, m.item.Qty)
				}
			} else {
				before, after, err = u.stock.RemoveStock(ctx, m.item.ProductID, m.rackID, m.item.Qty)
				if errors.Is(err, domain.ErrInsufficientStock) {
					return nil, fmt.Errorf("%w: %s di rak %s (diminta %d)", domain.ErrInsufficientStock, m.item.SKU, m.item.RackCode, m.item.Qty)
				}
			}
		} else {
			before, after, err = u.stock.AddStock(ctx, m.item.ProductID, m.rackID, m.item.Qty)
		}
		if err != nil {
			return nil, err
		}
		if err := u.stock.InsertLedger(ctx, domain.LedgerEntry{
			ProductID: m.item.ProductID, WarehouseID: m.lpb.WarehouseID, RackID: m.rackID,
			Direction: m.direction, Qty: m.item.Qty, Before: before, After: after, MovementType: m.lpb.SubType,
			LPBID: m.lpb.ID, LPBItemID: m.item.ID, SerialIDs: m.serialIDs, UnitCost: m.item.UnitCost, CreatedBy: actor.UserID,
		}); err != nil {
			return nil, err
		}
		out = append(out, moveResult{lpbID: m.lpb.ID, entry: map[string]any{
			"product_id": m.item.ProductID, "rack_id": m.rackID, "direction": m.direction, "qty": m.item.Qty,
			"balance_before": before, "balance_after": after, "serials": len(m.serialIDs),
		}})
	}
	return out, nil
}

func entriesFor(results []moveResult, lpbID int64) []map[string]any {
	out := make([]map[string]any, 0, len(results))
	for _, r := range results {
		if r.lpbID == lpbID {
			out = append(out, r.entry)
		}
	}
	return out
}

// unitIDs: id unit terdaftar dari baris scan item (terurut, tanpa duplikat). Baris scan tanpa unit terdaftar
// (SN baru) tidak sah untuk LPB yang memindahkan unit yang sudah ada.
func unitIDs(item domain.LPBItem) ([]int64, error) {
	ids := make([]int64, 0, len(item.Serials))
	for _, s := range item.Serials {
		if s.ProductSerialID == nil {
			return nil, fmt.Errorf("%w: %s belum terkait unit terdaftar", domain.ErrSerialNotValid, s.SerialNumber)
		}
		ids = append(ids, *s.ProductSerialID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

// lockUnits mengunci semua unit yang disentuh dokumen sekaligus, urut id (FOR UPDATE).
func (u *LPBUsecase) lockUnits(ctx context.Context, items []domain.LPBItem) (map[int64]*domain.SerialLock, error) {
	set := map[int64]bool{}
	for _, it := range items {
		ids, err := unitIDs(it)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			set[id] = true
		}
	}
	ids := sortedKeys(set)
	locked, err := u.stock.LockSerials(ctx, ids)
	if err != nil {
		return nil, err
	}
	if len(locked) != len(ids) {
		return nil, fmt.Errorf("%w: ada unit yang tidak lagi terdaftar", domain.ErrSerialNotValid)
	}
	out := make(map[int64]*domain.SerialLock, len(locked))
	for i := range locked {
		out[locked[i].ID] = &locked[i]
	}
	return out, nil
}

// expectUnit memeriksa posisi unit terhadap item: produk sama, status yang diwajibkan, dan (bila rackID ≠ nil)
// berada di rak itu.
func expectUnit(cur *domain.SerialLock, productID int64, want domain.SerialStatus, rackID *int64, rackCode string) error {
	if cur.ProductID != productID || cur.Status != want || (rackID != nil && (cur.RackID == nil || *cur.RackID != *rackID)) {
		where := "tidak berada di perjalanan"
		if rackID != nil {
			where = "tidak berada di rak " + rackCode
		}
		return fmt.Errorf("%w: unit %s berstatus %s dan %s sebagai %s", domain.ErrSerialNotValid, cur.AssetTag, cur.Status, where, want)
	}
	return nil
}

// moveUnit mengubah status/lokasi unit dan menulis SATU riwayat; `cur` ikut diperbarui agar langkah
// berikutnya (mis. IN_TRANSIT → AVAILABLE pada transfer satu gudang) membaca keadaan terbaru.
func (u *LPBUsecase) moveUnit(ctx context.Context, actor domain.Principal, cur *domain.SerialLock, to domain.SerialStatus, wh, rack *int64, l *domain.LPB) error {
	if err := u.stock.MoveSerial(ctx, cur.ID, to, wh, rack); err != nil {
		return err
	}
	from := string(cur.Status)
	refType := "LPB"
	note := "LPB " + l.Number
	if err := u.stock.InsertSerialHistory(ctx, domain.SerialHistoryEntry{
		SerialID: cur.ID, FromStatus: &from, ToStatus: string(to), FromRackID: cur.RackID, ToRackID: rack,
		EventType: "LPB_APPROVED", ReferenceType: &refType, ReferenceID: &l.ID, UserID: &actor.UserID, Notes: &note,
	}); err != nil {
		return err
	}
	cur.Status, cur.RackID, cur.WarehouseID = to, rack, wh
	return nil
}

// applyDisposal: DISPOSAL — saldo berkurang dari rak asal; unit BROKEN → RETIRED (keluar dari gudang).
func (u *LPBUsecase) applyDisposal(ctx context.Context, actor domain.Principal, l *domain.LPB, items []domain.LPBItem) ([]map[string]any, error) {
	moves := make([]stockMove, 0, len(items))
	for _, it := range items {
		ids, err := unitIDs(it)
		if err != nil {
			return nil, err
		}
		moves = append(moves, stockMove{lpb: l, item: it, rackID: it.RackID, direction: "OUT", serialIDs: ids})
	}
	results, err := u.applyMoves(ctx, actor, moves)
	if err != nil {
		return nil, err
	}

	units, err := u.lockUnits(ctx, items)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		for _, s := range it.Serials {
			cur := units[*s.ProductSerialID]
			rack := it.RackID
			if err := expectUnit(cur, it.ProductID, domain.SerialBroken, &rack, it.RackCode); err != nil {
				return nil, err
			}
			if err := u.moveUnit(ctx, actor, cur, domain.SerialRetired, nil, nil, l); err != nil {
				return nil, err
			}
		}
	}
	return entriesFor(results, l.ID), nil
}

// applySparePartIssue: SPARE_PART_ISSUE — saldo berkurang dari rak asal; unit SN (jika ada) AVAILABLE → RETIRED (terpasang di unit pelanggan).
func (u *LPBUsecase) applySparePartIssue(ctx context.Context, actor domain.Principal, l *domain.LPB, items []domain.LPBItem) ([]map[string]any, error) {
	moves := make([]stockMove, 0, len(items))
	for _, it := range items {
		ids, err := unitIDs(it)
		if err != nil {
			return nil, err
		}
		moves = append(moves, stockMove{lpb: l, item: it, rackID: it.RackID, direction: "OUT", serialIDs: ids})
	}
	results, err := u.applyMoves(ctx, actor, moves)
	if err != nil {
		return nil, err
	}

	units, err := u.lockUnits(ctx, items)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		for _, s := range it.Serials {
			cur := units[*s.ProductSerialID]
			rack := it.RackID
			if err := expectUnit(cur, it.ProductID, domain.SerialAvailable, &rack, it.RackCode); err != nil {
				return nil, err
			}
			if err := u.moveUnit(ctx, actor, cur, domain.SerialRetired, nil, nil, l); err != nil {
				return nil, err
			}
		}
	}
	return entriesFor(results, l.ID), nil
}

// applyTransferIn: TRANSFER_IN (pasangan transfer antar gudang) — saldo rak tujuan bertambah; unit
// IN_TRANSIT → AVAILABLE di rak tujuan.
func (u *LPBUsecase) applyTransferIn(ctx context.Context, actor domain.Principal, l *domain.LPB, items []domain.LPBItem) ([]map[string]any, error) {
	moves := make([]stockMove, 0, len(items))
	for _, it := range items {
		ids, err := unitIDs(it)
		if err != nil {
			return nil, err
		}
		moves = append(moves, stockMove{lpb: l, item: it, rackID: it.RackID, direction: "IN", serialIDs: ids})
	}
	results, err := u.applyMoves(ctx, actor, moves)
	if err != nil {
		return nil, err
	}
	if err := u.receiveUnits(ctx, actor, l, items); err != nil {
		return nil, err
	}
	return entriesFor(results, l.ID), nil
}

// receiveUnits: unit IN_TRANSIT → AVAILABLE di rak item (gudang dokumen).
func (u *LPBUsecase) receiveUnits(ctx context.Context, actor domain.Principal, l *domain.LPB, items []domain.LPBItem) error {
	units, err := u.lockUnits(ctx, items)
	if err != nil {
		return err
	}
	for _, it := range items {
		for _, s := range it.Serials {
			cur := units[*s.ProductSerialID]
			if err := expectUnit(cur, it.ProductID, domain.SerialInTransit, nil, ""); err != nil {
				return err
			}
			rack, wh := it.RackID, l.WarehouseID
			if err := u.moveUnit(ctx, actor, cur, domain.SerialAvailable, &wh, &rack, l); err != nil {
				return err
			}
		}
	}
	return nil
}

// applyTransferOut: TRANSFER_OUT — saldo rak asal berkurang, unit AVAILABLE → IN_TRANSIT, dan LPB pasangan
// TRANSFER_IN dibuat. Satu gudang (pindah rak): pasangan langsung disetujui dalam transaksi yang sama
// (barang tidak benar-benar "di jalan"). Antar gudang: pasangan menunggu persetujuan Kepala Gudang tujuan.
func (u *LPBUsecase) applyTransferOut(ctx context.Context, actor domain.Principal, l *domain.LPB, items []domain.LPBItem, now time.Time, extra map[string]any) ([]map[string]any, error) {
	destWh, destCode, err := transferDestination(items)
	if err != nil {
		return nil, err
	}
	sameWarehouse := destWh == l.WarehouseID

	in, inItems, err := u.createTransferIn(ctx, actor, l, items, destWh, destCode, now)
	if err != nil {
		return nil, err
	}

	moves := make([]stockMove, 0, len(items)+len(inItems))
	for _, it := range items {
		ids, err := unitIDs(it)
		if err != nil {
			return nil, err
		}
		moves = append(moves, stockMove{lpb: l, item: it, rackID: it.RackID, direction: "OUT", serialIDs: ids})
	}
	if sameWarehouse {
		for _, it := range inItems {
			ids, err := unitIDs(it)
			if err != nil {
				return nil, err
			}
			moves = append(moves, stockMove{lpb: in, item: it, rackID: it.RackID, direction: "IN", serialIDs: ids})
		}
	}
	results, err := u.applyMoves(ctx, actor, moves)
	if err != nil {
		return nil, err
	}

	// Unit: AVAILABLE di rak asal → IN_TRANSIT; bila satu gudang langsung tiba di rak tujuan.
	units, err := u.lockUnits(ctx, items)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		for _, s := range it.Serials {
			cur := units[*s.ProductSerialID]
			rack := it.RackID
			if err := expectUnit(cur, it.ProductID, domain.SerialAvailable, &rack, it.RackCode); err != nil {
				return nil, err
			}
			if err := u.moveUnit(ctx, actor, cur, domain.SerialInTransit, nil, nil, l); err != nil {
				return nil, err
			}
		}
	}
	if sameWarehouse {
		if err := u.receiveUnits(ctx, actor, in, inItems); err != nil {
			return nil, err
		}
		token, err := newVerifyToken()
		if err != nil {
			return nil, err
		}
		if err := u.repo.MarkApproved(ctx, in.ID, actor.UserID, now, token); err != nil {
			return nil, err
		}
		if err := u.audit.Insert(ctx, domain.AuditEntry{
			Action: domain.AuditLPBApprove, EntityType: domain.EntityLPB, EntityID: &in.ID,
			Before: map[string]any{"status": string(domain.LPBPendingApproval)},
			After:  map[string]any{"status": string(domain.LPBApproved), "items": entriesFor(results, in.ID), "auto_from": l.Number},
		}); err != nil {
			return nil, err
		}
		extra["counterpart_status"] = string(domain.LPBApproved)
	} else {
		extra["counterpart_status"] = string(domain.LPBPendingApproval)
	}
	extra["counterpart_id"], extra["counterpart_number"] = in.ID, in.Number
	return entriesFor(results, l.ID), nil
}

// transferDestination: gudang tujuan tunggal seluruh item transfer.
func transferDestination(items []domain.LPBItem) (int64, string, error) {
	var wh int64
	var code string
	for _, it := range items {
		if it.DestRackID == nil || it.DestWarehouseID == nil {
			return 0, "", lpbInvalid([]domain.FieldError{{Field: "items", Message: "rak tujuan transfer belum diisi"}})
		}
		if wh != 0 && wh != *it.DestWarehouseID {
			return 0, "", lpbInvalid([]domain.FieldError{{Field: "items", Message: "semua item transfer harus menuju satu gudang tujuan"}})
		}
		wh, code = *it.DestWarehouseID, derefStr(it.DestWarehouseCode)
	}
	return wh, code, nil
}

// createTransferIn membuat LPB pasangan TRANSFER_IN di gudang tujuan, lengkap dengan item (digabung per
// produk + rak tujuan) dan baris scan unit yang sama. Status PENDING_APPROVAL; pengaju = penyetuju
// TRANSFER_OUT, pembuat = pembuat TRANSFER_OUT.
func (u *LPBUsecase) createTransferIn(ctx context.Context, actor domain.Principal, out *domain.LPB, items []domain.LPBItem, destWh int64, destCode string, now time.Time) (*domain.LPB, []domain.LPBItem, error) {
	number, err := u.docs.Next(ctx, "LPBI", destCode, now)
	if err != nil {
		return nil, nil, err
	}
	refType := "LPB"
	notes := "Transfer dari " + out.Number
	submittedAt := now
	in, err := u.repo.Create(ctx, &domain.LPB{
		Number: number, Type: domain.LPBInbound, SubType: domain.LPBTransferIn, Status: domain.LPBPendingApproval,
		WarehouseID: destWh, DocDate: clock.DateWIB(now), ReferenceType: &refType, ReferenceNumber: &out.Number,
		Notes: &notes, CreatedBy: out.CreatedBy, SubmittedBy: &actor.UserID, SubmittedAt: &submittedAt,
		CounterpartID: &out.ID,
	})
	if err != nil {
		return nil, nil, err
	}

	// Gabungkan item sumber yang menuju (produk, rak tujuan) yang sama (batas unik lpb_items).
	type key struct{ product, rack int64 }
	merged := map[key]*domain.LPBItem{}
	var order []key
	for _, it := range items {
		k := key{it.ProductID, *it.DestRackID}
		m, ok := merged[k]
		if !ok {
			m = &domain.LPBItem{
				ProductID: it.ProductID, SKU: it.SKU, ProductName: it.ProductName, Unit: it.Unit,
				HasSerialNumber: it.HasSerialNumber, RackID: *it.DestRackID, RackCode: derefStr(it.DestRackCode),
			}
			merged[k] = m
			order = append(order, k)
		}
		m.Qty += it.Qty
		m.Serials = append(m.Serials, it.Serials...)
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].product != order[j].product {
			return order[i].product < order[j].product
		}
		return order[i].rack < order[j].rack
	})

	inItems := make([]domain.LPBItem, 0, len(order))
	for _, k := range order {
		m := merged[k]
		id, err := u.repo.InsertItem(ctx, in.ID, m)
		if err != nil {
			return nil, nil, err
		}
		m.ID, m.LPBID = id, in.ID
		scans := m.Serials
		m.Serials = nil
		for _, s := range scans {
			saved, err := u.repo.InsertItemSerial(ctx, &domain.LPBItemSerial{
				LPBItemID: id, ProductSerialID: s.ProductSerialID, SerialNumber: s.SerialNumber,
				Condition: s.Condition, ScannedBy: s.ScannedBy,
			})
			if err != nil {
				return nil, nil, err
			}
			saved.AssetTag = s.AssetTag
			m.Serials = append(m.Serials, *saved)
		}
		inItems = append(inItems, *m)
	}
	if err := u.repo.SetCounterpart(ctx, out.ID, in.ID); err != nil {
		return nil, nil, err
	}

	view := map[string]any{
		"lpb_number": in.Number, "sub_type": string(domain.LPBTransferIn), "warehouse_id": destWh,
		"status": string(domain.LPBPendingApproval), "auto_from": out.Number, "items": itemsView(inItems),
	}
	if err := u.audit.Insert(ctx, domain.AuditEntry{
		Action: domain.AuditLPBCreate, EntityType: domain.EntityLPB, EntityID: &in.ID, After: view,
	}); err != nil {
		return nil, nil, err
	}
	if err := u.audit.Insert(ctx, domain.AuditEntry{
		Action: domain.AuditLPBSubmit, EntityType: domain.EntityLPB, EntityID: &in.ID,
		Before: map[string]any{"status": string(domain.LPBDraft)},
		After:  map[string]any{"status": string(domain.LPBPendingApproval)},
	}); err != nil {
		return nil, nil, err
	}
	return in, inItems, nil
}

// applyReceipt: PURCHASE / INITIAL_STOCK — saldo bertambah dan unit baru didaftarkan dengan asset tag.
func (u *LPBUsecase) applyReceipt(ctx context.Context, actor domain.Principal, l *domain.LPB, items []domain.LPBItem, racks map[int64]domain.Rack, now time.Time) ([]map[string]any, error) {
	summary := make([]map[string]any, 0, len(items))
	for _, it := range items {
		before, after, err := u.stock.AddStock(ctx, it.ProductID, it.RackID, it.Qty)
		if err != nil {
			return nil, err
		}

		var serialIDs []int64
		if it.HasSerialNumber {
			serials := append([]domain.LPBItemSerial(nil), it.Serials...)
			sort.Slice(serials, func(i, j int) bool { return serials[i].SerialNumber < serials[j].SerialNumber })
			racksellable := racks[it.RackID].IsSellable
			for _, s := range serials {
				status := domain.SerialAvailable
				if s.Condition == domain.ConditionBroken {
					if racksellable {
						return nil, fmt.Errorf("%w: unit rusak %s harus berada di rak yang tidak dapat dijual", domain.ErrSerialNotValid, s.SerialNumber)
					}
					status = domain.SerialBroken
				}
				tag, err := u.docs.NextAssetTag(ctx, now)
				if err != nil {
					return nil, err
				}
				serialID, err := u.stock.InsertSerial(ctx, &domain.NewSerial{
					ProductID: it.ProductID, WarehouseID: l.WarehouseID, RackID: it.RackID,
					AssetTag: tag, SerialNumber: s.SerialNumber, Status: status, Condition: s.Condition,
					SupplierID: l.SupplierID, PurchaseDate: &l.DocDate, PurchasePrice: it.UnitCost,
				})
				if err != nil {
					if errors.Is(err, domain.ErrSerialAlreadyExists) {
						return nil, fmt.Errorf("%w: %s", domain.ErrSerialAlreadyExists, s.SerialNumber)
					}
					return nil, err
				}
				if err := u.repo.SetItemSerialLink(ctx, s.ID, serialID); err != nil {
					return nil, err
				}
				refType := "LPB"
				note := "LPB " + l.Number
				if err := u.stock.InsertSerialHistory(ctx, domain.SerialHistoryEntry{
					SerialID: serialID, ToStatus: string(status), ToRackID: &it.RackID,
					EventType: "LPB_APPROVED", ReferenceType: &refType, ReferenceID: &l.ID,
					UserID: &actor.UserID, Notes: &note,
				}); err != nil {
					return nil, err
				}
				serialIDs = append(serialIDs, serialID)
			}
		}

		if err := u.stock.InsertLedger(ctx, domain.LedgerEntry{
			ProductID: it.ProductID, WarehouseID: l.WarehouseID, RackID: it.RackID,
			Direction: "IN", Qty: it.Qty, Before: before, After: after, MovementType: l.SubType,
			LPBID: l.ID, LPBItemID: it.ID, SerialIDs: serialIDs, UnitCost: it.UnitCost, CreatedBy: actor.UserID,
		}); err != nil {
			return nil, err
		}
		summary = append(summary, map[string]any{
			"product_id": it.ProductID, "rack_id": it.RackID, "qty": it.Qty,
			"balance_before": before, "balance_after": after, "serials": len(serialIDs),
		})
	}
	return summary, nil
}

func sortedKeys(set map[int64]bool) []int64 {
	out := make([]int64, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
