import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import type { Page } from '@/services/masterService'
import { useAuthStore } from '@/stores/auth'
import type { Permission } from '@/types/auth'
import type { DeliveryOrder, DeliveryOrderDetail } from '@/types/deliveryOrder'
import DeliveryOrderDetailView from '@/views/deliveryOrders/DeliveryOrderDetailView.vue'
import DeliveryOrderListView from '@/views/deliveryOrders/DeliveryOrderListView.vue'

vi.mock('primevue/usetoast', () => ({
  useToast: () => ({ add: vi.fn<() => void>() }),
}))

const doSvc = vi.hoisted(() => ({
  list: vi.fn<() => Promise<Page<DeliveryOrder>>>(),
  get: vi.fn<(id: number) => Promise<DeliveryOrderDetail>>(),
  reserve: vi.fn<(id: number, items: unknown[]) => Promise<DeliveryOrderDetail>>(),
  release: vi.fn<(id: number) => Promise<DeliveryOrderDetail>>(),
  cancel: vi.fn<(id: number) => Promise<DeliveryOrderDetail>>(),
  createLPB: vi.fn<(id: number, body?: unknown) => Promise<unknown>>(),
}))

const rackSvc = vi.hoisted(() => ({
  list: vi.fn<() => Promise<Page<unknown>>>(),
}))

vi.mock('@/services/deliveryOrderService', () => ({ deliveryOrderService: doSvc }))
vi.mock('@/services/masterService', () => ({ rackService: rackSvc }))

const sampleDO: DeliveryOrder = {
  id: 31,
  do_number: 'DO/WH-SMG-01/2026/11/0001',
  so_id: 21,
  so_number: 'SO/GLOBAL/2026/11/0001',
  order_type: 'NEW_DELIVERY',
  customer_id: 1,
  customer_code: 'CUST-001',
  customer_name: 'PT Maju Bersama',
  customer_site_id: 2,
  site_name: 'Kantor Pusat',
  site_city: 'Semarang',
  contract_id: 10,
  warehouse_id: 1,
  warehouse_code: 'WH-SMG-01',
  warehouse_name: 'Gudang Semarang',
  direction: 'OUTBOUND',
  status: 'RESERVED',
  scheduled_date: '2026-11-26',
  lpb_id: null,
  lpb_number: null,
  notes: null,
  created_by_name: 'Admin Bisnis',
  item_count: 1,
  total_qty: 1,
  reserved_qty: 1,
  created_at: '2026-11-25T06:00:00Z',
  updated_at: '2026-11-25T06:00:00Z',
}

const sampleDetail: DeliveryOrderDetail = {
  ...sampleDO,
  items: [
    {
      id: 1,
      do_id: 31,
      so_item_id: 1,
      product_id: 5,
      product_sku: 'PRN-HP-01',
      product_name: 'HP LaserJet Pro',
      has_serial_number: true,
      qty: 1,
      reserved_qty: 1,
      remaining_qty: 0,
      so_product_serial_id: null,
      so_serial_number: null,
      reservations: [
        {
          id: 9,
          do_item_id: 1,
          rack_id: 2,
          rack_code: 'A-01-01',
          product_serial_id: 50,
          serial_number: 'SN-HP-001',
          asset_tag: 'AST-2611-00001',
          qty: 1,
          reserved_at: '2026-11-25T06:10:00Z',
        },
      ],
    },
  ],
}

function setupAuth(permissions: Permission[]) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.user = {
    id: 2,
    name: 'Staf Gudang',
    email: 'staf@contoh.id',
    phone: null,
    role: 'STAFF_WAREHOUSE',
    warehouse_id: 1,
    must_change_password: false,
  }
  auth.permissions = permissions
  return pinia
}

function makeTestRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/delivery-orders', component: { template: '<div />' } },
      { path: '/delivery-orders/:id', component: { template: '<div />' } },
      { path: '/sales-orders/:id', component: { template: '<div />' } },
      { path: '/lpb/:id', component: { template: '<div />' } },
    ],
  })
}

describe('DeliveryOrderListView & DeliveryOrderDetailView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    doSvc.list.mockResolvedValue({
      items: [sampleDO],
      meta: { page: 1, limit: 20, total: 1, total_pages: 1 },
    })
    doSvc.get.mockResolvedValue(sampleDetail)
    rackSvc.list.mockResolvedValue({
      items: [],
      meta: { page: 1, limit: 100, total: 0, total_pages: 0 },
    })
  })

  it('menampilkan antrean DO dengan progres reservasi dan status', async () => {
    const pinia = setupAuth(['do.read', 'do.reserve'])
    const router = makeTestRouter()
    await router.push('/delivery-orders')
    await router.isReady()

    const w = mount(DeliveryOrderListView, { global: { plugins: [pinia, router] } })
    await flushPromises()

    expect(w.text()).toContain('DO/WH-SMG-01/2026/11/0001')
    expect(w.text()).toContain('PT Maju Bersama')
    expect(w.text()).toContain('1 / 1')
  })

  it('menampilkan detail DO RESERVED beserta SN tereservasi dan tombol Buat Draft LPB', async () => {
    const pinia = setupAuth(['do.read', 'do.reserve'])
    const router = makeTestRouter()
    await router.push('/delivery-orders/31')
    await router.isReady()

    const w = mount(DeliveryOrderDetailView, { global: { plugins: [pinia, router] } })
    await flushPromises()

    expect(w.text()).toContain('DO/WH-SMG-01/2026/11/0001')
    expect(w.text()).toContain('SN-HP-001')
    expect(w.text()).toContain('Buat Draft LPB')
    expect(w.text()).toContain('Lepas Reservasi')
  })
})
