import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { VueWrapper } from '@vue/test-utils'
import { ApiError } from '@/lib/apiError'
import { authService } from '@/services/authService'
import { lpbService, resetLpbMocks } from '@/test/mockLpbService'
import { resetMasterMocks } from '@/test/mockMasterService'
import { lpbDetail, lpbItem, lpbSerial, PERMS } from '@/test/fixtures'
import { makeSession } from '@/test/helpers'
import { mountApp, settle } from '@/test/mountApp'
import type { AddSerialResult } from '@/types/lpb'

vi.mock('@/services/authService', () => import('@/test/mockAuthService'))
vi.mock('@/services/masterService', () => import('@/test/mockMasterService'))
vi.mock('@/services/lpbService', () => import('@/test/mockLpbService'))
vi.mock('@/lib/audio', () => ({
  beep: vi.fn<(tone: string) => void>(),
  unlockAudio: vi.fn<() => void>(),
  setMuted: vi.fn<(v: boolean) => void>(),
  isMuted: () => false,
}))

const auth = vi.mocked(authService)
let wrapper: VueWrapper | undefined

const CREATOR = 10

beforeEach(() => {
  vi.clearAllMocks()
  resetMasterMocks()
  resetLpbMocks()
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  document.body.innerHTML = ''
})

async function open(
  detail: ReturnType<typeof lpbDetail>,
  opts: {
    perms?: readonly string[]
    userId?: number
    role?: 'STAFF_WAREHOUSE' | 'HEAD_WAREHOUSE'
  } = {},
) {
  lpbService.get.mockResolvedValue(detail)
  auth.refresh.mockResolvedValue(
    makeSession({
      role: opts.role ?? 'STAFF_WAREHOUSE',
      permissions: [...(opts.perms ?? PERMS.staff)],
      user: { id: opts.userId ?? CREATOR },
    }),
  )
  const app = await mountApp(`/lpb/${detail.id}`)
  wrapper = app.wrapper
  return app
}

// Tombol di dalam <dialog> yang tertutup tidak ikut dihitung (tidak terlihat/tidak dapat dijangkau pengguna).
const buttons = (w: VueWrapper) =>
  w
    .findAll('button, a')
    .filter((b) => !b.element.closest('dialog:not([open])'))
    .map((b) => b.text().replace(/\s+/g, ' '))
const button = (w: VueWrapper, label: string) =>
  w.findAll('button').find((b) => b.text().replace(/\s+/g, ' ').startsWith(label))!
const dialogFor = (w: VueWrapper, title: string) =>
  w.findAll('dialog.confirm-dialog').find((d) => d.find('h2').text().includes(title))!

const saved = (sn: string, scanned: number, qty = 2): AddSerialResult => ({
  ...lpbSerial({ id: 500 + scanned, serial_number: sn }),
  scanned_count: scanned,
  qty,
})

async function scan(w: VueWrapper, code: string) {
  await w.find('#scan-code').setValue(code)
  await w.find('#scan-form').trigger('submit')
  await settle()
}

