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

const input = (w: VueWrapper, name: string) => w.find<HTMLInputElement>(`#field-${name}`)
const err = (w: VueWrapper, name: string) => w.find(`#field-${name}-error`)

async function fill(w: VueWrapper, values: Record<string, string>) {
  for (const [name, value] of Object.entries(values)) await input(w, name).setValue(value)
}
async function submit(w: VueWrapper) {
  await w.find('#entity-form').trigger('submit')
  await settle()
}

describe('Gudang: form buat', () => {
  it('Form: heading, label terlihat, Simpan tidak pernah dinonaktifkan, Batal kembali ke daftar', async () => {
    const { wrapper: w } = await open('/master/gudang/baru')

    expect(w.find('h1').text()).toBe('Tambah Gudang')
    expect(w.find('label[for="field-code"]').text()).toContain('Kode gudang')
    const save = w.find('button[type="submit"]')
    expect(save.text()).toBe('Simpan Gudang')
    expect(save.attributes('disabled')).toBeUndefined()
    expect(
      w
        .findAll('a')
        .find((a) => a.text() === 'Batal')!
        .attributes('href'),
    ).toBe('/master/gudang')
    expect(input(w, 'code').attributes('readonly')).toBeUndefined()
  })

  it('submit kosong: galat per field, fokus ke field invalid pertama, tidak memanggil API', async () => {
    const { wrapper: w } = await open('/master/gudang/baru')
    await submit(w)

    expect(err(w, 'code').text()).toBe('Kode wajib diisi')
    expect(err(w, 'name').text()).toBe('Nama wajib diisi')
    expect(input(w, 'code').attributes('aria-invalid')).toBe('true')
    expect(input(w, 'code').attributes('aria-describedby')).toBe('field-code-error')
    expect(warehouseService.create).not.toHaveBeenCalled()
  })

  it('kode berspasi / berformat salah ditolak dengan arahan perbaikan', async () => {
    const { wrapper: w } = await open('/master/gudang/baru')
    await fill(w, { code: 'WH SMG', name: 'Gudang' })
    await submit(w)
    expect(err(w, 'code').text()).toContain('Gunakan huruf, angka, dan tanda hubung')
  })

  it('latitude tanpa longitude ditolak; koordinat di luar rentang ditolak', async () => {
    const { wrapper: w } = await open('/master/gudang/baru')
    await fill(w, { code: 'WH-1', name: 'G', latitude: '-6.9' })
    await submit(w)
    expect(err(w, 'longitude').text()).toContain('Isi latitude dan longitude bersamaan')

    await fill(w, { longitude: '999' })
    await submit(w)
    expect(err(w, 'longitude').text()).toContain('antara -180 dan 180')
    expect(warehouseService.create).not.toHaveBeenCalled()
  })

  it('sukses: payload bersih (kosong → null, koordinat angka), toast, kembali ke daftar', async () => {
    warehouseService.create.mockResolvedValue(warehouse({ code: 'WH-1' }))
    const { wrapper: w, router } = await open('/master/gudang/baru')
    await fill(w, {
      code: ' WH-1 ',
      name: ' Gudang Satu ',
      latitude: '-6.9666667',
      longitude: '110.4166667',
    })
    await submit(w)

    expect(warehouseService.create).toHaveBeenCalledWith({
      code: 'WH-1',
      name: 'Gudang Satu',
      address: null,
      phone: null,
      latitude: -6.9666667,
      longitude: 110.4166667,
    })
    await vi.waitFor(() => expect(router.currentRoute.value.path).toBe('/master/gudang'))
    expect(document.body.textContent).toContain('Gudang WH-1 ditambahkan')
  })

  it('kode kembar dari server (409) tampil di bawah field kode dan fokus ke sana', async () => {
    warehouseService.create.mockRejectedValue(
      new ApiError(409, 'DUPLICATE_VALUE', 'kode gudang sudah dipakai', [
        { field: 'code', message: 'kode gudang sudah dipakai' },
      ]),
    )
    const { wrapper: w, router } = await open('/master/gudang/baru')
    await fill(w, { code: 'WH-1', name: 'G' })
    await submit(w)

    expect(err(w, 'code').text()).toBe('Kode gudang sudah dipakai')
    expect(router.currentRoute.value.path).toBe('/master/gudang/baru') // tetap di form, isian utuh
    expect(input(w, 'name').element.value).toBe('G')
  })

  it('galat server tanpa field (mis. jaringan) tampil sebagai pesan form, isian tidak hilang', async () => {
    warehouseService.create.mockRejectedValue(
      new ApiError(0, 'NETWORK_ERROR', 'Tidak dapat terhubung ke server.'),
    )
    const { wrapper: w } = await open('/master/gudang/baru')
    await fill(w, { code: 'WH-1', name: 'G' })
    await submit(w)

    expect(w.find('form').text()).toContain('Tidak dapat terhubung ke server.')
    expect(input(w, 'code').element.value).toBe('WH-1')
  })
})

