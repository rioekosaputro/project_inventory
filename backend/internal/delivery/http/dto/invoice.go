package dto

import "rental-inventory-api/internal/usecase"

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
