package domain

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

const EntityContract = "rental_contract"

type ContractStatus string

const (
	ContractDraft      ContractStatus = "DRAFT"
	ContractActive     ContractStatus = "ACTIVE"
	ContractExpired    ContractStatus = "EXPIRED"
	ContractTerminated ContractStatus = "TERMINATED"
)

func (s ContractStatus) Valid() bool {
	return s == ContractDraft || s == ContractActive || s == ContractExpired || s == ContractTerminated
}

type BillingMode string

const (
	BillingModeProrata   BillingMode = "PRORATA"
	BillingModeFullMonth BillingMode = "FULL_MONTH"
)

func (m BillingMode) Valid() bool {
	return m == BillingModeProrata || m == BillingModeFullMonth
}

// RentalContract adalah header kontrak sewa B2B.
type RentalContract struct {
	ID                 int64
	ContractNumber     string
	CustomerID         int64
	CustomerCode       string // dari JOIN
	CustomerName       string // dari JOIN
	StartDate          time.Time
	EndDate            time.Time
	DurationMonths     int
	BillingDayOfMonth  int
	BillingMode        BillingMode
	BillingInAdvance   bool
	IsPPN              bool
	PaymentTermDays    int
	DepositAmount      decimal.Decimal
	Status             ContractStatus
	TerminatedAt       *time.Time
	TerminationReason  *string
	RenewedFromID      *int64
	RenewedFromNumber  *string // dari JOIN
	SignedDocumentPath *string
	Notes              *string
	CreatedBy          int64
	CreatedByName      string // dari JOIN
	ActivatedBy        *int64
	ActivatedByName    *string // dari JOIN
	ActivatedAt        *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time

	// Ringkasan baca (untuk List / Detail).
	ItemCount          int
	TotalQty           int
	EstimatedMonthly   decimal.Decimal
	ActivePlacementQty int
}

// RentalContractItem adalah baris produk & tarif bulanan pada kontrak sewa.
type RentalContractItem struct {
	ID             int64
	ContractID     int64
	ProductID      int64
	ProductSKU     string // dari JOIN
	ProductName    string // dari JOIN
	CustomerSiteID *int64
	SiteName       *string // dari JOIN
	Qty            int
	MonthlyRate    decimal.Decimal
	Notes          *string
}

type ContractDetail struct {
	Contract *RentalContract
	Items    []RentalContractItem
}

type ContractFilter struct {
	Query      string // contract_number, customer_code, company_name
	CustomerID *int64
	Status     *ContractStatus
	ExpiringIn *int // hari ke depan (mis. 30 → kontrak ACTIVE yang end_date <= today+30)
	Today      time.Time
}

type ContractRepository interface {
	FindByID(ctx context.Context, id int64) (*RentalContract, error)
	FindByIDForUpdate(ctx context.Context, id int64) (*RentalContract, error)
	List(ctx context.Context, f ContractFilter, p PageParams) ([]RentalContract, int64, error)
	Create(ctx context.Context, c *RentalContract) (*RentalContract, error)
	Update(ctx context.Context, c *RentalContract) error
	UpdateStatus(ctx context.Context, c *RentalContract) error

	ListItems(ctx context.Context, contractID int64) ([]RentalContractItem, error)
	ReplaceItems(ctx context.Context, contractID int64, items []RentalContractItem) ([]RentalContractItem, error)
}
