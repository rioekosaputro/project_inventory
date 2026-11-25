package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/clock"
	"rental-inventory-api/pkg/docnum"
	"rental-inventory-api/pkg/money"
)

const (
	docTypeSO = "SO"
	docTypeDO = "DO"
)

// SalesOrderUsecase mengelola Sales Order (4 tipe), kalkulasi pajak server-side, confirm/cancel,
// dan penerbitan Delivery Order (OUTBOUND/INBOUND) ke gudang (T2.5).
type SalesOrderUsecase struct {
	tx         domain.TxManager
	orders     domain.SalesOrderRepository
	customers  domain.CustomerRepository
	contracts  domain.ContractRepository
	products   domain.ProductRepository
	warehouses domain.WarehouseRepository
	inventory  domain.InventoryRepository
	taxes      domain.TaxSettingRepository
	audit      domain.AuditRepository
	docGen     domain.DocNumberGenerator
	clk        clock.Clock
}

func NewSalesOrderUsecase(
	tx domain.TxManager,
	orders domain.SalesOrderRepository,
	customers domain.CustomerRepository,
	contracts domain.ContractRepository,
	products domain.ProductRepository,
	warehouses domain.WarehouseRepository,
	inventory domain.InventoryRepository,
	taxes domain.TaxSettingRepository,
	audit domain.AuditRepository,
	docGen domain.DocNumberGenerator,
	clk clock.Clock,
) *SalesOrderUsecase {
	return &SalesOrderUsecase{
		tx: tx, orders: orders, customers: customers, contracts: contracts,
		products: products, warehouses: warehouses, inventory: inventory,
		taxes: taxes, audit: audit, docGen: docGen, clk: clk,
	}
}

type SOItemInput struct {
	ProductID       int64
	Direction       *string // opsional: default OUT untuk NEW_DELIVERY/CONSUMABLE, IN untuk WITHDRAWAL; wajib untuk UNIT_SWAP
	Qty             int
	UnitPrice       *string
	ProductSerialID *int64
	Notes           *string
}

type CreateSOInput struct {
	OrderType      string
	CustomerID     int64
	CustomerSiteID int64
	ContractID     *int64
	OrderDate      *string // YYYY-MM-DD, default hari ini WIB
	RequestedDate  *string // YYYY-MM-DD opsional
	IsPPN          *bool
	DiscountAmount *string
	Notes          *string
	Items          []SOItemInput
}

type UpdateSOInput struct {
	CustomerSiteID *int64
	OrderDate      *string
	RequestedDate  Patch[string]
	IsPPN          *bool
	DiscountAmount *string
	Notes          Patch[string]
	Items          *[]SOItemInput
}

type CancelSOInput struct {
	CancelReason string
}

type IssueDOInput struct {
	WarehouseID   int64
	ScheduledDate *string // YYYY-MM-DD opsional
	Notes         *string
}

func (u *SalesOrderUsecase) List(ctx context.Context, f domain.SalesOrderFilter, p domain.PageParams) ([]domain.SalesOrder, int64, error) {
	return u.orders.List(ctx, f, p)
}

func (u *SalesOrderUsecase) Get(ctx context.Context, id int64) (*domain.SalesOrderDetail, error) {
	so, err := u.orders.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	items, err := u.orders.ListItems(ctx, id)
	if err != nil {
		return nil, err
	}
	dos, err := u.orders.ListDeliveryOrdersBySO(ctx, id)
	if err != nil {
		return nil, err
	}
	return &domain.SalesOrderDetail{SO: so, Items: items, DeliveryOrders: dos}, nil
}

