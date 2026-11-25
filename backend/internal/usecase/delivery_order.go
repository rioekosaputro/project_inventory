package usecase

import (
	"context"
	"fmt"
	"sort"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/clock"
)

// DeliveryOrderUsecase mengelola antrean DO gudang, reservasi stok/SN, pelepasan reservasi,
// pembatalan DO, dan pembuatan draft LPB dari DO (T2.6, plan Bagian 6.3).
type DeliveryOrderUsecase struct {
	tx     domain.TxManager
	dos    domain.DeliveryOrderRepository
	racks  domain.RackRepository
	stock  domain.StockRepository
	lpbs   domain.LPBRepository
	audit  domain.AuditRepository
	docGen domain.DocNumberGenerator
	clk    clock.Clock
}

func NewDeliveryOrderUsecase(
	tx domain.TxManager,
	dos domain.DeliveryOrderRepository,
	racks domain.RackRepository,
	stock domain.StockRepository,
	lpbs domain.LPBRepository,
	audit domain.AuditRepository,
	docGen domain.DocNumberGenerator,
	clk clock.Clock,
) *DeliveryOrderUsecase {
	return &DeliveryOrderUsecase{
		tx: tx, dos: dos, racks: racks, stock: stock, lpbs: lpbs,
		audit: audit, docGen: docGen, clk: clk,
	}
}

type ReserveDOItemInput struct {
	DOItemID  int64
	RackID    int64
	Qty       int
	SerialIDs []int64
}

type ReserveDOInput struct {
	Items []ReserveDOItemInput
}

type CreateLPBFromDOInput struct {
	RackID *int64 // wajib hanya untuk DO INBOUND (rak tujuan penerimaan retur)
	Notes  *string
}

func (u *DeliveryOrderUsecase) List(ctx context.Context, actor domain.Principal, f domain.DeliveryOrderFilter, p domain.PageParams) ([]domain.DeliveryOrder, int64, error) {
	scope := scopeOf(actor)
	if scope != nil && f.WarehouseID != nil && *f.WarehouseID != *scope {
		return nil, 0, domain.ErrForbiddenWarehouse
	}
	return u.dos.List(ctx, f, scope, p)
}

func (u *DeliveryOrderUsecase) Get(ctx context.Context, actor domain.Principal, id int64) (*domain.DeliveryOrderDetail, error) {
	do, err := u.dos.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !actor.CanAccessWarehouse(do.WarehouseID) {
		return nil, domain.ErrForbiddenWarehouse
	}
	items, err := u.dos.ListItems(ctx, id)
	if err != nil {
		return nil, err
	}
	return &domain.DeliveryOrderDetail{DO: do, Items: items}, nil
}

