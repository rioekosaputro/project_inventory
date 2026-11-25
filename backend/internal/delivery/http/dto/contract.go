package dto

import (
	"time"

	"github.com/shopspring/decimal"

	"rental-inventory-api/internal/domain"
)

type ListContractsQuery struct {
	Q          string `form:"q" binding:"omitempty,max=100"`
	CustomerID *int64 `form:"customer_id" binding:"omitempty,gte=1"`
	Status     string `form:"status" binding:"omitempty,oneof=DRAFT ACTIVE EXPIRED TERMINATED"`
	ExpiringIn *int   `form:"expiring_in" binding:"omitempty,gte=1,lte=365"`
}

type ContractItemRequest struct {
	ProductID      int64   `json:"product_id" binding:"required,gte=1"`
	CustomerSiteID *int64  `json:"customer_site_id" binding:"omitempty,gte=1"`
	Qty            int     `json:"qty" binding:"required,gte=1"`
	MonthlyRate    *string `json:"monthly_rate"`
	Notes          *string `json:"notes" binding:"omitempty,max=1000"`
}

type CreateContractRequest struct {
	CustomerID         int64                 `json:"customer_id" binding:"required,gte=1"`
	StartDate          string                `json:"start_date" binding:"required"`
	EndDate            string                `json:"end_date"`
	DurationMonths     int                   `json:"duration_months" binding:"required,gte=1,lte=120"`
	BillingDayOfMonth  *int                  `json:"billing_day_of_month" binding:"omitempty,gte=1,lte=28"`
	BillingMode        *string               `json:"billing_mode" binding:"omitempty,oneof=PRORATA FULL_MONTH"`
	BillingInAdvance   *bool                 `json:"billing_in_advance"`
	IsPPN              *bool                 `json:"is_ppn"`
	PaymentTermDays    *int                  `json:"payment_term_days" binding:"omitempty,gte=0,lte=365"`
	DepositAmount      *string               `json:"deposit_amount"`
	SignedDocumentPath *string               `json:"signed_document_path" binding:"omitempty,max=500"`
	Notes              *string               `json:"notes" binding:"omitempty,max=1000"`
	Items              []ContractItemRequest `json:"items" binding:"required,min=1,dive"`
}

type UpdateContractRequest struct {
	StartDate          *string                `json:"start_date"`
	EndDate            *string                `json:"end_date"`
	DurationMonths     *int                   `json:"duration_months" binding:"omitempty,gte=1,lte=120"`
	BillingDayOfMonth  *int                   `json:"billing_day_of_month" binding:"omitempty,gte=1,lte=28"`
	BillingMode        *string                `json:"billing_mode" binding:"omitempty,oneof=PRORATA FULL_MONTH"`
	BillingInAdvance   *bool                  `json:"billing_in_advance"`
	IsPPN              *bool                  `json:"is_ppn"`
	PaymentTermDays    *int                   `json:"payment_term_days" binding:"omitempty,gte=0,lte=365"`
	DepositAmount      *string                `json:"deposit_amount"`
	SignedDocumentPath Optional[string]       `json:"signed_document_path"`
	Notes              Optional[string]       `json:"notes"`
	Items              *[]ContractItemRequest `json:"items" binding:"omitempty,min=1,dive"`
}

type TerminateContractRequest struct {
	TerminatedAt      *string `json:"terminated_at"`
	TerminationReason string  `json:"termination_reason" binding:"required,min=5,max=500"`
}

type RenewContractRequest struct {
	StartDate      *string `json:"start_date"`
	EndDate        *string `json:"end_date"`
	DurationMonths *int    `json:"duration_months" binding:"omitempty,gte=1,lte=120"`
	Notes          *string `json:"notes" binding:"omitempty,max=1000"`
}

type ContractItemResponse struct {
	ID             int64   `json:"id"`
	ContractID     int64   `json:"contract_id"`
	ProductID      int64   `json:"product_id"`
	ProductSKU     string  `json:"product_sku"`
	ProductName    string  `json:"product_name"`
	CustomerSiteID *int64  `json:"customer_site_id"`
	SiteName       *string `json:"site_name"`
	Qty            int     `json:"qty"`
	MonthlyRate    string  `json:"monthly_rate"`
	LineMonthly    string  `json:"line_monthly"`
	Notes          *string `json:"notes"`
}

