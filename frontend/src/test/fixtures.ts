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

/** Permission per peran (mengikuti internal/domain/permission.go). */
export const PERMS = {
  head: ['master.read', 'master.write', 'rack.write', 'product.read', 'inventory.read'],
  staff: ['master.read', 'rack.write', 'product.read'],
  tech: ['product.read'],
  mgmt: ['master.read', 'product.read'],
} as const
