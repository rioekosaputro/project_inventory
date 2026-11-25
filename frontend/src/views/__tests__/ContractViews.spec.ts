import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import type { Page } from '@/services/masterService'
import { useAuthStore } from '@/stores/auth'
import type { Permission } from '@/types/auth'
import type { RentalContract, RentalContractDetail } from '@/types/contract'
import ContractDetailView from '@/views/contracts/ContractDetailView.vue'
import ContractListView from '@/views/contracts/ContractListView.vue'

vi.mock('primevue/usetoast', () => ({
  useToast: () => ({ add: vi.fn<() => void>() }),
}))

const svc = vi.hoisted(() => ({
  list: vi.fn<() => Promise<Page<RentalContract>>>(),
  get: vi.fn<(id: number) => Promise<RentalContractDetail>>(),
  activate: vi.fn<(id: number) => Promise<RentalContractDetail>>(),
  terminate: vi.fn<(id: number, body: unknown) => Promise<RentalContractDetail>>(),
  renew: vi.fn<(id: number, body?: unknown) => Promise<RentalContractDetail>>(),
}))

vi.mock('@/services/contractService', () => ({ contractService: svc }))

const sampleContract: RentalContract = {
  id: 10,
  contract_number: 'KTR/GLOBAL/2026/01/0001',
  customer_id: 1,
  customer_code: 'CUST-001',
  customer_name: 'PT Maju Bersama',
  start_date: '2026-01-01',
  end_date: '2026-12-31',
  duration_months: 12,
  billing_day_of_month: 1,
  billing_mode: 'PRORATA',
  billing_in_advance: false,
  is_ppn: true,
  payment_term_days: 30,
  deposit_amount: '1000000.00',
  status: 'DRAFT',
  terminated_at: null,
  termination_reason: null,
  renewed_from_id: null,
  renewed_from_number: null,
  signed_document_path: null,
  notes: null,
  created_by: 1,
  created_by_name: 'Admin Bisnis',
  activated_by: null,
  activated_by_name: null,
  activated_at: null,
  item_count: 1,
  total_qty: 5,
  estimated_monthly: '2500000.00',
  active_placement_qty: 0,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

const sampleDetail: RentalContractDetail = {
  ...sampleContract,
  items: [
    {
      id: 100,
      contract_id: 10,
      product_id: 5,
      product_sku: 'PRN-HP-01',
      product_name: 'HP LaserJet Pro',
      customer_site_id: 2,
      site_name: 'Kantor Pusat',
      qty: 5,
      monthly_rate: '500000.00',
      line_monthly: '2500000.00',
      notes: null,
    },
  ],
}

function setupAuth(permissions: Permission[]) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.user = {
    id: 1,
    name: 'Admin Bisnis',
    email: 'biz@contoh.id',
    phone: null,
    role: 'BUSINESS_ADMIN',
    warehouse_id: null,
    must_change_password: false,
  }
  auth.permissions = permissions
  return pinia
}

function makeTestRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/kontrak', component: { template: '<div />' } },
      { path: '/kontrak/baru', component: { template: '<div />' } },
      { path: '/kontrak/:id', component: { template: '<div />' } },
      { path: '/kontrak/:id/ubah', component: { template: '<div />' } },
      { path: '/pelanggan/:id', component: { template: '<div />' } },
    ],
  })
}

describe('ContractListView & ContractDetailView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    svc.list.mockResolvedValue({
      items: [sampleContract],
      meta: { page: 1, limit: 20, total: 1, total_pages: 1 },
    })
    svc.get.mockResolvedValue(sampleDetail)
  })

  it('menampilkan daftar kontrak dengan periode, estimasi bulanan, dan tombol buat', async () => {
    const pinia = setupAuth(['contract.read', 'contract.write'])
    const router = makeTestRouter()
    await router.push('/kontrak')
    await router.isReady()

    const w = mount(ContractListView, { global: { plugins: [pinia, router] } })
    await flushPromises()

    expect(w.text()).toContain('KTR/GLOBAL/2026/01/0001')
    expect(w.text()).toContain('PT Maju Bersama')
    expect(w.text()).toContain('2026-01-01 s/d 2026-12-31')
    expect(w.text()).toContain('Buat Kontrak')
  })

  it('menampilkan detail kontrak DRAFT dengan item tarif dan tombol Aktifkan Kontrak', async () => {
    const pinia = setupAuth(['contract.read', 'contract.write', 'contract.approve'])
    const router = makeTestRouter()
    await router.push('/kontrak/10')
    await router.isReady()

    const w = mount(ContractDetailView, { global: { plugins: [pinia, router] } })
    await flushPromises()

    expect(w.text()).toContain('KTR/GLOBAL/2026/01/0001')
    expect(w.text()).toContain('HP LaserJet Pro')
    expect(w.text()).toContain('Kantor Pusat')
    expect(w.text()).toContain('Aktifkan Kontrak')
    expect(w.text()).toContain('Ubah Draft')
  })
})
