package usecase

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/clock"
)

// WithPOD memasang repository POD/rental_placements pada LPBUsecase (T2.7).
func (u *LPBUsecase) WithPOD(pod domain.DeliveryPODRepository) *LPBUsecase {
	u.pod = pod
	return u
}

type DeliverLPBInput struct {
	ReceiverName  string
	PODPhotoPath  *string
	DeliveredDate *string // opsional YYYY-MM-DD (default hari ini WIB)
	Notes         *string
}

type PickupLPBInput struct {
	PickupDate *string // opsional YYYY-MM-DD (default hari ini WIB)
	Notes      *string
}

// Deliver menandai LPB OUTBOUND pengiriman (RENTAL_DELIVERY / CONSUMABLE_DELIVERY) telah diterima pelanggan (POD).
// Untuk RENTAL_DELIVERY: SN IN_TRANSIT → RENTED + buat rental_placements (start_date = tanggal POD WIB).
// DO → COMPLETED; SO → COMPLETED bila seluruh DO selesai (plan Bagian 6.4).
func (u *LPBUsecase) Deliver(ctx context.Context, actor domain.Principal, lpbID int64, in DeliverLPBInput) (*LPBDetail, error) {
	receiver := strings.TrimSpace(in.ReceiverName)
	if n := utf8.RuneCountInString(receiver); n < 2 || n > 100 {
		return nil, lpbInvalid([]domain.FieldError{
			{Field: "receiver_name", Message: "nama penerima wajib diisi (2–100 karakter)"},
		})
	}
	now := u.clock.Now()
	deliveredDate := clock.DateWIB(now)
	if in.DeliveredDate != nil && strings.TrimSpace(*in.DeliveredDate) != "" {
		s := strings.TrimSpace(*in.DeliveredDate)
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return nil, lpbInvalid([]domain.FieldError{
				{Field: "delivered_date", Message: "format tanggal harus YYYY-MM-DD"},
			})
		}
		deliveredDate = s
	}

	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if u.pod == nil {
			return domain.ErrInvalidStateTransition
		}
		l, err := u.repo.FindByIDForUpdate(ctx, lpbID)
		if err != nil {
			return err
		}
		if !actor.CanAccessWarehouse(l.WarehouseID) {
			return domain.ErrForbiddenWarehouse
		}
		if l.Status != domain.LPBApproved {
			return fmt.Errorf("%w: hanya LPB APPROVED yang dapat ditandai terkirim", domain.ErrInvalidStateTransition)
		}
		if l.SubType != domain.LPBRentalDelivery && l.SubType != domain.LPBConsumableDelivery {
			return fmt.Errorf("%w: POD hanya berlaku untuk LPB pengiriman (RENTAL_DELIVERY / CONSUMABLE_DELIVERY)", domain.ErrInvalidStateTransition)
		}
		if l.DeliveryStatus != nil && (*l.DeliveryStatus == "DELIVERED" || *l.DeliveryStatus == "FAILED" || *l.DeliveryStatus == "RETURNED") {
			return fmt.Errorf("%w: pengiriman LPB ini sudah selesai (%s)", domain.ErrInvalidStateTransition, *l.DeliveryStatus)
		}

		var doCtx *domain.DOContext
		if l.ReferenceType != nil && *l.ReferenceType == "DELIVERY_ORDER" && l.ReferenceID != nil {
			doCtx, err = u.pod.LookupDOContext(ctx, *l.ReferenceID)
			if err != nil {
				return err
			}
		}

		if l.SubType == domain.LPBRentalDelivery {
			if doCtx == nil || doCtx.ContractID == nil {
				return fmt.Errorf("%w: LPB pengiriman sewa harus terhubung ke DO & kontrak", domain.ErrInvalidStateTransition)
			}
			items, err := u.repo.Items(ctx, l.ID)
			if err != nil {
				return err
			}
			serialIDs := make([]int64, 0)
			serialProd := make(map[int64]int64)
			for _, it := range items {
				if !it.HasSerialNumber {
					continue
				}
				ids, err := unitIDs(it)
				if err != nil {
					return err
				}
				for _, id := range ids {
					serialIDs = append(serialIDs, id)
					serialProd[id] = it.ProductID
				}
			}
			sort.Slice(serialIDs, func(i, j int) bool { return serialIDs[i] < serialIDs[j] })
			locked, err := u.stock.LockSerials(ctx, serialIDs)
			if err != nil {
				return err
			}
			if len(locked) != len(serialIDs) {
				return domain.ErrSerialNotValid
			}
			refType := "LPB"
			for _, s := range locked {
				if s.Status != domain.SerialInTransit {
					return fmt.Errorf("%w: unit %s harus berstatus IN_TRANSIT (saat ini %s)", domain.ErrSerialNotValid, s.Number, s.Status)
				}
				if err := u.pod.MarkSerialRented(ctx, s.ID, doCtx.CustomerID, doCtx.SiteID, doCtx.ContractID); err != nil {
					return err
				}
				cItemID, rate, err := u.pod.FindContractItemRate(ctx, *doCtx.ContractID, serialProd[s.ID], doCtx.SiteID)
				if err != nil {
					return err
				}
				if err := u.pod.InsertRentalPlacement(ctx, *doCtx.ContractID, cItemID, s.ID, doCtx.SiteID, rate, deliveredDate, l.ID); err != nil {
					return err
				}
				fromSt := string(s.Status)
				if err := u.stock.InsertSerialHistory(ctx, domain.SerialHistoryEntry{
					SerialID:      s.ID,
					FromStatus:    &fromSt,
					ToStatus:      string(domain.SerialRented),
					EventType:     "DELIVERED",
					ReferenceType: &refType,
					ReferenceID:   &l.ID,
					UserID:        &actor.UserID,
					Notes:         cleanOpt(in.Notes),
				}); err != nil {
					return err
				}
			}
		}

		if err := u.pod.MarkDelivered(ctx, l.ID, now, receiver, cleanOpt(in.PODPhotoPath)); err != nil {
			return err
		}
		if doCtx != nil {
			if err := u.pod.CompleteDOAndMaybeSO(ctx, doCtx.DOID); err != nil {
				return err
			}
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "LPB_DELIVERED",
			EntityType: domain.EntityLPB,
			EntityID:   &l.ID,
			After: map[string]any{
				"delivery_status": "DELIVERED",
				"receiver_name":   receiver,
				"delivered_date":  deliveredDate,
			},
		})
	})
	if err != nil {
		return nil, err
	}
	return u.reload(ctx, lpbID)
}

