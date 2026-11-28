<script setup lang="ts">
// Peta sebaran aset RENTED per cabang pelanggan + pelacakan rute checkpoint & foto POD (T3.7).
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import StatusBadge from '@/components/StatusBadge.vue'
import { toApiError, userMessage } from '@/lib/apiError'
import { formatDateTime } from '@/lib/format'
import { btnPrimary, btnSecondary } from '@/lib/ui'
import {
  trackingService,
  type MapSitePin,
  type TrackingRoute,
} from '@/services/trackingService'

const DEFAULT_CENTER: [number, number] = [-6.9932, 110.4203] // Semarang
const TILE_URL =
  (import.meta.env.VITE_MAP_TILE_URL as string | undefined) ||
  'https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png'

const route = useRoute()

const activeTab = ref<'assets' | 'route'>('assets')

// Mode Peta Aset
const searchQuery = ref('')
const pins = ref<MapSitePin[]>([])
const loadingPins = ref(true)
const pinsError = ref<string | null>(null)
const selectedPin = ref<MapSitePin | null>(null)

// Mode Rute Pengiriman & Foto POD
const routeLpbIdInput = ref('')
const routeData = ref<TrackingRoute | null>(null)
const loadingRoute = ref(false)
const routeError = ref<string | null>(null)

// Leaflet instances
const mapContainer = ref<HTMLDivElement | null>(null)
let map: L.Map | null = null
let layerGroup: L.LayerGroup | null = null

const totalUnits = computed(() => pins.value.reduce((acc, p) => acc + p.unit_count, 0))
const mappedPins = computed(() =>
  pins.value.filter((p) => p.latitude != null && p.longitude != null),
)
const unmappedCount = computed(() => pins.value.length - mappedPins.value.length)

function initMap(): void {
  if (!mapContainer.value || map) return
  map = L.map(mapContainer.value, { center: DEFAULT_CENTER, zoom: 11 })
  L.tileLayer(TILE_URL, {
    maxZoom: 19,
    attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
  }).addTo(map)
  layerGroup = L.layerGroup().addTo(map)
}

function makeAssetDivIcon(count: number, selected: boolean): L.DivIcon {
  const bg = selected ? '#312e81' : '#4338ca'
  const ring = selected ? '0 0 0 4px rgba(67,56,202,0.28)' : '0 2px 6px rgba(15,23,42,0.35)'
  return L.divIcon({
    className: 'erp-asset-pin',
    html: `<div style="display:flex;align-items:center;justify-content:center;min-width:32px;height:32px;padding:0 6px;border-radius:9999px;background:${bg};color:#fff;font-weight:700;font-size:12px;border:2px solid #fff;box-shadow:${ring};">${count}</div>`,
    iconSize: [32, 32],
    iconAnchor: [16, 16],
  })
}

function makeCheckpointDivIcon(index: number, status: string): L.DivIcon {
  const bg =
    status === 'DELIVERED' ? '#059669' : status === 'FAILED' ? '#dc2626' : '#4338ca'
  return L.divIcon({
    className: 'erp-route-pin',
    html: `<div style="display:flex;align-items:center;justify-content:center;width:28px;height:28px;border-radius:9999px;background:${bg};color:#fff;font-weight:700;font-size:12px;border:2px solid #fff;box-shadow:0 2px 6px rgba(15,23,42,0.35);">${index + 1}</div>`,
    iconSize: [28, 28],
    iconAnchor: [14, 14],
  })
}

function renderMapLayers(): void {
  if (!map || !layerGroup) return
  layerGroup.clearLayers()

  if (activeTab.value === 'assets') {
    const coords: [number, number][] = []
    for (const p of mappedPins.value) {
      const lat = p.latitude!
      const lng = p.longitude!
      coords.push([lat, lng])
      const marker = L.marker([lat, lng], {
        icon: makeAssetDivIcon(p.unit_count, selectedPin.value?.site_id === p.site_id),
      })
      marker.bindTooltip(`${p.customer_name} — ${p.site_name} (${p.unit_count} unit)`)
      marker.on('click', () => {
        selectedPin.value = p
      })
      marker.addTo(layerGroup)
    }
    if (coords.length === 1) {
      map.setView(coords[0]!, 14)
    } else if (coords.length > 1) {
      map.fitBounds(L.latLngBounds(coords), { padding: [36, 36] })
    }
    return
  }

  // Mode Rute Pengiriman
  const logs = routeData.value?.logs ?? []
  if (logs.length === 0) return
  const coords: [number, number][] = []
  logs.forEach((lg, idx) => {
    const pt: [number, number] = [lg.latitude, lg.longitude]
    coords.push(pt)
    const m = L.marker(pt, { icon: makeCheckpointDivIcon(idx, lg.status) })
    m.bindTooltip(`#${idx + 1} ${lg.status} · ${formatDateTime(lg.recorded_at)}`)
    m.addTo(layerGroup!)
  })
  if (coords.length > 1) {
    L.polyline(coords, { color: '#4338ca', weight: 4, opacity: 0.85 }).addTo(layerGroup)
    map.fitBounds(L.latLngBounds(coords), { padding: [36, 36] })
  } else if (coords.length === 1) {
    map.setView(coords[0]!, 15)
  }
}

