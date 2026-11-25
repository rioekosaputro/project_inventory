<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'

/**
 * Pemilih koordinat peta berbasis Leaflet + OpenStreetMap.
 * Klik di peta untuk menaruh atau memindahkan pin koordinat site.
 */
const props = withDefaults(
  defineProps<{
    latitude: number | null
    longitude: number | null
    height?: string
  }>(),
  { height: '240px' },
)

const emit = defineEmits<{
  'update:latitude': [value: number | null]
  'update:longitude': [value: number | null]
}>()

const mapEl = ref<HTMLDivElement | null>(null)
let map: L.Map | null = null
let marker: L.CircleMarker | null = null

const DEFAULT_LAT = -6.9825
const DEFAULT_LNG = 110.4091

function parseDefaultCenter(): [number, number] {
  const raw = import.meta.env.VITE_DEFAULT_CENTER as string | undefined
  if (raw) {
    const parts = raw.split(',').map((s) => Number(s.trim()))
    const lat = parts[0]
    const lng = parts[1]
    if (
      parts.length === 2 &&
      lat !== undefined &&
      lng !== undefined &&
      Number.isFinite(lat) &&
      Number.isFinite(lng)
    ) {
      return [lat, lng]
    }
  }
  return [DEFAULT_LAT, DEFAULT_LNG]
}

function round7(n: number): number {
  return Math.round(n * 1e7) / 1e7
}

function syncMarker(lat: number | null, lng: number | null, pan = false): void {
  if (!map) return
  if (lat === null || lng === null || !Number.isFinite(lat) || !Number.isFinite(lng)) {
    if (marker) {
      marker.remove()
      marker = null
    }
    return
  }
  const pos: L.LatLngExpression = [lat, lng]
  if (!marker) {
    marker = L.circleMarker(pos, {
      radius: 8,
      color: '#1d4ed8',
      weight: 3,
      fillColor: '#3b82f6',
      fillOpacity: 0.75,
    }).addTo(map)
  } else {
    marker.setLatLng(pos)
  }
  if (pan) {
    map.setView(pos, Math.max(map.getZoom(), 14))
  }
}

function clearPin(): void {
  emit('update:latitude', null)
  emit('update:longitude', null)
}

onMounted(() => {
  const el = mapEl.value
  if (!el) return
  const hasInitial =
    props.latitude !== null &&
    props.longitude !== null &&
    Number.isFinite(props.latitude) &&
    Number.isFinite(props.longitude)
  const center: [number, number] = hasInitial
    ? [props.latitude as number, props.longitude as number]
    : parseDefaultCenter()

  const tileUrl =
    (import.meta.env.VITE_MAP_TILE_URL as string | undefined) ||
    'https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png'
  const attribution =
    (import.meta.env.VITE_MAP_ATTRIBUTION as string | undefined) ||
    '&copy; OpenStreetMap contributors'

  map = L.map(el, { center, zoom: hasInitial ? 15 : 12 })
  L.tileLayer(tileUrl, { attribution, maxZoom: 19 }).addTo(map)

  if (hasInitial) {
    syncMarker(props.latitude, props.longitude, false)
  }

  map.on('click', (ev: L.LeafletMouseEvent) => {
    const lat = round7(ev.latlng.lat)
    const lng = round7(ev.latlng.lng)
    syncMarker(lat, lng, false)
    emit('update:latitude', lat)
    emit('update:longitude', lng)
  })

  // Pastikan ukuran peta terhitung benar bila dimuat di dalam dialog/panel.
  setTimeout(() => map?.invalidateSize(), 150)
})

watch(
  () => [props.latitude, props.longitude] as const,
  ([lat, lng]) => {
    syncMarker(lat, lng, true)
  },
)

onBeforeUnmount(() => {
  map?.remove()
  map = null
  marker = null
})
</script>

<template>
  <div class="space-y-2">
    <div class="flex items-center justify-between gap-2 text-xs text-slate-600">
      <span>Klik peta untuk menentukan titik koordinat lokasi site.</span>
      <button
        v-if="latitude !== null || longitude !== null"
        type="button"
        class="text-xs font-medium text-rose-700 hover:underline"
        @click="clearPin"
      >
        Hapus pin
      </button>
    </div>
    <div
      ref="mapEl"
      class="w-full overflow-hidden rounded-lg border border-slate-300 bg-slate-100"
      :style="{ height }"
      aria-label="Peta pemilih koordinat"
    />
  </div>
</template>
