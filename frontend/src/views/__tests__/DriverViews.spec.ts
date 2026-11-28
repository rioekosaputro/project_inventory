import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import DriverDeliveryListView from '@/views/driver/DriverDeliveryListView.vue'
import DriverDeliveryActionView from '@/views/driver/DriverDeliveryActionView.vue'
import type { DriverDeliveryTask } from '@/services/trackingService'

const mockService = vi.hoisted(() => ({
  myDeliveries: vi.fn<() => Promise<DriverDeliveryTask[]>>(),
  checkpoint: vi.fn<() => Promise<unknown>>(),
  deliver: vi.fn<() => Promise<unknown>>(),
  fail: vi.fn<() => Promise<unknown>>(),
  pickup: vi.fn<() => Promise<unknown>>(),
}))

vi.mock('@/services/trackingService', () => ({
  trackingService: mockService,
}))

vi.mock('@/services/lpbService', () => ({
  lpbService: {
    fetchPdf: vi.fn<() => Promise<Blob>>(),
  },
}))

const sampleOutbound: DriverDeliveryTask = {
  lpb_id: 10,
  lpb_number: 'LPB/OUT/2026/0010',
  type: 'OUTBOUND',
  sub_type: 'RENTAL_DELIVERY',
  status: 'APPROVED',
  delivery_status: 'PENDING',
  doc_date: '2026-11-28',
  warehouse_id: 1,
  warehouse_code: 'WH-SMG',
  warehouse_name: 'Gudang Semarang',
  customer_id: 5,
  customer_name: 'PT Pelanggan Maju',
  customer_site_id: 12,
  site_name: 'Kantor Cabang',
  site_address: 'Jl. Pemuda No. 10',
  site_city: 'Semarang',
  site_pic_name: 'Rina',
  site_pic_phone: '08123456789',
  site_latitude: -6.9932,
  site_longitude: 110.4203,
  item_count: 2,
  total_qty: 3,
  notes: null,
}

const sampleReturn: DriverDeliveryTask = {
  ...sampleOutbound,
  lpb_id: 11,
  lpb_number: 'LPB/IN/2026/0011',
  type: 'INBOUND',
  sub_type: 'RENTAL_RETURN',
  status: 'DRAFT',
  delivery_status: null,
}

function makeRouter(initialPath: string) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/driver/deliveries', name: 'driver-deliveries', component: DriverDeliveryListView },
      {
        path: '/driver/deliveries/:id',
        name: 'driver-delivery-detail',
        component: DriverDeliveryActionView,
      },
    ],
  })
  void router.push(initialPath)
  return router
}

describe('Driver Mobile Views (T3.4)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    Object.defineProperty(navigator, 'geolocation', {
      configurable: true,
      value: {
        getCurrentPosition: (ok: (pos: GeolocationPosition) => void) =>
          ok({
            coords: { latitude: -6.9932, longitude: 110.4203, accuracy: 12 },
          } as GeolocationPosition),
      },
    })
  })

  it('menampilkan daftar tugas antar dan jemput beserta tautan navigasi Google Maps', async () => {
    mockService.myDeliveries.mockResolvedValue([sampleOutbound, sampleReturn])
    const router = makeRouter('/driver/deliveries')
    await router.isReady()

    const w = mount(DriverDeliveryListView, {
      global: { plugins: [router, PrimeVue, ToastService] },
    })
    await flushPromises()

    expect(w.text()).toContain('LPB/OUT/2026/0010')
    expect(w.text()).toContain('KIRIM UNIT')
    expect(w.text()).toContain('LPB/IN/2026/0011')
    expect(w.text()).toContain('JEMPUT RETUR')
    expect(w.text()).toContain('1 antar · 1 jemput')
    expect(w.find('a[href*="google.com/maps"]').exists()).toBe(true)
  })

  it('mengirim checkpoint GPS dengan client_event_id UUID pada halaman detail pengiriman', async () => {
    mockService.myDeliveries.mockResolvedValue([sampleOutbound])
    mockService.checkpoint.mockResolvedValue({
      checkpoint: { id: 1, status: 'IN_TRANSIT' },
      idempotent: false,
    })

    const router = makeRouter('/driver/deliveries/10')
    await router.isReady()

    const w = mount(DriverDeliveryActionView, {
      global: { plugins: [router, PrimeVue, ToastService] },
    })
    await flushPromises()

    expect(w.text()).toContain('Bukti Serah Terima (POD)')
    const btn = w.findAll('button').find((b) => b.text().includes('Dalam Perjalanan'))
    expect(btn).toBeDefined()
    await btn!.trigger('click')
    await flushPromises()

    expect(mockService.checkpoint).toHaveBeenCalledTimes(1)
    const call = mockService.checkpoint.mock.calls[0] as unknown as [
      number,
      { status: string; latitude: number; longitude: number; client_event_id: string },
    ]
    expect(call[0]).toBe(10)
    expect(call[1].status).toBe('IN_TRANSIT')
    expect(call[1].latitude).toBe(-6.9932)
    expect(call[1].client_event_id).toMatch(/^[0-9a-f-]{36}$/i)
  })

  it('menampilkan tombol konfirmasi penjemputan pada LPB RENTAL_RETURN', async () => {
    mockService.myDeliveries.mockResolvedValue([sampleReturn])
    mockService.pickup.mockResolvedValue({
      checkpoint: { id: 2, status: 'PICKED_UP' },
      idempotent: false,
    })

    const router = makeRouter('/driver/deliveries/11')
    await router.isReady()

    const w = mount(DriverDeliveryActionView, {
      global: { plugins: [router, PrimeVue, ToastService] },
    })
    await flushPromises()

    expect(w.text()).toContain('Konfirmasi Penjemputan Unit Retur')
    const btn = w.findAll('button').find((b) => b.text().includes('Konfirmasi Penjemputan'))
    expect(btn).toBeDefined()
    await btn!.trigger('click')
    await flushPromises()

    expect(mockService.pickup).toHaveBeenCalledTimes(1)
  })
})
