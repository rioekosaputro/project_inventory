package domain

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// LowStockAlertItem adalah satu produk yang stok tersedianya di bawah min_stock_alert.
type LowStockAlertItem struct {
	ProductID     int64
	SKU           string
	ProductName   string
	Unit          string
	QtyAvailable  int
	MinStockAlert int
}

// PendingLPBSummary adalah ringkasan LPB yang menunggu persetujuan pada dashboard.
type PendingLPBSummary struct {
	ID            int64
	Number        string
	SubType       LPBSubType
	WarehouseCode string
	CreatedByName string
	SubmittedAt   *time.Time
}

// CommercialDashboardSummary merangkum metrik komersial (plan Bagian 9.5, T2.11).
type CommercialDashboardSummary struct {
	ActiveCustomers        int
	ActiveContracts        int
	RentedUnits            int
	TotalActiveAssets      int
	AssetUtilizationPct    decimal.Decimal
	MonthlyRevenue         decimal.Decimal
	MonthlyCollected       decimal.Decimal
	OutstandingReceivables decimal.Decimal
	OverdueInvoiceCount    int
	OverdueInvoiceAmount   decimal.Decimal
	DueSoonInvoiceCount    int
	ExpiringContractsCount int
	ExpiringContracts      []ExpiringContractAlert
	OverdueInvoices        []OverdueInvoiceAlert
}

// SLABreachTicketAlert adalah satu tiket aktif yang SLA-nya sudah terlewati atau kritis (T3.11).
type SLABreachTicketAlert struct {
	TicketID     int64      `json:"ticket_id"`
	TicketNumber string     `json:"ticket_number"`
	Title        string     `json:"title"`
	CustomerName string     `json:"customer_name"`
	AssetTag     string     `json:"asset_tag"`
	Priority     string     `json:"priority"`
	Status       string     `json:"status"`
	SLADueAt     *time.Time `json:"sla_due_at"`
	HoursOverdue int        `json:"hours_overdue"`
}

// FieldDashboardSummary merangkum metrik lapangan & kepatuhan SLA maintenance (T3.11).
type FieldDashboardSummary struct {
	OpenTickets               int
	WaitingPartTickets        int
	SLABreachedTickets        int
	ResolvedThisMonth         int
	SLACompliancePct          decimal.Decimal
	ActiveDeliveries          int
	DeliveredToday            int
	FailedDeliveriesThisMonth int
	PendingSparepartRequests  int
	SLABreachAlerts           []SLABreachTicketAlert
}

// WarehouseDashboardSummary merangkum metrik gudang, komersial, & lapangan (plan Bagian 9.5).
type WarehouseDashboardSummary struct {
	TotalActiveProducts   int
	SafeStockProducts     int
	LowStockProducts      int
	TotalQtyAvailable     int
	SerialUnitsTotal      int
	SerialUnitsAvailable  int
	PendingLPBApprovals   int
	ActiveOpnames         int
	PendingDOReservations int
	LowStockItems         []LowStockAlertItem
	RecentPendingLPBs     []PendingLPBSummary
	RecentMovements       []LedgerRow
	Commercial            *CommercialDashboardSummary
	Field                 *FieldDashboardSummary
}

type DashboardRepository interface {
	WarehouseSummary(ctx context.Context, warehouseID *int64) (*WarehouseDashboardSummary, error)
	CommercialSummary(ctx context.Context, today time.Time) (*CommercialDashboardSummary, error)
	FieldSummary(ctx context.Context, now time.Time) (*FieldDashboardSummary, error)
}
