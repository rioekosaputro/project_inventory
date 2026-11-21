import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { VueWrapper } from '@vue/test-utils'
import { ApiError } from '@/lib/apiError'
import { authService } from '@/services/authService'
import {
  categoryService,
  pageOf,
  productService,
  rackService,
  resetMasterMocks,
  supplierService,
  warehouseService,
} from '@/test/mockMasterService'
import { category, PERMS, product, rack, supplier, warehouse } from '@/test/fixtures'
import { makeSession } from '@/test/helpers'
import { mountApp, settle } from '@/test/mountApp'

vi.mock('@/services/authService', () => import('@/test/mockAuthService'))
vi.mock('@/services/masterService', () => import('@/test/mockMasterService'))

const auth = vi.mocked(authService)

let wrapper: VueWrapper | undefined

beforeEach(() => {
  vi.clearAllMocks()
  resetMasterMocks()
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  document.body.innerHTML = ''
})

async function open(path: string, perms: readonly string[] = PERMS.head) {
  auth.refresh.mockResolvedValue(makeSession({ permissions: [...perms] }))
  const app = await mountApp(path)
  wrapper = app.wrapper
  return app
}

const headers = (w: VueWrapper) => w.findAll('thead th').map((th) => th.text())
const bodyText = (w: VueWrapper) => w.find('tbody').text()

