import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter, type LocationQueryRaw } from 'vue-router'
import { toApiError } from '@/lib/apiError'
import type { ListParams, Page } from '@/services/masterService'
import type { PageMeta } from '@/types/api'

export const DEFAULT_LIMIT = 20
export const LIMIT_CHOICES = [10, 20, 50, 100] as const

export interface UseServerTableOptions<T> {
  fetch: (params: ListParams) => Promise<Page<T>>
  /** Urutan awal (nama kolom API), mis. "name" atau "-created_at". */
  defaultSort: string
  /** Kunci filter yang tersinkron ke URL (selain q, page, limit, sort). */
  filterKeys: readonly string[]
  /** Jeda pencarian saat mengetik (ms). */
  debounceMs?: number
}

/**
 * Tabel dengan paginasi, urutan, pencarian, dan filter di server. Seluruh status berada di
 * query string URL (bisa di-bookmark/dibagikan; reload mempertahankan status). Respons yang datang
 * terlambat dibuang agar tidak menimpa hasil permintaan yang lebih baru.
 */
export function useServerTable<T>(opts: UseServerTableOptions<T>) {
  const route = useRoute()
  const router = useRouter()
  const homePath = route.path // abaikan perubahan query bila pengguna sudah pindah halaman

  const rows = shallowRef<T[]>([])
  const meta = ref<PageMeta | null>(null)
  const loading = ref(false)
  const loaded = ref(false)
  const error = ref('')

  const str = (v: unknown): string => (typeof v === 'string' ? v : '')
  const posInt = (v: unknown, fallback: number): number => {
    const n = Number.parseInt(str(v), 10)
    return Number.isFinite(n) && n >= 1 ? n : fallback
  }

  // ---- status dari URL
  const page = computed(() => posInt(route.query.page, 1))
  const limit = computed(() => Math.min(posInt(route.query.limit, DEFAULT_LIMIT), 100))
  const sort = computed(() => str(route.query.sort) || opts.defaultSort)
  const q = computed(() => str(route.query.q))
  const filters = computed<Record<string, string>>(() =>
    Object.fromEntries(opts.filterKeys.map((k) => [k, str(route.query[k])])),
  )
  const activeFilters = computed(() => Object.entries(filters.value).filter(([, v]) => v !== ''))
  const hasCriteria = computed(() => q.value !== '' || activeFilters.value.length > 0)

  /** Kolom urutan utama dan arahnya, untuk aria-sort. */
  const sortColumn = computed(() => sort.value.split(',')[0]!.replace(/^-/, ''))
  const sortDirection = computed<'ascending' | 'descending'>(() =>
    sort.value.startsWith('-') ? 'descending' : 'ascending',
  )

  // ---- tulis ke URL (nilai bawaan dibuang agar URL tetap bersih)
  function navigate(patch: Record<string, string | number | undefined>): void {
    const next: Record<string, string | number | undefined> = {
      q: q.value || undefined,
      page: page.value > 1 ? page.value : undefined,
      limit: limit.value !== DEFAULT_LIMIT ? limit.value : undefined,
      sort: sort.value !== opts.defaultSort ? sort.value : undefined,
      ...filters.value,
      ...patch,
    }
    // Nilai bawaan tidak ditulis agar URL tetap bersih.
    if (next.sort === opts.defaultSort) next.sort = undefined
    if (Number(next.page) <= 1) next.page = undefined
    if (Number(next.limit) === DEFAULT_LIMIT) next.limit = undefined
    const query: LocationQueryRaw = {}
    for (const [k, v] of Object.entries(next)) {
      if (v !== undefined && v !== '') query[k] = String(v)
    }
    void router.replace({ path: route.path, query })
  }

  function setPage(p: number): void {
    navigate({ page: p > 1 ? p : undefined })
  }
  function setLimit(n: number): void {
    navigate({ limit: n !== DEFAULT_LIMIT ? n : undefined, page: undefined })
  }
  /** Klik kolom: pertama naik, klik lagi turun. */
  function toggleSort(column: string): void {
    const next =
      sortColumn.value === column && sortDirection.value === 'ascending' ? `-${column}` : column
    navigate({ sort: next, page: undefined })
  }
  function setFilter(key: string, value: string): void {
    navigate({ [key]: value, page: undefined })
  }
  function clearFilters(): void {
    const cleared = Object.fromEntries(opts.filterKeys.map((k) => [k, undefined]))
    navigate({ ...cleared, q: undefined, page: undefined })
  }

  // ---- pencarian: kotak input langsung diperbarui, URL/permintaan menunggu jeda
  const search = ref(q.value)
  let timer: ReturnType<typeof setTimeout> | undefined
  function setSearch(value: string): void {
    search.value = value
    clearTimeout(timer)
    timer = setTimeout(
      () => navigate({ q: value.trim() || undefined, page: undefined }),
      opts.debounceMs ?? 300,
    )
  }
  /** Terapkan pencarian sekarang (Enter), tanpa menunggu jeda. */
  function submitSearch(): void {
    clearTimeout(timer)
    navigate({ q: search.value.trim() || undefined, page: undefined })
  }
  watch(q, (v) => {
    if (v !== search.value.trim()) search.value = v // Back/Forward atau "hapus semua filter"
  })

  // ---- ambil data
  let seq = 0
  async function load(): Promise<void> {
    const mine = ++seq
    loading.value = true
    error.value = ''
    try {
      const params: ListParams = {
        page: page.value,
        limit: limit.value,
        sort: sort.value,
        q: q.value,
        ...filters.value,
      }
      const result = await opts.fetch(params)
      if (mine !== seq) return
      rows.value = result.items
      meta.value = result.meta
      loaded.value = true
      // Halaman melewati akhir (mis. setelah data berkurang): mundur ke halaman terakhir.
      if (result.items.length === 0 && result.meta.total > 0 && page.value > 1) {
        setPage(Math.max(result.meta.total_pages, 1))
      }
    } catch (e) {
      if (mine !== seq) return
      error.value = toApiError(e).message
    } finally {
      if (mine === seq) loading.value = false
    }
  }

  watch(
    () => route.fullPath,
    () => {
      if (route.path === homePath) void load()
    },
    { immediate: true },
  )
  onBeforeUnmount(() => {
    clearTimeout(timer)
    seq++ // buang respons yang masih di jalan
  })

  return {
    rows,
    meta,
    loading,
    loaded,
    error,
    page,
    limit,
    sort,
    sortColumn,
    sortDirection,
    q,
    filters,
    activeFilters,
    hasCriteria,
    search,
    setPage,
    setLimit,
    toggleSort,
    setFilter,
    clearFilters,
    setSearch,
    submitSearch,
    reload: load,
  }
}

export type ServerTable<T = unknown> = ReturnType<typeof useServerTable<T>>
