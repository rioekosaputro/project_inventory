import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { dashboardService, type WarehouseDashboardSummary } from '@/services/dashboardService'
import { useAuthStore } from '@/stores/auth'
import DashboardView from '@/views/dashboard/DashboardView.vue'

vi.mock('@/services/dashboardService', () => ({
  dashboardService: {
    summary: vi.fn<() => Promise<WarehouseDashboardSummary>>(),
  },
}))

const mockSummary = vi.mocked(dashboardService.summary)

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/dashboard', component: DashboardView },
      { path: '/:pathMatch(.*)*', component: { template: '<div />' } },
    ],
  })
}

const baseWarehouse: WarehouseDashboardSummary = {
  total_active_products: 10,
  safe_stock_products: 8,
  low_stock_products: 2,
  total_qty_available: 120,
  serial_units_total: 20,
  serial_units_available: 5,
  pending_lpb_approvals: 3,
  active_opnames: 1,
  pending_do_reservations: 4,
  low_stock_items: [],
  recent_pending_lpbs: [],
  recent_movements: [],
}

describe('DashboardView (T2.11)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    const auth = useAuthStore()
    auth.user = {
      id: 1,
      name: 'Manajer',
      email: 'mgmt@test.local',
      phone: null,
      role: 'MANAGEMENT',
      warehouse_id: null,
      must_change_password: false,
    }
    auth.permissions = ['dashboard.read', 'invoice.read', 'contract.read']
  })

  it('menampilkan metrik komersial (utilisasi, omzet, piutang overdue, kontrak berakhir)', async () => {
    mockSummary.mockResolvedValueOnce({
      ...baseWarehouse,
      commercial: {
        active_customers: 12,
        active_contracts: 15,
        rented_units: 14,
        total_active_assets: 20,
        asset_utilization_pct: '70.00',
        monthly_revenue: '55500000.00',
        monthly_collected: '30000000.00',
        outstanding_receivables: '25500000.00',
        overdue_invoice_count: 2,
        overdue_invoice_amount: '10000000.00',
        due_soon_invoice_count: 1,
        expiring_contracts_count: 1,
        expiring_contracts: [
          {
            contract_id: 7,
            contract_number: 'KTR/GLOBAL/2026/01/0001',
            customer_id: 3,
            customer_name: 'PT Maju Jaya',
            end_date: '2026-12-20',
            days_remaining: 5,
          },
        ],
        overdue_invoices: [
          {
            invoice_id: 9,
            invoice_number: 'INV/GLOBAL/2026/11/0004',
            customer_id: 3,
            customer_name: 'PT Maju Jaya',
            due_date: '2026-11-30',
            days_overdue: 15,
            remaining_amount: '5000000.00',
          },
        ],
      },
    })

    const router = makeRouter()
    await router.push('/dashboard')
    await router.isReady()
    const wrapper = mount(DashboardView, { global: { plugins: [router] } })
    await flushPromises()

    expect(wrapper.text()).toContain('Dashboard Komersial & Gudang')
    expect(wrapper.text()).toContain('70.00%')
    expect(wrapper.text()).toContain('KTR/GLOBAL/2026/01/0001')
    expect(wrapper.text()).toContain('Sisa 5 hari')
    expect(wrapper.text()).toContain('INV/GLOBAL/2026/11/0004')
    expect(wrapper.text()).toContain('Terlambat 15 hari')
    expect(wrapper.text()).toContain('4 DO menunggu reservasi')
  })

  it('menampilkan versi gudang saja bila commercial tidak ada', async () => {
    mockSummary.mockResolvedValueOnce(baseWarehouse)

    const router = makeRouter()
    await router.push('/dashboard')
    await router.isReady()
    const wrapper = mount(DashboardView, { global: { plugins: [router] } })
    await flushPromises()

    expect(wrapper.text()).toContain('Dashboard Gudang')
    expect(wrapper.text()).not.toContain('Utilisasi Aset Sewa')
  })
})