func (u *SalesOrderUsecase) Create(ctx context.Context, actor domain.Principal, in CreateSOInput) (*domain.SalesOrderDetail, error) {
	otype := domain.OrderType(strings.ToUpper(strings.TrimSpace(in.OrderType)))
	if !otype.Valid() {
		return nil, masterInvalid("Data Sales Order", []domain.FieldError{
			{Field: "order_type", Message: "jenis SO harus NEW_DELIVERY, WITHDRAWAL, UNIT_SWAP, atau CONSUMABLE"},
		})
	}

	var detail *domain.SalesOrderDetail
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		var errs []domain.FieldError

		cust, err := u.customers.FindByID(ctx, in.CustomerID)
		if err != nil {
			errs = append(errs, domain.FieldError{Field: "customer_id", Message: "pelanggan tidak ditemukan"})
		} else if !cust.IsActive {
			errs = append(errs, domain.FieldError{Field: "customer_id", Message: "pelanggan sedang nonaktif"})
		}

		site, err := u.customers.FindSiteByID(ctx, in.CustomerSiteID)
		if err != nil {
			errs = append(errs, domain.FieldError{Field: "customer_site_id", Message: "site pelanggan tidak ditemukan"})
		} else if site.CustomerID != in.CustomerID {
			errs = append(errs, domain.FieldError{Field: "customer_site_id", Message: "site bukan milik pelanggan ini"})
		} else if !site.IsActive {
			errs = append(errs, domain.FieldError{Field: "customer_site_id", Message: "site pelanggan sedang nonaktif"})
		}

		var contract *domain.RentalContract
		if otype != domain.OrderTypeConsumable && in.ContractID == nil {
			errs = append(errs, domain.FieldError{Field: "contract_id", Message: "kontrak wajib diisi untuk jenis SO ini"})
		}
		if in.ContractID != nil {
			c, err := u.contracts.FindByID(ctx, *in.ContractID)
			if err != nil {
				errs = append(errs, domain.FieldError{Field: "contract_id", Message: "kontrak tidak ditemukan"})
			} else if c.CustomerID != in.CustomerID {
				errs = append(errs, domain.FieldError{Field: "contract_id", Message: "kontrak bukan milik pelanggan ini"})
			} else {
				switch otype {
				case domain.OrderTypeNewDelivery, domain.OrderTypeUnitSwap:
					if c.Status != domain.ContractActive {
						errs = append(errs, domain.FieldError{Field: "contract_id", Message: "kontrak harus berstatus ACTIVE"})
					}
				case domain.OrderTypeWithdrawal:
					if c.Status == domain.ContractDraft {
						errs = append(errs, domain.FieldError{Field: "contract_id", Message: "kontrak masih berstatus DRAFT"})
					}
				}
				contract = c
			}
		}

		orderDate := todayDateWIB(u.clk.Now())
		if in.OrderDate != nil && strings.TrimSpace(*in.OrderDate) != "" {
			parsed, err := time.Parse("2006-01-02", strings.TrimSpace(*in.OrderDate))
			if err != nil {
				errs = append(errs, domain.FieldError{Field: "order_date", Message: "format tanggal harus YYYY-MM-DD"})
			} else {
				orderDate = parsed
			}
		}

		var reqDate *time.Time
		if in.RequestedDate != nil && strings.TrimSpace(*in.RequestedDate) != "" {
			parsed, err := time.Parse("2006-01-02", strings.TrimSpace(*in.RequestedDate))
			if err != nil {
				errs = append(errs, domain.FieldError{Field: "requested_date", Message: "format tanggal harus YYYY-MM-DD"})
			} else {
				reqDate = &parsed
			}
		}

		discount := money.Zero
		if in.DiscountAmount != nil && strings.TrimSpace(*in.DiscountAmount) != "" {
			d, err := money.ParseNonNegative(*in.DiscountAmount)
			if err != nil {
				errs = append(errs, domain.FieldError{Field: "discount_amount", Message: "diskon harus berupa angka tidak negatif"})
			} else {
				discount = d
			}
		}

		notes := cleanOpt(in.Notes)
		errs = append(errs, optionalText("notes", notes, maxContractNoteLen)...)

		parsedItems, lineTotals, itemErrs := u.validateSOItems(ctx, otype, in.CustomerID, in.Items)
		errs = append(errs, itemErrs...)
		if len(errs) > 0 {
			return masterInvalid("Data Sales Order", errs)
		}

		isPPN := true
		if cust != nil {
			isPPN = cust.IsPKP
		}
		if contract != nil {
			isPPN = contract.IsPPN
		}
		if in.IsPPN != nil {
			isPPN = *in.IsPPN
		}

		rule, err := u.resolveTaxRule(ctx, orderDate)
		if err != nil {
			return err
		}
		tb, err := money.CalculateLinesTax(lineTotals, discount, isPPN, rule)
		if err != nil {
			if errors.Is(err, money.ErrDiscountExceeds) {
				return masterInvalid("Data Sales Order", []domain.FieldError{
					{Field: "discount_amount", Message: "diskon tidak boleh melebihi subtotal"},
				})
			}
			return err
		}

		soNum, err := u.docGen.Next(ctx, docTypeSO, docnum.ScopeGlobal, u.clk.Now())
		if err != nil {
			return err
		}

		so := &domain.SalesOrder{
			SONumber:       soNum,
			OrderType:      otype,
			CustomerID:     in.CustomerID,
			CustomerSiteID: in.CustomerSiteID,
			ContractID:     in.ContractID,
			Status:         domain.SODraft,
			OrderDate:      orderDate,
			RequestedDate:  reqDate,
			IsPPN:          isPPN,
			Subtotal:       tb.Subtotal,
			DiscountAmount: tb.DiscountAmount,
			DPP:            tb.DPP,
			DPPOtherValue:  tb.DPPOtherValue,
			TaxRate:        tb.TaxRate,
			TaxAmount:      tb.TaxAmount,
			GrandTotal:     tb.GrandTotal,
			Notes:          notes,
			CreatedBy:      actor.UserID,
		}
		created, err := u.orders.Create(ctx, so)
		if err != nil {
			return err
		}
		savedItems, err := u.orders.ReplaceItems(ctx, created.ID, parsedItems)
		if err != nil {
			return err
		}
		created, err = u.orders.FindByID(ctx, created.ID)
		if err != nil {
			return err
		}
		if err := auditCreate(ctx, u.audit, domain.EntitySalesOrder, created.ID, soView(created)); err != nil {
			return err
		}
		detail = &domain.SalesOrderDetail{SO: created, Items: savedItems, DeliveryOrders: []domain.DeliveryOrderSummary{}}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return detail, nil
}