describe('LPB: rincian draft (pembuat)', () => {
  it('menampilkan ringkasan, item, scanner, dan aksi draft; peringatan serial belum lengkap', async () => {
    const { wrapper: w } = await open(lpbDetail())

    expect(w.find('h1').text()).toContain('LPBI-WH-A-2611-00007')
    expect(w.find('h1').text()).toContain('Draft')
    expect(w.text()).toContain('PT Maju Jaya')
    expect(w.text()).toContain('INV-77')
    expect(w.find('tbody').text()).toContain('PRN-1')
    expect(w.find('tbody').text()).toContain('0 dari 2 ter-scan')
    expect(w.text()).toContain('masih kurang 2 unit')
    expect(w.find('#scan-code').exists()).toBe(true)

    const labels = buttons(w)
    expect(labels).toContain('Ubah Draft')
    expect(labels).toContain('Batalkan Draft')
    expect(labels).toContain('Ajukan untuk Persetujuan')
    expect(labels).not.toContain('Setujui LPB')
  })

  it('dampak stok memberi prakiraan saldo sekarang → setelah disetujui', async () => {
    const { wrapper: w } = await open(lpbDetail({ items: [lpbItem({ qty: 2, current_qty: 5 })] }))
    const impact = w.find('tbody td:last-child').text().replace(/\s+/g, ' ')
    expect(impact).toContain('5')
    expect(impact).toContain('7')
    expect(impact).toContain('prakiraan')
  })

  it('scan: SN dinormalisasi, dikirim ke item terpilih dengan kondisi, daftar & progres ikut bertambah', async () => {
    lpbService.addSerial.mockResolvedValue(saved('SN-1', 1))
    const { wrapper: w } = await open(lpbDetail())
    await scan(w, '  sn-1 ')

    expect(lpbService.addSerial).toHaveBeenCalledWith(7, 70, {
      serial_number: 'SN-1',
      condition: 'GOOD',
    })
    expect(w.find('tbody').text()).toContain('1 dari 2 ter-scan')
    expect(w.text()).toContain('SN-1')
    expect(w.find('[role="log"]').text()).toContain('SN-1 ditambahkan (1/2)')
    // Kolom isian dikosongkan dan tetap terfokus untuk scan berikutnya.
    expect(w.find<HTMLInputElement>('#scan-code').element.value).toBe('')
  })

  it('scan dobel dikenali lokal: tidak memanggil API lagi, pesan menyebut kodenya', async () => {
    lpbService.addSerial.mockResolvedValue(saved('SN-1', 1))
    const { wrapper: w } = await open(lpbDetail())
    await scan(w, 'SN-1')
    await scan(w, 'sn-1')

    expect(lpbService.addSerial).toHaveBeenCalledTimes(1)
    expect(w.find('[role="log"]').text()).toContain('SN-1 sudah di-scan')
  })

  it('galat server pada scan (SN sudah terdaftar) tampil dengan kodenya; progres tidak berubah', async () => {
    lpbService.addSerial.mockRejectedValue(
      new ApiError(409, 'SERIAL_EXISTS', 'Serial number sudah terdaftar'),
    )
    const { wrapper: w } = await open(lpbDetail())
    await scan(w, 'SN-9')

    expect(w.find('[role="log"]').text()).toContain('SN-9: Serial number sudah terdaftar')
    expect(w.find('tbody').text()).toContain('0 dari 2 ter-scan')
  })

  it('jumlah scan penuh: scan berikutnya ditolak lokal tanpa API', async () => {
    const full = lpbItem({
      qty: 1,
      serials: [lpbSerial()],
      scanned_count: 1,
    })
    const { wrapper: w } = await open(lpbDetail({ items: [full] }))
    await scan(w, 'SN-NEW')

    expect(lpbService.addSerial).not.toHaveBeenCalled()
    expect(w.find('[role="log"]').text()).toContain('sudah lengkap')
  })

  it('item penuh → scanner pindah otomatis ke item berikutnya yang belum lengkap', async () => {
    const a = lpbItem({ id: 70, qty: 1 })
    const b = lpbItem({ id: 71, product_id: 2, sku: 'PRN-2', product_name: 'HP Laser', rack_id: 2 })
    lpbService.addSerial.mockResolvedValue(saved('SN-A', 1, 1))
    const { wrapper: w } = await open(lpbDetail({ items: [a, b] }))
    expect(w.find<HTMLSelectElement>('#scan-item').element.value).toBe('70')

    await scan(w, 'SN-A')
    expect(w.find<HTMLSelectElement>('#scan-item').element.value).toBe('71')
  })

  it('menghapus scan memanggil API dan memperbarui daftar', async () => {
    lpbService.removeSerial.mockResolvedValue(undefined)
    const item = lpbItem({ serials: [lpbSerial({ id: 100, serial_number: 'SN-001' })] })
    const { wrapper: w } = await open(lpbDetail({ items: [item] }))

    const remove = w.findAll('button').find((b) => b.text().includes('serial number SN-001'))!
    await remove.trigger('click')
    await settle()

    expect(lpbService.removeSerial).toHaveBeenCalledWith(7, 70, 100)
    expect(w.find('tbody').text()).toContain('0 dari 2 ter-scan')
  })

  it('Ajukan: berhasil → status menunggu persetujuan, tombol draft hilang, toast', async () => {
    lpbService.submit.mockResolvedValue(lpbDetail({ status: 'PENDING_APPROVAL' }))
    const { wrapper: w } = await open(
      lpbDetail({ items: [lpbItem({ qty: 1, serials: [lpbSerial()] })] }),
    )
    await button(w, 'Ajukan untuk Persetujuan').trigger('click')
    await settle()

    expect(lpbService.submit).toHaveBeenCalledWith(7)
    expect(w.find('h1').text()).toContain('Menunggu persetujuan')
    expect(buttons(w)).not.toContain('Ajukan untuk Persetujuan')
    expect(document.body.textContent).toContain('diajukan untuk persetujuan')
  })

  it('Ajukan ditolak server (jumlah SN kurang): pesan server tampil, status tetap draft', async () => {
    lpbService.submit.mockRejectedValue(
      new ApiError(
        422,
        'SERIAL_COUNT_MISMATCH',
        'Jumlah serial number tidak sama dengan qty: ter-scan 0 dari 2',
      ),
    )
    const { wrapper: w } = await open(lpbDetail())
    await button(w, 'Ajukan untuk Persetujuan').trigger('click')
    await settle()

    expect(document.body.textContent).toContain('ter-scan 0 dari 2')
    expect(w.find('h1').text()).toContain('Draft')
  })

  it('Batalkan Draft: dialog menyebut nomor dokumen; fokus awal di Batal; konfirmasi memanggil API', async () => {
    lpbService.cancel.mockResolvedValue(lpbDetail({ status: 'CANCELLED' }))
    const { wrapper: w } = await open(lpbDetail())
    await button(w, 'Batalkan Draft').trigger('click')
    await settle()

    const dialog = dialogFor(w, 'Batalkan draft LPBI-WH-A-2611-00007?')
    expect(dialog.attributes('open')).toBeDefined()
    expect(dialog.find('button[autofocus]').text()).toBe('Batal')
    expect(lpbService.cancel).not.toHaveBeenCalled()

    await dialog
      .findAll('button')
      .find((b) => b.text() === 'Batalkan Draft')!
      .trigger('click')
    await settle()

    expect(lpbService.cancel).toHaveBeenCalledWith(7)
    expect(w.find('h1').text()).toContain('Dibatalkan')
  })

  it('serial number sudah tidak dapat dihapus/scanner hilang pada dokumen non-draft', async () => {
    const { wrapper: w } = await open(
      lpbDetail({
        status: 'PENDING_APPROVAL',
        items: [lpbItem({ serials: [lpbSerial()], qty: 1 })],
      }),
    )
    expect(w.find('#scan-code').exists()).toBe(false)
    expect(w.findAll('button').some((b) => b.text().startsWith('Hapus'))).toBe(false)
    expect(w.text()).toContain('SN-001') // tetap terlihat, hanya tidak bisa diubah
  })
})

