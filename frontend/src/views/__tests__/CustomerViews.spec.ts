import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import type { Page } from '@/services/masterService'
import { useAuthStore } from '@/stores/auth'
import type { Customer, CustomerAsset, CustomerDetail, CustomerSite } from '@/types/customer'
import CustomerDetailView from '@/views/customers/CustomerDetailView.vue'
import CustomerListView from '@/views/customers/CustomerListView.vue'

// Mock Leaflet agar komponen CoordinatePickerMap aman di lingkungan jsdom.
vi.mock('leaflet', () => {
  const markerObj = {
    addTo: vi.fn<() => unknown>(),
    setLatLng: vi.fn<() => void>(),
    remove: vi.fn<() => void>(),
  }
  markerObj.addTo.mockReturnValue(markerObj)
  const mapObj = {
    on: vi.fn<() => void>(),
    setView: vi.fn<() => void>(),
    getZoom: vi.fn<() => number>(() => 12),
    invalidateSize: vi.fn<() => void>(),
    remove: vi.fn<() => void>(),
  }
  return {
    default: {
      map: vi.fn<() => typeof mapObj>(() => mapObj),
      tileLayer: vi.fn<() => { addTo: () => void }>(() => ({ addTo: vi.fn<() => void>() })),
      circleMarker: vi.fn<() => typeof markerObj>(() => markerObj),
    },
  }
})

const svc = vi.hoisted(() => ({
  list: vi.fn<() => Promise<Page<Customer>>>(),
  get: vi.fn<(id: number) => Promise<CustomerDetail>>(),
  update: vi.fn<(id: number, body: unknown) => Promise<Customer>>(),
  createSite: vi.fn<(cid: number, body: unknown) => Promise<CustomerSite>>(),
  updateSite: vi.fn<(sid: number, body: unknown) => Promise<CustomerSite>>(),
  listAssets: vi.fn<(cid: number, sid?: number) => Promise<CustomerAsset[]>>(),
}))

vi.mock('primevue/usetoast', () => ({
  useToast: () => ({ add: vi.fn<() => void>() }),
}))

vi.mock('@/services/customerService', () => ({ customerService: svc }))

const sampleCustomer: Customer = {
  id: 1,
  customer_code: 'CUST-001',
  company_name: 'PT Maju Bersama',
  pic_name: 'Siti Rahayu',
  phone: '024-8310001',
  email: 'siti@majubersama.co.id',
  is_pkp: true,
  npwp: '01.234.567.8-504.000',
  tax_address: 'Jl. Pandanaran 50',
  billing_address: 'Jl. Pandanaran 50',
  billing_postal_code: '50241',
  billing_email: 'finance@majubersama.co.id',
  payment_term_days: 30,
  is_active: true,
  site_count: 1,
  active_asset_count: 1,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

const sampleSite: CustomerSite = {
  id: 10,
  customer_id: 1,
  site_name: 'Kantor Pusat Semarang',
  pic_name: 'Siti Rahayu',
  pic_phone: '024-8310001',
  address_line: 'Jl. Pandanaran 50',
  subdistrict: 'Mugassari',
  district: 'Semarang Selatan',
  city: 'Semarang',
  province: 'Jawa Tengah',
  postal_code: '50241',
  latitude: -6.9825,
  longitude: 110.4091,
  is_primary: true,
  is_active: true,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

const sampleAsset: CustomerAsset = {
  serial_id: 99,
  serial_number: 'SN-PRN-001',
  asset_tag: 'AST-001',
  product_id: 5,
  product_sku: 'PRN-HP-01',
  product_name: 'HP LaserJet Pro',
  condition: 'GOOD',
  site_id: 10,
  site_name: 'Kantor Pusat Semarang',
  site_city: 'Semarang',
  contract_id: null,
  contract_number: null,
  updated_at: '2026-01-02T00:00:00Z',
}

function setupAuth(permissions: string[]) {
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
      { path: '/pelanggan', component: { template: '<div />' } },
      { path: '/pelanggan/baru', component: { template: '<div />' } },
      { path: '/pelanggan/:id', component: { template: '<div />' } },
      { path: '/pelanggan/:id/ubah', component: { template: '<div />' } },
      { path: '/stok/serial/:id', component: { template: '<div />' } },
    ],
  })
}

describe('CustomerListView & CustomerDetailView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    svc.list.mockResolvedValue({
      items: [sampleCustomer],
      meta: { page: 1, limit: 20, total: 1, total_pages: 1 },
    })
    svc.get.mockResolvedValue({ ...sampleCustomer, sites: [sampleSite] })
    svc.listAssets.mockResolvedValue([sampleAsset])
  })

  it('menampilkan daftar pelanggan dengan status PKP dan jumlah site/aset', async () => {
    const pinia = setupAuth(['customer.read', 'customer.write'])
    const router = makeTestRouter()
    await router.push('/pelanggan')
    await router.isReady()

    const w = mount(CustomerListView, {
      global: { plugins: [pinia, router, PrimeVue, ToastService] },
    })
    await flushPromises()

    expect(w.text()).toContain('CUST-001')
    expect(w.text()).toContain('PT Maju Bersama')
    expect(w.text()).toContain('PKP')
    expect(w.text()).toContain('Tambah Pelanggan')
  })

  it('menampilkan detail pelanggan, cabang/site, dan tab aset terpasang', async () => {
    const pinia = setupAuth(['customer.read', 'customer.write'])
    const router = makeTestRouter()
    await router.push('/pelanggan/1')
    await router.isReady()

    const w = mount(CustomerDetailView, {
      global: { plugins: [pinia, router, PrimeVue, ToastService] },
    })
    await flushPromises()

    expect(w.text()).toContain('PT Maju Bersama')
    expect(w.text()).toContain('Kantor Pusat Semarang')
    expect(w.text()).toContain('Site Utama')
    expect(w.text()).toContain('-6.982500, 110.409100')

    // Pindah ke tab Aset Terpasang.
    const assetTab = w.findAll('nav button').find((b) => b.text().includes('Aset Terpasang'))
    expect(assetTab).toBeDefined()
    await assetTab!.trigger('click')
    await flushPromises()

    expect(svc.listAssets).toHaveBeenCalledWith(1, undefined)
    expect(w.text()).toContain('SN-PRN-001')
    expect(w.text()).toContain('HP LaserJet Pro')
  })
})
