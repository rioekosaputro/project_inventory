<script setup lang="ts">
// Halaman aksi tugas driver: GPS checkpoint, kamera+kompresi foto POD, gagal kirim, dan pickup retur (T3.4).
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import BackLink from '@/components/BackLink.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { toApiError, userMessage } from '@/lib/apiError'
import { formatDate } from '@/lib/format'
import { compressImageToJpeg, generateEventId } from '@/lib/imageCompress'
import { openPdfBlob } from '@/lib/pdf'
import { lpbService } from '@/services/lpbService'
import { trackingService, type DriverDeliveryTask } from '@/services/trackingService'

const route = useRoute()
const router = useRouter()
const toast = useToast()

const lpbId = computed(() => Number(route.params.id))
const task = ref<DriverDeliveryTask | null>(null)
const loading = ref(true)
const loadError = ref<string | null>(null)

// State GPS
const gpsLat = ref<number | null>(null)
const gpsLng = ref<number | null>(null)
const gpsAcc = ref<number | null>(null)
const gpsBusy = ref(false)
const gpsError = ref<string | null>(null)

// State Checkpoint
const checkpointBusy = ref<string | null>(null)

// State POD (DELIVERED)
const receiverName = ref('')
const podNotes = ref('')
const podFile = ref<File | null>(null)
const podPreviewUrl = ref<string | null>(null)
const compressing = ref(false)
const podBusy = ref(false)
const podEventId = ref(generateEventId())

// State Gagal Kirim (FAILED)
const showFailForm = ref(false)
const failReason = ref('')
const failBusy = ref(false)
const failEventId = ref(generateEventId())

// State Pickup Retur (RENTAL_RETURN)
const pickupNotes = ref('')
const pickupBusy = ref(false)
const pickupEventId = ref(generateEventId())

// State Cetak Surat Jalan
const printing = ref(false)

const isReturnPickup = computed(() => task.value?.sub_type === 'RENTAL_RETURN')

const mapsHref = computed(() => {
  const t = task.value
  if (!t) return null
  if (t.site_latitude != null && t.site_longitude != null) {
    return `https://www.google.com/maps/dir/?api=1&destination=${t.site_latitude},${t.site_longitude}`
  }
  const addr = [t.site_address, t.site_city].filter(Boolean).join(', ')
  return addr ? `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(addr)}` : null
})

async function loadTask(): Promise<void> {
  loading.value = true
  loadError.value = null
  try {
    const all = await trackingService.myDeliveries()
    task.value = all.find((t) => t.lpb_id === lpbId.value) ?? null
    if (!task.value) {
      loadError.value = 'Tugas tidak ditemukan atau sudah selesai.'
    }
  } catch (err) {
    loadError.value = userMessage(toApiError(err))
  } finally {
    loading.value = false
  }
}

function captureGPS(): Promise<boolean> {
  if (typeof navigator === 'undefined' || !navigator.geolocation) {
    gpsError.value = 'Perangkat ini tidak mendukung GPS browser.'
    return Promise.resolve(false)
  }
  gpsBusy.value = true
  gpsError.value = null
  return new Promise((resolve) => {
    navigator.geolocation.getCurrentPosition(
      (pos) => {
        gpsLat.value = Number(pos.coords.latitude.toFixed(7))
        gpsLng.value = Number(pos.coords.longitude.toFixed(7))
        gpsAcc.value = Math.round(pos.coords.accuracy)
        gpsBusy.value = false
        resolve(true)
      },
      (err) => {
        gpsBusy.value = false
        gpsError.value =
          err.code === err.PERMISSION_DENIED
            ? 'Izin lokasi ditolak. Aktifkan GPS untuk browser ini.'
            : 'Gagal mengambil lokasi GPS. Pastikan GPS aktif dan coba lagi.'
        resolve(false)
      },
      { enableHighAccuracy: true, timeout: 15_000, maximumAge: 10_000 },
    )
  })
}

async function ensureGPS(): Promise<boolean> {
  if (gpsLat.value != null && gpsLng.value != null) return true
  return captureGPS()
}

async function sendCheckpoint(status: 'PICKED_UP' | 'IN_TRANSIT' | 'ARRIVED'): Promise<void> {
  if (!(await ensureGPS()) || gpsLat.value == null || gpsLng.value == null) return
  checkpointBusy.value = status
  try {
    await trackingService.checkpoint(lpbId.value, {
      status,
      latitude: gpsLat.value,
      longitude: gpsLng.value,
      accuracy_m: gpsAcc.value ?? undefined,
      client_time: new Date().toISOString(),
      client_event_id: generateEventId(),
    })
    if (task.value) {
      task.value = { ...task.value, delivery_status: status }
    }
    toast.add({
      severity: 'success',
      summary: 'Checkpoint tercatat',
      detail: `Status diperbarui menjadi ${status}.`,
      life: 3000,
    })
  } catch (err) {
    toast.add({
      severity: 'error',
      summary: 'Gagal mencatat checkpoint',
      detail: userMessage(toApiError(err)),
      life: 5000,
    })
  } finally {
    checkpointBusy.value = null
  }
}