describe('LPB: hak akses tampilan', () => {
  it('manajemen (hanya lpb.read): tidak ada tombol aksi dan tidak ada scanner', async () => {
    const { wrapper: w } = await open(lpbDetail(), { perms: ['lpb.read'] })
    expect(buttons(w)).not.toContain('Ubah Draft')
    expect(buttons(w)).not.toContain('Ajukan untuk Persetujuan')
    expect(w.find('#scan-code').exists()).toBe(false)
  })

  it('staf lain (bukan pembuat) tidak ditawari mengubah draft orang lain', async () => {
    const { wrapper: w } = await open(lpbDetail(), { userId: 99 })
    expect(buttons(w)).not.toContain('Batalkan Draft')
    expect(w.find('#scan-code').exists()).toBe(false)
  })

  it('Kepala Gudang boleh membantu menyentuh draft staf', async () => {
    const { wrapper: w } = await open(lpbDetail(), {
      perms: PERMS.head,
      userId: 99,
      role: 'HEAD_WAREHOUSE',
    })
    expect(buttons(w)).toContain('Batalkan Draft')
  })
})

describe('LPB: persetujuan', () => {
  const pending = (over = {}) =>
    lpbDetail({
      status: 'PENDING_APPROVAL',
      submitted_at: '2026-11-21T10:00:00+07:00',
      items: [lpbItem({ qty: 2, current_qty: 5, serials: [lpbSerial(), lpbSerial({ id: 101 })] })],
      ...over,
    })
  const head = { perms: PERMS.head, userId: 50, role: 'HEAD_WAREHOUSE' as const }

  it('approver melihat dampak stok sebelum memutuskan dan tombol Setujui/Tolak', async () => {
    const { wrapper: w } = await open(pending(), head)

    expect(w.text()).toContain('Periksa dampak stok sebelum memutuskan')
    const impact = w.find('tbody td:last-child').text().replace(/\s+/g, ' ')
    expect(impact).toContain('5')
    expect(impact).toContain('7')
    expect(buttons(w)).toContain('Setujui LPB')
    expect(buttons(w)).toContain('Tolak LPB')
    expect(buttons(w)).not.toContain('Ubah Draft')
  })

  it('pembuat biasa (tanpa lpb.approve) tidak melihat tombol keputusan', async () => {
    const { wrapper: w } = await open(pending())
    expect(buttons(w)).not.toContain('Setujui LPB')
    expect(buttons(w)).not.toContain('Tolak LPB')
  })

  it('Setujui: dialog konfirmasi menyebut dokumen; konfirmasi mengirim Idempotency-Key; hasilnya final dari ledger', async () => {
    lpbService.approve.mockResolvedValue(
      lpbDetail({
        status: 'APPROVED',
        approved_at: '2026-11-21T11:00:00+07:00',
        items: [
          lpbItem({
            qty: 2,
            current_qty: 7,
            balance_before: 5,
            balance_after: 7,
            serials: [
              lpbSerial({ asset_tag: 'AST-2611-00001' }),
              lpbSerial({ id: 101, serial_number: 'SN-002', asset_tag: 'AST-2611-00002' }),
            ],
          }),
        ],
      }),
    )
    const { wrapper: w } = await open(pending(), head)
    await button(w, 'Setujui LPB').trigger('click')
    await settle()

    const dialog = dialogFor(w, 'Setujui LPB LPBI-WH-A-2611-00007?')
    expect(dialog.attributes('open')).toBeDefined()
    expect(lpbService.approve).not.toHaveBeenCalled()
    await dialog
      .findAll('button')
      .find((b) => b.text() === 'Setujui LPB')!
      .trigger('click')
    await settle()

    expect(lpbService.approve).toHaveBeenCalledTimes(1)
    const [id, key] = lpbService.approve.mock.calls[0]!
    expect(id).toBe(7)
    expect(key.length).toBeGreaterThanOrEqual(8) // kunci layak (8–100 karakter, aturan backend)

    expect(w.find('h1').text()).toContain('Disetujui')
    const impact = w.find('tbody td:last-child').text().replace(/\s+/g, ' ')
    expect(impact).toContain('5')
    expect(impact).toContain('7')
    expect(impact).not.toContain('prakiraan') // angka ledger, bukan prakiraan
    expect(w.text()).toContain('AST-2611-00001')
    expect(buttons(w)).not.toContain('Setujui LPB')
    expect(document.body.textContent).toContain('disetujui; stok diperbarui')
  })

  it('gagal lalu diulang memakai kunci yang SAMA (aman dari stok bertambah dua kali)', async () => {
    lpbService.approve.mockRejectedValueOnce(new ApiError(0, 'NETWORK_ERROR', 'Tidak terhubung'))
    lpbService.approve.mockResolvedValueOnce(lpbDetail({ status: 'APPROVED' }))
    const { wrapper: w } = await open(pending(), head)
    await button(w, 'Setujui LPB').trigger('click')
    await settle()

    const confirm = () =>
      dialogFor(w, 'Setujui LPB LPBI')
        .findAll('button')
        .find((b) => b.text() === 'Setujui LPB')!
    await confirm().trigger('click')
    await settle()
    expect(document.body.textContent).toContain('Tidak terhubung')
    expect(w.find('h1').text()).toContain('Menunggu persetujuan')

    await confirm().trigger('click')
    await settle()

    expect(lpbService.approve).toHaveBeenCalledTimes(2)
    expect(lpbService.approve.mock.calls[1]![1]).toBe(lpbService.approve.mock.calls[0]![1])
  })

  it('niat baru (dialog dibuka lagi) memakai kunci baru', async () => {
    lpbService.approve.mockRejectedValue(
      new ApiError(403, 'SELF_APPROVE_FORBIDDEN', 'Pembuat LPB tidak boleh menyetujui LPB sendiri'),
    )
    const { wrapper: w } = await open(pending(), head)
    const confirmOnce = async () => {
      await button(w, 'Setujui LPB').trigger('click')
      await settle()
      await dialogFor(w, 'Setujui LPB LPBI')
        .findAll('button')
        .find((b) => b.text() === 'Setujui LPB')!
        .trigger('click')
      await settle()
      await dialogFor(w, 'Setujui LPB LPBI').find('button[autofocus]').trigger('click') // Batal
      await settle()
    }
    await confirmOnce()
    await confirmOnce()

    expect(lpbService.approve).toHaveBeenCalledTimes(2)
    expect(lpbService.approve.mock.calls[1]![1]).not.toBe(lpbService.approve.mock.calls[0]![1])
    expect(document.body.textContent).toContain('tidak boleh menyetujui LPB sendiri')
  })

  it('Tolak: alasan wajib (blur-first); alasan terlalu pendek ditolak; alasan valid dikirim terpangkas', async () => {
    lpbService.reject.mockResolvedValue(
      lpbDetail({ status: 'REJECTED', rejection_reason: 'Jumlah tidak sesuai surat jalan' }),
    )
    const { wrapper: w } = await open(pending(), head)
    await button(w, 'Tolak LPB').trigger('click')
    await settle()

    const form = () => w.find('dialog.slide-over form')
    const reason = () => w.find<HTMLTextAreaElement>('#reject-reason')
    await form().trigger('submit')
    await settle()
    expect(w.find('#reject-reason-error').text()).toContain('minimal 3 karakter')
    expect(lpbService.reject).not.toHaveBeenCalled()

    await reason().setValue('ab')
    await form().trigger('submit')
    await settle()
    expect(lpbService.reject).not.toHaveBeenCalled()

    await reason().setValue('  Jumlah tidak sesuai surat jalan  ')
    await form().trigger('submit')
    await settle()

    expect(lpbService.reject).toHaveBeenCalledWith(7, 'Jumlah tidak sesuai surat jalan')
    expect(w.find('h1').text()).toContain('Ditolak')
    expect(w.text()).toContain('Alasan: Jumlah tidak sesuai surat jalan')
  })
})

