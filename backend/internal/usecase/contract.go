package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/clock"
	"rental-inventory-api/pkg/docnum"
)

const (
	docTypeContract    = "KTR"
	maxContractMonths  = 120
	maxContractNoteLen = 1000
)

// ContractUsecase mengelola kontrak sewa B2B: header, item tarif, aktivasi, terminasi, dan perpanjangan (T2.3).
type ContractUsecase struct {
	tx        domain.TxManager
	contracts domain.ContractRepository
	customers domain.CustomerRepository
	products  domain.ProductRepository
	audit     domain.AuditRepository
	docGen    domain.DocNumberGenerator
	clk       clock.Clock
}

func NewContractUsecase(
	tx domain.TxManager,
	contracts domain.ContractRepository,
	customers domain.CustomerRepository,
	products domain.ProductRepository,
	audit domain.AuditRepository,
	docGen domain.DocNumberGenerator,
	clk clock.Clock,
) *ContractUsecase {
	return &ContractUsecase{
		tx: tx, contracts: contracts, customers: customers, products: products,
		audit: audit, docGen: docGen, clk: clk,
	}
}

type ContractItemInput struct {
	ProductID      int64
	CustomerSiteID *int64
	Qty            int
	MonthlyRate    *string // opsional bila produk punya default_monthly_rate
	Notes          *string
}

type CreateContractInput struct {
	CustomerID         int64
	StartDate          string // YYYY-MM-DD
	EndDate            string // opsional; bila kosong dihitung dari start_date + duration_months - 1 hari
	DurationMonths     int
	BillingDayOfMonth  *int
	BillingMode        *string
	BillingInAdvance   *bool
	IsPPN              *bool
	PaymentTermDays    *int
	DepositAmount      *string
	SignedDocumentPath *string
	Notes              *string
	Items              []ContractItemInput
}

type UpdateContractInput struct {
	StartDate          *string
	EndDate            *string
	DurationMonths     *int
	BillingDayOfMonth  *int
	BillingMode        *string
	BillingInAdvance   *bool
	IsPPN              *bool
	PaymentTermDays    *int
	DepositAmount      *string
	SignedDocumentPath Patch[string]
	Notes              Patch[string]
	Items              *[]ContractItemInput
}

type TerminateContractInput struct {
	TerminatedAt      *string // opsional YYYY-MM-DD, default hari ini WIB
	TerminationReason string
}

type RenewContractInput struct {
	StartDate      *string // opsional, default end_date lama + 1 hari
	EndDate        *string // opsional, default start_date + duration_months - 1 hari
	DurationMonths *int    // opsional, default durasi kontrak lama
	Notes          *string
}

func todayDateWIB(t time.Time) time.Time {
	d, _ := time.Parse("2006-01-02", clock.DateWIB(t))
	return d
}

func (u *ContractUsecase) List(ctx context.Context, f domain.ContractFilter, p domain.PageParams) ([]domain.RentalContract, int64, error) {
	f.Today = todayDateWIB(u.clk.Now())
	return u.contracts.List(ctx, f, p)
}

func (u *ContractUsecase) Get(ctx context.Context, id int64) (*domain.ContractDetail, error) {
	c, err := u.contracts.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	items, err := u.contracts.ListItems(ctx, id)
	if err != nil {
		return nil, err
	}
	return &domain.ContractDetail{Contract: c, Items: items}, nil
}

