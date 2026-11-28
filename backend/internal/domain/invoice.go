package domain

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

const EntityInvoice = "invoice"

type InvoiceType string

const (
	InvoiceTypeRental     InvoiceType = "RENTAL"
	InvoiceTypeConsumable InvoiceType = "CONSUMABLE"
	InvoiceTypeService    InvoiceType = "SERVICE"
	InvoiceTypeOther      InvoiceType = "OTHER"
)

func (t InvoiceType) Valid() bool {
	return t == InvoiceTypeRental || t == InvoiceTypeConsumable || t == InvoiceTypeService || t == InvoiceTypeOther
}

type InvoiceStatus string

const (
	InvoiceDraft         InvoiceStatus = "DRAFT"
	InvoiceIssued        InvoiceStatus = "ISSUED"
	InvoicePartiallyPaid InvoiceStatus = "PARTIALLY_PAID"
	InvoicePaid          InvoiceStatus = "PAID"
	InvoiceVoid          InvoiceStatus = "VOID"
)

func (s InvoiceStatus) Valid() bool {
	return s == InvoiceDraft || s == InvoiceIssued || s == InvoicePartiallyPaid || s == InvoicePaid || s == InvoiceVoid
}

type PaymentMethod string

const (
	PaymentTransfer PaymentMethod = "TRANSFER"
	PaymentCash     PaymentMethod = "CASH"
	PaymentGiro     PaymentMethod = "GIRO"
	PaymentOther    PaymentMethod = "OTHER"
)

func (m PaymentMethod) Valid() bool {
	return m == PaymentTransfer || m == PaymentCash || m == PaymentGiro || m == PaymentOther
}

type Invoice struct {
	ID                     int64
	InvoiceNumber          string
	InvoiceType            InvoiceType
	CustomerID             int64
	CustomerCode           string  // dari JOIN
	CustomerName           string  // dari JOIN
	CustomerNPWP           *string // dari JOIN
	CustomerBillingAddress string  // dari JOIN
	ContractID             *int64
	ContractNumber         *string // dari JOIN
	SOID                   *int64
	SONumber               *string // dari JOIN
	BillingPeriodStart     *time.Time
	BillingPeriodEnd       *time.Time
	InvoiceDate            time.Time
	DueDate                time.Time
	Status                 InvoiceStatus
	IsPPN                  bool
	Subtotal               decimal.Decimal
	DiscountAmount         decimal.Decimal
	DPP                    decimal.Decimal
	DPPOtherValue          decimal.Decimal
	TaxRate                decimal.Decimal
	TaxAmount              decimal.Decimal
	GrandTotal             decimal.Decimal
	PaidAmount             decimal.Decimal
	FakturPajakNumber      *string
	PDFPath                *string
	Notes                  *string
	GeneratedByJob         bool
	CreatedBy              *int64
	IssuedBy               *int64
	IssuedAt               *time.Time
	VoidedBy               *int64
	VoidedAt               *time.Time
	VoidReason             *string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type InvoiceItem struct {
	ID                int64
	InvoiceID         int64
	Description       string
	ProductID         *int64
	ProductSerialID   *int64
	CustomerSiteID    *int64
	RentalPlacementID *int64
	PeriodStart       *time.Time
	PeriodEnd         *time.Time
	DaysBilled        *int
	DaysInPeriod      *int
	Qty               decimal.Decimal
	UnitPrice         decimal.Decimal
	LineTotal         decimal.Decimal
}

type Payment struct {
	ID              int64
	InvoiceID       int64
	PaymentDate     time.Time
	AmountReceived  decimal.Decimal
	PPh23Amount     decimal.Decimal
	PPh23SlipNumber *string
	Method          PaymentMethod
	ReferenceNumber *string
	ProofPath       *string
	Notes           *string
	RecordedBy      int64
	RecordedByName  string
	CreatedAt       time.Time
}

// BillableContract adalah kontrak kandidat penagihan bulanan pada tanggal tertentu.
type BillableContract struct {
	ID                int64
	ContractNumber    string
	CustomerID        int64
	CustomerName      string
	BillingDayOfMonth int
	BillingMode       BillingMode
	IsPPN             bool
	PaymentTermDays   int
}

// BillablePlacement adalah hasil query irisan rental_placements dengan periode tagihan (Bagian 6.5).
type BillablePlacement struct {
	PlacementID     int64
	ProductID       int64
	ProductName     string
	ProductSerialID int64
	SerialNumber    string
	CustomerSiteID  int64
	SiteName        string
	MonthlyRate     decimal.Decimal
	BillFrom        time.Time
	BillToExcl      time.Time
	DaysBilled      int
	DaysInPeriod    int
}

type InvoiceDetail struct {
	Invoice  *Invoice
	Items    []InvoiceItem
	Payments []Payment
}

type InvoiceFilter struct {
	Query       string
	InvoiceType *InvoiceType
	Status      *InvoiceStatus
	CustomerID  *int64
	ContractID  *int64
	DateFrom    *time.Time
	DateTo      *time.Time
	OverdueOnly bool
	Today       time.Time
}

type InvoiceRepository interface {
	ListBillableContracts(ctx context.Context, billingDay int, contractID *int64) ([]BillableContract, error)
	HasActiveRentalInvoice(ctx context.Context, contractID int64, periodStart, periodEnd time.Time) (bool, error)
	ListPlacementsForPeriod(ctx context.Context, contractID int64, periodStart, periodEnd time.Time) ([]BillablePlacement, error)
	CreateInvoiceWithItems(ctx context.Context, inv *Invoice, items []InvoiceItem) (*Invoice, error)

	FindByID(ctx context.Context, id int64) (*Invoice, error)
	FindByIDForUpdate(ctx context.Context, id int64) (*Invoice, error)
	List(ctx context.Context, f InvoiceFilter, p PageParams) ([]Invoice, int64, error)
	ListAll(ctx context.Context, f InvoiceFilter) ([]Invoice, error)
	ListItems(ctx context.Context, invoiceID int64) ([]InvoiceItem, error)
	ListPayments(ctx context.Context, invoiceID int64) ([]Payment, error)
	MarkIssued(ctx context.Context, id, userID int64, at time.Time, fakturPajak *string) error
	MarkVoided(ctx context.Context, id, userID int64, at time.Time, reason string) error
	InsertPaymentAndUpdateInvoice(ctx context.Context, p *Payment, newPaid decimal.Decimal, newStatus InvoiceStatus) (*Payment, error)
}