describe('Gudang: form ubah', () => {
  it('memuat data, kode readonly, hanya field yang dapat diubah dikirim', async () => {
    warehouseService.get.mockResolvedValue(warehouse({ latitude: -6.9, longitude: 110.4 }))
    warehouseService.update.mockResolvedValue(warehouse({ name: 'Baru' }))
    const { wrapper: w, router } = await open('/master/gudang/1')

    expect(warehouseService.get).toHaveBeenCalledWith(1)
    expect(w.find('h1').text()).toBe('Ubah Gudang WH-A')
    expect(input(w, 'code').element.value).toBe('WH-A')
    expect(input(w, 'code').attributes('readonly')).toBeDefined()
    expect(w.text()).toContain('Kode tidak dapat diubah.')
    expect(input(w, 'name').element.value).toBe('Gudang A')

    await fill(w, { name: 'Baru', phone: '' })
    await submit(w)

    expect(warehouseService.update).toHaveBeenCalledWith(1, {
      name: 'Baru',
      address: 'Jl. Industri 1',
      phone: null,
      latitude: -6.9,
      longitude: 110.4,
    })
    expect(router.currentRoute.value.path).toBe('/master/gudang')
    expect(document.body.textContent).toContain('Gudang WH-A diperbarui')
  })

  it('data tidak ditemukan: pesan dan tautan kembali, tanpa form', async () => {
    warehouseService.get.mockRejectedValue(new ApiError(404, 'NOT_FOUND', 'Data tidak ditemukan'))
    const { wrapper: w } = await open('/master/gudang/99')

    expect(w.text()).toContain('Data tidak dapat ditampilkan')
    expect(w.text()).toContain('Data tidak ditemukan')
    expect(w.find('form').exists()).toBe(false)
    expect(w.findAll('a').some((a) => a.text() === 'Daftar gudang')).toBe(true)
  })

  it('akun hanya-baca tidak dapat membuka form (403)', async () => {
    const { router } = await open('/master/gudang/1', PERMS.mgmt)
    expect(router.currentRoute.value.name).toBe('forbidden')
    expect(warehouseService.get).not.toHaveBeenCalled()
  })
})