func (u *ContractUsecase) Create(ctx context.Context, actor domain.Principal, in CreateContractInput) (*domain.ContractDetail, error) {
	var detail *domain.ContractDetail
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		cust, err := u.customers.FindByID(ctx, in.CustomerID)
		if err != nil {
			return &domain.ValidationError{
				Message: "Data kontrak tidak valid",
				Errors:  []domain.FieldError{{Field: "customer_id", Message: "pelanggan tidak ditemukan"}},
			}
		}
		if !cust.IsActive {
			return &domain.ValidationError{
				Message: "Data kontrak tidak valid",
				Errors:  []domain.FieldError{{Field: "customer_id", Message: "pelanggan sedang nonaktif"}},
			}
		}

		startDate, endDate, errs := parseContractDates(in.StartDate, in.EndDate, in.DurationMonths)

		billingDay := 1
		if in.BillingDayOfMonth != nil {
			billingDay = *in.BillingDayOfMonth
		}
		mode := domain.BillingModeProrata
		if in.BillingMode != nil && strings.TrimSpace(*in.BillingMode) != "" {
			mode = domain.BillingMode(strings.ToUpper(strings.TrimSpace(*in.BillingMode)))
		}
		billingInAdvance := false
		if in.BillingInAdvance != nil {
			billingInAdvance = *in.BillingInAdvance
		}
		isPPN := cust.IsPKP
		if in.IsPPN != nil {
			isPPN = *in.IsPPN
		}
		termDays := cust.PaymentTermDays
		if in.PaymentTermDays != nil {
			termDays = *in.PaymentTermDays
		}

		deposit := decimal.Zero
		if in.DepositAmount != nil && strings.TrimSpace(*in.DepositAmount) != "" {
			d, mErrs := parseMoney("deposit_amount", in.DepositAmount)
			errs = append(errs, mErrs...)
			if d != nil {
				deposit = *d
			}
		}

		notes := cleanOpt(in.Notes)
		signedPath := cleanOpt(in.SignedDocumentPath)
		errs = append(errs, validateContractHeader(in.DurationMonths, billingDay, mode, termDays, notes)...)

		parsedItems, itemErrs := u.validateContractItems(ctx, cust.ID, in.Items)
		errs = append(errs, itemErrs...)
		if len(errs) > 0 {
			return masterInvalid("Data kontrak", errs)
		}

		num, err := u.docGen.Next(ctx, docTypeContract, docnum.ScopeGlobal, u.clk.Now())
		if err != nil {
			return err
		}

		rc := &domain.RentalContract{
			ContractNumber:     num,
			CustomerID:         cust.ID,
			StartDate:          startDate,
			EndDate:            endDate,
			DurationMonths:     in.DurationMonths,
			BillingDayOfMonth:  billingDay,
			BillingMode:        mode,
			BillingInAdvance:   billingInAdvance,
			IsPPN:              isPPN,
			PaymentTermDays:    termDays,
			DepositAmount:      deposit,
			Status:             domain.ContractDraft,
			SignedDocumentPath: signedPath,
			Notes:              notes,
			CreatedBy:          actor.UserID,
		}
		created, err := u.contracts.Create(ctx, rc)
		if err != nil {
			return err
		}
		savedItems, err := u.contracts.ReplaceItems(ctx, created.ID, parsedItems)
		if err != nil {
			return err
		}
		created, err = u.contracts.FindByID(ctx, created.ID)
		if err != nil {
			return err
		}
		if err := auditCreate(ctx, u.audit, domain.EntityContract, created.ID, contractView(created)); err != nil {
			return err
		}
		detail = &domain.ContractDetail{Contract: created, Items: savedItems}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return detail, nil
}

