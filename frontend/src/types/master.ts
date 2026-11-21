// Bentuk data master (plan Bagian 8.2). Ditulis tangan sampai openapi.yaml ada.
// Uang selalu string desimal ("550000.00"); tidak pernah number.

export type ItemType = 'ASSET' | 'CONSUMABLE'

export interface Warehouse {
  id: number
  code: string
  name: string
  address: string | null
  phone: string | null
  latitude: number | null
  longitude: number | null
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface Rack {
  id: number
  warehouse_id: number
  warehouse_code: string
  code: string
  zone: string | null
  is_sellable: boolean
  description: string | null
  is_active: boolean
  locked_by_opname_id: number | null
  created_at: string
  updated_at: string
}

export interface Category {
  id: number
  name: string
  item_type: ItemType
  is_active: boolean
}

export interface Product {
  id: number
  sku: string
  name: string
  category_id: number
  category_name: string
  item_type: ItemType
  brand: string | null
  model: string | null
  unit: string
  has_serial_number: boolean
  min_stock_alert: number
  default_monthly_rate: string | null
  default_sell_price: string | null
  description: string | null
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface Supplier {
  id: number
  code: string
  name: string
  pic_name: string | null
  phone: string | null
  email: string | null
  npwp: string | null
  address: string | null
  is_active: boolean
  created_at: string
  updated_at: string
}

// ---- payload (null pada PATCH = kosongkan field) ----

export interface WarehouseCreate {
  code: string
  name: string
  address: string | null
  phone: string | null
  latitude: number | null
  longitude: number | null
}
export type WarehousePatch = Partial<Omit<WarehouseCreate, 'code'>> & { is_active?: boolean }

export interface RackCreate {
  warehouse_id: number
  code: string
  zone: string | null
  /** undefined = ikuti zona (KARANTINA/WORKSHOP tidak dapat dijual). */
  is_sellable?: boolean
  description: string | null
}
export type RackPatch = Partial<Pick<RackCreate, 'zone' | 'is_sellable' | 'description'>> & {
  is_active?: boolean
}

export interface CategoryCreate {
  name: string
  item_type: ItemType
}
export type CategoryPatch = Partial<CategoryCreate> & { is_active?: boolean }

export interface ProductCreate {
  sku: string
  name: string
  category_id: number
  brand: string | null
  model: string | null
  unit: string
  has_serial_number: boolean
  min_stock_alert: number
  default_monthly_rate: string | null
  default_sell_price: string | null
  description: string | null
}
export type ProductPatch = Partial<ProductCreate> & { is_active?: boolean }

export interface SupplierCreate {
  code: string
  name: string
  pic_name: string | null
  phone: string | null
  email: string | null
  npwp: string | null
  address: string | null
}
export type SupplierPatch = Partial<Omit<SupplierCreate, 'code'>> & { is_active?: boolean }