async function onPhotoSelected(event: Event): Promise<void> {
  const input = event.target as HTMLInputElement
  const raw = input.files?.[0]
  if (!raw) return
  compressing.value = true
  try {
    const compressed = await compressImageToJpeg(raw)
    if (podPreviewUrl.value) URL.revokeObjectURL(podPreviewUrl.value)
    podFile.value = compressed
    podPreviewUrl.value = URL.createObjectURL(compressed)
  } catch (err) {
    toast.add({
      severity: 'error',
      summary: 'Gagal memproses foto',
      detail: err instanceof Error ? err.message : 'Coba ambil ulang foto.',
      life: 4000,
    })
  } finally {
    compressing.value = false
  }
}

async function submitDeliver(): Promise<void> {
  if (receiverName.value.trim().length < 2) {
    toast.add({
      severity: 'warn',
      summary: 'Nama penerima wajib diisi',
      detail: 'Masukkan nama penerima minimal 2 karakter.',
      life: 4000,
    })
    return
  }
  if (!podFile.value) {
    toast.add({
      severity: 'warn',
      summary: 'Foto POD wajib diunggah',
      detail: 'Ambil foto bukti serah terima terlebih dahulu.',
      life: 4000,
    })
    return
  }
  if (!(await ensureGPS()) || gpsLat.value == null || gpsLng.value == null) return

  podBusy.value = true
  try {
    const fd = new FormData()
    fd.append('receiver_name', receiverName.value.trim())
    fd.append('latitude', String(gpsLat.value))
    fd.append('longitude', String(gpsLng.value))
    if (gpsAcc.value != null) fd.append('accuracy_m', String(gpsAcc.value))
    fd.append('client_time', new Date().toISOString())
    fd.append('client_event_id', podEventId.value)
    if (podNotes.value.trim()) fd.append('notes', podNotes.value.trim())
    fd.append('pod_photo', podFile.value, podFile.value.name)

    const res = await trackingService.deliver(lpbId.value, fd)
    toast.add({
      severity: res.distance_warning ? 'warn' : 'success',
      summary: 'Pengiriman selesai (POD)',
      detail: res.distance_warning ?? 'Bukti serah terima berhasil disimpan.',
      life: 5000,
    })
    await router.replace({ name: 'driver-deliveries' })
  } catch (err) {
    toast.add({
      severity: 'error',
      summary: 'Gagal menyimpan POD',
      detail: userMessage(toApiError(err)),
      life: 5000,
    })
  } finally {
    podBusy.value = false
  }
}

async function submitFail(): Promise<void> {
  if (failReason.value.trim().length < 3) {
    toast.add({
      severity: 'warn',
      summary: 'Alasan wajib diisi',
      detail: 'Jelaskan alasan gagal kirim minimal 3 karakter.',
      life: 4000,
    })
    return
  }
  if (!(await ensureGPS()) || gpsLat.value == null || gpsLng.value == null) return

  failBusy.value = true
  try {
    await trackingService.fail(lpbId.value, {
      reason: failReason.value.trim(),
      latitude: gpsLat.value,
      longitude: gpsLng.value,
      accuracy_m: gpsAcc.value ?? undefined,
      client_time: new Date().toISOString(),
      client_event_id: failEventId.value,
    })
    toast.add({
      severity: 'info',
      summary: 'Gagal kirim dilaporkan',
      detail: 'Status pengiriman dicatat sebagai FAILED.',
      life: 4000,
    })
    await router.replace({ name: 'driver-deliveries' })
  } catch (err) {
    toast.add({
      severity: 'error',
      summary: 'Gagal melaporkan kendala',
      detail: userMessage(toApiError(err)),
      life: 5000,
    })
  } finally {
    failBusy.value = false
  }
}