func (u *SalesOrderUsecase) Update(ctx context.Context, id int64, in UpdateSOInput) (*domain.SalesOrderDetail, error) {
	var detail *domain.SalesOrderDetail
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		cur, err := u.orders.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if cur.Status != domain.SODraft {
			return fmt.Errorf("%w: hanya Sales Order berstatus DRAFT yang dapat diubah", domain.ErrInvalidStateTransition)
		}

		next := *cur
		var errs []domain.FieldError

		if in.CustomerSiteID != nil {
			site, err := u.customers.FindSiteByID(ctx, *in.CustomerSiteID)
			if err != nil {
				errs = append(errs, domain.FieldError{Field: "customer_site_id", Message: "site pelanggan tidak ditemukan"})
			} else if site.CustomerID != cur.CustomerID {
				errs = append(errs, domain.FieldError{Field: "customer_site_id", Message: "site bukan milik pelanggan ini"})
			} else if !site.IsActive {
				errs = append(errs, domain.FieldError{Field: "customer_site_id", Message: "site pelanggan sedang nonaktif"})
			} else {
				next.CustomerSiteID = *in.CustomerSiteID
			}
		}

		if in.OrderDate != nil && strings.TrimSpace(*in.OrderDate) != "" {
			parsed, err := time.Parse("2006-01-02", strings.TrimSpace(*in.OrderDate))
			if err != nil {
				errs = append(errs, domain.FieldError{Field: "order_date", Message: "format tanggal harus YYYY-MM-DD"})
			} else {
				next.OrderDate = parsed
			}
		}
		if in.RequestedDate.Set {
			if in.RequestedDate.Value == nil || strings.TrimSpace(*in.RequestedDate.Value) == "" {
				next.RequestedDate = nil
			} else {
				parsed, err := time.Parse("2006-01-02", strings.TrimSpace(*in.RequestedDate.Value))
				if err != nil {
					errs = append(errs, domain.FieldError{Field: "requested_date", Message: "format tanggal harus YYYY-MM-DD"})
				} else {
					next.RequestedDate = &parsed
				}
			}
		}
		if in.IsPPN != nil {
			next.IsPPN = *in.IsPPN
		}
		if in.DiscountAmount != nil {
			if strings.TrimSpace(*in.DiscountAmount) == "" {
				next.DiscountAmount = money.Zero
			} else {
				d, err := money.ParseNonNegative(*in.DiscountAmount)
				if err != nil {
					errs = append(errs, domain.FieldError{Field: "discount_amount", Message: "diskon harus berupa angka tidak negatif"})
				} else {
					next.DiscountAmount = d
				}
			}
		}
		next.Notes = applyPatch(cur.Notes, in.Notes)
		errs = append(errs, optionalText("notes", next.Notes, maxContractNoteLen)...)

		var parsedItems []domain.SalesOrderItem
		var lineTotals []decimal.Decimal
		if in.Items != nil {
			var itemErrs []domain.FieldError
			parsedItems, lineTotals, itemErrs = u.validateSOItems(ctx, cur.OrderType, cur.CustomerID, *in.Items)
			errs = append(errs, itemErrs...)
		} else {
			existingItems, err := u.orders.ListItems(ctx, id)
			if err != nil {
				return err
			}
			for _, it := range existingItems {
				lineTotals = append(lineTotals, it.LineTotal)
			}
		}

		if len(errs) > 0 {
			return masterInvalid("Data Sales Order", errs)
		}

		rule, err := u.resolveTaxRule(ctx, next.OrderDate)
		if err != nil {
			return err
		}
		tb, err := money.CalculateLinesTax(lineTotals, next.DiscountAmount, next.IsPPN, rule)
		if err != nil {
			if errors.Is(err, money.ErrDiscountExceeds) {
				return masterInvalid("Data Sales Order", []domain.FieldError{
					{Field: "discount_amount", Message: "diskon tidak boleh melebihi subtotal"},
				})
			}
			return err
		}
		next.Subtotal = tb.Subtotal
		next.DiscountAmount = tb.DiscountAmount
		next.DPP = tb.DPP
		next.DPPOtherValue = tb.DPPOtherValue
		next.TaxRate = tb.TaxRate
		next.TaxAmount = tb.TaxAmount
		next.GrandTotal = tb.GrandTotal

		if err := u.orders.Update(ctx, &next); err != nil {
			return err
		}
		var savedItems []domain.SalesOrderItem
		if in.Items != nil {
			if savedItems, err = u.orders.ReplaceItems(ctx, id, parsedItems); err != nil {
				return err
			}
		} else {
			if savedItems, err = u.orders.ListItems(ctx, id); err != nil {
				return err
			}
		}
		updated, err := u.orders.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if err := auditUpdate(ctx, u.audit, domain.EntitySalesOrder, id, soView(cur), soView(updated)); err != nil {
			return err
		}
		dos, err := u.orders.ListDeliveryOrdersBySO(ctx, id)
		if err != nil {
			return err
		}
		detail = &domain.SalesOrderDetail{SO: updated, Items: savedItems, DeliveryOrders: dos}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return detail, nil
}

