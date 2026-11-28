package usecase

import (
	"context"
	"fmt"
	"strings"

	"rental-inventory-api/internal/domain"
)

// QCSerialInput adalah parameter transisi QC unit SN di gudang (T3.5).
type QCSerialInput struct {
	ToStatus string
	Notes    *string
}

// QCSerial memproses hasil QC unit SN di gudang:
//   - MAINTENANCE → AVAILABLE (lulus QC, kondisi GOOD)
//   - MAINTENANCE → BROKEN    (gagal QC, kondisi BROKEN)
//   - BROKEN → MAINTENANCE    (masuk perbaikan / inspeksi ulang)
//
// Invariant I1 (stock_balances.qty_on_hand) tetap terjaga karena seluruh status
// berada di dalam himpunan unit gudang (AVAILABLE/RESERVED/MAINTENANCE/BROKEN).
func (u *InventoryUsecase) QCSerial(ctx context.Context, actor domain.Principal, serialID int64, in QCSerialInput) (*domain.Serial, []domain.SerialHistory, error) {
	if u.tx == nil || u.stock == nil || u.audit == nil {
		return nil, nil, domain.ErrInvalidStateTransition
	}
	target := domain.SerialStatus(strings.ToUpper(strings.TrimSpace(in.ToStatus)))
	if target != domain.SerialAvailable && target != domain.SerialMaintenance && target != domain.SerialBroken {
		return nil, nil, &domain.ValidationError{
			Message: "Status QC tidak valid",
			Errors:  []domain.FieldError{{Field: "to_status", Message: "status QC harus AVAILABLE, MAINTENANCE, atau BROKEN"}},
		}
	}

	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		locked, err := u.stock.LockSerials(ctx, []int64{serialID})
		if err != nil {
			return err
		}
		if len(locked) == 0 {
			return domain.ErrNotFound
		}
		s := locked[0]
		if s.WarehouseID == nil || s.RackID == nil {
			return fmt.Errorf("%w: unit tidak berada di rak gudang", domain.ErrInvalidStateTransition)
		}
		if !actor.CanAccessWarehouse(*s.WarehouseID) {
			return domain.ErrForbiddenWarehouse
		}

		var cond domain.SerialCondition
		var eventType string
		switch {
		case s.Status == domain.SerialMaintenance && target == domain.SerialAvailable:
			cond = domain.ConditionGood
			eventType = "QC_PASSED"
		case s.Status == domain.SerialMaintenance && target == domain.SerialBroken:
			cond = domain.ConditionBroken
			eventType = "QC_FAILED"
		case s.Status == domain.SerialBroken && target == domain.SerialMaintenance:
			cond = domain.ConditionMinorDefect
			eventType = "QC_STARTED"
		default:
			return fmt.Errorf("%w: transisi QC %s → %s tidak diizinkan", domain.ErrInvalidStateTransition, s.Status, target)
		}

		if err := u.stock.UpdateSerialQC(ctx, s.ID, target, cond); err != nil {
			return err
		}
		fromSt := string(s.Status)
		if err := u.stock.InsertSerialHistory(ctx, domain.SerialHistoryEntry{
			SerialID:   s.ID,
			FromStatus: &fromSt,
			ToStatus:   string(target),
			FromRackID: s.RackID,
			ToRackID:   s.RackID,
			EventType:  eventType,
			UserID:     &actor.UserID,
			Notes:      cleanOpt(in.Notes),
		}); err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "SERIAL_QC",
			EntityType: "product_serial",
			EntityID:   &s.ID,
			Before:     map[string]any{"status": fromSt},
			After:      map[string]any{"status": string(target), "condition": string(cond), "event": eventType},
		})
	})
	if err != nil {
		return nil, nil, err
	}

	unit, err := u.repo.FindSerialByID(ctx, serialID)
	if err != nil {
		return nil, nil, err
	}
	history, err := u.repo.RecentSerialHistory(ctx, serialID, 50)
	if err != nil {
		return nil, nil, err
	}
	return unit, history, nil
}
