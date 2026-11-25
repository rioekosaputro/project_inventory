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

const docTypeInvoice = "INV"

// BillingUsecase menjalankan penagihan bulanan (arrears, PRORATA / FULL_MONTH, PMK 131/2024)
// baik secara terjadwal (job) maupun manual via POST /invoices/generate(?preview=1) (T2.8).
type BillingUsecase struct {
	tx       domain.TxManager
	invoices domain.InvoiceRepository
	taxes    domain.TaxSettingRepository
	audit    domain.AuditRepository
	docGen   domain.DocNumberGenerator
	clk      clock.Clock
}

func NewBillingUsecase(
	tx domain.TxManager,
	invoices domain.InvoiceRepository,
	taxes domain.TaxSettingRepository,
	audit domain.AuditRepository,
	docGen domain.DocNumberGenerator,
	clk clock.Clock,
) *BillingUsecase {
	return &BillingUsecase{
		tx: tx, invoices: invoices, taxes: taxes, audit: audit, docGen: docGen, clk: clk,
	}
}

type GenerateBillingInput struct {
	RunDate        *string // opsional YYYY-MM-DD (default hari ini WIB)
	ContractID     *int64  // opsional: batasi ke 1 kontrak
	Preview        bool    // true = dry-run, tidak menulis ke DB
	GeneratedByJob bool
}

type BillingLinePreview struct {
	PlacementID  int64           `json:"placement_id"`
	SerialNumber string          `json:"serial_number"`
	ProductName  string          `json:"product_name"`
	SiteName     string          `json:"site_name"`
	PeriodStart  string          `json:"period_start"`
	PeriodEnd    string          `json:"period_end"`
	DaysBilled   int             `json:"days_billed"`
	DaysInPeriod int             `json:"days_in_period"`
	MonthlyRate  decimal.Decimal `json:"-"`
	LineTotal    decimal.Decimal `json:"-"`
	Description  string          `json:"description"`
}

type BillingContractResult struct {
	ContractID     int64
	ContractNumber string
	CustomerName   string
	PeriodStart    string
	PeriodEnd      string
	Status         string // CREATED | WOULD_CREATE | SKIPPED_EXISTS | SKIPPED_NO_PLACEMENTS | ERROR
	InvoiceID      *int64
	InvoiceNumber  *string
	Subtotal       decimal.Decimal
	DPP            decimal.Decimal
	DPPOtherValue  decimal.Decimal
	TaxRate        decimal.Decimal
	TaxAmount      decimal.Decimal
	GrandTotal     decimal.Decimal
	Lines          []BillingLinePreview
	Error          string
}

type BillingRunResult struct {
	RunDate          string
	Preview          bool
	ContractsChecked int
	InvoicesCreated  int
	Skipped          int
	Errors           int
	Results          []BillingContractResult
}

// PeriodForRunDate menghitung periode tagihan arrears [start, end] yang baru berakhir
// sehari sebelum runDate (runDate.Day() == billingDay). Mengembalikan ok=false bila
// bukan hari tagih kontrak tersebut.
func PeriodForRunDate(runDate time.Time, billingDay int) (start, end time.Time, ok bool) {
	if billingDay < 1 || billingDay > 28 || runDate.Day() != billingDay {
		return time.Time{}, time.Time{}, false
	}
	d := time.Date(runDate.Year(), runDate.Month(), runDate.Day(), 0, 0, 0, 0, time.UTC)
	return d.AddDate(0, -1, 0), d.AddDate(0, 0, -1), true
}