describe('Gudang: daftar', () => {
  it('memuat dengan urutan bawaan dan menampilkan baris, status, serta tautan ubah', async () => {
    warehouseService.list.mockResolvedValue(
      pageOf([warehouse(), warehouse({ id: 2, code: 'WH-B', name: 'Gudang B', is_active: false })]),
    )
    const { wrapper: w } = await open('/master/gudang')

    expect(warehouseService.list).toHaveBeenCalledWith(
      expect.objectContaining({ page: 1, limit: 20, sort: 'code' }),
    )
    expect(w.find('h1').text()).toBe('Gudang')
    expect(bodyText(w)).toContain('Gudang A')
    expect(bodyText(w)).toContain('Nonaktif')
    expect(w.find('tbody a[href="/master/gudang/1"]').text()).toBe('WH-A')
    expect(w.find('tbody tr:last-child').text()).toContain('Aktifkan')
  })

  it('kolom Kode terurut naik; hanya satu kolom memiliki aria-sort', async () => {
    warehouseService.list.mockResolvedValue(pageOf([warehouse()]))
    const { wrapper: w } = await open('/master/gudang')

    const sorted = w.findAll('thead th[aria-sort]')
    expect(sorted).toHaveLength(1)
    expect(sorted[0]!.text()).toContain('Kode')
    expect(sorted[0]!.attributes('aria-sort')).toBe('ascending')
    // Tombol memakai nama kolom, bukan "urutkan ..."; kolom tanpa sort tidak punya tombol.
    expect(w.findAll('thead th button').map((b) => b.text())).toEqual(['Kode', 'Nama'])
  })

  it('klik kolom yang sedang naik → turun, tersinkron ke URL dan ke permintaan', async () => {
    warehouseService.list.mockResolvedValue(pageOf([warehouse()]))
    const { wrapper: w, router } = await open('/master/gudang')

    await w.findAll('thead th button')[0]!.trigger('click')
    await settle()

    expect(router.currentRoute.value.query.sort).toBe('-code')
    expect(warehouseService.list).toHaveBeenLastCalledWith(
      expect.objectContaining({ sort: '-code' }),
    )
    expect(w.find('thead th[aria-sort]').attributes('aria-sort')).toBe('descending')
  })

  it('pertama kali kosong: judul, nilai, dan tombol buat di dalam bingkai tabel', async () => {
    const { wrapper: w } = await open('/master/gudang')

    expect(w.text()).toContain('Belum ada gudang')
    expect(w.text()).toContain('Tambahkan gudang pertama')
    const cta = w.findAll('a').filter((a) => a.text() === 'Tambah Gudang')
    expect(cta.length).toBeGreaterThanOrEqual(1)
    expect(cta[0]!.attributes('href')).toBe('/master/gudang/baru')
    // Kerangka halaman tetap ada: header tabel dan paginasi.
    expect(headers(w)).toContain('Kode')
    expect(w.text()).toContain('Sebelumnya')
    expect(w.text()).toContain('Berikutnya')
  })

  it('pengguna tanpa hak tulis: keadaan kosong tanpa tombol buat', async () => {
    const { wrapper: w } = await open('/master/gudang', PERMS.mgmt)
    expect(w.text()).toContain('Belum ada gudang')
    expect(w.findAll('a').some((a) => a.text() === 'Tambah Gudang')).toBe(false)
  })

  it('pengguna tanpa hak tulis: baris tanpa aksi dan tanpa tautan ubah', async () => {
    warehouseService.list.mockResolvedValue(pageOf([warehouse()]))
    const { wrapper: w } = await open('/master/gudang', PERMS.mgmt)
    expect(w.find('tbody a').exists()).toBe(false)
    expect(w.find('tbody').text()).not.toContain('Nonaktifkan')
  })

  it('hasil kosong karena pencarian: pesan "tidak cocok" dan tombol hapus semua filter', async () => {
    warehouseService.list.mockResolvedValue(pageOf([]))
    const { wrapper: w, router } = await open('/master/gudang?q=zzz&is_active=true')

    expect(w.text()).toContain('Tidak ada gudang yang cocok')
    expect(w.text()).not.toContain('Belum ada gudang')
    expect(w.find('ul[aria-label="Filter aktif"]').text()).toContain('Status: Aktif')

    const clear = w.findAll('button').filter((b) => b.text() === 'Hapus semua filter')
    await clear[clear.length - 1]!.trigger('click')
    await settle()

    expect(router.currentRoute.value.query).toEqual({})
    expect((w.find('#list-search').element as HTMLInputElement).value).toBe('')
  })

  it('chip filter aktif dapat dilepas satu per satu', async () => {
    warehouseService.list.mockResolvedValue(pageOf([warehouse()]))
    const { wrapper: w, router } = await open('/master/gudang?is_active=false')

    expect(w.find('.filter-trigger').text()).toContain('1')
    await w
      .find('ul[aria-label="Filter aktif"] button[aria-label^="Hapus filter"]')
      .trigger('click')
    await settle()
    expect(router.currentRoute.value.query.is_active).toBeUndefined()
  })

  it('pencarian: label terlihat, menunggu jeda, lalu meminta data dan menulis ?q=', async () => {
    warehouseService.list.mockResolvedValue(pageOf([warehouse()]))
    const { wrapper: w, router } = await open('/master/gudang')

    expect(w.find('label[for="list-search"]').text()).toBe('Cari gudang')
    warehouseService.list.mockClear()
    await w.find('#list-search').setValue('semarang')
    expect(warehouseService.list).not.toHaveBeenCalled()

    await new Promise((r) => setTimeout(r, 450))
    await settle()
    expect(warehouseService.list).toHaveBeenCalledTimes(1)
    expect(warehouseService.list).toHaveBeenCalledWith(
      expect.objectContaining({ q: 'semarang', page: 1 }),
    )
    expect(router.currentRoute.value.query.q).toBe('semarang')
  })

  it('tombol "/" memfokuskan pencarian kecuali saat mengetik di kolom isian', async () => {
    warehouseService.list.mockResolvedValue(pageOf([warehouse()]))
    const { wrapper: w } = await open('/master/gudang')
    const search = w.find('#list-search').element as HTMLInputElement

    document.body.dispatchEvent(new KeyboardEvent('keydown', { key: '/', bubbles: true }))
    expect(document.activeElement).toBe(search)

    search.blur()
    const other = document.createElement('input')
    document.body.appendChild(other)
    other.focus()
    other.dispatchEvent(new KeyboardEvent('keydown', { key: '/', bubbles: true }))
    expect(document.activeElement).toBe(other)
  })

  it('paginasi: info, halaman, dan tombol (nonaktif tetap berlabel)', async () => {
    warehouseService.list.mockResolvedValue(pageOf([warehouse()], { total: 45 }))
    const { wrapper: w, router } = await open('/master/gudang')

    const nav = w.find('nav[aria-label="Paginasi"]')
    expect(nav.text()).toContain('Menampilkan 1–20 dari 45')
    expect(nav.text()).toContain('Halaman 1 dari 3')
    const [prev, next] = nav.findAll('button')
    expect(prev!.text()).toBe('Sebelumnya')
    expect(prev!.attributes('disabled')).toBeDefined()
    expect(next!.attributes('disabled')).toBeUndefined()

    await next!.trigger('click')
    await settle()
    expect(router.currentRoute.value.query.page).toBe('2')
    expect(warehouseService.list).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2 }))
  })

  it('galat memuat: pesan, tombol coba lagi memuat ulang', async () => {
    warehouseService.list.mockRejectedValueOnce(
      new ApiError(503, 'X', 'Server sedang tidak dapat dijangkau.'),
    )
    const { wrapper: w } = await open('/master/gudang')

    expect(w.find('[role="alert"]').text()).toContain('Data gagal dimuat')
    expect(w.find('[role="alert"]').text()).toContain('Server sedang tidak dapat dijangkau.')

    warehouseService.list.mockResolvedValue(pageOf([warehouse()]))
    await w
      .findAll('button')
      .find((b) => b.text() === 'Coba lagi')!
      .trigger('click')
    await settle()
    expect(w.find('[role="alert"]').exists()).toBe(false)
    expect(bodyText(w)).toContain('Gudang A')
  })

  it('wilayah gulir tabel: role region berlabel dan dapat difokus keyboard', async () => {
    warehouseService.list.mockResolvedValue(pageOf([warehouse()]))
    const { wrapper: w } = await open('/master/gudang')
    const region = w.find('[role="region"]')
    expect(region.attributes('tabindex')).toBe('0')
    const label = document.getElementById(region.attributes('aria-labelledby')!)
    expect(label?.textContent?.trim()).toBe('Daftar gudang')
  })
})

