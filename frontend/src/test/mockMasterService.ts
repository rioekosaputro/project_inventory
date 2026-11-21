import { vi } from 'vitest'
import type * as Real from '@/services/masterService'

type Fn = (...args: never[]) => unknown
type Crud = { list: Fn; get: Fn; create: Fn; update: Fn }

/** Mock bertipe mengikuti service aslinya (tiap sumber data punya tipe item sendiri). */
function crud<S extends Crud>() {
  return {
    list: vi.fn<S['list']>(),
    get: vi.fn<S['get']>(),
    create: vi.fn<S['create']>(),
    update: vi.fn<S['update']>(),
  }
}

/**
 * Pengganti masterService untuk test:
 *   vi.mock('@/services/masterService', () => import('@/test/mockMasterService'))
 */
export const warehouseService = crud<typeof Real.warehouseService>()
export const rackService = crud<typeof Real.rackService>()
export const categoryService = crud<typeof Real.categoryService>()
export const productService = crud<typeof Real.productService>()
export const supplierService = crud<typeof Real.supplierService>()

/** Halaman hasil daftar dengan meta yang konsisten. */
export function pageOf<T>(
  items: T[],
  over: { page?: number; limit?: number; total?: number } = {},
) {
  const limit = over.limit ?? 20
  const total = over.total ?? items.length
  return {
    items,
    meta: {
      page: over.page ?? 1,
      limit,
      total,
      total_pages: total === 0 ? 0 : Math.ceil(total / limit),
    },
  }
}

export function resetMasterMocks(): void {
  for (const s of [
    warehouseService,
    rackService,
    categoryService,
    productService,
    supplierService,
  ]) {
    for (const fn of Object.values(s)) fn.mockReset()
    s.list.mockResolvedValue(pageOf([]) as never)
  }
}
