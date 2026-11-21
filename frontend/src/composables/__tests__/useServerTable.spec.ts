import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { useServerTable } from '@/composables/useServerTable'
import type { ListParams, Page } from '@/services/masterService'

interface Row {
  id: number
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

let router: Router
let wrapper: VueWrapper | undefined
let table: ReturnType<typeof useServerTable<Row>>
const fetch = vi.fn<(p: ListParams) => Promise<Page<Row>>>()

const page = (ids: number[], over: Partial<Page<Row>['meta']> = {}): Page<Row> => ({
  items: ids.map((id) => ({ id })),
  meta: { page: 1, limit: 20, total: ids.length, total_pages: 1, ...over },
})

async function setup(initial = '/tabel') {
  router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/tabel', component: { render: () => null } },
      { path: '/lain', component: { render: () => null } },
    ],
  })
  await router.push(initial)
  await router.isReady()
  const Harness = defineComponent({
    setup() {
      table = useServerTable<Row>({
        fetch,
        defaultSort: 'name',
        filterKeys: ['is_active', 'item_type'],
        debounceMs: 30,
      })
      return () => h('div')
    },
  })
  wrapper = mount(Harness, { global: { plugins: [router] } })
  await flushPromises()
}

beforeEach(() => {
  fetch.mockReset()
  fetch.mockResolvedValue(page([1, 2]))
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
})

const lastParams = () => fetch.mock.calls[fetch.mock.calls.length - 1]![0]

describe('muat awal', () => {
  it('memakai nilai bawaan: halaman 1, 20 baris, urutan bawaan', async () => {
    await setup()
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(lastParams()).toMatchObject({ page: 1, limit: 20, sort: 'name', q: '' })
    expect(table.rows.value).toEqual([{ id: 1 }, { id: 2 }])
    expect(table.loaded.value).toBe(true)
    expect(table.hasCriteria.value).toBe(false)
  })

  it('membaca status dari URL (bisa di-bookmark)', async () => {
    await setup('/tabel?q=epson&page=3&limit=50&sort=-name&is_active=true')
    expect(lastParams()).toMatchObject({
      q: 'epson',
      page: 3,
      limit: 50,
      sort: '-name',
      is_active: 'true',
    })
    expect(table.sortColumn.value).toBe('name')
    expect(table.sortDirection.value).toBe('descending')
    expect(table.hasCriteria.value).toBe(true)
    expect(table.search.value).toBe('epson')
  })

  it('nilai URL yang rusak jatuh ke bawaan (limit dibatasi 100)', async () => {
    await setup('/tabel?page=abc&limit=9999')
    expect(lastParams()).toMatchObject({ page: 1, limit: 100 })
  })
})

describe('menulis ke URL', () => {
  it('nilai bawaan tidak ikut ke URL', async () => {
    await setup('/tabel?page=2&limit=50&sort=-name')
    table.setPage(1)
    await flushPromises()
    table.setLimit(20)
    await flushPromises()
    table.toggleSort('name') // -name -> name (bawaan)
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({})
  })

  it('mengganti filter mengembalikan ke halaman 1', async () => {
    await setup('/tabel?page=4')
    table.setFilter('item_type', 'ASSET')
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ item_type: 'ASSET' })
    expect(lastParams()).toMatchObject({ page: 1, item_type: 'ASSET' })
    expect(table.activeFilters.value).toEqual([['item_type', 'ASSET']])
  })

  it('klik kolom: naik lalu turun, kolom lain mulai naik', async () => {
    await setup()
    table.toggleSort('sku')
    await flushPromises()
    expect(lastParams().sort).toBe('sku')
    table.toggleSort('sku')
    await flushPromises()
    expect(lastParams().sort).toBe('-sku')
    table.toggleSort('name')
    await flushPromises()
    expect(lastParams().sort).toBe('name')
  })

  it('hapus semua filter juga mengosongkan pencarian dan halaman', async () => {
    await setup('/tabel?q=abc&page=2&is_active=false&item_type=ASSET&sort=-name')
    table.clearFilters()
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ sort: '-name' })
    expect(table.search.value).toBe('')
  })
})

describe('pencarian', () => {
  it('menunggu jeda sebelum meminta data dan tidak meminta tiap ketukan', async () => {
    await setup()
    fetch.mockClear()
    table.setSearch('e')
    table.setSearch('ep')
    table.setSearch('eps')
    expect(fetch).not.toHaveBeenCalled()
    await sleep(80)
    await flushPromises()
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(lastParams()).toMatchObject({ q: 'eps', page: 1 })
    expect(router.currentRoute.value.query.q).toBe('eps')
  })

  it('Enter menerapkan pencarian seketika', async () => {
    await setup()
    fetch.mockClear()
    table.setSearch('toner')
    table.submitSearch()
    await flushPromises()
    expect(lastParams().q).toBe('toner')
  })

  it('URL yang berubah dari luar menyinkronkan isi kotak pencarian', async () => {
    await setup('/tabel?q=awal')
    await router.push('/tabel?q=lain')
    await flushPromises()
    expect(table.search.value).toBe('lain')
    expect(lastParams().q).toBe('lain')
  })
})

describe('keandalan', () => {
  it('respons lama yang datang terlambat tidak menimpa hasil terbaru', async () => {
    await setup()
    let resolveSlow!: (p: Page<Row>) => void
    fetch.mockReset()
    fetch.mockImplementationOnce(() => new Promise<Page<Row>>((r) => (resolveSlow = r)))
    fetch.mockResolvedValueOnce(page([9]))

    table.setFilter('item_type', 'A') // permintaan lambat
    await flushPromises()
    table.setFilter('item_type', 'B') // permintaan cepat (terbaru)
    await flushPromises()
    expect(table.rows.value).toEqual([{ id: 9 }])

    resolveSlow(page([1, 2, 3]))
    await flushPromises()
    expect(table.rows.value).toEqual([{ id: 9 }])
  })

  it('galat dimuat jadi pesan, bisa dicoba lagi', async () => {
    fetch.mockRejectedValueOnce(new Error('putus'))
    await setup()
    expect(table.error.value).not.toBe('')
    expect(table.loading.value).toBe(false)

    fetch.mockResolvedValueOnce(page([5]))
    await table.reload()
    expect(table.error.value).toBe('')
    expect(table.rows.value).toEqual([{ id: 5 }])
  })

  it('halaman melewati akhir: mundur ke halaman terakhir', async () => {
    fetch.mockResolvedValueOnce({
      items: [],
      meta: { page: 5, limit: 20, total: 45, total_pages: 3 },
    })
    fetch.mockResolvedValue(page([1], { page: 3, total: 45, total_pages: 3 }))
    await setup('/tabel?page=5')
    await flushPromises()
    expect(router.currentRoute.value.query.page).toBe('3')
  })

  it('tidak memuat ulang saat pengguna sudah pindah halaman', async () => {
    await setup()
    fetch.mockClear()
    await router.push('/lain?q=x')
    await flushPromises()
    expect(fetch).not.toHaveBeenCalled()
  })
})
