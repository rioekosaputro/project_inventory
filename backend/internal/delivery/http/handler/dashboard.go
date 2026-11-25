package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"rental-inventory-api/internal/delivery/http/dto"
	"rental-inventory-api/internal/delivery/http/httperr"
	"rental-inventory-api/internal/domain"
	"rental-inventory-api/internal/usecase"
	"rental-inventory-api/pkg/response"
)

type DashboardHandler struct{ uc *usecase.DashboardUsecase }

func NewDashboardHandler(uc *usecase.DashboardUsecase) *DashboardHandler {
	return &DashboardHandler{uc: uc}
}

type dashboardQuery struct {
	WarehouseID *int64 `form:"warehouse_id" binding:"omitempty,gte=1"`
}

type lowStockItemResp struct {
	ProductID     int64  `json:"product_id"`
	SKU           string `json:"sku"`
	ProductName   string `json:"product_name"`
	Unit          string `json:"unit"`
	QtyAvailable  int    `json:"qty_available"`
	MinStockAlert int    `json:"min_stock_alert"`
}

type pendingLPBResp struct {
	ID            int64      `json:"id"`
	LPBNumber     string     `json:"lpb_number"`
	SubType       string     `json:"sub_type"`
	WarehouseCode string     `json:"warehouse_code"`
	CreatedByName string     `json:"created_by_name"`
	SubmittedAt   *time.Time `json:"submitted_at"`
}

type commercialDashboardResp struct {
	ActiveCustomers        int                            `json:"active_customers"`
	ActiveContracts        int                            `json:"active_contracts"`
	RentedUnits            int                            `json:"rented_units"`
	TotalActiveAssets      int                            `json:"total_active_assets"`
	AssetUtilizationPct    string                         `json:"asset_utilization_pct"`
	MonthlyRevenue         string                         `json:"monthly_revenue"`
	MonthlyCollected       string                         `json:"monthly_collected"`
	OutstandingReceivables string                         `json:"outstanding_receivables"`
	OverdueInvoiceCount    int                            `json:"overdue_invoice_count"`
	OverdueInvoiceAmount   string                         `json:"overdue_invoice_amount"`
	DueSoonInvoiceCount    int                            `json:"due_soon_invoice_count"`
	ExpiringContractsCount int                            `json:"expiring_contracts_count"`
	ExpiringContracts      []domain.ExpiringContractAlert `json:"expiring_contracts"`
	OverdueInvoices        []domain.OverdueInvoiceAlert   `json:"overdue_invoices"`
}

type dashboardSummaryResp struct {
	TotalActiveProducts   int                      `json:"total_active_products"`
	SafeStockProducts     int                      `json:"safe_stock_products"`
	LowStockProducts      int                      `json:"low_stock_products"`
	TotalQtyAvailable     int                      `json:"total_qty_available"`
	SerialUnitsTotal      int                      `json:"serial_units_total"`
	SerialUnitsAvailable  int                      `json:"serial_units_available"`
	PendingLPBApprovals   int                      `json:"pending_lpb_approvals"`
	ActiveOpnames         int                      `json:"active_opnames"`
	PendingDOReservations int                      `json:"pending_do_reservations"`
	LowStockItems         []lowStockItemResp       `json:"low_stock_items"`
	RecentPendingLPBs     []pendingLPBResp         `json:"recent_pending_lpbs"`
	RecentMovements       []dto.LedgerRowResponse  `json:"recent_movements"`
	Commercial            *commercialDashboardResp `json:"commercial,omitempty"`
}

// Summary — GET /dashboard/summary (plan Bagian 8.5 & 9.5)
func (h *DashboardHandler) Summary(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	var q dashboardQuery
	if !httperr.BindQuery(c, &q) {
		return
	}
	s, err := h.uc.Summary(c.Request.Context(), a, q.WarehouseID)
	if err != nil {
		httperr.Write(c, err)
		return
	}

	low := make([]lowStockItemResp, 0, len(s.LowStockItems))
	for _, it := range s.LowStockItems {
		low = append(low, lowStockItemResp{
			ProductID: it.ProductID, SKU: it.SKU, ProductName: it.ProductName,
			Unit: it.Unit, QtyAvailable: it.QtyAvailable, MinStockAlert: it.MinStockAlert,
		})
	}
	pend := make([]pendingLPBResp, 0, len(s.RecentPendingLPBs))
	for _, p := range s.RecentPendingLPBs {
		pend = append(pend, pendingLPBResp{
			ID: p.ID, LPBNumber: p.Number, SubType: string(p.SubType),
			WarehouseCode: p.WarehouseCode, CreatedByName: p.CreatedByName, SubmittedAt: p.SubmittedAt,
		})
	}
	moves := make([]dto.LedgerRowResponse, 0, len(s.RecentMovements))
	for i := range s.RecentMovements {
		moves = append(moves, dto.NewLedgerRowResponse(&s.RecentMovements[i]))
	}

	var comm *commercialDashboardResp
	if s.Commercial != nil {
		c := s.Commercial
		comm = &commercialDashboardResp{
			ActiveCustomers:        c.ActiveCustomers,
			ActiveContracts:        c.ActiveContracts,
			RentedUnits:            c.RentedUnits,
			TotalActiveAssets:      c.TotalActiveAssets,
			AssetUtilizationPct:    c.AssetUtilizationPct.StringFixed(2),
			MonthlyRevenue:         c.MonthlyRevenue.StringFixed(2),
			MonthlyCollected:       c.MonthlyCollected.StringFixed(2),
			OutstandingReceivables: c.OutstandingReceivables.StringFixed(2),
			OverdueInvoiceCount:    c.OverdueInvoiceCount,
			OverdueInvoiceAmount:   c.OverdueInvoiceAmount.StringFixed(2),
			DueSoonInvoiceCount:    c.DueSoonInvoiceCount,
			ExpiringContractsCount: c.ExpiringContractsCount,
			ExpiringContracts:      c.ExpiringContracts,
			OverdueInvoices:        c.OverdueInvoices,
		}
	}

	response.OK(c, dashboardSummaryResp{
		TotalActiveProducts:   s.TotalActiveProducts,
		SafeStockProducts:     s.SafeStockProducts,
		LowStockProducts:      s.LowStockProducts,
		TotalQtyAvailable:     s.TotalQtyAvailable,
		SerialUnitsTotal:      s.SerialUnitsTotal,
		SerialUnitsAvailable:  s.SerialUnitsAvailable,
		PendingLPBApprovals:   s.PendingLPBApprovals,
		ActiveOpnames:         s.ActiveOpnames,
		PendingDOReservations: s.PendingDOReservations,
		LowStockItems:         low,
		RecentPendingLPBs:     pend,
		RecentMovements:       moves,
		Commercial:            comm,
	})
}
