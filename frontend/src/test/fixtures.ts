import type { LPBDetail, LPBItem, LPBItemSerial, LPBListItem } from '@/types/lpb'
import type { Category, Product, Rack, Supplier, Warehouse } from '@/types/master'

const ts = '2026-11-21T10:00:00+07:00'

export const warehouse = (over: Partial<Warehouse> = {}): Warehouse => ({
  id: 1,
  code: 'WH-A',
  name: 'Gudang A',
  address: 'Jl. Industri 1',
  phone: '024-123',
  latitude: null,
  longitude: null,
  is_active: true,
  created_at: ts,
  updated_at: ts,
  ...over,
})

export const rack = (over: Partial<Rack> = {}): Rack => ({
  id: 1,
  warehouse_id: 1,
  warehouse_code: 'WH-A',
  code: 'A-01',
  zone: 'PRINTER',
  is_sellable: true,
  description: null,
  is_active: true,
  locked_by_opname_id: null,
  created_at: ts,
  updated_at: ts,
  ...over,
})

export const category = (over: Partial<Category> = {}): Category => ({
  id: 1,
  name: 'Printer',
  item_type: 'ASSET',
  is_active: true,
  ...over,
})

export const product = (over: Partial<Product> = {}): Product => ({
  id: 1,
  sku: 'PRN-1',
  name: 'Epson L3210',
  category_id: 1,
  category_name: 'Printer',
  item_type: 'ASSET',
  brand: 'Epson',
  model: null,
  unit: 'unit',
  has_serial_number: true,
  min_stock_alert: 2,
  default_monthly_rate: '550000.00',
  default_sell_price: null,
  description: null,
  is_active: true,
  created_at: ts,
  updated_at: ts,
  ...over,
})

export const supplier = (over: Partial<Supplier> = {}): Supplier => ({
  id: 1,
  code: 'SUP-1',
  name: 'PT Maju Jaya',
  pic_name: 'Budi',
  phone: null,
  email: 'sales@maju.id',
  npwp: null,
  address: null,
  is_active: true,
  created_at: ts,
  updated_at: ts,
  ...over,
})

export const lpbListItem = (over: Partial<LPBListItem> = {}): LPBListItem => ({
  id: 7,
  lpb_number: 'LPBI-WH-A-2611-00007',
  type: 'INBOUND',
  sub_type: 'PURCHASE',
  status: 'DRAFT',
  warehouse_id: 1,
  warehouse_code: 'WH-A',
  doc_date: '2026-11-21',
  reference_type: 'PURCHASE_INVOICE',
  reference_number: 'INV-77',
  supplier_id: 1,
  supplier_name: 'PT Maju Jaya',
  notes: null,
  created_by: 10,
  created_by_name: 'Budi Santoso',
  submitted_by: null,
  submitted_at: null,
  approved_by: null,
  approved_at: null,
  rejected_by: null,
  rejected_at: null,
  rejection_reason: null,
  created_at: ts,
  updated_at: ts,
  item_count: 1,
  total_qty: 2,
  ...over,
})

export const lpbSerial = (over: Partial<LPBItemSerial> = {}): LPBItemSerial => ({
  id: 100,
  product_serial_id: null,
  serial_number: 'SN-001',
  asset_tag: null,
  condition: 'GOOD',
  scanned_by: 10,
  scanned_at: ts,
  ...over,
})

export const lpbItem = (over: Partial<LPBItem> = {}): LPBItem => ({
  id: 70,
  product_id: 1,
  sku: 'PRN-1',
  product_name: 'Epson L3210',
  unit: 'unit',
  has_serial_number: true,
  rack_id: 1,
  rack_code: 'A-01',
  rack_is_sellable: true,
  qty: 2,
  unit_cost: '1500000.00',
  notes: null,
  scanned_count: 0,
  current_qty: 5,
  balance_before: null,
  balance_after: null,
  serials: [],
  ...over,
})

export const lpbDetail = (over: Partial<LPBDetail> = {}): LPBDetail => {
  const { item_count: _c, total_qty: _t, ...base } = lpbListItem()
  return {
    ...base,
    items: [lpbItem()],
    timeline: [
      { action: 'LPB_CREATE', user_id: 10, user_name: 'Budi Santoso', data: null, created_at: ts },
    ],
    ...over,
  }
}

/** Permission per peran (mengikuti internal/domain/permission.go). */
export const PERMS = {
  head: [
    'master.read',
    'master.write',
    'rack.write',
    'product.read',
    'inventory.read',
    'lpb.read',
    'lpb.create',
    'lpb.submit',
    'lpb.approve',
  ],
  staff: ['master.read', 'rack.write', 'product.read', 'lpb.read', 'lpb.create', 'lpb.submit'],
  tech: ['product.read'],
  mgmt: ['master.read', 'product.read'],
} as const