// Reserve memesan stok (qty_reserved) dan/atau unit SN (AVAILABLE → RESERVED) untuk DO OUTBOUND.
// Urutan lock tetap: DO (FOR UPDATE) → racks (FOR SHARE, urut id) → stock_balances (urut product_id, rack_id) → serials (urut id).
func (u *DeliveryOrderUsecase) Reserve(ctx context.Context, actor domain.Principal, doID int64, in ReserveDOInput) (*domain.DeliveryOrderDetail, error) {
	if len(in.Items) == 0 {
		return nil, masterInvalid("Reservasi DO", []domain.FieldError{
			{Field: "items", Message: "minimal 1 baris reservasi wajib diisi"},
		})
	}

	var detail *domain.DeliveryOrderDetail
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		do, err := u.dos.FindByIDForUpdate(ctx, doID)
		if err != nil {
			return err
		}
		if !actor.CanAccessWarehouse(do.WarehouseID) {
			return domain.ErrForbiddenWarehouse
		}
		if do.Direction != domain.DODirectionOutbound {
			return fmt.Errorf("%w: reservasi stok hanya berlaku untuk DO OUTBOUND", domain.ErrInvalidStateTransition)
		}
		if do.Status != domain.DOStatusOpen && do.Status != domain.DOStatusReserved {
			return fmt.Errorf("%w: DO berstatus %s tidak dapat direservasi", domain.ErrInvalidStateTransition, do.Status)
		}

		items, err := u.dos.ListItems(ctx, doID)
		if err != nil {
			return err
		}
		itemMap := make(map[int64]domain.DeliveryOrderItem, len(items))
		for _, it := range items {
			itemMap[it.ID] = it
		}

		// Validasi input & kumpulkan rackIDs.
		rackSet := make(map[int64]bool)
		addQtyByItem := make(map[int64]int)
		var errs []domain.FieldError
		for i, req := range in.Items {
			prefix := fmt.Sprintf("items[%d]", i)
			it, ok := itemMap[req.DOItemID]
			if !ok {
				errs = append(errs, domain.FieldError{Field: prefix + ".do_item_id", Message: "item DO tidak ditemukan"})
				continue
			}
			if req.Qty < 1 {
				errs = append(errs, domain.FieldError{Field: prefix + ".qty", Message: "qty minimal 1"})
			}
			if it.HasSerialNumber {
				if len(req.SerialIDs) != req.Qty {
					errs = append(errs, domain.FieldError{Field: prefix + ".serial_ids", Message: "jumlah serial number harus sama dengan qty"})
				}
			} else if len(req.SerialIDs) > 0 {
				errs = append(errs, domain.FieldError{Field: prefix + ".serial_ids", Message: "produk non-serial tidak menerima serial_ids"})
			}
			addQtyByItem[req.DOItemID] += req.Qty
			rackSet[req.RackID] = true
		}
		for itemID, addQty := range addQtyByItem {
			it := itemMap[itemID]
			if it.ReservedQty+addQty > it.Qty {
				errs = append(errs, domain.FieldError{
					Field:   "items",
					Message: fmt.Sprintf("reservasi %s melebihi kebutuhan DO (%d + %d > %d)", it.ProductSKU, it.ReservedQty, addQty, it.Qty),
				})
			}
		}
		if len(errs) > 0 {
			return masterInvalid("Reservasi DO", errs)
		}

		// Kunci rak (FOR SHARE, urut id) dan validasi sellable + tidak terkunci opname.
		lockedRacks, err := u.racks.LockForShare(ctx, sortedKeys(rackSet))
		if err != nil {
			return err
		}
		rackMap := make(map[int64]domain.Rack, len(lockedRacks))
		for _, rk := range lockedRacks {
			rackMap[rk.ID] = rk
		}
		for rackID := range rackSet {
			rk, ok := rackMap[rackID]
			if !ok || !rk.IsActive || rk.WarehouseID != do.WarehouseID {
				return masterInvalid("Reservasi DO", []domain.FieldError{
					{Field: "rack_id", Message: "rak tidak aktif atau bukan milik gudang DO"},
				})
			}
			if !rk.IsSellable {
				return masterInvalid("Reservasi DO", []domain.FieldError{
					{Field: "rack_id", Message: fmt.Sprintf("rak %s bukan rak sellable (karantina/workshop)", rk.Code)},
				})
			}
			if rk.LockedByOpnameID != nil {
				return fmt.Errorf("%w: rak %s", domain.ErrRackLocked, rk.Code)
			}
		}

		// Agregasi qty per (product_id, rack_id) dan urutkan sebelum mengunci saldo (anti-deadlock).
		type prKey struct{ productID, rackID int64 }
		qtyByPR := make(map[prKey]int)
		prOrder := make([]prKey, 0)
		allSerialIDs := make([]int64, 0)
		serialExpected := make(map[int64]prKey)

		for _, req := range in.Items {
			it := itemMap[req.DOItemID]
			k := prKey{productID: it.ProductID, rackID: req.RackID}
			if _, seen := qtyByPR[k]; !seen {
				prOrder = append(prOrder, k)
			}
			qtyByPR[k] += req.Qty
			for _, sid := range req.SerialIDs {
				if _, dup := serialExpected[sid]; dup {
					return masterInvalid("Reservasi DO", []domain.FieldError{
						{Field: "serial_ids", Message: "serial number duplikat dalam permintaan reservasi"},
					})
				}
				serialExpected[sid] = k
				allSerialIDs = append(allSerialIDs, sid)
			}
		}
		sort.Slice(prOrder, func(i, j int) bool {
			if prOrder[i].productID != prOrder[j].productID {
				return prOrder[i].productID < prOrder[j].productID
			}
			return prOrder[i].rackID < prOrder[j].rackID
		})
		for _, k := range prOrder {
			if err := u.stock.ReserveStock(ctx, k.productID, k.rackID, qtyByPR[k]); err != nil {
				return err
			}
		}

		// Kunci dan reservasi serial number (urut id).
		if len(allSerialIDs) > 0 {
			sort.Slice(allSerialIDs, func(i, j int) bool { return allSerialIDs[i] < allSerialIDs[j] })
			lockedSerials, err := u.stock.LockSerials(ctx, allSerialIDs)
			if err != nil {
				return err
			}
			if len(lockedSerials) != len(allSerialIDs) {
				return domain.ErrSerialNotValid
			}
			refType := "DELIVERY_ORDER"
			for _, s := range lockedSerials {
				exp := serialExpected[s.ID]
				if s.ProductID != exp.productID || s.RackID == nil || *s.RackID != exp.rackID {
					return fmt.Errorf("%w: unit %s tidak berada di rak/produk yang diminta", domain.ErrSerialNotValid, s.Number)
				}
				if s.Status != domain.SerialAvailable {
					return fmt.Errorf("%w: unit %s sudah berstatus %s", domain.ErrSerialNotValid, s.Number, s.Status)
				}
				if err := u.stock.ReserveSerial(ctx, s.ID, doID); err != nil {
					return err
				}
				fromSt := string(s.Status)
				if err := u.stock.InsertSerialHistory(ctx, domain.SerialHistoryEntry{
					SerialID:      s.ID,
					FromStatus:    &fromSt,
					ToStatus:      string(domain.SerialReserved),
					FromRackID:    s.RackID,
					ToRackID:      s.RackID,
					EventType:     "DO_RESERVED",
					ReferenceType: &refType,
					ReferenceID:   &doID,
					UserID:        &actor.UserID,
				}); err != nil {
					return err
				}
			}
		}

		// Simpan baris delivery_order_reservations.
		for _, req := range in.Items {
			it := itemMap[req.DOItemID]
			if it.HasSerialNumber {
				for _, sid := range req.SerialIDs {
					sidCopy := sid
					if _, err := u.dos.InsertReservation(ctx, &domain.DOReservation{
						DOItemID:        req.DOItemID,
						RackID:          req.RackID,
						ProductSerialID: &sidCopy,
						Qty:             1,
						ReservedBy:      actor.UserID,
					}); err != nil {
						return err
					}
				}
			} else {
				if _, err := u.dos.InsertReservation(ctx, &domain.DOReservation{
					DOItemID:   req.DOItemID,
					RackID:     req.RackID,
					Qty:        req.Qty,
					ReservedBy: actor.UserID,
				}); err != nil {
					return err
				}
			}
		}

		// Cek apakah seluruh item DO sudah terpenuhi → status DO menjadi RESERVED.
		updatedItems, err := u.dos.ListItems(ctx, doID)
		if err != nil {
			return err
		}
		allFulfilled := true
		for _, it := range updatedItems {
			if it.ReservedQty < it.Qty {
				allFulfilled = false
				break
			}
		}
		nextStatus := do.Status
		if allFulfilled {
			nextStatus = domain.DOStatusReserved
			if err := u.dos.UpdateStatus(ctx, doID, nextStatus, nil); err != nil {
				return err
			}
		}

		if err := u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "DELIVERY_ORDER_RESERVE",
			EntityType: domain.EntityDeliveryOrder,
			EntityID:   &doID,
			Before:     map[string]any{"status": string(do.Status)},
			After:      map[string]any{"status": string(nextStatus), "serials_reserved": len(allSerialIDs)},
		}); err != nil {
			return err
		}

		updatedDO, err := u.dos.FindByID(ctx, doID)
		if err != nil {
			return err
		}
		detail = &domain.DeliveryOrderDetail{DO: updatedDO, Items: updatedItems}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return detail, nil
}

