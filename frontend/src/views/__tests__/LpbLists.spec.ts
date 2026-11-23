import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { VueWrapper } from '@vue/test-utils'
import { authService } from '@/services/authService'
import { lpbService, resetLpbMocks } from '@/test/mockLpbService'
import { pageOf, resetMasterMocks } from '@/test/mockMasterService'
import { lpbListItem, PERMS } from '@/test/fixtures'
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
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  document.body.innerHTML = ''
})

async function open(path: string, perms: readonly string[] = PERMS.staff) {
  auth.refresh.mockResolvedValue(makeSession({ permissions: [...perms] }))
  const app = await mountApp(path)
  wrapper = app.wrapper
  return app
}

describe('LPB: daftar', () => {
  it('memuat urut terbaru dulu, menampilkan baris, status, ringkasan isi, dan tautan ke rincian', async () => {
    lpbService.list.mockResolvedValue(
      pageOf([
        lpbListItem(),
        lpbListItem({ id: 8, lpb_number: 'LPBI-WH-A-2611-00008', status: 'PENDING_APPROVAL' }),
      ]),
    )
    const { wrapper: w } = await open('/lpb')

    expect(lpbService.list).toHaveBeenCalledWith(
      expect.objectContaining({ page: 1, limit: 20, sort: '-created_at' }),
    )
    expect(w.find('h1').text()).toContain('LPB')
    const body = w.find('tbody').text()
    expect(body).toContain('Draft')
    expect(body).toContain('Menunggu persetujuan')
    expect(body).toContain('1 item · 2 unit')
    expect(body).toContain('Pembelian')
    expect(body).toContain('PT Maju Jaya')
    expect(w.find('tbody a[href="/lpb/7"]').text()).toBe('LPBI-WH-A-2611-00007')
  })

  it('filter status & jenis tersinkron ke URL dan dikirim ke API', async () => {
    const { router } = await open('/lpb?status=DRAFT&sub_type=PURCHASE&mine=true')

    expect(lpbService.list).toHaveBeenCalledWith(
      expect.objectContaining({ status: 'DRAFT', sub_type: 'PURCHASE', mine: 'true' }),
    )
    expect(router.currentRoute.value.query.status).toBe('DRAFT')
    // Ketiganya tampil sebagai chip yang dapat dilepas sendiri-sendiri.
    expect(document.body.textContent).toContain('Status: Draft')
    expect(document.body.textContent).toContain('Jenis: Pembelian')
    expect(document.body.textContent).toContain('Pembuat: Dokumen saya')
  })

  it('urutan kolom Nomor LPB lewat URL; hanya satu kolom beraria-sort', async () => {
    lpbService.list.mockResolvedValue(pageOf([lpbListItem()]))
    const { wrapper: w, router } = await open('/lpb')

    // Urutan bawaan (terbaru dulu) harus terlihat pada kolomnya.
    const sorted = w.findAll('thead th[aria-sort]')
    expect(sorted).toHaveLength(1)
    expect(sorted[0]!.text()).toContain('Dibuat')
    expect(sorted[0]!.attributes('aria-sort')).toBe('descending')
    await w
      .findAll('thead th button')
      .find((b) => b.text() === 'Nomor LPB')!
      .trigger('click')
    await settle()

    expect(router.currentRoute.value.query.sort).toBe('lpb_number')
    expect(lpbService.list).toHaveBeenLastCalledWith(
      expect.objectContaining({ sort: 'lpb_number' }),
    )
  })

  it('daftar kosong pertama kali: ajakan membuat LPB (dengan hak buat)', async () => {
    const { wrapper: w } = await open('/lpb')

    expect(w.text()).toContain('Belum ada LPB')
    expect(w.find('a[href="/lpb/baru"]').exists()).toBe(true)
  })

  it('tanpa hak buat (manajemen): tidak ada tombol Buat LPB', async () => {
    const { wrapper: w } = await open('/lpb', ['lpb.read'])
    expect(w.find('a[href="/lpb/baru"]').exists()).toBe(false)
  })

  it('galat memuat: pesan + Coba lagi yang memuat ulang', async () => {
    lpbService.list.mockRejectedValueOnce(new Error('putus'))
    const { wrapper: w } = await open('/lpb')

    expect(w.find('[role="alert"]').text()).toContain('Data gagal dimuat')
    await w
      .findAll('button')
      .find((b) => b.text() === 'Coba lagi')!
      .trigger('click')
    await settle()
    expect(lpbService.list).toHaveBeenCalledTimes(2)
  })
})

describe('LPB: antrean persetujuan', () => {
  it('Kepala Gudang: memuat antrean tertua dulu dengan tombol Tinjau ke rincian', async () => {
    lpbService.approvals.mockResolvedValue(
      pageOf([
        lpbListItem({
          status: 'PENDING_APPROVAL',
          submitted_at: '2026-11-21T10:00:00+07:00',
          created_by_name: 'Siti',
        }),
      ]),
    )
    const { wrapper: w } = await open('/lpb/approvals', PERMS.head)

    expect(lpbService.approvals).toHaveBeenCalledWith(
      expect.objectContaining({ sort: 'submitted_at' }),
    )
    expect(w.find('h1').text()).toBe('Persetujuan LPB')
    const link = w.find('tbody a[href="/lpb/7"].min-h-11:last-of-type')
    expect(w.findAll('tbody a[href="/lpb/7"]').length).toBeGreaterThanOrEqual(2)
    expect(link.exists()).toBe(true)
    expect(w.find('tbody').text()).toContain('Siti')
    expect(w.find('tbody').text()).toContain('Tinjau')
    // Tombol Tinjau punya nama aksesibel yang menyebut dokumennya.
    expect(w.find('tbody .sr-only').text()).toContain('LPBI-WH-A-2611-00007')
  })

  it('antrean kosong: pesan menjelaskan kapan dokumen muncul', async () => {
    const { wrapper: w } = await open('/lpb/approvals', PERMS.head)
    expect(w.text()).toContain('Tidak ada LPB yang menunggu persetujuan')
  })

  it('staf tanpa hak approve ditolak guard ke /403', async () => {
    const { router } = await open('/lpb/approvals', PERMS.staff)
    expect(router.currentRoute.value.name).toBe('forbidden')
    expect(lpbService.approvals).not.toHaveBeenCalled()
  })
})
