import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import type { Page } from '@/services/masterService'
import { useAuthStore } from '@/stores/auth'
import type { Permission } from '@/types/auth'
import type { SalesOrder, SalesOrderDetail } from '@/types/salesOrder'
import SalesOrderDetailView from '@/views/salesOrders/SalesOrderDetailView.vue'
import SalesOrderListView from '@/views/salesOrders/SalesOrderListView.vue'

vi.mock('primevue/usetoast', () => ({
  useToast: () => ({ add: vi.fn<() => void>() }),
}))

const svc = vi.hoisted(() => ({
  list: vi.fn<() => Promise<Page<SalesOrder>>>(),
  get: vi.fn<(id: number) => Promise<SalesOrderDetail>>(),
  confirm: vi.fn<(id: number) => Promise<SalesOrderDetail>>(),
  cancel: vi.fn<(id: number, reason: string) => Promise<SalesOrderDetail>>(),
  issueDeliveryOrders: vi.fn<(id: number, body: unknown) => Promise<unknown[]>>(),
}))

vi.mock('@/services/salesOrderService', () => ({ salesOrderService: svc }))

const sampleSO: SalesOrder = {
  id: 21,
  so_number: 'SO/GLOBAL/2026/11/0001',
  order_type: 'CONSUMABLE',
  customer_id: 1,
  customer_code: 'CUST-001',
  customer_name: 'PT Maju Bersama',
  customer_site_id: 2,
  site_name: 'Kantor Pusat',
  site_city: 'Semarang',
  contract_id: null,
  contract_number: null,
  status: 'CONFIRMED',
  order_date: '2026-11-25',
  requested_date: '2026-11-26',
  is_ppn: true,
  subtotal: '5000000.00',
  discount_amount: '0.00',
  dpp: '5000000.00',
  dpp_other_value: '4583333.00',
  tax_rate: '12.00',
  tax_amount: '550000.00',
  grand_total: '5550000.00',
  notes: null,
  created_by: 1,
  created_by_name: 'Admin Bisnis',
  confirmed_by: 1,
  confirmed_by_name: 'Admin Bisnis',
  confirmed_at: '2026-11-25T05:00:00Z',
  cancelled_by: null,
  cancelled_by_name: null,
  cancelled_at: null,
  cancel_reason: null,
  item_count: 1,
  total_qty: 10,
  do_count: 0,
  created_at: '2026-11-25T04:00:00Z',
  updated_at: '2026-11-25T05:00:00Z',
}

const sampleDetail: SalesOrderDetail = {
  ...sampleSO,
  items: [
    {
      id: 1,
      so_id: 21,
      product_id: 3,
      product_sku: 'TNR-HP-01',
      product_name: 'Toner HP 85A',
      direction: 'OUT',
      qty: 10,
      unit_price: '500000.00',
      line_total: '5000000.00',
      product_serial_id: null,
      serial_number: null,
      asset_tag: null,
      notes: null,
    },
  ],
  delivery_orders: [],
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
      { path: '/sales-orders', component: { template: '<div />' } },
      { path: '/sales-orders/baru', component: { template: '<div />' } },
      { path: '/sales-orders/:id', component: { template: '<div />' } },
      { path: '/sales-orders/:id/ubah', component: { template: '<div />' } },
      { path: '/pelanggan/:id', component: { template: '<div />' } },
      { path: '/kontrak/:id', component: { template: '<div />' } },
    ],
  })
}

describe('SalesOrderListView & SalesOrderDetailView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    svc.list.mockResolvedValue({
      items: [sampleSO],
      meta: { page: 1, limit: 20, total: 1, total_pages: 1 },
    })
    svc.get.mockResolvedValue(sampleDetail)
  })

  it('menampilkan daftar Sales Order beserta grand total dan tombol buat', async () => {
    const pinia = setupAuth(['so.read', 'so.write'])
    const router = makeTestRouter()
    await router.push('/sales-orders')
    await router.isReady()

    const w = mount(SalesOrderListView, { global: { plugins: [pinia, router] } })
    await flushPromises()

    expect(w.text()).toContain('SO/GLOBAL/2026/11/0001')
    expect(w.text()).toContain('PT Maju Bersama')
    expect(w.text()).toContain('Buat Sales Order')
  })

  it('menampilkan detail SO CONFIRMED dengan rincian pajak PMK 131/2024 dan tombol Terbitkan DO', async () => {
    const pinia = setupAuth(['so.read', 'so.approve', 'do.create'])
    const router = makeTestRouter()
    await router.push('/sales-orders/21')
    await router.isReady()

    const w = mount(SalesOrderDetailView, { global: { plugins: [pinia, router] } })
    await flushPromises()

    expect(w.text()).toContain('SO/GLOBAL/2026/11/0001')
    expect(w.text()).toContain('DPP Nilai Lain (11/12)')
    expect(w.text()).toContain('Toner HP 85A')
    expect(w.text()).toContain('Terbitkan DO ke Gudang')
  })
})
