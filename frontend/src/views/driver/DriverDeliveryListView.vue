<script setup lang="ts">
// Halaman daftar tugas aktif driver (PWA mobile-first, T3.4).
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import StatusBadge from '@/components/StatusBadge.vue'
import { toApiError, userMessage } from '@/lib/apiError'
import { formatDate } from '@/lib/format'
import { trackingService, type DriverDeliveryTask } from '@/services/trackingService'

const router = useRouter()
const tasks = ref<DriverDeliveryTask[]>([])
const loading = ref(true)
const error = ref<string | null>(null)
const isOnline = ref(typeof navigator !== 'undefined' ? navigator.onLine : true)

const deliveryCount = computed(() => tasks.value.filter((t) => t.sub_type !== 'RENTAL_RETURN').length)
const pickupCount = computed(() => tasks.value.filter((t) => t.sub_type === 'RENTAL_RETURN').length)

function onOnline(): void {
  isOnline.value = true
}
function onOffline(): void {
  isOnline.value = false
}

async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    tasks.value = await trackingService.myDeliveries()
  } catch (err) {
    error.value = userMessage(toApiError(err))
  } finally {
    loading.value = false
  }
}

function mapsUrl(t: DriverDeliveryTask): string | null {
  if (t.site_latitude != null && t.site_longitude != null) {
    return `https://www.google.com/maps/dir/?api=1&destination=${t.site_latitude},${t.site_longitude}`
  }
  const addr = [t.site_address, t.site_city].filter(Boolean).join(', ')
  if (!addr) return null
  return `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(addr)}`
}

function openTask(t: DriverDeliveryTask): void {
  void router.push({ name: 'driver-delivery-detail', params: { id: t.lpb_id } })
}

onMounted(() => {
  window.addEventListener('online', onOnline)
  window.addEventListener('offline', onOffline)
  void load()
})

onUnmounted(() => {
  window.removeEventListener('online', onOnline)
  window.removeEventListener('offline', onOffline)
})
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-start justify-between gap-3">
      <div>
        <h1 class="text-xl font-bold text-slate-900">Tugas Pengiriman</h1>
        <p class="mt-0.5 text-sm text-slate-600">
          {{ deliveryCount }} antar · {{ pickupCount }} jemput
        </p>
      </div>
      <div class="flex items-center gap-2">
        <span
          class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium"
          :class="isOnline ? 'bg-emerald-50 text-emerald-700' : 'bg-amber-100 text-amber-800'"
        >
          <span
            class="h-2 w-2 rounded-full"
            :class="isOnline ? 'bg-emerald-500' : 'bg-amber-500'"
            aria-hidden="true"
          />
          {{ isOnline ? 'Online' : 'Offline' }}
        </span>
        <button
          type="button"
          :disabled="loading"
          class="inline-flex min-h-11 items-center gap-1.5 rounded-lg border border-slate-300 bg-white px-3 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-60"
          @click="load"
        >
          <i class="pi pi-refresh" :class="{ 'pi-spin': loading }" aria-hidden="true" />
          Muat Ulang
        </button>
      </div>
    </div>

    <div
      v-if="error"
      role="alert"
      class="rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800"
    >
      {{ error }}
    </div>

    <div
      v-if="loading && tasks.length === 0"
      class="rounded-xl border border-slate-200 bg-white p-8 text-center text-sm text-slate-500"
      role="status"
    >
      <i class="pi pi-spinner pi-spin mr-1.5" aria-hidden="true" />
      Memuat daftar tugas…
    </div>

    <div
      v-else-if="tasks.length === 0"
      class="rounded-xl border border-dashed border-slate-300 bg-white p-8 text-center"
    >
      <i class="pi pi-check-circle text-3xl text-emerald-500" aria-hidden="true" />
      <p class="mt-2 font-medium text-slate-900">Tidak ada tugas aktif</p>
      <p class="mt-1 text-sm text-slate-500">
        Semua pengiriman dan penjemputan yang ditugaskan kepada Anda sudah selesai.
      </p>
    </div>

    <ul v-else class="space-y-3">
      <li
        v-for="t in tasks"
        :key="t.lpb_id"
        class="rounded-xl border border-slate-200 bg-white p-4 shadow-xs"
      >
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div class="flex items-center gap-2">
            <span
              class="inline-flex items-center rounded-md px-2 py-0.5 text-xs font-semibold"
              :class="
                t.sub_type === 'RENTAL_RETURN'
                  ? 'bg-amber-100 text-amber-800'
                  : 'bg-indigo-100 text-indigo-800'
              "
            >
              {{ t.sub_type === 'RENTAL_RETURN' ? 'JEMPUT RETUR' : 'KIRIM UNIT' }}
            </span>
            <span class="font-mono text-sm font-semibold text-slate-900">{{ t.lpb_number }}</span>
          </div>
          <StatusBadge :value="t.delivery_status ?? 'PENDING'" />
        </div>

        <div class="mt-3 space-y-1 text-sm text-slate-700">
          <p class="font-semibold text-slate-900">
            {{ t.customer_name ?? 'Pelanggan' }}
            <span v-if="t.site_name" class="font-normal text-slate-600">— {{ t.site_name }}</span>
          </p>
          <p v-if="t.site_address || t.site_city" class="text-slate-600">
            <i class="pi pi-map-marker mr-1 text-slate-400" aria-hidden="true" />
            {{ [t.site_address, t.site_city].filter(Boolean).join(', ') }}
          </p>
          <p class="text-xs text-slate-500">
            Gudang: {{ t.warehouse_name }} · {{ formatDate(t.doc_date) }} · {{ t.item_count }} item
            ({{ t.total_qty }} qty)
          </p>
        </div>

        <div class="mt-4 flex flex-wrap gap-2">
          <button
            type="button"
            class="inline-flex min-h-11 flex-1 items-center justify-center gap-2 rounded-lg bg-indigo-600 px-4 text-sm font-semibold text-white hover:bg-indigo-700"
            @click="openTask(t)"
          >
            <i class="pi pi-arrow-right" aria-hidden="true" />
            Buka Tugas
          </button>

          <a
            v-if="mapsUrl(t)"
            :href="mapsUrl(t)!"
            target="_blank"
            rel="noopener noreferrer"
            class="inline-flex min-h-11 items-center justify-center gap-1.5 rounded-lg border border-slate-300 bg-white px-3 text-sm font-medium text-slate-700 hover:bg-slate-50"
          >
            <i class="pi pi-directions" aria-hidden="true" />
            Navigasi
          </a>

          <a
            v-if="t.site_pic_phone"
            :href="`tel:${t.site_pic_phone}`"
            class="inline-flex min-h-11 items-center justify-center gap-1.5 rounded-lg border border-slate-300 bg-white px-3 text-sm font-medium text-slate-700 hover:bg-slate-50"
          >
            <i class="pi pi-phone" aria-hidden="true" />
            Hubungi PIC
          </a>
        </div>
      </li>
    </ul>
  </div>
</template>