// Pickup menandai unit penarikan pada LPB RENTAL_RETURN (DRAFT/PENDING) sudah dijemput di lokasi pelanggan:
// SN RENTED → IN_TRANSIT, menutup rental_placements aktif (end_date = pickup_date WIB), sehingga tagihan berhenti.
func (u *LPBUsecase) Pickup(ctx context.Context, actor domain.Principal, lpbID int64, in PickupLPBInput) (*LPBDetail, error) {
	now := u.clock.Now()
	pickupDate := clock.DateWIB(now)
	if in.PickupDate != nil && strings.TrimSpace(*in.PickupDate) != "" {
		s := strings.TrimSpace(*in.PickupDate)
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return nil, lpbInvalid([]domain.FieldError{
				{Field: "pickup_date", Message: "format tanggal harus YYYY-MM-DD"},
			})
		}
		pickupDate = s
	}

	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if u.pod == nil {
			return domain.ErrInvalidStateTransition
		}
		l, err := u.repo.FindByIDForUpdate(ctx, lpbID)
		if err != nil {
			return err
		}
		if !actor.CanAccessWarehouse(l.WarehouseID) {
			return domain.ErrForbiddenWarehouse
		}
		if l.SubType != domain.LPBRentalReturn {
			return fmt.Errorf("%w: pickup penarikan hanya berlaku untuk LPB RENTAL_RETURN", domain.ErrInvalidStateTransition)
		}
		if l.Status != domain.LPBDraft && l.Status != domain.LPBPendingApproval {
			return fmt.Errorf("%w: LPB RENTAL_RETURN berstatus %s tidak dapat ditandai pickup", domain.ErrInvalidStateTransition, l.Status)
		}
		if l.DeliveryStatus != nil && *l.DeliveryStatus == "PICKED_UP" {
			return fmt.Errorf("%w: unit pada LPB ini sudah ditandai PICKED_UP", domain.ErrInvalidStateTransition)
		}

		endReason := "WITHDRAWAL"
		if l.ReferenceType != nil && *l.ReferenceType == "DELIVERY_ORDER" && l.ReferenceID != nil {
			if doCtx, err := u.pod.LookupDOContext(ctx, *l.ReferenceID); err == nil && doCtx.OrderType == domain.OrderTypeUnitSwap {
				endReason = "SWAP"
			}
		}

		items, err := u.repo.Items(ctx, l.ID)
		if err != nil {
			return err
		}
		serialIDs := make([]int64, 0)
		for _, it := range items {
			if !it.HasSerialNumber {
				continue
			}
			ids, err := unitIDs(it)
			if err != nil {
				return err
			}
			serialIDs = append(serialIDs, ids...)
		}
		if len(serialIDs) == 0 {
			return lpbInvalid([]domain.FieldError{
				{Field: "serials", Message: "scan/tautkan minimal 1 unit SN sebelum menandai pickup"},
			})
		}

		sort.Slice(serialIDs, func(i, j int) bool { return serialIDs[i] < serialIDs[j] })
		locked, err := u.stock.LockSerials(ctx, serialIDs)
		if err != nil {
			return err
		}
		if len(locked) != len(serialIDs) {
			return domain.ErrSerialNotValid
		}
		refType := "LPB"
		for _, s := range locked {
			if s.Status != domain.SerialRented {
				return fmt.Errorf("%w: unit %s harus berstatus RENTED untuk dijemput (saat ini %s)", domain.ErrSerialNotValid, s.Number, s.Status)
			}
			if err := u.pod.MarkSerialPickupInTransit(ctx, s.ID); err != nil {
				return err
			}
			if err := u.pod.CloseActivePlacement(ctx, s.ID, pickupDate, endReason, l.ID); err != nil {
				return err
			}
			fromSt := string(s.Status)
			if err := u.stock.InsertSerialHistory(ctx, domain.SerialHistoryEntry{
				SerialID:      s.ID,
				FromStatus:    &fromSt,
				ToStatus:      string(domain.SerialInTransit),
				EventType:     "PICKED_UP",
				ReferenceType: &refType,
				ReferenceID:   &l.ID,
				UserID:        &actor.UserID,
				Notes:         cleanOpt(in.Notes),
			}); err != nil {
				return err
			}
		}

		if err := u.pod.MarkPickedUp(ctx, l.ID); err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "LPB_PICKED_UP",
			EntityType: domain.EntityLPB,
			EntityID:   &l.ID,
			After: map[string]any{
				"delivery_status": "PICKED_UP",
				"pickup_date":     pickupDate,
				"end_reason":      endReason,
				"serials":         len(serialIDs),
			},
		})
	})
	if err != nil {
		return nil, err
	}
	return u.reload(ctx, lpbID)
}