func (u *ContractUsecase) Update(ctx context.Context, id int64, in UpdateContractInput) (*domain.ContractDetail, error) {
	var detail *domain.ContractDetail
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		cur, err := u.contracts.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if cur.Status != domain.ContractDraft {
			return fmt.Errorf("%w: hanya kontrak berstatus DRAFT yang dapat diubah", domain.ErrInvalidStateTransition)
		}

		next := *cur
		startStr := cur.StartDate.Format("2006-01-02")
		endStr := cur.EndDate.Format("2006-01-02")
		if in.StartDate != nil {
			startStr = *in.StartDate
		}
		if in.EndDate != nil {
			endStr = *in.EndDate
		}
		if in.DurationMonths != nil {
			next.DurationMonths = *in.DurationMonths
		}
		startDate, endDate, errs := parseContractDates(startStr, endStr, next.DurationMonths)
		next.StartDate = startDate
		next.EndDate = endDate

		if in.BillingDayOfMonth != nil {
			next.BillingDayOfMonth = *in.BillingDayOfMonth
		}
		if in.BillingMode != nil {
			next.BillingMode = domain.BillingMode(strings.ToUpper(strings.TrimSpace(*in.BillingMode)))
		}
		if in.BillingInAdvance != nil {
			next.BillingInAdvance = *in.BillingInAdvance
		}
		if in.IsPPN != nil {
			next.IsPPN = *in.IsPPN
		}
		if in.PaymentTermDays != nil {
			next.PaymentTermDays = *in.PaymentTermDays
		}
		if in.DepositAmount != nil {
			if strings.TrimSpace(*in.DepositAmount) == "" {
				next.DepositAmount = decimal.Zero
			} else {
				d, mErrs := parseMoney("deposit_amount", in.DepositAmount)
				errs = append(errs, mErrs...)
				if d != nil {
					next.DepositAmount = *d
				}
			}
		}
		next.SignedDocumentPath = applyPatch(cur.SignedDocumentPath, in.SignedDocumentPath)
		next.Notes = applyPatch(cur.Notes, in.Notes)

		errs = append(errs, validateContractHeader(next.DurationMonths, next.BillingDayOfMonth, next.BillingMode, next.PaymentTermDays, next.Notes)...)

		var parsedItems []domain.RentalContractItem
		if in.Items != nil {
			var itemErrs []domain.FieldError
			parsedItems, itemErrs = u.validateContractItems(ctx, cur.CustomerID, *in.Items)
			errs = append(errs, itemErrs...)
		}
		if len(errs) > 0 {
			return masterInvalid("Data kontrak", errs)
		}

		if err := u.contracts.Update(ctx, &next); err != nil {
			return err
		}
		var savedItems []domain.RentalContractItem
		if in.Items != nil {
			if savedItems, err = u.contracts.ReplaceItems(ctx, id, parsedItems); err != nil {
				return err
			}
		} else {
			if savedItems, err = u.contracts.ListItems(ctx, id); err != nil {
				return err
			}
		}
		updated, err := u.contracts.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if err := auditUpdate(ctx, u.audit, domain.EntityContract, id, contractView(cur), contractView(updated)); err != nil {
			return err
		}
		detail = &domain.ContractDetail{Contract: updated, Items: savedItems}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return detail, nil
}

func (u *ContractUsecase) Activate(ctx context.Context, actor domain.Principal, id int64) (*domain.ContractDetail, error) {
	var detail *domain.ContractDetail
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		cur, err := u.contracts.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if cur.Status != domain.ContractDraft {
			return fmt.Errorf("%w: hanya kontrak berstatus DRAFT yang dapat diaktifkan", domain.ErrInvalidStateTransition)
		}
		cust, err := u.customers.FindByID(ctx, cur.CustomerID)
		if err != nil || !cust.IsActive {
			return &domain.ValidationError{
				Message: "Kontrak tidak dapat diaktifkan",
				Errors:  []domain.FieldError{{Field: "customer_id", Message: "pelanggan sudah tidak aktif"}},
			}
		}
		items, err := u.contracts.ListItems(ctx, id)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return &domain.ValidationError{
				Message: "Kontrak tidak dapat diaktifkan",
				Errors:  []domain.FieldError{{Field: "items", Message: "kontrak wajib memiliki minimal 1 item tarif"}},
			}
		}

		now := u.clk.Now()
		next := *cur
		next.Status = domain.ContractActive
		next.ActivatedBy = &actor.UserID
		next.ActivatedAt = &now
		if err := u.contracts.UpdateStatus(ctx, &next); err != nil {
			return err
		}
		updated, err := u.contracts.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if err := u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "RENTAL_CONTRACT_ACTIVATE",
			EntityType: domain.EntityContract,
			EntityID:   &id,
			Before:     map[string]any{"status": string(cur.Status)},
			After:      map[string]any{"status": string(updated.Status), "activated_by": actor.UserID},
		}); err != nil {
			return err
		}
		detail = &domain.ContractDetail{Contract: updated, Items: items}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return detail, nil
}

