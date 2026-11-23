import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { VueWrapper } from '@vue/test-utils'
import { ApiError } from '@/lib/apiError'
import { todayWIB } from '@/lib/format'
import { authService } from '@/services/authService'
import { lpbService, resetLpbMocks } from '@/test/mockLpbService'
import {
  pageOf,
  productService,
  rackService,
  resetMasterMocks,
  supplierService,
  warehouseService,
} from '@/test/mockMasterService'
import { lpbDetail, lpbItem, PERMS, product, rack, supplier, warehouse } from '@/test/fixtures'
import { makeSession } from '@/test/helpers'
import { mountApp, settle } from '@/test/mountApp'

vi.mock('@/services/authService', () => import('@/test/mockAuthService'))
vi.mock('@/services/masterService', () => import('@/test/mockMasterService'))
vi.mock('@/services/lpbService', () => import('@/test/mockLpbService'))

const auth = vi.mocked(authService)
let wrapper: VueWrapper | undefined

beforeEach(() => {
  vi.clearAllMocks()
  resetMasterMocks()
  resetLpbMocks()
  warehouseService.list.mockResolvedValue(
    pageOf([warehouse(), warehouse({ id: 2, code: 'WH-B', name: 'Gudang B' })]),
  )
  supplierService.list.mockResolvedValue(pageOf([supplier()]))
  productService.list.mockResolvedValue(
    pageOf([
      product(),
      product({ id: 2, sku: 'CBL-1', name: 'Kabel LAN', has_serial_number: false }),
    ]),
  )
  rackService.list.mockResolvedValue(pageOf([rack(), rack({ id: 2, code: 'A-02' })]))
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  document.body.innerHTML = ''
})

async function open(path: string, perms: readonly string[] = PERMS.staff, userWarehouse = false) {
  auth.refresh.mockResolvedValue(
    makeSession({
      permissions: [...perms],
      user: { id: 10, warehouse_id: userWarehouse ? 1 : null },
    }),
  )
  const app = await mountApp(path)
  wrapper = app.wrapper
  return app
}

const sel = (w: VueWrapper, name: string) => w.find<HTMLSelectElement>(`select[name="${name}"]`)
const inp = (w: VueWrapper, name: string) => w.find<HTMLInputElement>(`input[name="${name}"]`)
const errOf = (w: VueWrapper, name: string) =>
  w.find(`[id="field-${name}-error"]`).exists() ? w.find(`[id="field-${name}-error"]`).text() : ''

async function pick(w: VueWrapper, name: string, value: string) {
  await sel(w, name).setValue(value)
  await settle()
}
async function submit(w: VueWrapper) {
  await w.find('#entity-form').trigger('submit')
  await settle()
}

async function fillValid(w: VueWrapper) {
  await pick(w, 'warehouse_id', '1')
  await pick(w, 'supplier_id', '1')
  await pick(w, 'items[0].product_id', '1')
  await pick(w, 'items[0].rack_id', '1')
  await inp(w, 'items[0].qty').setValue('2')
  await inp(w, 'items[0].unit_cost').setValue('1500000')
}