async function submitPickup(): Promise<void> {
  if (!(await ensureGPS()) || gpsLat.value == null || gpsLng.value == null) return
  pickupBusy.value = true
  try {
    await trackingService.pickup(lpbId.value, {
      latitude: gpsLat.value,
      longitude: gpsLng.value,
      accuracy_m: gpsAcc.value ?? undefined,
      client_time: new Date().toISOString(),
      client_event_id: pickupEventId.value,
      notes: pickupNotes.value.trim() || undefined,
    })
    toast.add({
      severity: 'success',
      summary: 'Penjemputan tercatat',
      detail: 'Unit ditandai PICKED_UP dan siap dibawa kembali ke gudang.',
      life: 4000,
    })
    await router.replace({ name: 'driver-deliveries' })
  } catch (err) {
    toast.add({
      severity: 'error',
      summary: 'Gagal mencatat penjemputan',
      detail: userMessage(toApiError(err)),
      life: 5000,
    })
  } finally {
    pickupBusy.value = false
  }
}

async function printSuratJalan(): Promise<void> {
  if (!task.value) return
  printing.value = true
  try {
    const blob = await lpbService.fetchPdf(task.value.lpb_id)
    await openPdfBlob(blob, `${task.value.lpb_number.replace(/\//g, '-')}.pdf`)
  } catch (err) {
    toast.add({
      severity: 'error',
      summary: 'Gagal membuka Surat Jalan',
      detail: userMessage(toApiError(err)),
      life: 5000,
    })
  } finally {
    printing.value = false
  }
}

onMounted(() => {
  void loadTask()
  void captureGPS()
})

onUnmounted(() => {
  if (podPreviewUrl.value) URL.revokeObjectURL(podPreviewUrl.value)
})
</script>

