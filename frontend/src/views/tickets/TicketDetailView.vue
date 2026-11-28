<script setup lang="ts">
// Detail Tiket Maintenance: assign teknisi, ubah status, timeline komentar/GPS check-in/foto (T3.8).
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import BackLink from '@/components/BackLink.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { toApiError, userMessage } from '@/lib/apiError'
import { formatDateTime } from '@/lib/format'
import { compressImageToJpeg } from '@/lib/imageCompress'
import { btnPrimary, btnSecondary } from '@/lib/ui'
import { ticketService, type TicketDetail, type TicketStatus } from '@/services/ticketService'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const toast = useToast()
const auth = useAuthStore()

const id = computed(() => Number(route.params.id))
const ticket = ref<TicketDetail | null>(null)
const loading = ref(true)
const loadError = ref('')

// Assign
const assignTechId = ref('')
const assignBusy = ref(false)

// Status
const statusBusy = ref(false)
const resolutionNotes = ref('')

// Aktivitas
const commentText = ref('')
const activityBusy = ref(false)
const photoFile = ref<File | null>(null)

const canWrite = computed(() => auth.can('ticket.write'))
const canWork = computed(() => auth.can('ticket.work'))
const isClosedOrCancelled = computed(
  () => ticket.value?.status === 'CLOSED' || ticket.value?.status === 'CANCELLED',
)

const nextStatuses = computed<TicketStatus[]>(() => {
  switch (ticket.value?.status) {
    case 'OPEN':
      return ['IN_PROGRESS', 'CANCELLED']
    case 'ASSIGNED':
      return ['IN_PROGRESS', 'WAITING_PART', 'RESOLVED', 'CANCELLED']
    case 'IN_PROGRESS':
      return ['WAITING_PART', 'RESOLVED', 'CANCELLED']
    case 'WAITING_PART':
      return ['IN_PROGRESS', 'RESOLVED', 'CANCELLED']
    case 'RESOLVED':
      return ['CLOSED', 'IN_PROGRESS']
    default:
      return []
  }
})

async function load(): Promise<void> {
  loading.value = true
  loadError.value = ''
  try {
    ticket.value = await ticketService.get(id.value)
  } catch (err) {
    loadError.value = userMessage(toApiError(err))
  } finally {
    loading.value = false
  }
}

async function submitAssign(): Promise<void> {
  const techId = Number(assignTechId.value)
  if (!Number.isInteger(techId) || techId <= 0) return
  assignBusy.value = true
  try {
    ticket.value = await ticketService.assign(id.value, techId)
    assignTechId.value = ''
    toast.add({ severity: 'success', summary: 'Teknisi ditugaskan', life: 3000 })
  } catch (err) {
    toast.add({ severity: 'error', summary: 'Gagal assign', detail: userMessage(toApiError(err)), life: 5000 })
  } finally {
    assignBusy.value = false
  }
}

async function transitionTo(st: TicketStatus): Promise<void> {
  statusBusy.value = true
  try {
    ticket.value = await ticketService.changeStatus(id.value, {
      status: st,
      resolution_notes: st === 'RESOLVED' ? resolutionNotes.value.trim() || undefined : undefined,
    })
    resolutionNotes.value = ''
    toast.add({ severity: 'success', summary: `Status diubah ke ${st}`, life: 3000 })
  } catch (err) {
    toast.add({ severity: 'error', summary: 'Gagal ubah status', detail: userMessage(toApiError(err)), life: 5000 })
  } finally {
    statusBusy.value = false
  }
}

async function postComment(): Promise<void> {
  if (!commentText.value.trim()) return
  activityBusy.value = true
  try {
    await ticketService.addActivity(id.value, {
      activity_type: 'COMMENT',
      message: commentText.value.trim(),
    })
    commentText.value = ''
    await load()
  } catch (err) {
    toast.add({ severity: 'error', summary: 'Gagal kirim komentar', detail: userMessage(toApiError(err)), life: 5000 })
  } finally {
    activityBusy.value = false
  }
}

