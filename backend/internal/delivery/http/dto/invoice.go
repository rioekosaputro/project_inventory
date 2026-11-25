package dto

import (
	"time"

	"github.com/shopspring/decimal"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/usecase"
)

type ListInvoicesQuery struct {
	Q           string `form:"q" binding:"omitempty,max=100"`
	InvoiceType string `form:"invoice_type" binding:"omitempty,oneof=RENTAL CONSUMABLE SERVICE OTHER"`
	Status      string `form:"status" binding:"omitempty,oneof=DRAFT ISSUED PARTIALLY_PAID PAID VOID"`
	CustomerID  *int64 `form:"customer_id" binding:"omitempty,gte=1"`
	ContractID  *int64 `form:"contract_id" binding:"omitempty,gte=1"`
	DateFrom    string `form:"date_from" binding:"omitempty,datetime=2006-01-02"`
	DateTo      string `form:"date_to" binding:"omitempty,datetime=2006-01-02"`
	Overdue     bool   `form:"overdue"`
}

type IssueInvoiceRequest struct {
	FakturPajakNumber *string `json:"faktur_pajak_number" binding:"omitempty,max=40"`
}

type VoidInvoiceRequest struct {
	VoidReason string `json:"void_reason" binding:"required,min=5,max=500"`
}

type RecordPaymentRequest struct {
	PaymentDate     *string `json:"payment_date"`
	AmountReceived  string  `json:"amount_received" binding:"required"`
	PPh23Amount     *string `json:"pph23_amount"`
	PPh23SlipNumber *string `json:"pph23_slip_number" binding:"omitempty,max=60"`
	Method          string  `json:"method" binding:"required,oneof=TRANSFER CASH GIRO OTHER"`
	ReferenceNumber *string `json:"reference_number" binding:"omitempty,max=80"`
	ProofPath       *string `json:"proof_path" binding:"omitempty,max=500"`
	Notes           *string `json:"notes" binding:"omitempty,max=500"`
}

type InvoiceItemResponse struct {
	ID                int64   `json:"id"`
	Description       string  `json:"description"`
	ProductID         *int64  `json:"product_id"`
	ProductSerialID   *int64  `json:"product_serial_id"`
	CustomerSiteID    *int64  `json:"customer_site_id"`
	RentalPlacementID *int64  `json:"rental_placement_id"`
	PeriodStart       *string `json:"period_start"`
	PeriodEnd         *string `json:"period_end"`
	DaysBilled        *int    `json:"days_billed"`
	DaysInPeriod      *int    `json:"days_in_period"`
	Qty               string  `json:"qty"`
	UnitPrice         string  `json:"unit_price"`
	LineTotal         string  `json:"line_total"`
}

type PaymentResponse struct {
	ID              int64     `json:"id"`
	InvoiceID       int64     `json:"invoice_id"`
	PaymentDate     string    `json:"payment_date"`
	AmountReceived  string    `json:"amount_received"`
	PPh23Amount     string    `json:"pph23_amount"`
	TotalApplied    string    `json:"total_applied"`
	PPh23SlipNumber *string   `json:"pph23_slip_number"`
	Method          string    `json:"method"`
	ReferenceNumber *string   `json:"reference_number"`
	ProofPath       *string   `json:"proof_path"`
	Notes           *string   `json:"notes"`
	RecordedByName  string    `json:"recorded_by_name"`
	CreatedAt       time.Time `json:"created_at"`
}