// Release melepas seluruh reservasi aktif pada DO (OPEN/RESERVED) dan mengembalikan stok & SN ke AVAILABLE.
func (u *DeliveryOrderUsecase) Release(ctx context.Context, actor domain.Principal, doID int64) (*domain.DeliveryOrderDetail, error) {
	return u.releaseOrCancel(ctx, actor, doID, false)
}

// Cancel membatalkan DO (OPEN/RESERVED) dan melepaskan seluruh reservasi aktifnya secara atomik.
func (u *DeliveryOrderUsecase) Cancel(ctx context.Context, actor domain.Principal, doID int64) (*domain.DeliveryOrderDetail, error) {
	return u.releaseOrCancel(ctx, actor, doID, true)
}

func (u *DeliveryOrderUsecase) releaseOrCancel(ctx context.Context, actor domain.Principal, doID int64, cancel bool) (*domain.DeliveryOrderDetail, error) {
	var detail *domain.DeliveryOrderDetail
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		do, err := u.dos.FindByIDForUpdate(ctx, doID)
		if err != nil {
			return err
		}
		if !actor.CanAccessWarehouse(do.WarehouseID) {
			return domain.ErrForbiddenWarehouse
		}
		if do.Status != domain.DOStatusOpen && do.Status != domain.DOStatusReserved {
			return fmt.Errorf("%w: DO berstatus %s tidak dapat dilepas/dibatalkan", domain.ErrInvalidStateTransition, do.Status)
		}

		activeRes, err := u.dos.ListActiveReservations(ctx, doID)
		if err != nil {
			return err
		}

		type prKey struct{ productID, rackID int64 }
		qtyByPR := make(map[prKey]int)
		prOrder := make([]prKey, 0)
		serialIDs := make([]int64, 0)

		for _, rs := range activeRes {
			k := prKey{productID: rs.ProductID, rackID: rs.RackID}
			if _, seen := qtyByPR[k]; !seen {
				prOrder = append(prOrder, k)
			}
			qtyByPR[k] += rs.Qty
			if rs.ProductSerialID != nil {
				serialIDs = append(serialIDs, *rs.ProductSerialID)
			}
		}
		sort.Slice(prOrder, func(i, j int) bool {
			if prOrder[i].productID != prOrder[j].productID {
				return prOrder[i].productID < prOrder[j].productID
			}
			return prOrder[i].rackID < prOrder[j].rackID
		})
		for _, k := range prOrder {
			if err := u.stock.ReleaseStock(ctx, k.productID, k.rackID, qtyByPR[k]); err != nil {
				return err
			}
		}

		if len(serialIDs) > 0 {
			sort.Slice(serialIDs, func(i, j int) bool { return serialIDs[i] < serialIDs[j] })
			locked, err := u.stock.LockSerials(ctx, serialIDs)
			if err != nil {
				return err
			}
			refType := "DELIVERY_ORDER"
			for _, s := range locked {
				if err := u.stock.ReleaseSerial(ctx, s.ID, doID); err != nil {
					return err
				}
				fromSt := string(s.Status)
				if err := u.stock.InsertSerialHistory(ctx, domain.SerialHistoryEntry{
					SerialID:      s.ID,
					FromStatus:    &fromSt,
					ToStatus:      string(domain.SerialAvailable),
					FromRackID:    s.RackID,
					ToRackID:      s.RackID,
					EventType:     "DO_RELEASED",
					ReferenceType: &refType,
					ReferenceID:   &doID,
					UserID:        &actor.UserID,
				}); err != nil {
					return err
				}
			}
		}

		if len(activeRes) > 0 {
			if err := u.dos.MarkReservationsReleased(ctx, doID, u.clk.Now()); err != nil {
				return err
			}
		}

		nextStatus := domain.DOStatusOpen
		action := "DELIVERY_ORDER_RELEASE"
		if cancel {
			nextStatus = domain.DOStatusCancelled
			action = "DELIVERY_ORDER_CANCEL"
		}
		if err := u.dos.UpdateStatus(ctx, doID, nextStatus, nil); err != nil {
			return err
		}
		if err := u.audit.Insert(ctx, domain.AuditEntry{
			Action:     action,
			EntityType: domain.EntityDeliveryOrder,
			EntityID:   &doID,
			Before:     map[string]any{"status": string(do.Status)},
			After:      map[string]any{"status": string(nextStatus), "released_count": len(activeRes)},
		}); err != nil {
			return err
		}

		updatedDO, err := u.dos.FindByID(ctx, doID)
		if err != nil {
			return err
		}
		updatedItems, err := u.dos.ListItems(ctx, doID)
		if err != nil {
			return err
		}
		detail = &domain.DeliveryOrderDetail{DO: updatedDO, Items: updatedItems}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return detail, nil
}