func (u *ContractUsecase) Terminate(ctx context.Context, actor domain.Principal, id int64, in TerminateContractInput) (*domain.ContractDetail, error) {
	reason := trimmed(in.TerminationReason)
	if len([]rune(reason)) < 5 || len([]rune(reason)) > 500 {
		return nil, masterInvalid("Terminasi kontrak", []domain.FieldError{
			{Field: "termination_reason", Message: "alasan terminasi wajib diisi (5–500 karakter)"},
		})
	}

	var detail *domain.ContractDetail
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		cur, err := u.contracts.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if cur.Status != domain.ContractActive {
			return fmt.Errorf("%w: hanya kontrak berstatus ACTIVE yang dapat diterminasi", domain.ErrInvalidStateTransition)
		}

		termDate := todayDateWIB(u.clk.Now())
		if in.TerminatedAt != nil && strings.TrimSpace(*in.TerminatedAt) != "" {
			parsed, err := time.Parse("2006-01-02", strings.TrimSpace(*in.TerminatedAt))
			if err != nil {
				return masterInvalid("Terminasi kontrak", []domain.FieldError{
					{Field: "terminated_at", Message: "format tanggal harus YYYY-MM-DD"},
				})
			}
			termDate = parsed
		}
		if termDate.Before(cur.StartDate) {
			return masterInvalid("Terminasi kontrak", []domain.FieldError{
				{Field: "terminated_at", Message: "tanggal terminasi tidak boleh sebelum tanggal mulai kontrak"},
			})
		}

		next := *cur
		next.Status = domain.ContractTerminated
		next.TerminatedAt = &termDate
		next.TerminationReason = &reason
		if err := u.contracts.UpdateStatus(ctx, &next); err != nil {
			return err
		}
		updated, err := u.contracts.FindByID(ctx, id)
		if err != nil {
			return err
		}
		items, err := u.contracts.ListItems(ctx, id)
		if err != nil {
			return err
		}
		if err := u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "RENTAL_CONTRACT_TERMINATE",
			EntityType: domain.EntityContract,
			EntityID:   &id,
			Before:     map[string]any{"status": string(cur.Status)},
			After: map[string]any{
				"status":             string(updated.Status),
				"terminated_at":      termDate.Format("2006-01-02"),
				"termination_reason": reason,
				"terminated_by":      actor.UserID,
			},
		}); err != nil {
			return err
		}
		detail = &domain.ContractDetail{Contract: updated, Items: items}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return detail, nil
}

func (u *ContractUsecase) Renew(ctx context.Context, actor domain.Principal, sourceID int64, in RenewContractInput) (*domain.ContractDetail, error) {
	var detail *domain.ContractDetail
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		src, err := u.contracts.FindByIDForUpdate(ctx, sourceID)
		if err != nil {
			return err
		}
		if src.Status != domain.ContractActive && src.Status != domain.ContractExpired {
			return fmt.Errorf("%w: hanya kontrak ACTIVE atau EXPIRED yang dapat diperpanjang", domain.ErrInvalidStateTransition)
		}
		cust, err := u.customers.FindByID(ctx, src.CustomerID)
		if err != nil || !cust.IsActive {
			return &domain.ValidationError{
				Message: "Kontrak tidak dapat diperpanjang",
				Errors:  []domain.FieldError{{Field: "customer_id", Message: "pelanggan sudah tidak aktif"}},
			}
		}

		duration := src.DurationMonths
		if in.DurationMonths != nil {
			duration = *in.DurationMonths
		}
		startStr := src.EndDate.AddDate(0, 0, 1).Format("2006-01-02")
		if in.StartDate != nil && strings.TrimSpace(*in.StartDate) != "" {
			startStr = *in.StartDate
		}
		endStr := ""
		if in.EndDate != nil && strings.TrimSpace(*in.EndDate) != "" {
			endStr = *in.EndDate
		}

		startDate, endDate, errs := parseContractDates(startStr, endStr, duration)
		notes := cleanOpt(in.Notes)
		if notes == nil {
			notes = src.Notes
		}
		errs = append(errs, validateContractHeader(duration, src.BillingDayOfMonth, src.BillingMode, src.PaymentTermDays, notes)...)
		if len(errs) > 0 {
			return masterInvalid("Perpanjangan kontrak", errs)
		}

		srcItems, err := u.contracts.ListItems(ctx, sourceID)
		if err != nil {
			return err
		}
		num, err := u.docGen.Next(ctx, docTypeContract, docnum.ScopeGlobal, u.clk.Now())
		if err != nil {
			return err
		}

		renewedFrom := src.ID
		rc := &domain.RentalContract{
			ContractNumber:    num,
			CustomerID:        src.CustomerID,
			StartDate:         startDate,
			EndDate:           endDate,
			DurationMonths:    duration,
			BillingDayOfMonth: src.BillingDayOfMonth,
			BillingMode:       src.BillingMode,
			BillingInAdvance:  src.BillingInAdvance,
			IsPPN:             src.IsPPN,
			PaymentTermDays:   src.PaymentTermDays,
			DepositAmount:     src.DepositAmount,
			Status:            domain.ContractDraft,
			RenewedFromID:     &renewedFrom,
			Notes:             notes,
			CreatedBy:         actor.UserID,
		}
		created, err := u.contracts.Create(ctx, rc)
		if err != nil {
			return err
		}
		copied := make([]domain.RentalContractItem, 0, len(srcItems))
		for _, it := range srcItems {
			copied = append(copied, domain.RentalContractItem{
				ProductID:      it.ProductID,
				CustomerSiteID: it.CustomerSiteID,
				Qty:            it.Qty,
				MonthlyRate:    it.MonthlyRate,
				Notes:          it.Notes,
			})
		}
		savedItems, err := u.contracts.ReplaceItems(ctx, created.ID, copied)
		if err != nil {
			return err
		}
		created, err = u.contracts.FindByID(ctx, created.ID)
		if err != nil {
			return err
		}
		if err := u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "RENTAL_CONTRACT_RENEW",
			EntityType: domain.EntityContract,
			EntityID:   &created.ID,
			After: map[string]any{
				"contract_number": created.ContractNumber,
				"renewed_from_id": sourceID,
				"start_date":      startDate.Format("2006-01-02"),
				"end_date":        endDate.Format("2006-01-02"),
				"duration_months": duration,
			},
		}); err != nil {
			return err
		}
		detail = &domain.ContractDetail{Contract: created, Items: savedItems}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return detail, nil
}