describe('menonaktifkan / mengaktifkan', () => {
  it('menonaktifkan meminta konfirmasi yang menyebut entitas; Batal tidak mengubah apa pun', async () => {
    warehouseService.list.mockResolvedValue(pageOf([warehouse()]))
    const { wrapper: w } = await open('/master/gudang')

    await w.find('tbody button').trigger('click')
    await settle()
    const dialog = w.find('dialog.confirm-dialog')
    expect(dialog.attributes('open')).toBeDefined()
    expect(dialog.find('h2').text()).toBe("Nonaktifkan gudang 'WH-A'?")

    await dialog
      .findAll('button')
      .find((b) => b.text() === 'Batal')!
      .trigger('click')
    await settle()
    expect(dialog.attributes('open')).toBeUndefined()
    expect(warehouseService.update).not.toHaveBeenCalled()
  })

  it('konfirmasi memanggil API, memuat ulang daftar, dan menampilkan toast', async () => {
    warehouseService.list.mockResolvedValue(pageOf([warehouse()]))
    warehouseService.update.mockResolvedValue(warehouse({ is_active: false }))
    const { wrapper: w } = await open('/master/gudang')

    await w.find('tbody button').trigger('click')
    await settle()
    warehouseService.list.mockClear()
    await w
      .findAll('dialog.confirm-dialog button')
      .find((b) => b.text() === 'Nonaktifkan Gudang')!
      .trigger('click')
    await settle()

    expect(warehouseService.update).toHaveBeenCalledWith(1, { is_active: false })
    expect(warehouseService.list).toHaveBeenCalledTimes(1)
    expect(document.body.textContent).toContain('Gudang WH-A dinonaktifkan')
  })

  it('penolakan server (masih menyimpan stok) ditampilkan apa adanya', async () => {
    warehouseService.list.mockResolvedValue(pageOf([warehouse()]))
    warehouseService.update.mockRejectedValue(
      new ApiError(
        409,
        'LOCATION_HAS_STOCK',
        'Lokasi masih menyimpan stok; kosongkan terlebih dahulu',
      ),
    )
    const { wrapper: w } = await open('/master/gudang')

    await w.find('tbody button').trigger('click')
    await settle()
    await w
      .findAll('dialog.confirm-dialog button')
      .find((b) => b.text() === 'Nonaktifkan Gudang')!
      .trigger('click')
    await settle()

    expect(document.body.textContent).toContain('Perubahan gagal')
    expect(document.body.textContent).toContain('Lokasi masih menyimpan stok')
  })

  it('mengaktifkan kembali langsung, tanpa dialog', async () => {
    warehouseService.list.mockResolvedValue(pageOf([warehouse({ is_active: false })]))
    warehouseService.update.mockResolvedValue(warehouse())
    const { wrapper: w } = await open('/master/gudang')

    await w.find('tbody button').trigger('click')
    await settle()

    expect(warehouseService.update).toHaveBeenCalledWith(1, { is_active: true })
    expect(w.find('dialog.confirm-dialog').attributes('open')).toBeUndefined()
  })
})

