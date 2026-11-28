package usecase

import (
	"context"
	"fmt"
	"sort"

	"rental-inventory-api/internal/domain"
)

// applyDeliveryOut memproses persetujuan LPB OUTBOUND dari Delivery Order
// (RENTAL_DELIVERY / CONSUMABLE_DELIVERY): mengurangi qty_on_hand dan qty_reserved sekaligus,
// serta memindahkan unit SN dari RESERVED (milik DO) → IN_TRANSIT menuju customer/site.
func (u *LPBUsecase) applyDeliveryOut(ctx context.Context, actor domain.Principal, l *domain.LPB, items []domain.LPBItem) ([]map[string]any, error) {
	fromDO := l.ReferenceType != nil && *l.ReferenceType == "DELIVERY_ORDER" && l.ReferenceID != nil
	if !fromDO {
		return nil, fmt.Errorf("%w: LPB pengiriman wajib terhubung ke Delivery Order", domain.ErrReservationMismatch)
	}
	doID := *l.ReferenceID

	moves := make([]stockMove, 0, len(items))
	allSerialIDs := make([]int64, 0)
	serialRack := make(map[int64]int64)
	serialProd := make(map[int64]int64)

	for _, it := range items {
		var ids []int64
		if it.HasSerialNumber {
			var err error
			ids, err = unitIDs(it)
			if err != nil {
				return nil, err
			}
			for _, id := range ids {
				allSerialIDs = append(allSerialIDs, id)
				serialRack[id] = it.RackID
				serialProd[id] = it.ProductID
			}
		}
		moves = append(moves, stockMove{
			lpb:             l,
			item:            it,
			rackID:          it.RackID,
			direction:       "OUT",
			consumeReserved: true,
			serialIDs:       ids,
		})
	}

	// 1. Saldo dulu (urut produk, rak).
	results, err := u.applyMoves(ctx, actor, moves)
	if err != nil {
		return nil, err
	}

	// 2. Unit SN (urut id).
	if len(allSerialIDs) > 0 {
		sort.Slice(allSerialIDs, func(i, j int) bool { return allSerialIDs[i] < allSerialIDs[j] })
		locked, err := u.stock.LockSerials(ctx, allSerialIDs)
		if err != nil {
			return nil, err
		}
		if len(locked) != len(allSerialIDs) {
			return nil, domain.ErrSerialNotValid
		}
		refType := "LPB"
		for _, s := range locked {
			if s.ProductID != serialProd[s.ID] || s.RackID == nil || *s.RackID != serialRack[s.ID] {
				return nil, fmt.Errorf("%w: unit %s tidak berada di rak item", domain.ErrReservationMismatch, s.Number)
			}
			if s.Status != domain.SerialReserved || s.ReservedForDOID == nil || *s.ReservedForDOID != doID {
				return nil, fmt.Errorf("%w: unit %s tidak tereservasi untuk DO ini", domain.ErrReservationMismatch, s.Number)
			}
			if err := u.stock.DispatchReservedSerial(ctx, s.ID, doID, l.CustomerID, l.CustomerSiteID); err != nil {
				return nil, err
			}
			fromSt := string(s.Status)
			if err := u.stock.InsertSerialHistory(ctx, domain.SerialHistoryEntry{
				SerialID:      s.ID,
				FromStatus:    &fromSt,
				ToStatus:      string(domain.SerialInTransit),
				FromRackID:    s.RackID,
				EventType:     "LPB_APPROVED",
				ReferenceType: &refType,
				ReferenceID:   &l.ID,
				UserID:        &actor.UserID,
			}); err != nil {
				return nil, err
			}
		}
	}

	return entriesFor(results, l.ID), nil
}

// applyRentalReturn memproses persetujuan LPB INBOUND RENTAL_RETURN:
// unit IN_TRANSIT dikembalikan ke gudang/rak (AVAILABLE atau BROKEN sesuai kondisi scan)
// dan customer/site/contract pada unit dikosongkan.
func (u *LPBUsecase) applyRentalReturn(ctx context.Context, actor domain.Principal, l *domain.LPB, items []domain.LPBItem, racks map[int64]domain.Rack) ([]map[string]any, error) {
	moves := make([]stockMove, 0, len(items))
	allSerialIDs := make([]int64, 0)
	serialRack := make(map[int64]int64)
	serialProd := make(map[int64]int64)
	serialCond := make(map[int64]domain.SerialCondition)

	for _, it := range items {
		var ids []int64
		if it.HasSerialNumber {
			var err error
			ids, err = unitIDs(it)
			if err != nil {
				return nil, err
			}
			for _, sc := range it.Serials {
				id := *sc.ProductSerialID
				allSerialIDs = append(allSerialIDs, id)
				serialRack[id] = it.RackID
				serialProd[id] = it.ProductID
				serialCond[id] = sc.Condition
			}
		}
		moves = append(moves, stockMove{
			lpb:       l,
			item:      it,
			rackID:    it.RackID,
			direction: "IN",
			serialIDs: ids,
		})
	}

	results, err := u.applyMoves(ctx, actor, moves)
	if err != nil {
		return nil, err
	}

	if len(allSerialIDs) > 0 {
		sort.Slice(allSerialIDs, func(i, j int) bool { return allSerialIDs[i] < allSerialIDs[j] })
		locked, err := u.stock.LockSerials(ctx, allSerialIDs)
		if err != nil {
			return nil, err
		}
		if len(locked) != len(allSerialIDs) {
			return nil, domain.ErrSerialNotValid
		}
		refType := "LPB"
		for _, s := range locked {
			if s.ProductID != serialProd[s.ID] {
				return nil, fmt.Errorf("%w: unit %s bukan produk item ini", domain.ErrSerialNotValid, s.Number)
			}
			if s.Status != domain.SerialInTransit {
				return nil, fmt.Errorf("%w: unit %s harus berstatus IN_TRANSIT (saat ini %s)", domain.ErrSerialNotValid, s.Number, s.Status)
			}
			cond := serialCond[s.ID]
			rackID := serialRack[s.ID]
			toStatus := domain.SerialAvailable
			switch {
			case cond == domain.ConditionBroken:
				toStatus = domain.SerialBroken
			case cond == domain.ConditionMinorDefect || !racks[rackID].IsSellable:
				toStatus = domain.SerialMaintenance
			}
			if err := u.stock.ReturnSerialToRack(ctx, s.ID, toStatus, cond, l.WarehouseID, rackID); err != nil {
				return nil, err
			}
			fromSt := string(s.Status)
			if err := u.stock.InsertSerialHistory(ctx, domain.SerialHistoryEntry{
				SerialID:      s.ID,
				FromStatus:    &fromSt,
				ToStatus:      string(toStatus),
				ToRackID:      &rackID,
				EventType:     "LPB_APPROVED",
				ReferenceType: &refType,
				ReferenceID:   &l.ID,
				UserID:        &actor.UserID,
			}); err != nil {
				return nil, err
			}
		}
	}

	return entriesFor(results, l.ID), nil
}