func (u *SalesOrderUsecase) Confirm(ctx context.Context, actor domain.Principal, id int64) (*domain.SalesOrderDetail, error) {
	var detail *domain.SalesOrderDetail
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		cur, err := u.orders.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if cur.Status != domain.SODraft {
			return fmt.Errorf("%w: hanya Sales Order berstatus DRAFT yang dapat dikonfirmasi", domain.ErrInvalidStateTransition)
		}
		items, err := u.orders.ListItems(ctx, id)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return masterInvalid("Konfirmasi Sales Order", []domain.FieldError{
				{Field: "items", Message: "Sales Order wajib memiliki minimal 1 item"},
			})
		}

		now := u.clk.Now()
		next := *cur
		next.Status = domain.SOConfirmed
		next.ConfirmedBy = &actor.UserID
		next.ConfirmedAt = &now
		if err := u.orders.UpdateStatus(ctx, &next); err != nil {
			return err
		}
		updated, err := u.orders.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if err := u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "SALES_ORDER_CONFIRM",
			EntityType: domain.EntitySalesOrder,
			EntityID:   &id,
			Before:     map[string]any{"status": string(cur.Status)},
			After:      map[string]any{"status": string(updated.Status), "confirmed_by": actor.UserID},
		}); err != nil {
			return err
		}
		dos, err := u.orders.ListDeliveryOrdersBySO(ctx, id)
		if err != nil {
			return err
		}
		detail = &domain.SalesOrderDetail{SO: updated, Items: items, DeliveryOrders: dos}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return detail, nil
}