async function loadAssets(): Promise<void> {
  loadingPins.value = true
  pinsError.value = null
  try {
    pins.value = await trackingService.mapAssets(
      searchQuery.value.trim() ? { q: searchQuery.value.trim() } : undefined,
    )
    if (selectedPin.value) {
      selectedPin.value =
        pins.value.find((p) => p.site_id === selectedPin.value?.site_id) ?? null
    }
    if (!selectedPin.value && pins.value.length > 0) {
      selectedPin.value = pins.value[0] ?? null
    }
    renderMapLayers()
  } catch (err) {
    pinsError.value = userMessage(toApiError(err))
  } finally {
    loadingPins.value = false
  }
}

async function loadRoute(): Promise<void> {
  const id = Number(routeLpbIdInput.value)
  if (!Number.isInteger(id) || id <= 0) {
    routeError.value = 'Masukkan ID LPB yang valid (angka positif).'
    return
  }
  loadingRoute.value = true
  routeError.value = null
  try {
    routeData.value = await trackingService.route(id)
    renderMapLayers()
  } catch (err) {
    routeData.value = null
    routeError.value = userMessage(toApiError(err))
  } finally {
    loadingRoute.value = false
  }
}

function selectSite(p: MapSitePin): void {
  selectedPin.value = p
  if (map && p.latitude != null && p.longitude != null) {
    map.setView([p.latitude, p.longitude], 15)
  }
  renderMapLayers()
}

watch(activeTab, async () => {
  await nextTick()
  map?.invalidateSize()
  renderMapLayers()
})

onMounted(async () => {
  await nextTick()
  initMap()

  const qLpbId = route.query.lpb_id
  if (typeof qLpbId === 'string' && /^\d+$/.test(qLpbId)) {
    routeLpbIdInput.value = qLpbId
    activeTab.value = 'route'
    await loadRoute()
  }
  await loadAssets()
})

onBeforeUnmount(() => {
  if (map) {
    map.remove()
    map = null
    layerGroup = null
  }
})
</script>