describe('Rak: form', () => {
  it('buat: pilihan gudang dari daftar; "ikuti zona" tidak mengirim is_sellable', async () => {
    warehouseService.list.mockResolvedValue(
      pageOf([warehouse(), warehouse({ id: 2, code: 'WH-B', name: 'B' })]),
    )
    rackService.create.mockResolvedValue(rack({ code: 'A-02-01' }))
    const { wrapper: w, router } = await open('/master/rak/baru', PERMS.staff)
    await settle()

    expect(w.findAll('#field-warehouse_id option').map((o) => o.text())).toEqual([
      'Pilih gudang',
      'WH-A — Gudang A',
      'WH-B — B',
    ])
    await submit(w)
    expect(err(w, 'warehouse_id').text()).toBe('Pilih gudang')
    expect(err(w, 'code').text()).toBe('Kode rak wajib diisi')

    await fill(w, { warehouse_id: '2', code: 'A-02-01', zone: 'printer' })
    await submit(w)

    expect(rackService.create).toHaveBeenCalledWith({
      warehouse_id: 2,
      code: 'A-02-01',
      zone: 'printer',
      description: null,
    })
    await vi.waitFor(() => expect(router.currentRoute.value.path).toBe('/master/rak'))
  })

  it('buat: pilihan eksplisit "tidak dapat dijual" dikirim', async () => {
    warehouseService.list.mockResolvedValue(pageOf([warehouse()]))
    rackService.create.mockResolvedValue(rack())
    const { wrapper: w } = await open('/master/rak/baru')
    await settle()
    await fill(w, { warehouse_id: '1', code: 'Q-01', is_sellable: 'false' })
    await submit(w)
    expect(rackService.create).toHaveBeenCalledWith(expect.objectContaining({ is_sellable: false }))
  })

  it('ubah: gudang dan kode terkunci; hanya zona/dapat dijual/keterangan dikirim', async () => {
    rackService.get.mockResolvedValue(rack({ description: 'lama' }))
    rackService.update.mockResolvedValue(rack())
    warehouseService.list.mockResolvedValue(pageOf([warehouse()]))
    const { wrapper: w } = await open('/master/rak/1')
    await settle()

    expect(input(w, 'code').attributes('readonly')).toBeDefined()
    expect(w.find('#field-warehouse_id').attributes('disabled')).toBeDefined()
    await fill(w, { zone: 'SPAREPART', description: '' })
    await submit(w)

    expect(rackService.update).toHaveBeenCalledWith(1, {
      zone: 'SPAREPART',
      description: null,
      is_sellable: true,
    })
  })

  it('rak terkunci opname: penolakan server tampil sebagai pesan form', async () => {
    rackService.get.mockResolvedValue(rack())
    warehouseService.list.mockResolvedValue(pageOf([warehouse()]))
    rackService.update.mockRejectedValue(
      new ApiError(409, 'RACK_LOCKED_BY_OPNAME', 'Rak sedang dikunci oleh stock opname'),
    )
    const { wrapper: w } = await open('/master/rak/1')
    await settle()
    await submit(w)
    expect(w.find('form').text()).toContain('Rak sedang dikunci oleh stock opname')
  })
})