// CreateLPB membuat DRAFT LPB dari DO (OUTBOUND yang sudah RESERVED → LPBO RENTAL_DELIVERY/CONSUMABLE_DELIVERY;
// INBOUND yang masih OPEN → LPBI RENTAL_RETURN).
func (u *DeliveryOrderUsecase) CreateLPB(ctx context.Context, actor domain.Principal, doID int64, in CreateLPBFromDOInput) (*domain.LPB, error) {
	var createdLPB *domain.LPB
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		do, err := u.dos.FindByIDForUpdate(ctx, doID)
		if err != nil {
			return err
		}
		if !actor.CanAccessWarehouse(do.WarehouseID) {
			return domain.ErrForbiddenWarehouse
		}

		items, err := u.dos.ListItems(ctx, doID)
		if err != nil {
			return err
		}

		var subType domain.LPBSubType
		var docType string
		if do.Direction == domain.DODirectionOutbound {
			if do.Status != domain.DOStatusReserved {
				return fmt.Errorf("%w: DO OUTBOUND harus berstatus RESERVED sebelum dibuatkan LPB", domain.ErrInvalidStateTransition)
			}
			docType = "LPBO"
			if do.OrderType == domain.OrderTypeConsumable {
				subType = domain.LPBConsumableDelivery
			} else {
				subType = domain.LPBRentalDelivery
			}
		} else {
			if do.Status != domain.DOStatusOpen {
				return fmt.Errorf("%w: DO INBOUND harus berstatus OPEN sebelum dibuatkan LPB", domain.ErrInvalidStateTransition)
			}
			if in.RackID == nil {
				return masterInvalid("Buat LPB Retur", []domain.FieldError{
					{Field: "rack_id", Message: "rak tujuan penerimaan retur wajib dipilih"},
				})
			}
			rk, err := u.racks.FindByID(ctx, *in.RackID)
			if err != nil || !rk.IsActive || rk.WarehouseID != do.WarehouseID {
				return masterInvalid("Buat LPB Retur", []domain.FieldError{
					{Field: "rack_id", Message: "rak tujuan tidak aktif atau bukan milik gudang DO"},
				})
			}
			docType = "LPBI"
			subType = domain.LPBRentalReturn
		}

		now := u.clk.Now()
		lpbNum, err := u.docGen.Next(ctx, docType, do.WarehouseCode, now)
		if err != nil {
			return err
		}
		refType := "DELIVERY_ORDER"
		refNum := do.DONumber
		custID := do.CustomerID
		siteID := do.CustomerSiteID

		lpbHeader := &domain.LPB{
			Number:           lpbNum,
			Type:             subType.Type(),
			SubType:          subType,
			Status:           domain.LPBDraft,
			WarehouseID:      do.WarehouseID,
			DocDate:          clock.DateWIB(now),
			ReferenceType:    &refType,
			ReferenceID:      &do.ID,
			ReferenceNumber:  &refNum,
			CustomerID:       &custID,
			CustomerSiteID:   &siteID,
			RequiresDelivery: true,
			Notes:            cleanOpt(in.Notes),
			CreatedBy:        actor.UserID,
		}
		lpbDoc, err := u.lpbs.Create(ctx, lpbHeader)
		if err != nil {
			return err
		}

		if do.Direction == domain.DODirectionOutbound {
			// Kelompokkan reservasi aktif per (do_item_id, product_id, rack_id) menjadi lpb_items.
			type grpKey struct{ doItemID, productID, rackID int64 }
			type grpVal struct {
				qty     int
				serials []domain.DOReservation
			}
			groups := make(map[grpKey]*grpVal)
			order := make([]grpKey, 0)
			for _, it := range items {
				for _, rs := range it.Reservations {
					k := grpKey{doItemID: it.ID, productID: it.ProductID, rackID: rs.RackID}
					g, ok := groups[k]
					if !ok {
						g = &grpVal{}
						groups[k] = g
						order = append(order, k)
					}
					g.qty += rs.Qty
					if rs.ProductSerialID != nil {
						g.serials = append(g.serials, rs)
					}
				}
			}
			for _, k := range order {
				g := groups[k]
				savedItemID, err := u.lpbs.InsertItem(ctx, lpbDoc.ID, &domain.LPBItem{
					LPBID:     lpbDoc.ID,
					ProductID: k.productID,
					RackID:    k.rackID,
					Qty:       g.qty,
				})
				if err != nil {
					return err
				}
				scannedBy := actor.UserID
				for _, rs := range g.serials {
					snText := ""
					if rs.SerialNumber != nil {
						snText = *rs.SerialNumber
					}
					if _, err := u.lpbs.InsertItemSerial(ctx, &domain.LPBItemSerial{
						LPBItemID:       savedItemID,
						ProductSerialID: rs.ProductSerialID,
						SerialNumber:    snText,
						Condition:       domain.ConditionGood,
						ScannedBy:       &scannedBy,
					}); err != nil {
						return err
					}
				}
			}
		} else {
			// INBOUND RENTAL_RETURN: buat item ke rak tujuan; bila SO sudah mencantumkan SN, masukkan langsung.
			scannedBy := actor.UserID
			for _, it := range items {
				savedItemID, err := u.lpbs.InsertItem(ctx, lpbDoc.ID, &domain.LPBItem{
					LPBID:     lpbDoc.ID,
					ProductID: it.ProductID,
					RackID:    *in.RackID,
					Qty:       it.Qty,
				})
				if err != nil {
					return err
				}
				if it.SOProductSerial != nil && it.SOSerialNumber != nil {
					if _, err := u.lpbs.InsertItemSerial(ctx, &domain.LPBItemSerial{
						LPBItemID:       savedItemID,
						ProductSerialID: it.SOProductSerial,
						SerialNumber:    *it.SOSerialNumber,
						Condition:       domain.ConditionGood,
						ScannedBy:       &scannedBy,
					}); err != nil {
						return err
					}
				}
			}
		}

		if err := u.dos.UpdateStatus(ctx, doID, domain.DOStatusLPBCreated, &lpbDoc.ID); err != nil {
			return err
		}
		if err := u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "DELIVERY_ORDER_CREATE_LPB",
			EntityType: domain.EntityDeliveryOrder,
			EntityID:   &doID,
			Before:     map[string]any{"status": string(do.Status)},
			After:      map[string]any{"status": string(domain.DOStatusLPBCreated), "lpb_id": lpbDoc.ID, "lpb_number": lpbDoc.Number},
		}); err != nil {
			return err
		}

		createdLPB = lpbDoc
		return nil
	})
	if err != nil {
		return nil, err
	}
	return createdLPB, nil
}