<template>
  <div class="space-y-4">
    <header class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 class="text-xl font-bold text-slate-900">Peta Aset &amp; Rute Pengiriman</h1>
        <p class="mt-0.5 text-sm text-slate-600">
          Sebaran unit tersewa (RENTED) per cabang pelanggan serta riwayat checkpoint GPS &amp; foto
          POD.
        </p>
      </div>
      <div class="inline-flex rounded-lg border border-slate-200 bg-white p-1">
        <button
          type="button"
          class="rounded-md px-3 py-1.5 text-sm font-medium transition"
          :class="
            activeTab === 'assets'
              ? 'bg-indigo-600 text-white'
              : 'text-slate-700 hover:bg-slate-100'
          "
          @click="activeTab = 'assets'"
        >
          Sebaran Aset
        </button>
        <button
          type="button"
          class="rounded-md px-3 py-1.5 text-sm font-medium transition"
          :class="
            activeTab === 'route'
              ? 'bg-indigo-600 text-white'
              : 'text-slate-700 hover:bg-slate-100'
          "
          @click="activeTab = 'route'"
        >
          Rute &amp; Foto POD
        </button>
      </div>
    </header>

    <!-- Toolbar Filter -->
    <div
      v-if="activeTab === 'assets'"
      class="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-slate-200 bg-white p-3"
    >
      <form class="flex flex-1 flex-wrap items-center gap-2" @submit.prevent="loadAssets">
        <input
          v-model="searchQuery"
          type="search"
          placeholder="Cari pelanggan, cabang, kota, asset tag, atau SN…"
          class="min-h-10 min-w-60 flex-1 rounded-lg border border-slate-300 px-3 text-sm focus:border-indigo-500 focus:outline-none"
        />
        <button type="submit" :class="btnPrimary" :disabled="loadingPins">
          <i class="pi pi-search text-xs" aria-hidden="true" />
          Cari
        </button>
      </form>
      <div class="flex flex-wrap items-center gap-3 text-xs text-slate-600">
        <span><strong>{{ pins.length }}</strong> lokasi</span>
        <span><strong>{{ totalUnits }}</strong> unit tersewa</span>
        <span v-if="unmappedCount > 0" class="text-amber-700">
          {{ unmappedCount }} lokasi belum punya koordinat GPS
        </span>
      </div>
    </div>

    <div
      v-else
      class="flex flex-wrap items-center gap-2 rounded-xl border border-slate-200 bg-white p-3"
    >
      <form class="flex flex-wrap items-center gap-2" @submit.prevent="loadRoute">
        <label for="route-lpb-id" class="text-sm font-medium text-slate-700">ID LPB:</label>
        <input
          id="route-lpb-id"
          v-model="routeLpbIdInput"
          type="number"
          min="1"
          placeholder="Contoh: 12"
          class="min-h-10 w-36 rounded-lg border border-slate-300 px-3 text-sm focus:border-indigo-500 focus:outline-none"
        />
        <button type="submit" :class="btnPrimary" :disabled="loadingRoute">
          <i class="pi pi-directions text-xs" aria-hidden="true" />
          {{ loadingRoute ? 'Memuat…' : 'Tampilkan Rute' }}
        </button>
      </form>
    </div>

    <!-- Layout Peta + Panel Detail -->
    <div class="grid gap-4 lg:grid-cols-3">
      <div class="overflow-hidden rounded-xl border border-slate-200 bg-white lg:col-span-2">
        <div ref="mapContainer" class="h-[460px] w-full" aria-label="Peta interaktif Leaflet" />
      </div>

      <!-- Panel Samping: Aset -->
      <aside
        v-if="activeTab === 'assets'"
        class="flex max-h-[460px] flex-col rounded-xl border border-slate-200 bg-white"
      >
        <div class="border-b border-slate-200 px-4 py-3">
          <h2 class="text-sm font-semibold text-slate-900">
            {{ selectedPin ? `${selectedPin.customer_name} — ${selectedPin.site_name}` : 'Daftar Lokasi' }}
          </h2>
          <p v-if="selectedPin" class="mt-0.5 text-xs text-slate-500">
            {{ selectedPin.address_line }}, {{ selectedPin.city }}
          </p>
        </div>

        <div class="flex-1 space-y-3 overflow-y-auto p-4">
          <p v-if="pinsError" role="alert" class="text-sm text-rose-700">{{ pinsError }}</p>
          <p v-else-if="loadingPins" class="text-sm text-slate-500" role="status">Memuat peta…</p>
          <p v-else-if="pins.length === 0" class="text-sm text-slate-500">
            Belum ada unit tersewa (RENTED) yang cocok dengan pencarian.
          </p>

          <template v-else>
            <div v-if="selectedPin" class="space-y-2 rounded-lg border border-indigo-100 bg-indigo-50/40 p-3">
              <div class="flex items-center justify-between gap-2 text-xs text-slate-600">
                <span>PIC: <strong>{{ selectedPin.pic_name }}</strong> ({{ selectedPin.pic_phone }})</span>
                <span class="rounded bg-indigo-100 px-2 py-0.5 font-semibold text-indigo-800">
                  {{ selectedPin.unit_count }} unit
                </span>
              </div>
              <ul class="divide-y divide-indigo-100 text-xs">
                <li
                  v-for="u in selectedPin.units"
                  :key="u.serial_id"
                  class="flex items-center justify-between gap-2 py-1.5"
                >
                  <div>
                    <RouterLink
                      :to="`/stok/serial/${u.serial_id}`"
                      class="font-mono font-semibold text-indigo-700 hover:underline"
                    >
                      {{ u.asset_tag }}
                    </RouterLink>
                    <span class="ml-1.5 text-slate-500">({{ u.serial_number }})</span>
                    <p class="text-slate-700">{{ u.product_name }}</p>
                  </div>
                </li>
              </ul>
            </div>

            <div class="space-y-1.5">
              <p class="text-xs font-semibold uppercase tracking-wide text-slate-500">
                Semua Cabang ({{ pins.length }})
              </p>
              <button
                v-for="p in pins"
                :key="p.site_id"
                type="button"
                class="flex w-full items-center justify-between rounded-lg border px-3 py-2 text-left text-xs transition"
                :class="
                  selectedPin?.site_id === p.site_id
                    ? 'border-indigo-400 bg-indigo-50 text-indigo-900'
                    : 'border-slate-200 bg-white text-slate-700 hover:bg-slate-50'
                "
                @click="selectSite(p)"
              >
                <div class="min-w-0 pr-2">
                  <p class="truncate font-semibold">{{ p.customer_name }}</p>
                  <p class="truncate text-slate-500">{{ p.site_name }} · {{ p.city }}</p>
                </div>
                <span class="shrink-0 rounded-full bg-slate-100 px-2 py-0.5 font-semibold text-slate-700">
                  {{ p.unit_count }}
                </span>
              </button>
            </div>
          </template>
        </div>
      </aside>

      <!-- Panel Samping: Rute & Foto POD -->
      <aside
        v-else
        class="flex max-h-[460px] flex-col rounded-xl border border-slate-200 bg-white"
      >
        <div class="border-b border-slate-200 px-4 py-3">
          <div class="flex items-center justify-between gap-2">
            <h2 class="text-sm font-semibold text-slate-900">
              {{ routeData ? routeData.lpb_number : 'Detail Rute & POD' }}
            </h2>
            <StatusBadge
              v-if="routeData"
              :value="routeData.delivery_status ?? 'PENDING'"
            />
          </div>
        </div>

        <div class="flex-1 space-y-3 overflow-y-auto p-4 text-sm">
          <p v-if="routeError" role="alert" class="text-rose-700">{{ routeError }}</p>
          <p v-else-if="!routeData" class="text-slate-500">
            Masukkan ID LPB pengiriman di atas untuk melihat polyline checkpoint GPS dan foto POD.
          </p>
          <template v-else>
            <div v-if="routeData.receiver_name" class="rounded-lg bg-emerald-50 p-3 text-xs text-emerald-900">
              <p>Penerima: <strong>{{ routeData.receiver_name }}</strong></p>
              <p v-if="routeData.delivered_at">
                Waktu POD: {{ formatDateTime(routeData.delivered_at) }}
              </p>
            </div>

            <div v-if="routeData.pod_photo_url" class="space-y-1.5">
              <p class="text-xs font-semibold text-slate-700">Foto Bukti Serah Terima (POD)</p>
              <a :href="routeData.pod_photo_url" target="_blank" rel="noopener noreferrer">
                <img
                  :src="routeData.pod_photo_url"
                  alt="Foto bukti serah terima (POD)"
                  class="max-h-48 w-full rounded-lg border border-slate-200 object-contain"
                />
              </a>
            </div>

            <div class="space-y-2">
              <p class="text-xs font-semibold uppercase tracking-wide text-slate-500">
                Riwayat Checkpoint ({{ routeData.logs.length }})
              </p>
              <p v-if="routeData.logs.length === 0" class="text-xs text-slate-500">
                Belum ada checkpoint GPS yang tercatat untuk LPB ini.
              </p>
              <ol v-else class="space-y-2">
                <li
                  v-for="(lg, i) in routeData.logs"
                  :key="lg.id"
                  class="rounded-lg border border-slate-200 p-2.5 text-xs"
                >
                  <div class="flex items-center justify-between gap-2">
                    <span class="font-semibold text-slate-900">#{{ i + 1 }} {{ lg.status }}</span>
                    <span class="text-slate-500">{{ formatDateTime(lg.recorded_at) }}</span>
                  </div>
                  <p class="mt-1 font-mono text-[11px] text-slate-600">
                    {{ lg.latitude }}, {{ lg.longitude }}
                    <span v-if="lg.accuracy_m != null">(±{{ lg.accuracy_m }} m)</span>
                  </p>
                  <p v-if="lg.failure_reason" class="mt-1 text-rose-700">
                    Alasan: {{ lg.failure_reason }}
                  </p>
                  <p v-if="lg.notes" class="mt-1 text-slate-600">{{ lg.notes }}</p>
                </li>
              </ol>
            </div>

            <RouterLink
              :to="`/lpb/${routeData.lpb_id}`"
              :class="[btnSecondary, 'w-full justify-center']"
            >
              Buka Dokumen LPB
            </RouterLink>
          </template>
        </div>
      </aside>
    </div>
  </div>
</template>