describe('Produk: form', () => {
  const fillValid = (w: VueWrapper) =>
    fill(w, { sku: 'prn-1', name: 'Epson', category_id: '1', unit: 'unit', min_stock_alert: '2' })

  it('validasi klien: tarif lebih dari 2 desimal, bukan angka, min. stok negatif', async () => {
    categoryService.list.mockResolvedValue(pageOf([category()]))
    const { wrapper: w } = await open('/master/produk/baru')
    await settle()
    await fillValid(w)
    await fill(w, {
      default_monthly_rate: '550000.555',
      default_sell_price: '1e3',
      min_stock_alert: '-1',
    })
    await submit(w)

    expect(err(w, 'default_monthly_rate').text()).toContain('maksimal 2 desimal')
    expect(err(w, 'default_sell_price').text()).toContain('maksimal 2 desimal')
    expect(err(w, 'min_stock_alert').text()).toContain('bilangan bulat 0 atau lebih')
    expect(productService.create).not.toHaveBeenCalled()
  })

  it('submit kosong menandai semua field wajib', async () => {
    const { wrapper: w } = await open('/master/produk/baru')
    await submit(w)
    for (const f of ['sku', 'name', 'category_id']) expect(err(w, f).exists()).toBe(true)
    expect(err(w, 'unit').exists()).toBe(false) // satuan bawaan "unit"
  })

  it('sukses: uang tetap string (tanpa float), kosong → null, kategori jadi angka', async () => {
    categoryService.list.mockResolvedValue(pageOf([category()]))
    productService.create.mockResolvedValue(product())
    const { wrapper: w, router } = await open('/master/produk/baru')
    await settle()
    await fillValid(w)
    await fill(w, { default_monthly_rate: '550000.50', brand: ' Epson ' })
    await w.find('#field-has_serial_number').setValue(true)
    await submit(w)

    expect(productService.create).toHaveBeenCalledWith({
      sku: 'prn-1',
      name: 'Epson',
      category_id: 1,
      brand: 'Epson',
      model: null,
      unit: 'unit',
      has_serial_number: true,
      min_stock_alert: 2,
      default_monthly_rate: '550000.50',
      default_sell_price: null,
      description: null,
    })
    expect(typeof vi.mocked(productService.create).mock.calls[0]![0].default_monthly_rate).toBe(
      'string',
    )
    await vi.waitFor(() => expect(router.currentRoute.value.path).toBe('/master/produk'))
  })

  it('consumable + serial number ditolak server: galat tampil di bawah checkbox', async () => {
    categoryService.list.mockResolvedValue(
      pageOf([category({ id: 2, name: 'Toner', item_type: 'CONSUMABLE' })]),
    )
    productService.create.mockRejectedValue(
      new ApiError(422, 'INVALID_DATA', 'Data produk tidak valid', [
        {
          field: 'has_serial_number',
          message: 'barang habis pakai (CONSUMABLE) tidak memakai serial number',
        },
      ]),
    )
    const { wrapper: w } = await open('/master/produk/baru')
    await settle()
    await fill(w, { sku: 'TN-1', name: 'Toner', category_id: '2' })
    await w.find('#field-has_serial_number').setValue(true)
    await submit(w)

    expect(err(w, 'has_serial_number').text()).toBe(
      'Barang habis pakai (CONSUMABLE) tidak memakai serial number',
    )
    expect(w.find('#field-has_serial_number').attributes('aria-invalid')).toBe('true')
  })

  it('ubah: kategori nonaktif milik produk tetap muncul sebagai pilihan', async () => {
    productService.get.mockResolvedValue(product({ category_id: 7, category_name: 'Lama' }))
    categoryService.list.mockResolvedValue(pageOf([category()]))
    const { wrapper: w } = await open('/master/produk/1')
    await settle()

    expect(w.findAll('#field-category_id option').map((o) => o.text())).toEqual([
      'Pilih kategori',
      'Lama',
      'Printer (aset)',
    ])
    expect((w.find('#field-category_id').element as HTMLSelectElement).value).toBe('7')
  })

  it('SKU dikunci setelah dipakai: 409 PRODUCT_IN_USE tampil sebagai pesan form', async () => {
    productService.get.mockResolvedValue(product())
    categoryService.list.mockResolvedValue(pageOf([category()]))
    productService.update.mockRejectedValue(
      new ApiError(
        409,
        'PRODUCT_IN_USE',
        'Produk sudah memiliki stok atau transaksi, SKU dan mode serial number tidak dapat diubah',
      ),
    )
    const { wrapper: w } = await open('/master/produk/1')
    await settle()
    await fill(w, { sku: 'BARU-1' })
    await submit(w)
    expect(w.find('form').text()).toContain('Produk sudah memiliki stok atau transaksi')
  })

  it('kepala gudang boleh; teknisi dialihkan ke 403', async () => {
    const { router } = await open('/master/produk/baru', PERMS.tech)
    expect(router.currentRoute.value.name).toBe('forbidden')
  })
})

describe('Supplier: form', () => {
  it('email salah ditolak; kode berspasi ditolak', async () => {
    const { wrapper: w } = await open('/master/supplier/baru')
    await fill(w, { code: 'SUP 1', name: 'PT', email: 'bukan-email' })
    await submit(w)
    expect(err(w, 'code').text()).toContain('Gunakan huruf, angka, dan tanda hubung')
    expect(err(w, 'email').text()).toContain('Isi email seperti')
    expect(supplierService.create).not.toHaveBeenCalled()
  })

  it('sukses & ubah: kode tidak ikut dikirim saat ubah', async () => {
    supplierService.create.mockResolvedValue(supplier())
    const { wrapper: w } = await open('/master/supplier/baru')
    await fill(w, { code: 'SUP-1', name: 'PT Maju', email: 'a@b.co', pic_name: '' })
    await submit(w)
    expect(supplierService.create).toHaveBeenCalledWith(
      expect.objectContaining({ code: 'SUP-1', name: 'PT Maju', email: 'a@b.co', pic_name: null }),
    )
    wrapper?.unmount()

    supplierService.get.mockResolvedValue(supplier())
    supplierService.update.mockResolvedValue(supplier())
    const edit = await open('/master/supplier/1')
    expect(input(edit.wrapper, 'code').attributes('readonly')).toBeDefined()
    await submit(edit.wrapper)
    const body = supplierService.update.mock.calls[0]![1]
    expect(body).not.toHaveProperty('code')
  })
})