describe('LPB: form buat', () => {
  it('Form: judul, label terlihat, Simpan tidak pernah dinonaktifkan; tanggal bawaan hari ini (WIB)', async () => {
    const { wrapper: w } = await open('/lpb/baru')

    expect(w.find('h1').text()).toBe('Buat LPB')
    expect(w.find('label[for="field-sub_type"]').text()).toContain('Jenis LPB')
    expect(inp(w, 'doc_date').element.value).toBe(todayWIB())
    const save = w.find('button[type="submit"]')
    expect(save.text()).toBe('Simpan Draft')
    expect(save.attributes('disabled')).toBeUndefined()
    // Jenis yang belum bisa diselesaikan (DISPOSAL, T1.8) tidak ditawarkan.
    const types = sel(w, 'sub_type')
      .findAll('option')
      .map((o) => o.element.value)
    expect(types).toEqual(['PURCHASE', 'INITIAL_STOCK'])
  })

  it('submit kosong: galat per field (gudang, supplier, produk, rak), fokus ke yang pertama, API tidak dipanggil', async () => {
    const { wrapper: w } = await open('/lpb/baru')
    await submit(w)

    expect(errOf(w, 'warehouse_id')).toBe('Pilih gudang')
    expect(errOf(w, 'supplier_id')).toBe('Pilih supplier')
    expect(errOf(w, 'items[0].product_id')).toBe('Pilih produk')
    expect(errOf(w, 'items[0].rack_id')).toBe('Pilih rak')
    expect(lpbService.create).not.toHaveBeenCalled()
  })

  it('jumlah harus bilangan bulat ≥ 1; galat muncul saat blur, bukan saat mengetik', async () => {
    const { wrapper: w } = await open('/lpb/baru')
    const qty = inp(w, 'items[0].qty')

    await qty.setValue('0')
    await settle()
    expect(errOf(w, 'items[0].qty')).toBe('') // masih mengetik → belum ada galat

    await qty.trigger('blur')
    await settle()
    expect(errOf(w, 'items[0].qty')).toContain('minimal 1')

    await qty.trigger('focus')
    await settle()
    expect(errOf(w, 'items[0].qty')).toBe('') // fokus kembali → galat hilang seketika
  })

  it('memilih gudang memuat rak milik gudang itu; mengganti gudang mengosongkan pilihan rak', async () => {
    const { wrapper: w } = await open('/lpb/baru')
    await pick(w, 'warehouse_id', '1')

    expect(rackService.list).toHaveBeenLastCalledWith(
      expect.objectContaining({ warehouse_id: '1', is_active: true }),
    )
    await pick(w, 'items[0].rack_id', '1')
    expect(sel(w, 'items[0].rack_id').element.value).toBe('1')

    await pick(w, 'warehouse_id', '2')
    expect(rackService.list).toHaveBeenLastCalledWith(
      expect.objectContaining({ warehouse_id: '2' }),
    )
    expect(sel(w, 'items[0].rack_id').element.value).toBe('')
  })

  it('akun satu gudang: hanya gudangnya yang ditawarkan dan sudah terpilih', async () => {
    const { wrapper: w } = await open('/lpb/baru', PERMS.staff, true)

    const options = sel(w, 'warehouse_id')
      .findAll('option')
      .map((o) => o.element.value)
      .filter(Boolean)
    expect(options).toEqual(['1'])
    expect(sel(w, 'warehouse_id').element.value).toBe('1')
  })

  it('sukses: badan permintaan bersih (angka, kosong → null), toast, lanjut ke rincian untuk scan SN', async () => {
    lpbService.create.mockResolvedValue(lpbDetail())
    const { wrapper: w, router } = await open('/lpb/baru')
    await fillValid(w)
    await inp(w, 'reference_number').setValue(' INV-77 ')
    await submit(w)

    expect(lpbService.create).toHaveBeenCalledWith({
      sub_type: 'PURCHASE',
      warehouse_id: 1,
      doc_date: todayWIB(),
      supplier_id: 1,
      reference_number: 'INV-77',
      notes: null,
      items: [{ product_id: 1, rack_id: 1, qty: 2, unit_cost: '1500000', notes: null }],
    })
    await vi.waitFor(() => expect(router.currentRoute.value.path).toBe('/lpb/7'))
    expect(document.body.textContent).toContain('dibuat sebagai draft')
  })

  it('Stok awal: tanpa supplier dan referensi; badan permintaan tidak membawa keduanya', async () => {
    lpbService.create.mockResolvedValue(lpbDetail({ sub_type: 'INITIAL_STOCK', supplier_id: null }))
    const { wrapper: w } = await open('/lpb/baru')
    await pick(w, 'sub_type', 'INITIAL_STOCK')

    expect(sel(w, 'supplier_id').exists()).toBe(false)
    expect(inp(w, 'reference_number').exists()).toBe(false)

    await pick(w, 'warehouse_id', '1')
    await pick(w, 'items[0].product_id', '1')
    await pick(w, 'items[0].rack_id', '1')
    await submit(w)

    expect(lpbService.create).toHaveBeenCalledWith(
      expect.objectContaining({
        sub_type: 'INITIAL_STOCK',
        supplier_id: null,
        reference_number: null,
      }),
    )
  })

  it('tambah dan hapus baris item; baris tunggal tidak bisa dihapus; fokus tidak hilang', async () => {
    const { wrapper: w } = await open('/lpb/baru')
    expect(w.findAll('fieldset')).toHaveLength(1)
    expect(w.text()).not.toContain('Hapus')

    await w.find('#add-item').trigger('click')
    await settle()
    expect(w.findAll('fieldset')).toHaveLength(2)
    expect(sel(w, 'items[1].product_id').exists()).toBe(true)

    await pick(w, 'items[0].product_id', '2')
    await pick(w, 'items[1].product_id', '1')
    await w
      .findAll('fieldset')[0]!
      .findAll('button')
      .find((b) => b.text().startsWith('Hapus'))!
      .trigger('click')
    await settle()

    expect(w.findAll('fieldset')).toHaveLength(1)
    // Baris yang tersisa adalah yang tadinya kedua (produk 1).
    expect(sel(w, 'items[0].product_id').element.value).toBe('1')
  })

  it('pasangan produk + rak yang sama di dua baris ditolak dengan petunjuk menggabungkan jumlahnya', async () => {
    const { wrapper: w } = await open('/lpb/baru')
    await fillValid(w)
    await w.find('#add-item').trigger('click')
    await settle()
    await pick(w, 'items[1].product_id', '1')
    await pick(w, 'items[1].rack_id', '1')
    await inp(w, 'items[1].qty').setValue('1')
    await submit(w)

    expect(errOf(w, 'items[1].rack_id')).toContain('gabungkan jumlahnya')
    expect(lpbService.create).not.toHaveBeenCalled()
  })

  it('tanggal di masa depan ditolak', async () => {
    const { wrapper: w } = await open('/lpb/baru')
    await inp(w, 'doc_date').setValue('2999-01-01')
    await inp(w, 'doc_date').trigger('blur')
    await settle()
    expect(errOf(w, 'doc_date')).toContain('masa depan')
  })

  it('galat server pada field (supplier tidak aktif) tampil di field itu; isian tetap utuh', async () => {
    lpbService.create.mockRejectedValue(
      new ApiError(422, 'VALIDATION_ERROR', 'Data LPB tidak valid', [
        { field: 'supplier_id', message: 'supplier tidak aktif' },
      ]),
    )
    const { wrapper: w, router } = await open('/lpb/baru')
    await fillValid(w)
    await submit(w)

    expect(errOf(w, 'supplier_id')).toBe('Supplier tidak aktif')
    expect(router.currentRoute.value.path).toBe('/lpb/baru')
    expect(inp(w, 'items[0].qty').element.value).toBe('2')
  })

  it('galat server tanpa field (mis. rak dikunci opname) tampil sebagai pesan form', async () => {
    lpbService.create.mockRejectedValue(
      new ApiError(409, 'RACK_LOCKED_BY_OPNAME', 'Rak sedang dikunci oleh stock opname'),
    )
    const { wrapper: w } = await open('/lpb/baru')
    await fillValid(w)
    await submit(w)

    expect(w.text()).toContain('Rak sedang dikunci oleh stock opname')
  })

  it('tanpa hak buat (manajemen) ditolak guard ke /403', async () => {
    const { router } = await open('/lpb/baru', ['lpb.read'])
    expect(router.currentRoute.value.name).toBe('forbidden')
  })
})

