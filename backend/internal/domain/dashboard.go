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

// WarehouseDashboardSummary merangkum metrik gudang & (opsional) komersial (plan Bagian 9.5).
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
}

type DashboardRepository interface {
	WarehouseSummary(ctx context.Context, warehouseID *int64) (*WarehouseDashboardSummary, error)
	CommercialSummary(ctx context.Context, today time.Time) (*CommercialDashboardSummary, error)
}
