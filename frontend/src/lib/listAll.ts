import type { ListParams, Page } from '@/services/masterService'

/**
 * Mengambil beberapa halaman berturut-turut (maks. `maxPages` × 100 data) untuk pilihan select
 * yang harus memuat semuanya, mis. produk pada form LPB. Berhenti lebih awal bila halaman terakhir
 * tercapai. Lebih dari batas itu perlu kolom pencarian (belum ada) — batasnya dicatat di plan.
 */
export async function listAll<T>(
  list: (params: ListParams) => Promise<Page<T>>,
  params: ListParams,
  maxPages = 5,
): Promise<Page<T>> {
  const first = await list({ ...params, limit: 100, page: 1 })
  const items = [...first.items]
  const totalPages = Math.min(first.meta.total_pages, maxPages)
  for (let page = 2; page <= totalPages; page++) {
    items.push(...(await list({ ...params, limit: 100, page })).items)
  }
  return { items, meta: { ...first.meta, page: 1, limit: items.length } }
}
