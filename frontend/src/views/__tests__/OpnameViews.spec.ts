import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { opnameService } from '@/services/opnameService'
import { useAuthStore } from '@/stores/auth'
import type { OpnameDetail } from '@/types/opname'
import OpnameDetailView from '@/views/opname/OpnameDetailView.vue'

vi.mock('@/services/opnameService', () => ({
  opnameService: {
    get: vi.fn<(id: number) => Promise<OpnameDetail>>(),
    saveCounts: vi.fn<() => Promise<OpnameDetail>>(),
    scanSerial: vi.fn<() => Promise<unknown>>(),
    submit: vi.fn<() => Promise<OpnameDetail>>(),
    finalize: vi.fn<() => Promise<OpnameDetail>>(),
    cancel: vi.fn<() => Promise<OpnameDetail>>(),
  },
}))

vi.mock('@/lib/audio', () => ({
  beep: vi.fn<() => void>(),
}))

const sampleOpname: OpnameDetail = {
  id: 10,
  opname_number: 'OPN/WH-A/2026/11/0001',
  warehouse_id: 1,
  warehouse_code: 'WH-A',
  warehouse_name: 'Gudang Semarang',
  status: 'IN_PROGRESS',
  is_blind_count: true,
  started_by: 2,
  started_by_name: 'Staf Gudang',
  started_at: '2026-11-25T03:00:00Z',
  submitted_by: null,
  submitted_at: null,
  finalized_by: null,
  finalized_at: null,
  adjustment_in_lpb_id: null,
  adjustment_in_lpb_number: null,
  adjustment_out_lpb_id: null,
  adjustment_out_lpb_number: null,
  notes: null,
  racks: [{ rack_id: 1, rack_code: 'A-01', zone: 'PRINTER', is_sellable: true }],
  items: [
    {
      id: 1,
      product_id: 2,
      sku: 'TN-1',
      product_name: 'Toner HP',
      unit: 'pcs',
      rack_id: 1,
      rack_code: 'A-01',
      system_qty: null,
      counted_qty: 7,
      diff_qty: null,
      counted_by: 2,
      counted_at: '2026-11-25T03:05:00Z',
      notes: null,
    },
  ],
  serials: [
    {
      id: 1,
      rack_id: 1,
      rack_code: 'A-01',
      product_serial_id: 5,
      asset_tag: 'AST-2611-00001',
      serial_number: 'SN-1',
      sku: 'PRN-1',
      product_name: 'Epson L3210',
      scanned_code: 'AST-2611-00001',
      result: 'FOUND',
      scanned_by: 2,
      scanned_at: '2026-11-25T03:06:00Z',
    },
  ],
}

describe('OpnameDetailView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setActivePinia(createPinia())
    const auth = useAuthStore()
    auth.user = {
      id: 2,
      name: 'Staf Gudang',
      email: 'staf@x.id',
      phone: null,
      role: 'STAFF_WAREHOUSE',
      warehouse_id: 1,
      must_change_password: false,
    }
    auth.permissions = ['opname.read', 'opname.create']
  })

  it('menampilkan ringkasan opname, blind count, dan tab scan serial', async () => {
    vi.mocked(opnameService.get).mockResolvedValue(sampleOpname)
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/opname', component: { template: '<div />' } },
        { path: '/opname/:id', component: OpnameDetailView },
      ],
    })
    await router.push('/opname/10')
    await router.isReady()

    const w = mount(OpnameDetailView, {
      global: { plugins: [router, PrimeVue, ToastService] },
    })
    await flushPromises()

    expect(w.text()).toContain('OPN/WH-A/2026/11/0001')
    expect(w.text()).toContain('Toner HP')
    expect(w.text()).toContain('Blind count')
    expect(w.find('header').text()).toContain('Ajukan Opname')
    expect(w.find('header').text()).not.toContain('Finalisasi')
  })
})
