package domain

import (
	"context"
	"time"
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

// WarehouseDashboardSummary merangkum metrik gudang (plan Bagian 9.5).
type WarehouseDashboardSummary struct {
	TotalActiveProducts   int
	SafeStockProducts     int
	LowStockProducts      int
	TotalQtyAvailable     int
	SerialUnitsTotal      int
	SerialUnitsAvailable  int
	PendingLPBApprovals   int
	ActiveOpnames         int
	PendingDOReservations int // terisi di Fase 2 (T2.6)
	LowStockItems         []LowStockAlertItem
	RecentPendingLPBs     []PendingLPBSummary
	RecentMovements       []LedgerRow
}

type DashboardRepository interface {
	WarehouseSummary(ctx context.Context, warehouseID *int64) (*WarehouseDashboardSummary, error)
}