<template>
  <div class="space-y-4">
    <BackLink to="/driver/deliveries" label="Kembali ke Daftar Tugas" />

    <div
      v-if="loading"
      class="rounded-xl border border-slate-200 bg-white p-8 text-center text-sm text-slate-500"
      role="status"
    >
      <i class="pi pi-spinner pi-spin mr-1.5" aria-hidden="true" />
      Memuat rincian tugas…
    </div>

    <div
      v-else-if="loadError || !task"
      role="alert"
      class="rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800"
    >
      {{ loadError ?? 'Tugas tidak ditemukan.' }}
    </div>

    <template v-else>
      <!-- Ringkasan Tugas -->
      <section class="rounded-xl border border-slate-200 bg-white p-4 shadow-xs">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div>
            <p class="font-mono text-base font-bold text-slate-900">{{ task.lpb_number }}</p>
            <p class="text-xs text-slate-500">
              {{ task.warehouse_name }} · {{ formatDate(task.doc_date) }}
            </p>
          </div>
          <StatusBadge :value="task.delivery_status ?? 'PENDING'" />
        </div>

        <div class="mt-3 space-y-1 text-sm text-slate-700">
          <p class="font-semibold text-slate-900">
            {{ task.customer_name ?? 'Pelanggan' }}
            <span v-if="task.site_name" class="font-normal text-slate-600"
              >— {{ task.site_name }}</span
            >
          </p>
          <p v-if="task.site_address || task.site_city" class="text-slate-600">
            {{ [task.site_address, task.site_city].filter(Boolean).join(', ') }}
          </p>
          <p v-if="task.site_pic_name" class="text-xs text-slate-600">
            PIC: {{ task.site_pic_name }}
            <span v-if="task.site_pic_phone">({{ task.site_pic_phone }})</span>
          </p>
          <p class="text-xs text-slate-500">
            Total muatan: {{ task.item_count }} item ({{ task.total_qty }} qty)
          </p>
        </div>

        <div class="mt-4 flex flex-wrap gap-2">
          <a
            v-if="mapsHref"
            :href="mapsHref"
            target="_blank"
            rel="noopener noreferrer"
            class="inline-flex min-h-11 flex-1 items-center justify-center gap-1.5 rounded-lg border border-slate-300 bg-white px-3 text-sm font-medium text-slate-700 hover:bg-slate-50"
          >
            <i class="pi pi-directions" aria-hidden="true" />
            Buka Google Maps
          </a>
          <a
            v-if="task.site_pic_phone"
            :href="`tel:${task.site_pic_phone}`"
            class="inline-flex min-h-11 items-center justify-center gap-1.5 rounded-lg border border-slate-300 bg-white px-3 text-sm font-medium text-slate-700 hover:bg-slate-50"
          >
            <i class="pi pi-phone" aria-hidden="true" />
            Telepon PIC
          </a>
          <button
            v-if="!isReturnPickup"
            type="button"
            :disabled="printing"
            class="inline-flex min-h-11 items-center justify-center gap-1.5 rounded-lg border border-slate-300 bg-white px-3 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-60"
            @click="printSuratJalan"
          >
            <i class="pi pi-file-pdf" aria-hidden="true" />
            {{ printing ? 'Menyiapkan…' : 'Surat Jalan' }}
          </button>
        </div>
      </section>

      <!-- Lokasi GPS -->
      <section class="rounded-xl border border-slate-200 bg-white p-4 shadow-xs">
        <div class="flex items-center justify-between gap-2">
          <h2 class="text-sm font-semibold text-slate-900">Lokasi GPS Saat Ini</h2>
          <button
            type="button"
            :disabled="gpsBusy"
            class="inline-flex min-h-10 items-center gap-1.5 rounded-lg border border-slate-300 bg-white px-3 text-xs font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-60"
            @click="captureGPS"
          >
            <i class="pi pi-map-marker" :class="{ 'pi-spin': gpsBusy }" aria-hidden="true" />
            {{ gpsBusy ? 'Mengambil…' : 'Perbarui GPS' }}
          </button>
        </div>
        <p v-if="gpsLat != null && gpsLng != null" class="mt-2 font-mono text-xs text-emerald-700">
          {{ gpsLat }}, {{ gpsLng }}
          <span v-if="gpsAcc != null" class="text-slate-500">(akurasi ±{{ gpsAcc }} m)</span>
        </p>
        <p v-else-if="gpsError" class="mt-2 text-xs text-rose-700" role="alert">{{ gpsError }}</p>
        <p v-else class="mt-2 text-xs text-slate-500">Menunggu koordinat GPS…</p>
      </section>

      <!-- Alur Penjemputan Retur -->
      <section
        v-if="isReturnPickup"
        class="space-y-3 rounded-xl border border-amber-200 bg-white p-4 shadow-xs"
      >
        <h2 class="text-sm font-semibold text-slate-900">Konfirmasi Penjemputan Unit Retur</h2>
        <div>
          <label for="pickup-notes" class="block text-xs font-medium text-slate-700"
            >Catatan Penjemputan (opsional)</label
          >
          <textarea
            id="pickup-notes"
            v-model="pickupNotes"
            rows="2"
            placeholder="Mis. unit lengkap dengan kabel power"
            class="mt-1 w-full rounded-lg border border-slate-300 px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none"
          />
        </div>
        <button
          type="button"
          :disabled="pickupBusy"
          class="inline-flex min-h-12 w-full items-center justify-center gap-2 rounded-lg bg-amber-600 px-4 text-sm font-semibold text-white hover:bg-amber-700 disabled:opacity-60"
          @click="submitPickup"
        >
          <i class="pi pi-box" aria-hidden="true" />
          {{ pickupBusy ? 'Menyimpan…' : 'Konfirmasi Penjemputan (PICKED_UP)' }}
        </button>
      </section>

      <!-- Alur Pengiriman Outbound -->
      <template v-else>
        <!-- Checkpoint Cepat -->
        <section class="space-y-3 rounded-xl border border-slate-200 bg-white p-4 shadow-xs">
          <h2 class="text-sm font-semibold text-slate-900">Checkpoint Perjalanan</h2>
          <div class="grid grid-cols-1 gap-2 sm:grid-cols-3">
            <button
              type="button"
              :disabled="checkpointBusy !== null"
              class="inline-flex min-h-11 items-center justify-center gap-1.5 rounded-lg border border-indigo-200 bg-indigo-50 px-3 text-sm font-medium text-indigo-800 hover:bg-indigo-100 disabled:opacity-60"
              @click="sendCheckpoint('PICKED_UP')"
            >
              <i class="pi pi-box" aria-hidden="true" />
              {{ checkpointBusy === 'PICKED_UP' ? 'Menyimpan…' : 'Muat Barang' }}
            </button>
            <button
              type="button"
              :disabled="checkpointBusy !== null"
              class="inline-flex min-h-11 items-center justify-center gap-1.5 rounded-lg border border-indigo-200 bg-indigo-50 px-3 text-sm font-medium text-indigo-800 hover:bg-indigo-100 disabled:opacity-60"
              @click="sendCheckpoint('IN_TRANSIT')"
            >
              <i class="pi pi-truck" aria-hidden="true" />
              {{ checkpointBusy === 'IN_TRANSIT' ? 'Menyimpan…' : 'Dalam Perjalanan' }}
            </button>
            <button
              type="button"
              :disabled="checkpointBusy !== null"
              class="inline-flex min-h-11 items-center justify-center gap-1.5 rounded-lg border border-indigo-200 bg-indigo-50 px-3 text-sm font-medium text-indigo-800 hover:bg-indigo-100 disabled:opacity-60"
              @click="sendCheckpoint('ARRIVED')"
            >
              <i class="pi pi-flag" aria-hidden="true" />
              {{ checkpointBusy === 'ARRIVED' ? 'Menyimpan…' : 'Tiba di Lokasi' }}
            </button>
          </div>
        </section>

        <!-- Form Bukti Serah Terima (POD) -->
        <section class="space-y-3 rounded-xl border border-slate-200 bg-white p-4 shadow-xs">
          <h2 class="text-sm font-semibold text-slate-900">Bukti Serah Terima (POD)</h2>

          <div>
            <label for="receiver-name" class="block text-xs font-medium text-slate-700"
              >Nama Penerima *</label
            >
            <input
              id="receiver-name"
              v-model="receiverName"
              type="text"
              autocomplete="name"
              placeholder="Nama lengkap penerima di lokasi"
              class="mt-1 min-h-11 w-full rounded-lg border border-slate-300 px-3 text-sm focus:border-indigo-500 focus:outline-none"
            />
          </div>

          <div>
            <label for="pod-photo" class="block text-xs font-medium text-slate-700"
              >Foto Bukti Serah Terima (otomatis dikompres &lt; 300 KB) *</label
            >
            <input
              id="pod-photo"
              type="file"
              accept="image/*"
              capture="environment"
              class="mt-1 block w-full text-sm text-slate-600 file:mr-3 file:min-h-10 file:rounded-lg file:border-0 file:bg-indigo-50 file:px-3 file:text-sm file:font-semibold file:text-indigo-700 hover:file:bg-indigo-100"
              @change="onPhotoSelected"
            />
            <p v-if="compressing" class="mt-1 text-xs text-slate-500">Mengompres foto…</p>
            <div v-if="podPreviewUrl && podFile" class="mt-2 space-y-1">
              <img
                :src="podPreviewUrl"
                alt="Pratinjau foto POD"
                class="max-h-56 w-full rounded-lg border border-slate-200 object-contain"
              />
              <p class="text-xs text-slate-500">
                Ukuran siap unggah: {{ Math.max(1, Math.round(podFile.size / 1024)) }} KB
              </p>
            </div>
          </div>

          <div>
            <label for="pod-notes" class="block text-xs font-medium text-slate-700"
              >Catatan (opsional)</label
            >
            <textarea
              id="pod-notes"
              v-model="podNotes"
              rows="2"
              placeholder="Catatan kondisi serah terima"
              class="mt-1 w-full rounded-lg border border-slate-300 px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none"
            />
          </div>

          <button
            type="button"
            :disabled="podBusy || compressing"
            class="inline-flex min-h-12 w-full items-center justify-center gap-2 rounded-lg bg-emerald-600 px-4 text-sm font-semibold text-white hover:bg-emerald-700 disabled:opacity-60"
            @click="submitDeliver"
          >
            <i class="pi pi-check-circle" aria-hidden="true" />
            {{ podBusy ? 'Mengunggah POD…' : 'Selesaikan Pengiriman (POD)' }}
          </button>
        </section>

        <!-- Laporkan Gagal Kirim -->
        <section class="rounded-xl border border-rose-200 bg-white p-4 shadow-xs">
          <button
            type="button"
            class="flex w-full items-center justify-between text-left text-sm font-semibold text-rose-700"
            @click="showFailForm = !showFailForm"
          >
            <span>Laporkan Gagal Kirim</span>
            <i
              class="pi"
              :class="showFailForm ? 'pi-chevron-up' : 'pi-chevron-down'"
              aria-hidden="true"
            />
          </button>
          <div v-if="showFailForm" class="mt-3 space-y-3">
            <div>
              <label for="fail-reason" class="block text-xs font-medium text-slate-700"
                >Alasan Gagal Kirim *</label
              >
              <textarea
                id="fail-reason"
                v-model="failReason"
                rows="2"
                placeholder="Mis. kantor pelanggan tutup / PIC tidak di tempat"
                class="mt-1 w-full rounded-lg border border-slate-300 px-3 py-2 text-sm focus:border-rose-500 focus:outline-none"
              />
            </div>
            <button
              type="button"
              :disabled="failBusy"
              class="inline-flex min-h-11 w-full items-center justify-center gap-2 rounded-lg bg-rose-600 px-4 text-sm font-semibold text-white hover:bg-rose-700 disabled:opacity-60"
              @click="submitFail"
            >
              <i class="pi pi-times-circle" aria-hidden="true" />
              {{ failBusy ? 'Mengirim…' : 'Kirim Laporan Gagal Kirim' }}
            </button>
          </div>
        </section>
      </template>
    </template>
  </div>
</template>
