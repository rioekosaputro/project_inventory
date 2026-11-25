export type OpnameStatus = 'IN_PROGRESS' | 'SUBMITTED' | 'FINALIZED' | 'CANCELLED'
export type OpnameSerialResult = 'FOUND' | 'MISSING' | 'UNEXPECTED' | 'WRONG_RACK'

export interface Opname {
  id: number
  opname_number: string
  warehouse_id: number
  warehouse_code: string
  warehouse_name: string
  status: OpnameStatus
  is_blind_count: boolean
  started_by: number
  started_by_name: string
  started_at: string
  submitted_by: number | null
  submitted_at: string | null
  finalized_by: number | null
  finalized_at: string | null
  adjustment_in_lpb_id: number | null
  adjustment_in_lpb_number: string | null
  adjustment_out_lpb_id: number | null
  adjustment_out_lpb_number: string | null
  notes: string | null
}

export interface OpnameListItem extends Opname {
  rack_count: number
  item_count: number
  counted_count: number
  serial_count: number
  found_count: number
}

export interface OpnameRack {
  rack_id: number
  rack_code: string
  zone: string | null
  is_sellable: boolean
}

export interface OpnameItem {
  id: number
  product_id: number
  sku: string
  product_name: string
  unit: string
  rack_id: number
  rack_code: string
  system_qty: number | null
  counted_qty: number | null
  diff_qty: number | null
  counted_by: number | null
  counted_at: string | null
  notes: string | null
}

export interface OpnameSerial {
  id: number
  rack_id: number
  rack_code: string
  product_serial_id: number | null
  asset_tag: string | null
  serial_number: string | null
  sku: string | null
  product_name: string | null
  scanned_code: string | null
  result: OpnameSerialResult
  scanned_by: number | null
  scanned_at: string | null
}

export interface OpnameDetail extends Opname {
  racks: OpnameRack[]
  items: OpnameItem[]
  serials: OpnameSerial[]
}

export interface StartOpnameInput {
  warehouse_id: number
  rack_ids: number[]
  is_blind_count: boolean
  notes: string | null
}

export interface OpnameCountInput {
  product_id: number
  rack_id: number
  counted_qty: number
  notes: string | null
}
