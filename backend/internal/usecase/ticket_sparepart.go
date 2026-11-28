package usecase

import (
	"context"
	"fmt"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/clock"
)

type RequestSparepartInput struct {
	ProductID  int64
	Qty        int
	IsBillable bool
	Notes      *string
}

type CreateSparepartLPBInput struct {
	WarehouseID int64
	RackID      int64
	Notes       *string
}

// RequestSparepart mencatat permintaan sparepart dari teknisi untuk satu tiket (T3.9).
func (u *TicketUsecase) RequestSparepart(ctx context.Context, actor domain.Principal, ticketID int64, in RequestSparepartInput) (*domain.TicketDetail, error) {
	var errs []domain.FieldError
	if in.ProductID <= 0 {
		errs = append(errs, domain.FieldError{Field: "product_id", Message: "produk sparepart wajib dipilih"})
	}
	if in.Qty <= 0 {
		errs = append(errs, domain.FieldError{Field: "qty", Message: "jumlah sparepart minimal 1"})
	}
	if len(errs) > 0 {
		return nil, &domain.ValidationError{Message: "Permintaan sparepart tidak valid", Errors: errs}
	}

	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		t, err := u.repo.FindByIDForUpdate(ctx, ticketID)
		if err != nil {
			return err
		}
		if t.Status == domain.TicketClosed || t.Status == domain.TicketCancelled {
			return fmt.Errorf("%w: tidak dapat meminta sparepart untuk tiket berstatus %s", domain.ErrInvalidStateTransition, t.Status)
		}
		sp, err := u.repo.InsertSparepart(ctx, &domain.TicketSparepart{
			TicketID:    t.ID,
			ProductID:   in.ProductID,
			Qty:         in.Qty,
			IsBillable:  in.IsBillable,
			RequestedBy: actor.UserID,
			Notes:       cleanOpt(in.Notes),
		})
		if err != nil {
			return err
		}
		if t.Status == domain.TicketAssigned || t.Status == domain.TicketInProgress {
			if err := u.repo.UpdateStatus(ctx, t.ID, domain.TicketWaitingPart, nil, nil, nil); err != nil {
				return err
			}
		}
		msg := fmt.Sprintf("Meminta sparepart (produk #%d, qty %d)", in.ProductID, in.Qty)
		if _, err := u.repo.InsertActivity(ctx, &domain.TicketActivity{
			TicketID:     t.ID,
			UserID:       actor.UserID,
			ActivityType: domain.ActivityComment,
			Message:      &msg,
		}); err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "TICKET_SPAREPART_REQUEST",
			EntityType: "maintenance_ticket",
			EntityID:   &t.ID,
			After: map[string]any{
				"sparepart_id": sp.ID,
				"product_id":   in.ProductID,
				"qty":          in.Qty,
				"is_billable":  in.IsBillable,
			},
		})
	})
	if err != nil {
		return nil, err
	}
	return u.Get(ctx, ticketID)
}

// CreateSparepartLPB membuat draft LPB OUTBOUND `SPARE_PART_ISSUE` oleh staf gudang
// dari permintaan sparepart berstatus REQUESTED (T3.9).
func (u *TicketUsecase) CreateSparepartLPB(ctx context.Context, actor domain.Principal, sparepartID int64, in CreateSparepartLPBInput) (*domain.LPB, error) {
	if u.lpb == nil || u.warehouses == nil || u.racks == nil {
		return nil, domain.ErrInvalidStateTransition
	}
	var errs []domain.FieldError
	if in.WarehouseID <= 0 {
		errs = append(errs, domain.FieldError{Field: "warehouse_id", Message: "gudang wajib dipilih"})
	}
	if in.RackID <= 0 {
		errs = append(errs, domain.FieldError{Field: "rack_id", Message: "rak asal wajib dipilih"})
	}
	if len(errs) > 0 {
		return nil, &domain.ValidationError{Message: "Data LPB sparepart tidak valid", Errors: errs}
	}
	if !actor.CanAccessWarehouse(in.WarehouseID) {
		return nil, domain.ErrForbiddenWarehouse
	}

	now := u.clock.Now()
	var createdLPB *domain.LPB
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		sp, err := u.repo.FindSparepartByIDForUpdate(ctx, sparepartID)
		if err != nil {
			return err
		}
		if sp.Status != "REQUESTED" {
			return fmt.Errorf("%w: permintaan sparepart berstatus %s tidak dapat dibuatkan LPB", domain.ErrInvalidStateTransition, sp.Status)
		}
		t, err := u.repo.FindByID(ctx, sp.TicketID)
		if err != nil {
			return err
		}
		wh, err := u.warehouses.FindByID(ctx, in.WarehouseID)
		if err != nil {
			return err
		}
		rack, err := u.racks.FindByID(ctx, in.RackID)
		if err != nil {
			return err
		}
		if rack.WarehouseID != wh.ID {
			return &domain.ValidationError{
				Message: "Rak tidak berada di gudang terpilih",
				Errors:  []domain.FieldError{{Field: "rack_id", Message: "rak tidak cocok dengan gudang"}},
			}
		}

		num, err := u.docs.Next(ctx, "LPBO", wh.Code, now)
		if err != nil {
			return err
		}
		refType := "MAINTENANCE_TICKET"
		refNum := t.TicketNumber
		note := fmt.Sprintf("Pengeluaran sparepart untuk tiket %s", t.TicketNumber)
		if c := cleanOpt(in.Notes); c != nil {
			note = *c
		}
		createdLPB, err = u.lpb.Create(ctx, &domain.LPB{
			Number:          num,
			Type:            domain.LPBOutbound,
			SubType:         domain.LPBSparePartIssue,
			Status:          domain.LPBDraft,
			WarehouseID:     wh.ID,
			DocDate:         clock.DateWIB(now),
			ReferenceType:   &refType,
			ReferenceID:     &t.ID,
			ReferenceNumber: &refNum,
			CustomerID:      &t.CustomerID,
			CustomerSiteID:  &t.CustomerSiteID,
			Notes:           &note,
			CreatedBy:       actor.UserID,
		})
		if err != nil {
			return err
		}
		if _, err := u.lpb.InsertItem(ctx, createdLPB.ID, &domain.LPBItem{
			ProductID: sp.ProductID,
			RackID:    rack.ID,
			Qty:       sp.Qty,
			Notes:     sp.Notes,
		}); err != nil {
			return err
		}
		if err := u.repo.MarkSparepartLPBCreated(ctx, sp.ID, createdLPB.ID); err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "TICKET_SPAREPART_LPB",
			EntityType: "maintenance_ticket",
			EntityID:   &t.ID,
			After: map[string]any{
				"sparepart_id": sp.ID,
				"lpb_id":       createdLPB.ID,
				"lpb_number":   createdLPB.Number,
			},
		})
	})
	if err != nil {
		return nil, err
	}
	return createdLPB, nil
}
