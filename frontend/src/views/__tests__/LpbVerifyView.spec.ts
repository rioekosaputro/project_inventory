import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/lib/apiError'
import { lpbService } from '@/services/lpbService'
import type { LPBVerifySummary } from '@/types/lpb'
import LPBVerifyView from '@/views/public/LPBVerifyView.vue'

vi.mock('@/services/lpbService', () => ({
  lpbService: {
    verifyPublic: vi.fn<(token: string) => Promise<LPBVerifySummary>>(),
  },
}))

async function mountVerify(token: string) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/verify/lpb/:token', component: LPBVerifyView }],
  })
  await router.push(`/verify/lpb/${token}`)
  await router.isReady()
  const wrapper = mount(LPBVerifyView, { global: { plugins: [router] } })
  await flushPromises()
  return wrapper
}

describe('LPBVerifyView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('menampilkan ringkasan dokumen terverifikasi', async () => {
    vi.mocked(lpbService.verifyPublic).mockResolvedValue({
      lpb_number: 'LPBO/WH-A/2026/11/0007',
      type: 'OUTBOUND',
      sub_type: 'TRANSFER_OUT',
      status: 'APPROVED',
      doc_date: '2026-11-25',
      warehouse_code: 'WH-A',
      warehouse_name: 'Gudang Semarang',
      supplier_name: null,
      item_count: 2,
      total_qty: 5,
      approved_by_name: 'Kepala Gudang',
      approved_at: '2026-11-25T03:00:00Z',
    })

    const w = await mountVerify('valid-token-1234567890123456')
    expect(lpbService.verifyPublic).toHaveBeenCalledWith('valid-token-1234567890123456')
    expect(w.text()).toContain('Dokumen Asli & Terverifikasi')
    expect(w.text()).toContain('LPBO/WH-A/2026/11/0007')
    expect(w.text()).toContain('WH-A — Gudang Semarang')
    expect(w.text()).toContain('2 baris (5 unit/pcs)')
    expect(w.text()).toContain('Kepala Gudang')
  })

  it('menampilkan status tidak ditemukan saat token 404', async () => {
    vi.mocked(lpbService.verifyPublic).mockRejectedValue(
      new ApiError(404, 'LPB_NOT_FOUND', 'LPB tidak ditemukan'),
    )

    const w = await mountVerify('token-palsu-1234567890')
    expect(w.text()).toContain('Dokumen tidak ditemukan')
  })
})