describe('LPB: transfer dan penghapusan', () => {
  const transferItem = () =>
    lpbItem({
      qty: 1,
      dest_rack_id: 3,
      dest_rack_code: 'B-01',
      dest_warehouse_id: 2,
      dest_warehouse_code: 'WH-B',
    })
  const out = (over = {}) =>
    lpbDetail({ sub_type: 'TRANSFER_OUT', type: 'OUTBOUND', items: [transferItem()], ...over })
  const head = { perms: PERMS.head, userId: 50, role: 'HEAD_WAREHOUSE' as const }

  it('transfer keluar: tujuan (gudang / rak) tampil di tabel dan kartu', async () => {
    const { wrapper: w } = await open(out())
    expect(w.find('tbody').text()).toContain('WH-B / B-01')
    expect(w.find('ul[aria-label="Item LPB"]').text()).toContain('WH-B / B-01')
  })

  it('scanner transfer: label asset tag atau SN, tanpa pilihan kondisi, dan hanya serial_number dikirim', async () => {
    lpbService.addSerial.mockResolvedValue(saved('SN-9', 1, 1))
    const { wrapper: w } = await open(out())

    expect(w.text()).toContain('Scan unit yang dipindahkan')
    expect(w.find('label[for="scan-code"]').text()).toBe('Asset tag atau serial number')
    expect(w.find('#scan-condition').exists()).toBe(false)
    await scan(w, 'ast-2611-00001')
    expect(lpbService.addSerial).toHaveBeenCalledWith(7, 70, { serial_number: 'AST-2611-00001' })
  })

  it('penghapusan: judul scanner khusus; dampak stok mengurangi saldo', async () => {
    const { wrapper: w } = await open(
      lpbDetail({
        sub_type: 'DISPOSAL',
        type: 'OUTBOUND',
        items: [lpbItem({ qty: 2, current_qty: 5 })],
      }),
    )
    expect(w.text()).toContain('Scan unit yang dihapus')
    const impact = w.find('tbody td:last-child').text().replace(/\s+/g, ' ')
    expect(impact).toContain('5')
    expect(impact).toContain('3')
  })

  it('transfer keluar pending: dialog setujui menjelaskan pasangan LPB masuk dan unit dalam perjalanan', async () => {
    const { wrapper: w } = await open(out({ status: 'PENDING_APPROVAL' }), head)
    await button(w, 'Setujui LPB').trigger('click')
    await settle()

    const dialog = dialogFor(w, 'Setujui LPB LPBI')
    expect(dialog.text()).toContain('dalam perjalanan')
    expect(dialog.text()).toContain('LPB masuk pasangannya')
  })

  it('transfer masuk pending: tanpa tombol Tolak, ada penjelasan dan tautan ke transfer keluar asal', async () => {
    const { wrapper: w } = await open(
      lpbDetail({
        sub_type: 'TRANSFER_IN',
        type: 'INBOUND',
        status: 'PENDING_APPROVAL',
        counterpart_id: 11,
        counterpart_number: 'LPBO/WH-A/2026/11/0003',
        items: [lpbItem({ qty: 1 })],
      }),
      head,
    )

    expect(buttons(w)).toContain('Setujui LPB')
    expect(buttons(w)).not.toContain('Tolak LPB')
    expect(w.text()).toContain('transfer masuk tidak dapat ditolak')
    const link = w.find('a[href="/lpb/11"]')
    expect(link.text()).toBe('LPBO/WH-A/2026/11/0003')
    expect(w.text()).toContain('Transfer keluar asal')
  })

  it('transfer keluar yang sudah disetujui menautkan LPB masuk pasangannya', async () => {
    const { wrapper: w } = await open(
      out({
        status: 'APPROVED',
        counterpart_id: 12,
        counterpart_number: 'LPBI/WH-A/2026/11/0009',
      }),
    )
    expect(w.text()).toContain('LPB masuk pasangan')
    expect(w.find('a[href="/lpb/12"]').text()).toBe('LPBI/WH-A/2026/11/0009')
  })
})