func (u *SalesOrderUsecase) Cancel(ctx context.Context, actor domain.Principal, id int64, in CancelSOInput) (*domain.SalesOrderDetail, error) {
	reason := trimmed(in.CancelReason)
	if len([]rune(reason)) < 5 || len([]rune(reason)) > 500 {
		return nil, masterInvalid("Pembatalan Sales Order", []domain.FieldError{
			{Field: "cancel_reason", Message: "alasan pembatalan wajib diisi (5–500 karakter)"},
		})
	}

	var detail *domain.SalesOrderDetail
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		cur, err := u.orders.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if cur.Status != domain.SODraft && cur.Status != domain.SOConfirmed {
			return fmt.Errorf("%w: hanya Sales Order DRAFT atau CONFIRMED yang dapat dibatalkan", domain.ErrInvalidStateTransition)
		}
		hasDO, err := u.orders.HasActiveDeliveryOrders(ctx, id)
		if err != nil {
			return err
		}
		if hasDO {
			return fmt.Errorf("%w: batalkan Delivery Order aktif terlebih dahulu sebelum membatalkan SO", domain.ErrInvalidStateTransition)
		}

		now := u.clk.Now()
		next := *cur
		next.Status = domain.SOCancelled
		next.CancelledBy = &actor.UserID
		next.CancelledAt = &now
		next.CancelReason = &reason
		if err := u.orders.UpdateStatus(ctx, &next); err != nil {
			return err
		}
		updated, err := u.orders.FindByID(ctx, id)
		if err != nil {
			return err
		}
		items, err := u.orders.ListItems(ctx, id)
		if err != nil {
			return err
		}
		dos, err := u.orders.ListDeliveryOrdersBySO(ctx, id)
		if err != nil {
			return err
		}
		if err := u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "SALES_ORDER_CANCEL",
			EntityType: domain.EntitySalesOrder,
			EntityID:   &id,
			Before:     map[string]any{"status": string(cur.Status)},
			After:      map[string]any{"status": string(updated.Status), "cancel_reason": reason, "cancelled_by": actor.UserID},
		}); err != nil {
			return err
		}
		detail = &domain.SalesOrderDetail{SO: updated, Items: items, DeliveryOrders: dos}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return detail, nil
}

// IssueDeliveryOrders menerbitkan DO (OUTBOUND dan/atau INBOUND) ke gudang dari SO CONFIRMED.
func (u *SalesOrderUsecase) IssueDeliveryOrders(ctx context.Context, actor domain.Principal, soID int64, in IssueDOInput) ([]domain.DeliveryOrderSummary, error) {
	var created []domain.DeliveryOrderSummary
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		so, err := u.orders.FindByIDForUpdate(ctx, soID)
		if err != nil {
			return err
		}
		if so.Status != domain.SOConfirmed {
			return fmt.Errorf("%w: DO hanya dapat diterbitkan dari Sales Order berstatus CONFIRMED", domain.ErrInvalidStateTransition)
		}
		hasDO, err := u.orders.HasActiveDeliveryOrders(ctx, soID)
		if err != nil {
			return err
		}
		if hasDO {
			return fmt.Errorf("%w: Sales Order ini sudah memiliki Delivery Order aktif", domain.ErrInvalidStateTransition)
		}

		wh, err := u.warehouses.FindByID(ctx, in.WarehouseID)
		if err != nil {
			return masterInvalid("Penerbitan DO", []domain.FieldError{
				{Field: "warehouse_id", Message: "gudang tidak ditemukan"},
			})
		}
		if !wh.IsActive {
			return masterInvalid("Penerbitan DO", []domain.FieldError{
				{Field: "warehouse_id", Message: "gudang sedang nonaktif"},
			})
		}

		var schedDate *time.Time
		if in.ScheduledDate != nil && strings.TrimSpace(*in.ScheduledDate) != "" {
			parsed, err := time.Parse("2006-01-02", strings.TrimSpace(*in.ScheduledDate))
			if err != nil {
				return masterInvalid("Penerbitan DO", []domain.FieldError{
					{Field: "scheduled_date", Message: "format tanggal harus YYYY-MM-DD"},
				})
			}
			schedDate = &parsed
		}
		notes := cleanOpt(in.Notes)

		items, err := u.orders.ListItems(ctx, soID)
		if err != nil {
			return err
		}
		var outItems, inItems []domain.SalesOrderItem
		for _, it := range items {
			if it.Direction == domain.DirectionOut {
				outItems = append(outItems, it)
			} else {
				inItems = append(inItems, it)
			}
		}

		doNumbers := make([]string, 0, 2)
		if len(outItems) > 0 {
			num, err := u.docGen.Next(ctx, docTypeDO, wh.Code, u.clk.Now())
			if err != nil {
				return err
			}
			do, err := u.orders.CreateDeliveryOrder(ctx, &domain.DeliveryOrderSummary{
				DONumber:      num,
				SOID:          soID,
				WarehouseID:   wh.ID,
				WarehouseCode: wh.Code,
				WarehouseName: wh.Name,
				Direction:     domain.DODirectionOutbound,
				ScheduledDate: schedDate,
				Notes:         notes,
			}, outItems, actor.UserID)
			if err != nil {
				return err
			}
			created = append(created, *do)
			doNumbers = append(doNumbers, do.DONumber)
		}
		if len(inItems) > 0 {
			num, err := u.docGen.Next(ctx, docTypeDO, wh.Code, u.clk.Now())
			if err != nil {
				return err
			}
			do, err := u.orders.CreateDeliveryOrder(ctx, &domain.DeliveryOrderSummary{
				DONumber:      num,
				SOID:          soID,
				WarehouseID:   wh.ID,
				WarehouseCode: wh.Code,
				WarehouseName: wh.Name,
				Direction:     domain.DODirectionInbound,
				ScheduledDate: schedDate,
				Notes:         notes,
			}, inItems, actor.UserID)
			if err != nil {
				return err
			}
			created = append(created, *do)
			doNumbers = append(doNumbers, do.DONumber)
		}

		next := *so
		next.Status = domain.SOInProgress
		if err := u.orders.UpdateStatus(ctx, &next); err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "SALES_ORDER_ISSUE_DO",
			EntityType: domain.EntitySalesOrder,
			EntityID:   &soID,
			Before:     map[string]any{"status": string(so.Status)},
			After: map[string]any{
				"status":       string(next.Status),
				"warehouse_id": wh.ID,
				"do_numbers":   doNumbers,
			},
		})
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// ---------------------------------------------------------------- helpers

