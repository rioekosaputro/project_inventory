import type { LPBSubType } from '@/types/lpb'

export type SerialStatus =
  'AVAILABLE' | 'RESERVED' | 'IN_TRANSIT' | 'RENTED' | 'MAINTENANCE' | 'BROKEN' | 'LOST' | 'RETIRED'

export interface StockBalance {
  product_id: number
  sku: string
  product_name: string
  unit: string
  has_serial_number: boolean
  min_stock_alert: number
  rack_id: number
  rack_code: string
  zone: string | null
  is_sellable: boolean
  warehouse_id: number
  warehouse_code: string
  qty_on_hand: number
  qty_reserved: number
  qty_available: number
  is_low_stock: boolean
  updated_at: string
}

export interface SerialItem {
  id: number
  product_id: number
  sku: string
  product_name: string
  asset_tag: string
  serial_number: string
  status: SerialStatus
  condition: string
  rack_id: number | null
  rack_code: string | null
  warehouse_id: number | null
  warehouse_code: string | null
  customer_id: number | null
  site_id: number | null
  contract_id: number | null
  reserved_for_do_id: number | null
  warranty_until: string | null
  notes: string | null
  updated_at: string
}

export interface SerialHistoryItem {
  id: number
  from_status: string | null
  to_status: string
  from_rack_code: string | null
  to_rack_code: string | null
  customer_id: number | null
  event_type: string
  reference_type: string | null
  reference_id: number | null
  user_id: number | null
  user_name: string | null
  notes: string | null
  created_at: string
}

export interface SerialDetail extends SerialItem {
  history: SerialHistoryItem[]
}

export interface LedgerRow {
  id: number
  occurred_at: string
  product_id: number
  sku: string
  product_name: string
  unit: string
  warehouse_id: number
  warehouse_code: string
  rack_id: number
  rack_code: string
  direction: 'IN' | 'OUT'
  qty: number
  balance_before: number
  balance_after: number
  movement_type: LPBSubType
  lpb_id: number
  lpb_number: string
  serial_count: number
  created_by: number
  created_by_name: string
  notes: string | null
}

export interface StockCard {
  product_id: number
  sku: string
  product_name: string
  unit: string
  rack_id: number | null
  rack_code: string | null
  warehouse_id: number | null
  warehouse_code: string | null
  date_from: string | null
  date_to: string | null
  opening_balance: number
  total_in: number
  total_out: number
  closing_balance: number
  movements: LedgerRow[]
}