describe('LPB: berpindah antar dokumen', () => {
  it('membuka dokumen pasangan dari tautan memuat ulang data (komponen yang sama dipakai ulang)', async () => {
    const outDoc = lpbDetail({
      id: 7,
      lpb_number: 'LPBO/WH-A/2026/11/0001',
      sub_type: 'TRANSFER_OUT',
      type: 'OUTBOUND',
      status: 'APPROVED',
      counterpart_id: 8,
      counterpart_number: 'LPBI/WH-A/2026/11/0009',
    })
    const inDoc = lpbDetail({
      id: 8,
      lpb_number: 'LPBI/WH-A/2026/11/0009',
      sub_type: 'TRANSFER_IN',
      type: 'INBOUND',
      status: 'APPROVED',
      counterpart_id: 7,
      counterpart_number: 'LPBO/WH-A/2026/11/0001',
    })
    const { wrapper: w, router } = await open(outDoc)
    lpbService.get.mockImplementation((id) => Promise.resolve(id === 7 ? outDoc : inDoc))
    expect(w.find('h1').text()).toContain('LPBO/WH-A/2026/11/0001')

    await w.find('a[href="/lpb/8"]').trigger('click')
    await settle()

    expect(router.currentRoute.value.path).toBe('/lpb/8')
    expect(lpbService.get).toHaveBeenLastCalledWith(8)
    expect(w.find('h1').text()).toContain('LPBI/WH-A/2026/11/0009')
    expect(w.text()).toContain('Transfer keluar asal')
  })
})

