import type { DODirection, DOStatus, OrderType } from '@/types/salesOrder'

export interface DOReservation {
  id: number
  do_item_id: number
  rack_id: number
  rack_code: string
  product_serial_id: number | null
  serial_number: string | null
  asset_tag: string | null
  qty: number
  reserved_at: string
}

export interface DOItem {
  id: number
  do_id: number
  so_item_id: number
  product_id: number
  product_sku: string
  product_name: string
  has_serial_number: boolean
  qty: number
  reserved_qty: number
  remaining_qty: number
  so_product_serial_id: number | null
  so_serial_number: string | null
  reservations: DOReservation[]
}

export interface DeliveryOrder {
  id: number
  do_number: string
  so_id: number
  so_number: string
  order_type: OrderType
  customer_id: number
  customer_code: string
  customer_name: string
  customer_site_id: number
  site_name: string
  site_city: string
  contract_id: number | null
  warehouse_id: number
  warehouse_code: string
  warehouse_name: string
  direction: DODirection
  status: DOStatus
  scheduled_date: string | null
  lpb_id: number | null
  lpb_number: string | null
  notes: string | null
  created_by_name: string
  item_count: number
  total_qty: number
  reserved_qty: number
  created_at: string
  updated_at: string
}

export interface DeliveryOrderDetail extends DeliveryOrder {
  items: DOItem[]
}

export interface ReserveDOItemPayload {
  do_item_id: number
  rack_id: number
  qty: number
  serial_ids?: number[]
}

export interface CreatedLPBFromDO {
  id: number
  lpb_number: string
  type: string
  sub_type: string
  status: string
}
