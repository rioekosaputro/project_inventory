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
}

export const dashboardService = {
  summary: (warehouseId?: number): Promise<WarehouseDashboardSummary> =>
    api.get<WarehouseDashboardSummary>('/dashboard/summary', {
      params: warehouseId ? { warehouse_id: warehouseId } : undefined,
    }),
}
