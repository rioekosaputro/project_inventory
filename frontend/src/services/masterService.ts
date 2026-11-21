import { api } from '@/lib/axios'
import type { PageMeta } from '@/types/api'
import type {
  Category,
  CategoryCreate,
  CategoryPatch,
  Product,
  ProductCreate,
  ProductPatch,
  Rack,
  RackCreate,
  RackPatch,
  Supplier,
  SupplierCreate,
  SupplierPatch,
  Warehouse,
  WarehouseCreate,
  WarehousePatch,
} from '@/types/master'

/** Parameter daftar: page/limit/sort/q + filter khusus (nilai kosong dibuang). */
export type ListParams = Record<string, string | number | boolean | undefined>

export interface Page<T> {
  items: T[]
  meta: PageMeta
}

function clean(params: ListParams): ListParams {
  return Object.fromEntries(
    Object.entries(params).filter(([, v]) => v !== undefined && v !== ''),
  ) as ListParams
}

function crud<T, C, P>(base: string) {
  return {
    list: (params: ListParams = {}): Promise<Page<T>> =>
      api.page<T>(base, { params: clean(params) }),
    get: (id: number): Promise<T> => api.get<T>(`${base}/${id}`),
    create: (body: C): Promise<T> => api.post<T>(base, body),
    update: (id: number, body: P): Promise<T> => api.patch<T>(`${base}/${id}`, body),
  }
}

export const warehouseService = crud<Warehouse, WarehouseCreate, WarehousePatch>('/warehouses')
export const rackService = crud<Rack, RackCreate, RackPatch>('/racks')
export const categoryService = crud<Category, CategoryCreate, CategoryPatch>('/categories')
export const productService = crud<Product, ProductCreate, ProductPatch>('/products')
export const supplierService = crud<Supplier, SupplierCreate, SupplierPatch>('/suppliers')