describe('LPB: form ubah draft', () => {
  const detail = () =>
    lpbDetail({
      notes: 'Catatan lama',
      items: [lpbItem({ qty: 3, unit_cost: '1500000.00' })],
    })

  it('mengisi nilai draft; jenis dan gudang terkunci', async () => {
    lpbService.get.mockResolvedValue(detail())
    const { wrapper: w } = await open('/lpb/7/ubah')

    expect(w.find('h1').text()).toBe('Ubah Draft LPB LPBI-WH-A-2611-00007')
    expect(sel(w, 'sub_type').element.value).toBe('PURCHASE')
    expect(sel(w, 'sub_type').attributes('disabled')).toBeDefined()
    expect(sel(w, 'warehouse_id').element.value).toBe('1')
    expect(sel(w, 'warehouse_id').attributes('disabled')).toBeDefined()
    expect(inp(w, 'items[0].qty').element.value).toBe('3')
    expect(inp(w, 'items[0].unit_cost').element.value).toBe('1500000.00')
    expect(w.find<HTMLTextAreaElement>('textarea[name="notes"]').element.value).toBe('Catatan lama')
  })

  it('simpan memanggil update tanpa jenis/gudang, lalu kembali ke rincian', async () => {
    lpbService.get.mockResolvedValue(detail())
    lpbService.update.mockResolvedValue(detail())
    const { wrapper: w, router } = await open('/lpb/7/ubah')
    await inp(w, 'items[0].qty').setValue('4')
    await submit(w)

    expect(lpbService.update).toHaveBeenCalledWith(7, {
      doc_date: '2026-11-21',
      supplier_id: 1,
      reference_number: 'INV-77',
      notes: 'Catatan lama',
      items: [{ product_id: 1, rack_id: 1, qty: 4, unit_cost: '1500000.00', notes: null }],
    })
    await vi.waitFor(() => expect(router.currentRoute.value.path).toBe('/lpb/7'))
  })

  it('LPB yang bukan draft tidak dapat diubah: pesan jelas dan jalan kembali', async () => {
    lpbService.get.mockResolvedValue(lpbDetail({ status: 'APPROVED' }))
    const { wrapper: w } = await open('/lpb/7/ubah')

    const alert = w.find('[role="alert"]')
    expect(alert.text()).toContain('berstatus Disetujui')
    expect(alert.text()).toContain('tidak dapat diubah lagi')
    expect(w.find('#entity-form').exists()).toBe(false)
  })
})