func (u *BillingUsecase) GenerateMonthlyInvoices(ctx context.Context, actor *domain.Principal, in GenerateBillingInput) (*BillingRunResult, error) {
	runDate := todayDateWIB(u.clk.Now())
	if in.RunDate != nil && strings.TrimSpace(*in.RunDate) != "" {
		parsed, err := time.Parse("2006-01-02", strings.TrimSpace(*in.RunDate))
		if err != nil {
			return nil, masterInvalid("Generate tagihan", []domain.FieldError{
				{Field: "run_date", Message: "format tanggal harus YYYY-MM-DD"},
			})
		}
		runDate = parsed
	}

	res := &BillingRunResult{
		RunDate: runDate.Format("2006-01-02"),
		Preview: in.Preview,
		Results: []BillingContractResult{},
	}

	// Hanya hari 1..28 yang bisa menjadi billing_day_of_month.
	if runDate.Day() > 28 {
		return res, nil
	}

	contracts, err := u.invoices.ListBillableContracts(ctx, runDate.Day(), in.ContractID)
	if err != nil {
		return nil, err
	}
	res.ContractsChecked = len(contracts)

	for _, c := range contracts {
		ps, pe, ok := PeriodForRunDate(runDate, c.BillingDayOfMonth)
		if !ok {
			continue
		}
		item := u.processOneContract(ctx, actor, c, runDate, ps, pe, in.Preview, in.GeneratedByJob)
		res.Results = append(res.Results, item)
		switch item.Status {
		case "CREATED", "WOULD_CREATE":
			res.InvoicesCreated++
		case "SKIPPED_EXISTS", "SKIPPED_NO_PLACEMENTS":
			res.Skipped++
		case "ERROR":
			res.Errors++
		}
	}
	return res, nil
}