function postCheckin(type: 'VISIT_CHECKIN' | 'VISIT_CHECKOUT'): void {
  if (typeof navigator === 'undefined' || !navigator.geolocation) {
    toast.add({ severity: 'error', summary: 'GPS tidak tersedia di perangkat ini', life: 4000 })
    return
  }
  activityBusy.value = true
  navigator.geolocation.getCurrentPosition(
    async (pos) => {
      try {
        await ticketService.addActivity(id.value, {
          activity_type: type,
          message: type === 'VISIT_CHECKIN' ? 'Teknisi check-in di lokasi' : 'Teknisi check-out dari lokasi',
          latitude: Number(pos.coords.latitude.toFixed(7)),
          longitude: Number(pos.coords.longitude.toFixed(7)),
        })
        toast.add({ severity: 'success', summary: `${type} tercatat`, life: 3000 })
        await load()
      } catch (err) {
        toast.add({ severity: 'error', summary: 'Gagal check-in', detail: userMessage(toApiError(err)), life: 5000 })
      } finally {
        activityBusy.value = false
      }
    },
    () => {
      activityBusy.value = false
      toast.add({ severity: 'error', summary: 'Gagal mengambil lokasi GPS', life: 4000 })
    },
    { enableHighAccuracy: true, timeout: 15_000 },
  )
}

async function onPhotoChange(ev: Event): Promise<void> {
  const input = ev.target as HTMLInputElement
  const raw = input.files?.[0]
  if (!raw) return
  try {
    photoFile.value = await compressImageToJpeg(raw)
  } catch {
    photoFile.value = raw
  }
}