func (u *SalesOrderUsecase) resolveTaxRule(ctx context.Context, onDate time.Time) (money.TaxRule, error) {
	ts, err := u.taxes.FindEffective(ctx, domain.TaxCodePPN, onDate)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return money.PMK131Rule(), nil
		}
		return money.TaxRule{}, err
	}
	return ts.ToMoneyRule(), nil
}

func (u *SalesOrderUsecase) validateSOItems(
	ctx context.Context,
	otype domain.OrderType,
	customerID int64,
	items []SOItemInput,
) ([]domain.SalesOrderItem, []decimal.Decimal, []domain.FieldError) {
	if len(items) == 0 {
		return nil, nil, []domain.FieldError{{Field: "items", Message: "Sales Order wajib memiliki minimal 1 item"}}
	}

	var errs []domain.FieldError
	out := make([]domain.SalesOrderItem, 0, len(items))
	lines := make([]decimal.Decimal, 0, len(items))
	outCount, inCount := 0, 0

	for i, it := range items {
		prefix := fmt.Sprintf("items[%d]", i)
		if it.Qty < 1 {
			errs = append(errs, domain.FieldError{Field: prefix + ".qty", Message: "qty minimal 1"})
		}

		dir := defaultDirectionFor(otype, it.Direction)
		if !dir.Valid() {
			errs = append(errs, domain.FieldError{Field: prefix + ".direction", Message: "arah item harus OUT atau IN"})
			continue
		}
		switch otype {
		case domain.OrderTypeNewDelivery, domain.OrderTypeConsumable:
			if dir != domain.DirectionOut {
				errs = append(errs, domain.FieldError{Field: prefix + ".direction", Message: "jenis SO ini hanya menerima item keluar (OUT)"})
			}
		case domain.OrderTypeWithdrawal:
			if dir != domain.DirectionIn {
				errs = append(errs, domain.FieldError{Field: prefix + ".direction", Message: "penarikan hanya menerima item masuk (IN)"})
			}
		}

		if dir == domain.DirectionOut {
			outCount++
		} else {
			inCount++
		}

		prod, err := u.products.FindByID(ctx, it.ProductID)
		if err != nil {
			errs = append(errs, domain.FieldError{Field: prefix + ".product_id", Message: "produk tidak ditemukan"})
			continue
		}
		if !prod.IsActive {
			errs = append(errs, domain.FieldError{Field: prefix + ".product_id", Message: "produk tidak aktif"})
		}
		if otype == domain.OrderTypeConsumable && prod.ItemType != domain.ItemTypeConsumable {
			errs = append(errs, domain.FieldError{Field: prefix + ".product_id", Message: "SO CONSUMABLE hanya menerima produk bertipe CONSUMABLE"})
		}
		if otype != domain.OrderTypeConsumable && prod.ItemType != domain.ItemTypeAsset {
			errs = append(errs, domain.FieldError{Field: prefix + ".product_id", Message: "SO sewa/penarikan/tukar hanya menerima produk bertipe ASSET"})
		}

		if it.ProductSerialID != nil {
			if dir != domain.DirectionIn {
				errs = append(errs, domain.FieldError{Field: prefix + ".product_serial_id", Message: "serial hanya dipilih di SO untuk item penarikan (IN)"})
			} else if it.Qty != 1 {
				errs = append(errs, domain.FieldError{Field: prefix + ".qty", Message: "qty harus 1 bila serial number ditentukan"})
			} else {
				sn, err := u.inventory.FindSerialByID(ctx, *it.ProductSerialID)
				if err != nil {
					errs = append(errs, domain.FieldError{Field: prefix + ".product_serial_id", Message: "unit serial tidak ditemukan"})
				} else if sn.Status != domain.SerialRented || sn.CustomerID == nil || *sn.CustomerID != customerID {
					errs = append(errs, domain.FieldError{Field: prefix + ".product_serial_id", Message: "unit serial tidak sedang disewa oleh pelanggan ini"})
				} else if sn.ProductID != it.ProductID {
					errs = append(errs, domain.FieldError{Field: prefix + ".product_serial_id", Message: "unit serial tidak sesuai dengan produk yang dipilih"})
				}
			}
		}

		unitPrice := money.Zero
		if it.UnitPrice != nil && strings.TrimSpace(*it.UnitPrice) != "" {
			p, err := money.ParseNonNegative(*it.UnitPrice)
			if err != nil {
				errs = append(errs, domain.FieldError{Field: prefix + ".unit_price", Message: "harga satuan harus berupa angka tidak negatif"})
			} else {
				unitPrice = p
			}
		} else if otype == domain.OrderTypeConsumable && prod.DefaultSellPrice != nil {
			unitPrice = *prod.DefaultSellPrice
		} else if otype == domain.OrderTypeNewDelivery && prod.DefaultMonthlyRate != nil {
			unitPrice = *prod.DefaultMonthlyRate
		}

		lineTotal := money.LineTotal(it.Qty, unitPrice)
		if dir == domain.DirectionIn {
			lineTotal = money.Zero
		}
		lines = append(lines, lineTotal)

		notes := cleanOpt(it.Notes)
		errs = append(errs, optionalText(prefix+".notes", notes, maxDescriptionLen)...)

		out = append(out, domain.SalesOrderItem{
			ProductID:       it.ProductID,
			Direction:       dir,
			Qty:             it.Qty,
			UnitPrice:       unitPrice,
			LineTotal:       lineTotal,
			ProductSerialID: it.ProductSerialID,
			Notes:           notes,
		})
	}

	if otype == domain.OrderTypeUnitSwap && (outCount == 0 || inCount == 0) {
		errs = append(errs, domain.FieldError{Field: "items", Message: "SO UNIT_SWAP wajib memiliki minimal 1 item keluar (OUT) dan 1 item masuk (IN)"})
	}

	return out, lines, errs
}

func defaultDirectionFor(otype domain.OrderType, raw *string) domain.ItemDirection {
	if raw != nil && strings.TrimSpace(*raw) != "" {
		return domain.ItemDirection(strings.ToUpper(strings.TrimSpace(*raw)))
	}
	if otype == domain.OrderTypeWithdrawal {
		return domain.DirectionIn
	}
	return domain.DirectionOut
}

func soView(so *domain.SalesOrder) map[string]any {
	return map[string]any{
		"so_number":        so.SONumber,
		"order_type":       string(so.OrderType),
		"customer_id":      so.CustomerID,
		"customer_site_id": so.CustomerSiteID,
		"contract_id":      ptrVal(so.ContractID),
		"status":           string(so.Status),
		"is_ppn":           so.IsPPN,
		"subtotal":         so.Subtotal.StringFixed(2),
		"discount_amount":  so.DiscountAmount.StringFixed(2),
		"dpp":              so.DPP.StringFixed(2),
		"dpp_other_value":  so.DPPOtherValue.StringFixed(2),
		"tax_rate":         so.TaxRate.StringFixed(2),
		"tax_amount":       so.TaxAmount.StringFixed(2),
		"grand_total":      so.GrandTotal.StringFixed(2),
	}
}