func (u *BillingUsecase) processOneContract(
	ctx context.Context,
	actor *domain.Principal,
	c domain.BillableContract,
	runDate, ps, pe time.Time,
	preview, byJob bool,
) BillingContractResult {
	out := BillingContractResult{
		ContractID:     c.ID,
		ContractNumber: c.ContractNumber,
		CustomerName:   c.CustomerName,
		PeriodStart:    ps.Format("2006-01-02"),
		PeriodEnd:      pe.Format("2006-01-02"),
		Lines:          []BillingLinePreview{},
	}

	exists, err := u.invoices.HasActiveRentalInvoice(ctx, c.ID, ps, pe)
	if err != nil {
		out.Status = "ERROR"
		out.Error = err.Error()
		return out
	}
	if exists {
		out.Status = "SKIPPED_EXISTS"
		return out
	}

	placements, err := u.invoices.ListPlacementsForPeriod(ctx, c.ID, ps, pe)
	if err != nil {
		out.Status = "ERROR"
		out.Error = err.Error()
		return out
	}
	if len(placements) == 0 {
		out.Status = "SKIPPED_NO_PLACEMENTS"
		return out
	}

	invItems := make([]domain.InvoiceItem, 0, len(placements))
	lineTotals := make([]decimal.Decimal, 0, len(placements))
	previews := make([]BillingLinePreview, 0, len(placements))

	for _, p := range placements {
		var lineTotal decimal.Decimal
		if c.BillingMode == domain.BillingModeFullMonth {
			lineTotal = money.Round2(p.MonthlyRate)
		} else {
			lineTotal, err = money.Prorata(p.MonthlyRate, p.DaysBilled, p.DaysInPeriod)
			if err != nil {
				out.Status = "ERROR"
				out.Error = err.Error()
				return out
			}
		}

		// bill_to_excl bersifat eksklusif → tanggal inklusif terakhir adalah bill_to_excl - 1 hari.
		billToIncl := p.BillToExcl.AddDate(0, 0, -1)
		desc := fmt.Sprintf("Sewa %s (SN: %s) — %s · %s s/d %s (%d/%d hari)",
			p.ProductName, p.SerialNumber, p.SiteName,
			p.BillFrom.Format("2006-01-02"), billToIncl.Format("2006-01-02"),
			p.DaysBilled, p.DaysInPeriod,
		)

		prodID := p.ProductID
		snID := p.ProductSerialID
		siteID := p.CustomerSiteID
		plID := p.PlacementID
		pStart := p.BillFrom
		pEnd := billToIncl
		daysBilled := p.DaysBilled
		daysInPeriod := p.DaysInPeriod

		invItems = append(invItems, domain.InvoiceItem{
			Description:       desc,
			ProductID:         &prodID,
			ProductSerialID:   &snID,
			CustomerSiteID:    &siteID,
			RentalPlacementID: &plID,
			PeriodStart:       &pStart,
			PeriodEnd:         &pEnd,
			DaysBilled:        &daysBilled,
			DaysInPeriod:      &daysInPeriod,
			Qty:               decimal.NewFromInt(1),
			UnitPrice:         p.MonthlyRate,
			LineTotal:         lineTotal,
		})
		lineTotals = append(lineTotals, lineTotal)
		previews = append(previews, BillingLinePreview{
			PlacementID:  p.PlacementID,
			SerialNumber: p.SerialNumber,
			ProductName:  p.ProductName,
			SiteName:     p.SiteName,
			PeriodStart:  p.BillFrom.Format("2006-01-02"),
			PeriodEnd:    billToIncl.Format("2006-01-02"),
			DaysBilled:   p.DaysBilled,
			DaysInPeriod: p.DaysInPeriod,
			MonthlyRate:  p.MonthlyRate,
			LineTotal:    lineTotal,
			Description:  desc,
		})
	}

	rule, err := u.resolveTaxRule(ctx, pe)
	if err != nil {
		out.Status = "ERROR"
		out.Error = err.Error()
		return out
	}
	tb, err := money.CalculateLinesTax(lineTotals, money.Zero, c.IsPPN, rule)
	if err != nil {
		out.Status = "ERROR"
		out.Error = err.Error()
		return out
	}

	out.Subtotal = tb.Subtotal
	out.DPP = tb.DPP
	out.DPPOtherValue = tb.DPPOtherValue
	out.TaxRate = tb.TaxRate
	out.TaxAmount = tb.TaxAmount
	out.GrandTotal = tb.GrandTotal
	out.Lines = previews

	if preview {
		out.Status = "WOULD_CREATE"
		return out
	}

	// 1 transaksi per kontrak (gagal satu kontrak tidak menggagalkan kontrak lain).
	var created *domain.Invoice
	txErr := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		// Cek ulang di dalam transaksi untuk idempotensi.
		again, err := u.invoices.HasActiveRentalInvoice(ctx, c.ID, ps, pe)
		if err != nil {
			return err
		}
		if again {
			out.Status = "SKIPPED_EXISTS"
			return nil
		}

		num, err := u.docGen.Next(ctx, docTypeInvoice, docnum.ScopeGlobal, runDate)
		if err != nil {
			return err
		}
		var createdBy *int64
		if actor != nil {
			uid := actor.UserID
			createdBy = &uid
		}
		cid := c.ID
		psCopy, peCopy := ps, pe
		inv := &domain.Invoice{
			InvoiceNumber:      num,
			InvoiceType:        domain.InvoiceTypeRental,
			CustomerID:         c.CustomerID,
			ContractID:         &cid,
			BillingPeriodStart: &psCopy,
			BillingPeriodEnd:   &peCopy,
			InvoiceDate:        runDate,
			DueDate:            runDate.AddDate(0, 0, c.PaymentTermDays),
			Status:             domain.InvoiceDraft,
			IsPPN:              c.IsPPN,
			Subtotal:           tb.Subtotal,
			DiscountAmount:     tb.DiscountAmount,
			DPP:                tb.DPP,
			DPPOtherValue:      tb.DPPOtherValue,
			TaxRate:            tb.TaxRate,
			TaxAmount:          tb.TaxAmount,
			GrandTotal:         tb.GrandTotal,
			GeneratedByJob:     byJob,
			CreatedBy:          createdBy,
		}
		created, err = u.invoices.CreateInvoiceWithItems(ctx, inv, invItems)
		if err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "INVOICE_GENERATE",
			EntityType: domain.EntityInvoice,
			EntityID:   &created.ID,
			After: map[string]any{
				"invoice_number": created.InvoiceNumber,
				"contract_id":    c.ID,
				"period_start":   out.PeriodStart,
				"period_end":     out.PeriodEnd,
				"grand_total":    created.GrandTotal.StringFixed(2),
				"generated_by_job": byJob,
			},
		})
	})
	if txErr != nil {
		if errors.Is(txErr, domain.ErrConflict) {
			out.Status = "SKIPPED_EXISTS"
			return out
		}
		out.Status = "ERROR"
		out.Error = txErr.Error()
		return out
	}
	if out.Status == "SKIPPED_EXISTS" {
		return out
	}
	out.Status = "CREATED"
	out.InvoiceID = &created.ID
	out.InvoiceNumber = &created.InvoiceNumber
	return out
}

func (u *BillingUsecase) resolveTaxRule(ctx context.Context, onDate time.Time) (money.TaxRule, error) {
	ts, err := u.taxes.FindEffective(ctx, domain.TaxCodePPN, onDate)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return money.PMK131Rule(), nil
		}
		return money.TaxRule{}, err
	}
	return ts.ToMoneyRule(), nil
}
