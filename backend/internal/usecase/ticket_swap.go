package usecase

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/docnum"
)

type EscalateSwapInput struct {
	ReplacementProductID *int64
	Notes                *string
}

// EscalateSwap membuat draft Sales Order bertipe UNIT_SWAP dari tiket maintenance yang
// tidak bisa diperbaiki di tempat (unit rusak ditarik IN + unit pengganti dikirim OUT, T3.10).
func (u *TicketUsecase) EscalateSwap(ctx context.Context, actor domain.Principal, ticketID int64, in EscalateSwapInput) (*domain.TicketDetail, error) {
	if u.salesOrders == nil || u.inventory == nil {
		return nil, domain.ErrInvalidStateTransition
	}

	now := u.clock.Now()
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		t, err := u.repo.FindByIDForUpdate(ctx, ticketID)
		if err != nil {
			return err
		}
		if t.Status == domain.TicketClosed || t.Status == domain.TicketCancelled {
			return fmt.Errorf("%w: tiket berstatus %s tidak dapat dieskalasi ke swap", domain.ErrInvalidStateTransition, t.Status)
		}
		if t.SwapSOID != nil {
			return fmt.Errorf("%w: tiket ini sudah memiliki SO swap (#%d)", domain.ErrConflict, *t.SwapSOID)
		}

		serial, err := u.inventory.FindSerialByID(ctx, t.ProductSerialID)
		if err != nil {
			return err
		}
		if serial.ContractID == nil {
			return fmt.Errorf("%w: unit %s tidak terhubung ke kontrak sewa aktif untuk dibuatkan SO UNIT_SWAP",
				domain.ErrInvalidStateTransition, serial.AssetTag)
		}

		replProductID := serial.ProductID
		if in.ReplacementProductID != nil && *in.ReplacementProductID > 0 {
			replProductID = *in.ReplacementProductID
		}

		soNum, err := u.docs.Next(ctx, "SO", docnum.ScopeGlobal, now)
		if err != nil {
			return err
		}
		note := fmt.Sprintf("Eskalasi tukar unit dari tiket %s (%s)", t.TicketNumber, t.Title)
		if c := cleanOpt(in.Notes); c != nil {
			note = *c
		}
		tid := t.ID
		createdSO, err := u.salesOrders.Create(ctx, &domain.SalesOrder{
			SONumber:        soNum,
			OrderType:       domain.OrderTypeUnitSwap,
			CustomerID:      t.CustomerID,
			CustomerSiteID:  t.CustomerSiteID,
			ContractID:      serial.ContractID,
			Status:          domain.SODraft,
			OrderDate:       now,
			IsPPN:           false,
			Subtotal:        decimal.Zero,
			DiscountAmount:  decimal.Zero,
			DPP:             decimal.Zero,
			DPPOtherValue:   decimal.Zero,
			TaxRate:         decimal.Zero,
			TaxAmount:       decimal.Zero,
			GrandTotal:      decimal.Zero,
			RelatedTicketID: &tid,
			Notes:           &note,
			CreatedBy:       actor.UserID,
		})
		if err != nil {
			return err
		}

		sid := serial.ID
		inNote := fmt.Sprintf("Tarik unit rusak %s", serial.AssetTag)
		outNote := "Kirim unit pengganti (swap)"
		if _, err := u.salesOrders.ReplaceItems(ctx, createdSO.ID, []domain.SalesOrderItem{
			{
				ProductID:       serial.ProductID,
				Direction:       domain.DirectionIn,
				Qty:             1,
				UnitPrice:       decimal.Zero,
				LineTotal:       decimal.Zero,
				ProductSerialID: &sid,
				Notes:           &inNote,
			},
			{
				ProductID: replProductID,
				Direction: domain.DirectionOut,
				Qty:       1,
				UnitPrice: decimal.Zero,
				LineTotal: decimal.Zero,
				Notes:     &outNote,
			},
		}); err != nil {
			return err
		}

		if err := u.repo.SetSwapSO(ctx, t.ID, createdSO.ID); err != nil {
			return err
		}
		msg := fmt.Sprintf("Dieskalasi ke Sales Order Tukar Unit %s", createdSO.SONumber)
		if _, err := u.repo.InsertActivity(ctx, &domain.TicketActivity{
			TicketID:     t.ID,
			UserID:       actor.UserID,
			ActivityType: domain.ActivityComment,
			Message:      &msg,
		}); err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "TICKET_ESCALATE_SWAP",
			EntityType: "maintenance_ticket",
			EntityID:   &t.ID,
			After: map[string]any{
				"swap_so_id":     createdSO.ID,
				"swap_so_number": createdSO.SONumber,
			},
		})
	})
	if err != nil {
		return nil, err
	}
	return u.Get(ctx, ticketID)
}