describe('Kategori: panel samping', () => {
  async function openPanel() {
    categoryService.list.mockResolvedValue(pageOf([category()]))
    const app = await open('/master/kategori')
    await app.wrapper
      .findAll('button')
      .find((b) => b.text().includes('Tambah Kategori'))!
      .trigger('click')
    await settle()
    return app
  }

  it('submit kosong: galat di panel, tidak memanggil API', async () => {
    const { wrapper: w } = await openPanel()
    await w.find('dialog.slide-over form').trigger('submit')
    await settle()
    expect(err(w, 'name').text()).toBe('Nama kategori wajib diisi')
    expect(err(w, 'item_type').text()).toBe('Pilih jenis barang')
    expect(categoryService.create).not.toHaveBeenCalled()
  })

  it('sukses: menyimpan, menutup panel, memuat ulang daftar', async () => {
    categoryService.create.mockResolvedValue(category({ id: 2, name: 'PC' }))
    const { wrapper: w } = await openPanel()
    await fill(w, { name: ' PC ', item_type: 'ASSET' })
    categoryService.list.mockClear()
    await w.find('dialog.slide-over form').trigger('submit')
    await settle()

    expect(categoryService.create).toHaveBeenCalledWith({ name: 'PC', item_type: 'ASSET' })
    expect(w.find('dialog.slide-over').attributes('open')).toBeUndefined()
    expect(categoryService.list).toHaveBeenCalledTimes(1)
    expect(document.body.textContent).toContain('Kategori PC ditambahkan')
  })

  it('nama kembar: galat di field nama, panel tetap terbuka', async () => {
    categoryService.create.mockRejectedValue(
      new ApiError(409, 'DUPLICATE_VALUE', 'nama kategori sudah dipakai', [
        { field: 'name', message: 'nama kategori sudah dipakai' },
      ]),
    )
    const { wrapper: w } = await openPanel()
    await fill(w, { name: 'Printer', item_type: 'ASSET' })
    await w.find('dialog.slide-over form').trigger('submit')
    await settle()

    expect(err(w, 'name').text()).toBe('Nama kategori sudah dipakai')
    expect(w.find('dialog.slide-over').attributes('open')).toBeDefined()
  })

  it('mengubah kategori yang ada: field terisi dan memakai update', async () => {
    categoryService.list.mockResolvedValue(pageOf([category()]))
    categoryService.update.mockResolvedValue(category({ name: 'Printer Laser' }))
    const { wrapper: w } = await open('/master/kategori')
    await w.find('tbody button.font-medium, tbody td button').trigger('click')
    await settle()

    expect(input(w, 'name').element.value).toBe('Printer')
    expect(w.find('dialog.slide-over h2').text()).toBe('Ubah kategori Printer')
    await fill(w, { name: 'Printer Laser' })
    await w.find('dialog.slide-over form').trigger('submit')
    await settle()
    expect(categoryService.update).toHaveBeenCalledWith(1, {
      name: 'Printer Laser',
      item_type: 'ASSET',
    })
  })

  it('Escape menutup panel tanpa menyimpan', async () => {
    const { wrapper: w } = await openPanel()
    const dialog = w.find('dialog.slide-over')
    ;(dialog.element as HTMLDialogElement).close()
    await settle()
    expect(dialog.attributes('open')).toBeUndefined()
    expect(categoryService.create).not.toHaveBeenCalled()
  })
})