// ---------------------------------------------------------------- validation helpers

func parseContractDates(startRaw, endRaw string, durationMonths int) (time.Time, time.Time, []domain.FieldError) {
	var errs []domain.FieldError
	startDate, err := time.Parse("2006-01-02", strings.TrimSpace(startRaw))
	if err != nil {
		errs = append(errs, domain.FieldError{Field: "start_date", Message: "tanggal mulai wajib berformat YYYY-MM-DD"})
		return time.Time{}, time.Time{}, errs
	}

	var endDate time.Time
	if strings.TrimSpace(endRaw) == "" {
		if durationMonths > 0 {
			endDate = startDate.AddDate(0, durationMonths, 0).AddDate(0, 0, -1)
		}
	} else {
		endDate, err = time.Parse("2006-01-02", strings.TrimSpace(endRaw))
		if err != nil {
			errs = append(errs, domain.FieldError{Field: "end_date", Message: "tanggal berakhir wajib berformat YYYY-MM-DD"})
			return startDate, time.Time{}, errs
		}
	}

	if !endDate.IsZero() && !endDate.After(startDate) {
		errs = append(errs, domain.FieldError{Field: "end_date", Message: "tanggal berakhir harus setelah tanggal mulai"})
	}
	return startDate, endDate, errs
}

func validateContractHeader(duration, billingDay int, mode domain.BillingMode, termDays int, notes *string) []domain.FieldError {
	var errs []domain.FieldError
	if duration < 1 || duration > maxContractMonths {
		errs = append(errs, domain.FieldError{Field: "duration_months", Message: "durasi kontrak harus 1–120 bulan"})
	}
	if billingDay < 1 || billingDay > 28 {
		errs = append(errs, domain.FieldError{Field: "billing_day_of_month", Message: "tanggal tagih harus antara 1 dan 28"})
	}
	if !mode.Valid() {
		errs = append(errs, domain.FieldError{Field: "billing_mode", Message: "mode tagihan harus PRORATA atau FULL_MONTH"})
	}
	if termDays < 0 || termDays > maxPaymentTermDays {
		errs = append(errs, domain.FieldError{Field: "payment_term_days", Message: "termin pembayaran harus 0–365 hari"})
	}
	errs = append(errs, optionalText("notes", notes, maxContractNoteLen)...)
	return errs
}

