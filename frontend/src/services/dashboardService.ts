import { api } from '@/lib/axios'
import type { LedgerRow } from '@/types/inventory'
import type { LPBSubType } from '@/types/lpb'

export interface LowStockAlertItem {
  product_id: number
  sku: string
  product_name: string
  unit: string
  qty_available: number
  min_stock_alert: number
}

export interface PendingLPBSummary {
  id: number
  lpb_number: string
  sub_type: LPBSubType
  warehouse_code: string
  created_by_name: string
  submitted_at: string | null
}

export interface ExpiringContractAlert {
  contract_id: number
  contract_number: string
  customer_id: number
  customer_name: string
  end_date: string
  days_remaining: number
}

export interface OverdueInvoiceAlert {
  invoice_id: number
  invoice_number: string
  customer_id: number
  customer_name: string
  due_date: string
  days_overdue: number
  remaining_amount: string
}

export interface CommercialDashboardSummary {
  active_customers: number
  active_contracts: number
  rented_units: number
  total_active_assets: number
  asset_utilization_pct: string
  monthly_revenue: string
  monthly_collected: string
  outstanding_receivables: string
  overdue_invoice_count: number
  overdue_invoice_amount: string
  due_soon_invoice_count: number
  expiring_contracts_count: number
  expiring_contracts: ExpiringContractAlert[]
  overdue_invoices: OverdueInvoiceAlert[]
}

export interface SLABreachTicketAlert {
  ticket_id: number
  ticket_number: string
  title: string
  customer_name: string
  asset_tag: string
  priority: string
  status: string
  sla_due_at: string | null
  hours_overdue: number
}

export interface FieldDashboardSummary {
  open_tickets: number
  waiting_part_tickets: number
  sla_breached_tickets: number
  resolved_this_month: number
  sla_compliance_pct: string
  active_deliveries: number
  delivered_today: number
  failed_deliveries_this_month: number
  pending_sparepart_requests: number
  sla_breach_alerts: SLABreachTicketAlert[]
}

export interface WarehouseDashboardSummary {
  total_active_products: number
  safe_stock_products: number
  low_stock_products: number
  total_qty_available: number
  serial_units_total: number
  serial_units_available: number
  pending_lpb_approvals: number
  active_opnames: number
  pending_do_reservations: number
  low_stock_items: LowStockAlertItem[]
  recent_pending_lpbs: PendingLPBSummary[]
  recent_movements: LedgerRow[]
  commercial?: CommercialDashboardSummary
  field?: FieldDashboardSummary
}

export const dashboardService = {
  summary: (warehouseId?: number): Promise<WarehouseDashboardSummary> =>
    api.get<WarehouseDashboardSummary>('/dashboard/summary', {
      params: warehouseId ? { warehouse_id: warehouseId } : undefined,
    }),
}