describe('daftar lain', () => {
  it('Rak: staf gudang boleh menambah; kolom gudang/zona/dapat dijual; chip karantina', async () => {
    rackService.list.mockResolvedValue(
      pageOf([
        rack(),
        rack({
          id: 2,
          code: 'Q-01',
          zone: 'KARANTINA',
          is_sellable: false,
          locked_by_opname_id: 9,
        }),
      ]),
    )
    warehouseService.list.mockResolvedValue(pageOf([warehouse()]))
    const { wrapper: w } = await open('/master/rak', PERMS.staff)

    expect(rackService.list).toHaveBeenCalledWith(
      expect.objectContaining({ sort: 'warehouse,code' }),
    )
    expect(w.findAll('a').some((a) => a.text() === 'Tambah Rak')).toBe(true)
    expect(headers(w)).toEqual(
      expect.arrayContaining(['Gudang', 'Kode rak', 'Zona', 'Dapat dijual', 'Status']),
    )
    expect(w.find('thead th[aria-sort]').text()).toContain('Gudang')
    const rows = w.findAll('tbody tr')
    expect(rows[1]!.text()).toContain('Tidak')
    expect(rows[1]!.text()).toContain('Dikunci opname')
  })

  it('Rak: filter gudang memakai pilihan dari daftar gudang', async () => {
    rackService.list.mockResolvedValue(pageOf([rack()]))
    warehouseService.list.mockResolvedValue(
      pageOf([warehouse(), warehouse({ id: 2, code: 'WH-B', name: 'B' })]),
    )
    const { wrapper: w } = await open('/master/rak')
    await settle()

    const options = w.findAll('#filter-warehouse_id option').map((o) => o.text())
    expect(options).toEqual(['Semua', 'WH-A — Gudang A', 'WH-B — B'])
  })

  it('Rak: staf gudang tidak melihat tombol buat di Gudang (hanya baca)', async () => {
    warehouseService.list.mockResolvedValue(pageOf([warehouse()]))
    const { wrapper: w } = await open('/master/gudang', PERMS.staff)
    expect(w.findAll('a').some((a) => a.text() === 'Tambah Gudang')).toBe(false)
  })

  it('Produk: menampilkan jenis, serial number, tarif sebagai Rupiah; teknisi hanya membaca', async () => {
    productService.list.mockResolvedValue(
      pageOf([
        product(),
        product({
          id: 2,
          sku: 'TN-1',
          item_type: 'CONSUMABLE',
          has_serial_number: false,
          default_monthly_rate: null,
        }),
      ]),
    )
    const { wrapper: w } = await open('/master/produk', PERMS.tech)

    const text = bodyText(w)
    expect(text).toContain('Aset')
    expect(text).toContain('Habis pakai')
    expect(text.replace(/\s/g, ' ')).toMatch(/Rp\s?550\.000,00/)
    expect(w.findAll('nav[aria-label="Data master"] a').map((a) => a.text())).toEqual(['Produk'])
    expect(w.find('tbody a').exists()).toBe(false)
    expect(text).not.toContain('Nonaktifkan')
    expect(categoryService.list).not.toHaveBeenCalled() // teknisi tidak punya master.read
  })

  it('Produk: tab master menampilkan lima daftar untuk kepala gudang', async () => {
    productService.list.mockResolvedValue(pageOf([product()]))
    categoryService.list.mockResolvedValue(pageOf([category()]))
    const { wrapper: w } = await open('/master/produk')
    await settle()

    const tabs = w.findAll('nav[aria-label="Data master"] a')
    expect(tabs.map((a) => a.text())).toEqual(['Produk', 'Kategori', 'Gudang', 'Rak', 'Supplier'])
    expect(w.find('nav[aria-label="Data master"] a[aria-current="page"]').text()).toBe('Produk')
    expect(w.findAll('#filter-category_id option').map((o) => o.text())).toEqual([
      'Semua',
      'Printer',
    ])
  })

  it('Supplier: kolom kontak dan status', async () => {
    supplierService.list.mockResolvedValue(
      pageOf([supplier(), supplier({ id: 2, code: 'SUP-2', email: null, pic_name: null })]),
    )
    const { wrapper: w } = await open('/master/supplier')
    expect(bodyText(w)).toContain('sales@maju.id')
    expect(w.findAll('tbody tr')[1]!.text()).toContain('—')
  })

  it('Kategori: tombol buat membuka panel samping (bukan halaman baru)', async () => {
    categoryService.list.mockResolvedValue(pageOf([category()]))
    const { wrapper: w, router } = await open('/master/kategori')

    expect(w.find('dialog.slide-over').attributes('open')).toBeUndefined()
    await w
      .findAll('button')
      .find((b) => b.text().includes('Tambah Kategori'))!
      .trigger('click')
    await settle()

    expect(router.currentRoute.value.path).toBe('/master/kategori')
    expect(w.find('dialog.slide-over').attributes('open')).toBeDefined()
    expect(w.find('dialog.slide-over h2').text()).toBe('Tambah kategori')
  })
})

describe('sidebar', () => {
  it('Data Master tampil dan aktif di semua halaman master; teknisi juga melihatnya', async () => {
    productService.list.mockResolvedValue(pageOf([product()]))
    const { wrapper: w } = await open('/master/produk', PERMS.tech)
    const items = w.findAll('aside nav a').filter((a) => a.text().includes('Data Master'))
    expect(items).toHaveLength(1)
    expect(items[0]!.attributes('aria-current')).toBe('page')
  })
})