func (u *ContractUsecase) validateContractItems(ctx context.Context, customerID int64, items []ContractItemInput) ([]domain.RentalContractItem, []domain.FieldError) {
	if len(items) == 0 {
		return nil, []domain.FieldError{{Field: "items", Message: "kontrak wajib memiliki minimal 1 item tarif"}}
	}

	type itemKey struct {
		productID int64
		siteID    int64
	}
	seen := make(map[itemKey]bool, len(items))
	out := make([]domain.RentalContractItem, 0, len(items))
	var errs []domain.FieldError

	for i, it := range items {
		prefix := fmt.Sprintf("items[%d]", i)
		if it.Qty < 1 {
			errs = append(errs, domain.FieldError{Field: prefix + ".qty", Message: "qty minimal 1"})
		}

		prod, err := u.products.FindByID(ctx, it.ProductID)
		if err != nil {
			errs = append(errs, domain.FieldError{Field: prefix + ".product_id", Message: "produk tidak ditemukan"})
			continue
		}
		if !prod.IsActive {
			errs = append(errs, domain.FieldError{Field: prefix + ".product_id", Message: "produk tidak aktif"})
		}
		if prod.ItemType != domain.ItemTypeAsset {
			errs = append(errs, domain.FieldError{Field: prefix + ".product_id", Message: "kontrak sewa hanya menerima produk bertipe ASSET"})
		}

		var siteKey int64
		if it.CustomerSiteID != nil {
			site, err := u.customers.FindSiteByID(ctx, *it.CustomerSiteID)
			if err != nil {
				errs = append(errs, domain.FieldError{Field: prefix + ".customer_site_id", Message: "site pelanggan tidak ditemukan"})
			} else if site.CustomerID != customerID {
				errs = append(errs, domain.FieldError{Field: prefix + ".customer_site_id", Message: "site bukan milik pelanggan kontrak ini"})
			} else if !site.IsActive {
				errs = append(errs, domain.FieldError{Field: prefix + ".customer_site_id", Message: "site pelanggan sedang nonaktif"})
			}
			siteKey = *it.CustomerSiteID
		}

		k := itemKey{productID: it.ProductID, siteID: siteKey}
		if seen[k] {
			errs = append(errs, domain.FieldError{Field: prefix, Message: "kombinasi produk dan site duplikat dalam kontrak"})
		}
		seen[k] = true

		var rate decimal.Decimal
		if it.MonthlyRate != nil && strings.TrimSpace(*it.MonthlyRate) != "" {
			d, mErrs := parseMoney(prefix+".monthly_rate", it.MonthlyRate)
			errs = append(errs, mErrs...)
			if d != nil {
				rate = *d
			}
		} else if prod.DefaultMonthlyRate != nil {
			rate = *prod.DefaultMonthlyRate
		} else {
			errs = append(errs, domain.FieldError{Field: prefix + ".monthly_rate", Message: "tarif bulanan wajib diisi"})
		}

		notes := cleanOpt(it.Notes)
		errs = append(errs, optionalText(prefix+".notes", notes, maxDescriptionLen)...)

		out = append(out, domain.RentalContractItem{
			ProductID:      it.ProductID,
			CustomerSiteID: it.CustomerSiteID,
			Qty:            it.Qty,
			MonthlyRate:    rate,
			Notes:          notes,
		})
	}

	return out, errs
}

func contractView(c *domain.RentalContract) map[string]any {
	return map[string]any{
		"contract_number":      c.ContractNumber,
		"customer_id":          c.CustomerID,
		"start_date":           c.StartDate.Format("2006-01-02"),
		"end_date":             c.EndDate.Format("2006-01-02"),
		"duration_months":      c.DurationMonths,
		"billing_day_of_month": c.BillingDayOfMonth,
		"billing_mode":         string(c.BillingMode),
		"billing_in_advance":   c.BillingInAdvance,
		"is_ppn":               c.IsPPN,
		"payment_term_days":    c.PaymentTermDays,
		"deposit_amount":       c.DepositAmount.StringFixed(2),
		"status":               string(c.Status),
		"notes":                ptrVal(c.Notes),
	}
}