type ContractResponse struct {
	ID                 int64      `json:"id"`
	ContractNumber     string     `json:"contract_number"`
	CustomerID         int64      `json:"customer_id"`
	CustomerCode       string     `json:"customer_code"`
	CustomerName       string     `json:"customer_name"`
	StartDate          string     `json:"start_date"`
	EndDate            string     `json:"end_date"`
	DurationMonths     int        `json:"duration_months"`
	BillingDayOfMonth  int        `json:"billing_day_of_month"`
	BillingMode        string     `json:"billing_mode"`
	BillingInAdvance   bool       `json:"billing_in_advance"`
	IsPPN              bool       `json:"is_ppn"`
	PaymentTermDays    int        `json:"payment_term_days"`
	DepositAmount      string     `json:"deposit_amount"`
	Status             string     `json:"status"`
	TerminatedAt       *string    `json:"terminated_at"`
	TerminationReason  *string    `json:"termination_reason"`
	RenewedFromID      *int64     `json:"renewed_from_id"`
	RenewedFromNumber  *string    `json:"renewed_from_number"`
	SignedDocumentPath *string    `json:"signed_document_path"`
	Notes              *string    `json:"notes"`
	CreatedBy          int64      `json:"created_by"`
	CreatedByName      string     `json:"created_by_name"`
	ActivatedBy        *int64     `json:"activated_by"`
	ActivatedByName    *string    `json:"activated_by_name"`
	ActivatedAt        *time.Time `json:"activated_at"`
	ItemCount          int        `json:"item_count"`
	TotalQty           int        `json:"total_qty"`
	EstimatedMonthly   string     `json:"estimated_monthly"`
	ActivePlacementQty int        `json:"active_placement_qty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type ContractDetailResponse struct {
	ContractResponse
	Items []ContractItemResponse `json:"items"`
}

func NewContractResponse(c *domain.RentalContract) ContractResponse {
	var termAt *string
	if c.TerminatedAt != nil {
		s := c.TerminatedAt.Format("2006-01-02")
		termAt = &s
	}
	return ContractResponse{
		ID:                 c.ID,
		ContractNumber:     c.ContractNumber,
		CustomerID:         c.CustomerID,
		CustomerCode:       c.CustomerCode,
		CustomerName:       c.CustomerName,
		StartDate:          c.StartDate.Format("2006-01-02"),
		EndDate:            c.EndDate.Format("2006-01-02"),
		DurationMonths:     c.DurationMonths,
		BillingDayOfMonth:  c.BillingDayOfMonth,
		BillingMode:        string(c.BillingMode),
		BillingInAdvance:   c.BillingInAdvance,
		IsPPN:              c.IsPPN,
		PaymentTermDays:    c.PaymentTermDays,
		DepositAmount:      c.DepositAmount.StringFixed(2),
		Status:             string(c.Status),
		TerminatedAt:       termAt,
		TerminationReason:  c.TerminationReason,
		RenewedFromID:      c.RenewedFromID,
		RenewedFromNumber:  c.RenewedFromNumber,
		SignedDocumentPath: c.SignedDocumentPath,
		Notes:              c.Notes,
		CreatedBy:          c.CreatedBy,
		CreatedByName:      c.CreatedByName,
		ActivatedBy:        c.ActivatedBy,
		ActivatedByName:    c.ActivatedByName,
		ActivatedAt:        c.ActivatedAt,
		ItemCount:          c.ItemCount,
		TotalQty:           c.TotalQty,
		EstimatedMonthly:   c.EstimatedMonthly.StringFixed(2),
		ActivePlacementQty: c.ActivePlacementQty,
		CreatedAt:          c.CreatedAt,
		UpdatedAt:          c.UpdatedAt,
	}
}

func NewContractItemResponse(it *domain.RentalContractItem) ContractItemResponse {
	line := it.MonthlyRate.Mul(decimal.NewFromInt(int64(it.Qty)))
	return ContractItemResponse{
		ID:             it.ID,
		ContractID:     it.ContractID,
		ProductID:      it.ProductID,
		ProductSKU:     it.ProductSKU,
		ProductName:    it.ProductName,
		CustomerSiteID: it.CustomerSiteID,
		SiteName:       it.SiteName,
		Qty:            it.Qty,
		MonthlyRate:    it.MonthlyRate.StringFixed(2),
		LineMonthly:    line.StringFixed(2),
		Notes:          it.Notes,
	}
}

func NewContractDetailResponse(d *domain.ContractDetail) ContractDetailResponse {
	items := make([]ContractItemResponse, 0, len(d.Items))
	for i := range d.Items {
		items = append(items, NewContractItemResponse(&d.Items[i]))
	}
	return ContractDetailResponse{
		ContractResponse: NewContractResponse(d.Contract),
		Items:            items,
	}
}
