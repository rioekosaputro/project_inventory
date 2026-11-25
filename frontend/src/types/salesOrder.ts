export type OrderType = 'NEW_DELIVERY' | 'WITHDRAWAL' | 'UNIT_SWAP' | 'CONSUMABLE'
export type SOStatus = 'DRAFT' | 'CONFIRMED' | 'IN_PROGRESS' | 'COMPLETED' | 'CANCELLED'
export type ItemDirection = 'OUT' | 'IN'
export type DODirection = 'OUTBOUND' | 'INBOUND'
export type DOStatus = 'OPEN' | 'RESERVED' | 'LPB_CREATED' | 'COMPLETED' | 'CANCELLED'

export interface SalesOrderItem {
  id: number
  so_id: number
  product_id: number
  product_sku: string
  product_name: string
  direction: ItemDirection
  qty: number
  unit_price: string
  line_total: string
  product_serial_id: number | null
  serial_number: string | null
  asset_tag: string | null
  notes: string | null
}

export interface DeliveryOrderSummary {
  id: number
  do_number: string
  so_id: number
  warehouse_id: number
  warehouse_code: string
  warehouse_name: string
  direction: DODirection
  status: DOStatus
  scheduled_date: string | null
  lpb_id: number | null
  notes: string | null
  item_count: number
  total_qty: number
  created_at: string
}

export interface SalesOrder {
  id: number
  so_number: string
  order_type: OrderType
  customer_id: number
  customer_code: string
  customer_name: string
  customer_site_id: number
  site_name: string
  site_city: string
  contract_id: number | null
  contract_number: string | null
  status: SOStatus
  order_date: string
  requested_date: string | null
  is_ppn: boolean
  subtotal: string
  discount_amount: string
  dpp: string
  dpp_other_value: string
  tax_rate: string
  tax_amount: string
  grand_total: string
  notes: string | null
  created_by: number
  created_by_name: string
  confirmed_by: number | null
  confirmed_by_name: string | null
  confirmed_at: string | null
  cancelled_by: number | null
  cancelled_by_name: string | null
  cancelled_at: string | null
  cancel_reason: string | null
  item_count: number
  total_qty: number
  do_count: number
  created_at: string
  updated_at: string
}

export interface SalesOrderDetail extends SalesOrder {
  items: SalesOrderItem[]
  delivery_orders: DeliveryOrderSummary[]
}

export interface SOItemPayload {
  product_id: number
  direction?: ItemDirection
  qty: number
  unit_price?: string | null
  product_serial_id?: number | null
  notes?: string | null
}

export interface CreateSOPayload {
  order_type: OrderType
  customer_id: number
  customer_site_id: number
  contract_id?: number | null
  order_date?: string | null
  requested_date?: string | null
  is_ppn?: boolean
  discount_amount?: string | null
  notes?: string | null
  items: SOItemPayload[]
}

export interface UpdateSOPayload {
  customer_site_id?: number
  order_date?: string | null
  requested_date?: string | null
  is_ppn?: boolean
  discount_amount?: string | null
  notes?: string | null
  items?: SOItemPayload[]
}

export interface IssueDOPayload {
  warehouse_id: number
  scheduled_date?: string | null
  notes?: string | null
}