async function uploadPhoto(): Promise<void> {
  if (!photoFile.value) return
  activityBusy.value = true
  try {
    const fd = new FormData()
    fd.append('activity_type', 'PHOTO')
    if (commentText.value.trim()) fd.append('message', commentText.value.trim())
    fd.append('photo', photoFile.value, photoFile.value.name)
    await ticketService.addActivity(id.value, fd)
    photoFile.value = null
    commentText.value = ''
    await load()
    toast.add({ severity: 'success', summary: 'Foto berhasil diunggah', life: 3000 })
  } catch (err) {
    toast.add({ severity: 'error', summary: 'Gagal unggah foto', detail: userMessage(toApiError(err)), life: 5000 })
  } finally {
    activityBusy.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <BackLink to="/tickets" label="Kembali ke Tiket Maintenance" />

    <div v-if="loading" class="py-12 text-center text-sm text-slate-500" role="status">
      Memuat detail tiket…
    </div>
    <div v-else-if="loadError || !ticket" role="alert" class="rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800">
      {{ loadError || 'Tiket tidak ditemukan.' }}
    </div>

    <template v-else>
      <header class="rounded-xl border border-slate-200 bg-white p-5">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <p class="font-mono text-xs font-semibold text-slate-500">{{ ticket.ticket_number }}</p>
            <h1 class="mt-1 text-xl font-bold text-slate-900">{{ ticket.title }}</h1>
            <p class="mt-1 text-sm text-slate-600">
              {{ ticket.customer_name }} — {{ ticket.site_name }} ({{ ticket.site_city }})
            </p>
          </div>
          <div class="flex items-center gap-2">
            <StatusBadge :value="ticket.status" />
            <span class="rounded bg-slate-100 px-2 py-0.5 text-xs font-bold text-slate-800">
              {{ ticket.priority }}
            </span>
          </div>
        </div>

        <dl class="mt-4 grid gap-3 border-t border-slate-100 pt-4 text-xs sm:grid-cols-4">
          <div>
            <dt class="text-slate-500">Unit Serial</dt>
            <dd class="mt-0.5 font-mono font-semibold text-indigo-700">
              <RouterLink :to="`/stok/serial/${ticket.product_serial_id}`" class="hover:underline">
                {{ ticket.asset_tag }}
              </RouterLink>
              <span class="block font-sans font-normal text-slate-600">{{ ticket.product_name }}</span>
            </dd>
          </div>
          <div>
            <dt class="text-slate-500">Kategori</dt>
            <dd class="mt-0.5 font-medium text-slate-900">{{ ticket.category }}</dd>
          </div>
          <div>
            <dt class="text-slate-500">Teknisi</dt>
            <dd class="mt-0.5 font-medium text-slate-900">{{ ticket.assigned_to_name ?? 'Belum ditugaskan' }}</dd>
          </div>
          <div>
            <dt class="text-slate-500">Batas SLA</dt>
            <dd class="mt-0.5 font-medium" :class="ticket.sla_breached ? 'text-rose-700' : 'text-slate-900'">
              {{ ticket.sla_due_at ? formatDateTime(ticket.sla_due_at) : '—' }}
            </dd>
          </div>
        </dl>
        <p v-if="ticket.description" class="mt-3 text-sm text-slate-700">{{ ticket.description }}</p>
      </header>

      <!-- Aksi Assign & Ubah Status -->
      <section v-if="!isClosedOrCancelled" class="grid gap-4 md:grid-cols-2">
        <div v-if="canWrite" class="space-y-2 rounded-xl border border-slate-200 bg-white p-4">
          <h2 class="text-sm font-semibold text-slate-900">Tugaskan Teknisi</h2>
          <form class="flex gap-2" @submit.prevent="submitAssign">
            <input
              v-model="assignTechId"
              type="number"
              min="1"
              placeholder="ID User Teknisi"
              class="min-h-10 flex-1 rounded-lg border border-slate-300 px-3 text-sm"
            />
            <button type="submit" :class="btnPrimary" :disabled="assignBusy">Tugaskan</button>
          </form>
        </div>

        <div v-if="canWork && nextStatuses.length > 0" class="space-y-2 rounded-xl border border-slate-200 bg-white p-4">
          <h2 class="text-sm font-semibold text-slate-900">Perbarui Status</h2>
          <input
            v-if="nextStatuses.includes('RESOLVED')"
            v-model="resolutionNotes"
            type="text"
            placeholder="Catatan penyelesaian (untuk RESOLVED)"
            class="min-h-10 w-full rounded-lg border border-slate-300 px-3 text-sm"
          />
          <div class="flex flex-wrap gap-2">
            <button
              v-for="st in nextStatuses"
              :key="st"
              type="button"
              :class="st === 'RESOLVED' ? btnPrimary : btnSecondary"
              :disabled="statusBusy"
              @click="transitionTo(st)"
            >
              {{ st }}
            </button>
          </div>
        </div>
      </section>

      <!-- Aktivitas Teknisi -->
      <section class="space-y-4 rounded-xl border border-slate-200 bg-white p-5">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <h2 class="text-sm font-semibold text-slate-900">
            Timeline Aktivitas ({{ ticket.activities.length }})
          </h2>
          <div v-if="canWork && !isClosedOrCancelled" class="flex flex-wrap gap-2">
            <button type="button" :class="btnSecondary" :disabled="activityBusy" @click="postCheckin('VISIT_CHECKIN')">
              <i class="pi pi-map-marker text-xs" aria-hidden="true" />
              Check-in GPS
            </button>
            <button type="button" :class="btnSecondary" :disabled="activityBusy" @click="postCheckin('VISIT_CHECKOUT')">
              <i class="pi pi-sign-out text-xs" aria-hidden="true" />
              Check-out GPS
            </button>
          </div>
        </div>

        <ol class="space-y-3">
          <li v-for="a in ticket.activities" :key="a.id" class="rounded-lg border border-slate-200 p-3 text-xs">
            <div class="flex items-center justify-between gap-2">
              <span class="font-semibold text-slate-900">{{ a.user_name }} · {{ a.activity_type }}</span>
              <span class="text-slate-500">{{ formatDateTime(a.created_at) }}</span>
            </div>
            <p v-if="a.message" class="mt-1 text-slate-700">{{ a.message }}</p>
            <p v-if="a.latitude != null && a.longitude != null" class="mt-1 font-mono text-[11px] text-slate-500">
              GPS: {{ a.latitude }}, {{ a.longitude }}
            </p>
            <a v-if="a.photo_url" :href="a.photo_url" target="_blank" rel="noopener noreferrer" class="mt-2 block">
              <img :src="a.photo_url" alt="Foto aktivitas tiket" class="max-h-48 rounded-lg border border-slate-200 object-contain" />
            </a>
          </li>
        </ol>

        <div v-if="canWork && !isClosedOrCancelled" class="space-y-2 border-t border-slate-100 pt-4">
          <textarea
            v-model="commentText"
            rows="2"
            placeholder="Tulis komentar pengerjaan atau keterangan foto…"
            class="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm"
          />
          <div class="flex flex-wrap items-center justify-between gap-2">
            <div class="flex items-center gap-2">
              <input type="file" accept="image/*" capture="environment" class="text-xs" @change="onPhotoChange" />
              <button v-if="photoFile" type="button" :class="btnSecondary" :disabled="activityBusy" @click="uploadPhoto">
                Unggah Foto
              </button>
            </div>
            <button type="button" :class="btnPrimary" :disabled="activityBusy || !commentText.trim()" @click="postComment">
              Kirim Komentar
            </button>
          </div>
        </div>
      </section>
    </template>
  </div>
</template>