type InvoiceResponse struct {
	ID                 int64      `json:"id"`
	InvoiceNumber      string     `json:"invoice_number"`
	InvoiceType        string     `json:"invoice_type"`
	CustomerID         int64      `json:"customer_id"`
	CustomerCode       string     `json:"customer_code"`
	CustomerName       string     `json:"customer_name"`
	CustomerNPWP       *string    `json:"customer_npwp"`
	ContractID         *int64     `json:"contract_id"`
	ContractNumber     *string    `json:"contract_number"`
	SOID               *int64     `json:"so_id"`
	SONumber           *string    `json:"so_number"`
	BillingPeriodStart *string    `json:"billing_period_start"`
	BillingPeriodEnd   *string    `json:"billing_period_end"`
	InvoiceDate        string     `json:"invoice_date"`
	DueDate            string     `json:"due_date"`
	Status             string     `json:"status"`
	IsPPN              bool       `json:"is_ppn"`
	Subtotal           string     `json:"subtotal"`
	DiscountAmount     string     `json:"discount_amount"`
	DPP                string     `json:"dpp"`
	DPPOtherValue      string     `json:"dpp_other_value"`
	TaxRate            string     `json:"tax_rate"`
	TaxAmount          string     `json:"tax_amount"`
	GrandTotal         string     `json:"grand_total"`
	PaidAmount         string     `json:"paid_amount"`
	RemainingAmount    string     `json:"remaining_amount"`
	FakturPajakNumber  *string    `json:"faktur_pajak_number"`
	Notes              *string    `json:"notes"`
	GeneratedByJob     bool       `json:"generated_by_job"`
	IssuedAt           *time.Time `json:"issued_at"`
	VoidedAt           *time.Time `json:"voided_at"`
	VoidReason         *string    `json:"void_reason"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type InvoiceDetailResponse struct {
	InvoiceResponse
	Items    []InvoiceItemResponse `json:"items"`
	Payments []PaymentResponse     `json:"payments"`
}

func datePtrStr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02")
	return &s
}

func NewInvoiceResponse(inv *domain.Invoice) InvoiceResponse {
	rem := inv.GrandTotal.Sub(inv.PaidAmount)
	if rem.IsNegative() {
		rem = decimal.Zero
	}
	return InvoiceResponse{
		ID:                 inv.ID,
		InvoiceNumber:      inv.InvoiceNumber,
		InvoiceType:        string(inv.InvoiceType),
		CustomerID:         inv.CustomerID,
		CustomerCode:       inv.CustomerCode,
		CustomerName:       inv.CustomerName,
		CustomerNPWP:       inv.CustomerNPWP,
		ContractID:         inv.ContractID,
		ContractNumber:     inv.ContractNumber,
		SOID:               inv.SOID,
		SONumber:           inv.SONumber,
		BillingPeriodStart: datePtrStr(inv.BillingPeriodStart),
		BillingPeriodEnd:   datePtrStr(inv.BillingPeriodEnd),
		InvoiceDate:        inv.InvoiceDate.Format("2006-01-02"),
		DueDate:            inv.DueDate.Format("2006-01-02"),
		Status:             string(inv.Status),
		IsPPN:              inv.IsPPN,
		Subtotal:           inv.Subtotal.StringFixed(2),
		DiscountAmount:     inv.DiscountAmount.StringFixed(2),
		DPP:                inv.DPP.StringFixed(2),
		DPPOtherValue:      inv.DPPOtherValue.StringFixed(2),
		TaxRate:            inv.TaxRate.StringFixed(2),
		TaxAmount:          inv.TaxAmount.StringFixed(2),
		GrandTotal:         inv.GrandTotal.StringFixed(2),
		PaidAmount:         inv.PaidAmount.StringFixed(2),
		RemainingAmount:    rem.StringFixed(2),
		FakturPajakNumber:  inv.FakturPajakNumber,
		Notes:              inv.Notes,
		GeneratedByJob:     inv.GeneratedByJob,
		IssuedAt:           inv.IssuedAt,
		VoidedAt:           inv.VoidedAt,
		VoidReason:         inv.VoidReason,
		CreatedAt:          inv.CreatedAt,
		UpdatedAt:          inv.UpdatedAt,
	}
}

func NewInvoiceDetailResponse(d *domain.InvoiceDetail) InvoiceDetailResponse {
	items := make([]InvoiceItemResponse, 0, len(d.Items))
	for i := range d.Items {
		it := &d.Items[i]
		items = append(items, InvoiceItemResponse{
			ID:                it.ID,
			Description:       it.Description,
			ProductID:         it.ProductID,
			ProductSerialID:   it.ProductSerialID,
			CustomerSiteID:    it.CustomerSiteID,
			RentalPlacementID: it.RentalPlacementID,
			PeriodStart:       datePtrStr(it.PeriodStart),
			PeriodEnd:         datePtrStr(it.PeriodEnd),
			DaysBilled:        it.DaysBilled,
			DaysInPeriod:      it.DaysInPeriod,
			Qty:               it.Qty.String(),
			UnitPrice:         it.UnitPrice.StringFixed(2),
			LineTotal:         it.LineTotal.StringFixed(2),
		})
	}
	payments := make([]PaymentResponse, 0, len(d.Payments))
	for i := range d.Payments {
		pm := &d.Payments[i]
		payments = append(payments, PaymentResponse{
			ID:              pm.ID,
			InvoiceID:       pm.InvoiceID,
			PaymentDate:     pm.PaymentDate.Format("2006-01-02"),
			AmountReceived:  pm.AmountReceived.StringFixed(2),
			PPh23Amount:     pm.PPh23Amount.StringFixed(2),
			TotalApplied:    pm.AmountReceived.Add(pm.PPh23Amount).StringFixed(2),
			PPh23SlipNumber: pm.PPh23SlipNumber,
			Method:          string(pm.Method),
			ReferenceNumber: pm.ReferenceNumber,
			ProofPath:       pm.ProofPath,
			Notes:           pm.Notes,
			RecordedByName:  pm.RecordedByName,
			CreatedAt:       pm.CreatedAt,
		})
	}
	return InvoiceDetailResponse{
		InvoiceResponse: NewInvoiceResponse(d.Invoice),
		Items:           items,
		Payments:        payments,
	}
}

// ---------------------------------------------------------------- billing generate DTOs (T2.8)

type GenerateInvoicesRequest struct {
	RunDate    *string `json:"run_date"`
	ContractID *int64  `json:"contract_id" binding:"omitempty,gte=1"`
}

type BillingLineResponse struct {
	PlacementID  int64  `json:"placement_id"`
	SerialNumber string `json:"serial_number"`
	ProductName  string `json:"product_name"`
	SiteName     string `json:"site_name"`
	PeriodStart  string `json:"period_start"`
	PeriodEnd    string `json:"period_end"`
	DaysBilled   int    `json:"days_billed"`
	DaysInPeriod int    `json:"days_in_period"`
	MonthlyRate  string `json:"monthly_rate"`
	LineTotal    string `json:"line_total"`
	Description  string `json:"description"`
}

type BillingContractResponse struct {
	ContractID     int64                 `json:"contract_id"`
	ContractNumber string                `json:"contract_number"`
	CustomerName   string                `json:"customer_name"`
	PeriodStart    string                `json:"period_start"`
	PeriodEnd      string                `json:"period_end"`
	Status         string                `json:"status"`
	InvoiceID      *int64                `json:"invoice_id"`
	InvoiceNumber  *string               `json:"invoice_number"`
	Subtotal       string                `json:"subtotal"`
	DPP            string                `json:"dpp"`
	DPPOtherValue  string                `json:"dpp_other_value"`
	TaxRate        string                `json:"tax_rate"`
	TaxAmount      string                `json:"tax_amount"`
	GrandTotal     string                `json:"grand_total"`
	Lines          []BillingLineResponse `json:"lines"`
	Error          string                `json:"error,omitempty"`
}

type BillingRunResponse struct {
	RunDate          string                    `json:"run_date"`
	Preview          bool                      `json:"preview"`
	ContractsChecked int                       `json:"contracts_checked"`
	InvoicesCreated  int                       `json:"invoices_created"`
	Skipped          int                       `json:"skipped"`
	Errors           int                       `json:"errors"`
	Results          []BillingContractResponse `json:"results"`
}

func NewBillingRunResponse(r *usecase.BillingRunResult) BillingRunResponse {
	items := make([]BillingContractResponse, 0, len(r.Results))
	for _, c := range r.Results {
		lines := make([]BillingLineResponse, 0, len(c.Lines))
		for _, l := range c.Lines {
			lines = append(lines, BillingLineResponse{
				PlacementID:  l.PlacementID,
				SerialNumber: l.SerialNumber,
				ProductName:  l.ProductName,
				SiteName:     l.SiteName,
				PeriodStart:  l.PeriodStart,
				PeriodEnd:    l.PeriodEnd,
				DaysBilled:   l.DaysBilled,
				DaysInPeriod: l.DaysInPeriod,
				MonthlyRate:  l.MonthlyRate.StringFixed(2),
				LineTotal:    l.LineTotal.StringFixed(2),
				Description:  l.Description,
			})
		}
		items = append(items, BillingContractResponse{
			ContractID:     c.ContractID,
			ContractNumber: c.ContractNumber,
			CustomerName:   c.CustomerName,
			PeriodStart:    c.PeriodStart,
			PeriodEnd:      c.PeriodEnd,
			Status:         c.Status,
			InvoiceID:      c.InvoiceID,
			InvoiceNumber:  c.InvoiceNumber,
			Subtotal:       c.Subtotal.StringFixed(2),
			DPP:            c.DPP.StringFixed(2),
			DPPOtherValue:  c.DPPOtherValue.StringFixed(2),
			TaxRate:        c.TaxRate.StringFixed(2),
			TaxAmount:      c.TaxAmount.StringFixed(2),
			GrandTotal:     c.GrandTotal.StringFixed(2),
			Lines:          lines,
			Error:          c.Error,
		})
	}
	return BillingRunResponse{
		RunDate:          r.RunDate,
		Preview:          r.Preview,
		ContractsChecked: r.ContractsChecked,
		InvoicesCreated:  r.InvoicesCreated,
		Skipped:          r.Skipped,
		Errors:           r.Errors,
		Results:          items,
	}
}
