import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import AssetTrackingMapView from '@/views/tracking/AssetTrackingMapView.vue'
import type { MapSitePin, TrackingRoute } from '@/services/trackingService'

const mockService = vi.hoisted(() => ({
  mapAssets: vi.fn<() => Promise<MapSitePin[]>>(),
  route: vi.fn<() => Promise<TrackingRoute>>(),
}))

vi.mock('@/services/trackingService', () => ({
  trackingService: mockService,
}))

vi.mock('leaflet', () => {
  const layerGroup = {
    addTo: () => layerGroup,
    clearLayers: () => {},
  }
  const marker = {
    bindTooltip: () => marker,
    on: () => marker,
    addTo: () => marker,
  }
  const mapObj = {
    setView: () => mapObj,
    fitBounds: () => mapObj,
    invalidateSize: () => {},
    remove: () => {},
  }
  return {
    default: {
      map: () => mapObj,
      tileLayer: () => ({ addTo: () => {} }),
      layerGroup: () => layerGroup,
      divIcon: () => ({}),
      marker: () => marker,
      polyline: () => ({ addTo: () => {} }),
      latLngBounds: () => ({}),
    },
  }
})

describe('AssetTrackingMapView (T3.7)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('memuat pin peta aset RENTED dan menampilkan daftar unit pada cabang terpilih', async () => {
    mockService.mapAssets.mockResolvedValue([
      {
        site_id: 1,
        site_name: 'Kantor Pusat',
        customer_id: 10,
        customer_name: 'PT Maju Bersama',
        address_line: 'Jl. Pandanaran 10',
        city: 'Semarang',
        province: 'Jawa Tengah',
        pic_name: 'Andi',
        pic_phone: '081234',
        latitude: -6.9932,
        longitude: 110.4203,
        unit_count: 1,
        units: [
          {
            serial_id: 42,
            asset_tag: 'AST-2026-0042',
            serial_number: 'SN-0042',
            sku: 'PRT-01',
            product_name: 'Printer Laser A4',
            contract_id: 7,
          },
        ],
      },
    ])

    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/tracking/map', component: AssetTrackingMapView },
        { path: '/stok/serial/:id', component: { template: '<div />' } },
        { path: '/lpb/:id', component: { template: '<div />' } },
      ],
    })
    await router.push('/tracking/map')
    await router.isReady()

    const w = mount(AssetTrackingMapView, { global: { plugins: [router] } })
    await flushPromises()

    expect(w.text()).toContain('PT Maju Bersama')
    expect(w.text()).toContain('AST-2026-0042')
    expect(w.text()).toContain('Printer Laser A4')
  })

  it('memuat rute checkpoint dan foto POD ketika dibuka dengan query ?lpb_id=', async () => {
    mockService.mapAssets.mockResolvedValue([])
    mockService.route.mockResolvedValue({
      lpb_id: 99,
      lpb_number: 'LPB/OUT/2026/0099',
      delivery_status: 'DELIVERED',
      receiver_name: 'Siti',
      delivered_at: '2026-11-28T03:00:00Z',
      pod_photo_url: 'https://example.test/pod.jpg',
      logs: [
        {
          id: 1,
          lpb_id: 99,
          driver_id: 5,
          driver_name: 'Budi',
          status: 'DELIVERED',
          latitude: -6.9932,
          longitude: 110.4203,
          accuracy_m: 15,
          recorded_at: '2026-11-28T03:00:00Z',
          client_time: null,
          client_event_id: null,
          receiver_name: 'Siti',
          photo_path: 'pod/x.jpg',
          failure_reason: null,
          notes: 'Diterima resepsionis',
        },
      ],
    })

    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/tracking/map', component: AssetTrackingMapView },
        { path: '/lpb/:id', component: { template: '<div />' } },
      ],
    })
    await router.push('/tracking/map?lpb_id=99')
    await router.isReady()

    const w = mount(AssetTrackingMapView, { global: { plugins: [router] } })
    await flushPromises()

    expect(mockService.route).toHaveBeenCalledWith(99)
    expect(w.text()).toContain('LPB/OUT/2026/0099')
    expect(w.text()).toContain('Siti')
    expect(w.find('img[src="https://example.test/pod.jpg"]').exists()).toBe(true)
  })
})