describe('LPB: status lain & riwayat', () => {
  it('Ditolak: alasan terlihat dan pembuat dapat Revisi → kembali draft', async () => {
    lpbService.revise.mockResolvedValue(
      lpbDetail({ status: 'DRAFT', rejection_reason: 'Qty salah' }),
    )
    const { wrapper: w } = await open(
      lpbDetail({
        status: 'REJECTED',
        rejection_reason: 'Qty salah',
        rejected_at: '2026-11-21T12:00:00+07:00',
      }),
    )
    expect(w.text()).toContain('Alasan: Qty salah')
    expect(buttons(w)).toContain('Revisi LPB')

    await button(w, 'Revisi LPB').trigger('click')
    await settle()

    expect(lpbService.revise).toHaveBeenCalledWith(7)
    expect(w.find('h1').text()).toContain('Draft')
    expect(w.text()).toContain('Qty salah') // alasan tetap terlihat sampai diajukan ulang
  })

  it('Dibatalkan: tanpa aksi dan tanpa kolom saldo', async () => {
    const { wrapper: w } = await open(lpbDetail({ status: 'CANCELLED' }))
    expect(buttons(w)).not.toContain('Ubah Draft')
    expect(w.find('thead').text()).not.toContain('saldo')
  })

  it('riwayat: urutan kejadian dengan pelaku dan alasan penolakan', async () => {
    const { wrapper: w } = await open(
      lpbDetail({
        status: 'REJECTED',
        timeline: [
          {
            action: 'LPB_CREATE',
            user_id: 10,
            user_name: 'Budi',
            data: null,
            created_at: '2026-11-21T09:00:00+07:00',
          },
          {
            action: 'LPB_SUBMIT',
            user_id: 10,
            user_name: 'Budi',
            data: { status: 'PENDING_APPROVAL' },
            created_at: '2026-11-21T10:00:00+07:00',
          },
          {
            action: 'LPB_REJECT',
            user_id: 50,
            user_name: 'Kepala Gudang',
            data: { reason: 'Qty salah' },
            created_at: '2026-11-21T11:00:00+07:00',
          },
        ],
      }),
    )
    const items = w.findAll('ol[aria-label="Riwayat LPB"] li')
    expect(items.map((i) => i.find('p').text())).toEqual([
      'Draft dibuat',
      'Diajukan untuk persetujuan',
      'Ditolak',
    ])
    expect(items[2]!.text()).toContain('Kepala Gudang')
    expect(items[2]!.text()).toContain('Alasan: Qty salah')
  })

  it('LPB tidak ditemukan: pesan galat dan Coba lagi', async () => {
    lpbService.get.mockRejectedValue(new ApiError(404, 'NOT_FOUND', 'LPB tidak ditemukan'))
    auth.refresh.mockResolvedValue(makeSession({ permissions: [...PERMS.staff] }))
    const app = await mountApp('/lpb/999')
    wrapper = app.wrapper

    expect(app.wrapper.find('[role="alert"]').text()).toContain('LPB tidak ditemukan')
    await app.wrapper
      .findAll('button')
      .find((b) => b.text() === 'Coba lagi')!
      .trigger('click')
    await settle()
    expect(lpbService.get).toHaveBeenCalledTimes(2)
  })
})
