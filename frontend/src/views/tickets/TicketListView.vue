<script setup lang="ts">
import { vStackTable } from '@/directives/stackTable'
// Papan Kanban & Daftar Tiket Maintenance (T3.8).
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import StatusBadge from '@/components/StatusBadge.vue'
import { toApiError, userMessage } from '@/lib/apiError'
import { formatDateTime } from '@/lib/format'
import { btnPrimary, btnSecondary } from '@/lib/ui'
import { customerService } from '@/services/customerService'
import { inventoryService } from '@/services/inventoryService'
import {
  ticketService,
  type MaintenanceTicket,
  type TicketCategory,
  type TicketPriority,
  type TicketStatus,
} from '@/services/ticketService'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const toast = useToast()
const auth = useAuthStore()

const viewMode = ref<'kanban' | 'list'>('kanban')
const q = ref('')
const priorityFilter = ref('')
const tickets = ref<MaintenanceTicket[]>([])
const loading = ref(true)
const error = ref<string | null>(null)

const kanbanColumns: { status: TicketStatus; label: string; accent: string }[] = [
  { status: 'OPEN', label: 'Terbuka', accent: 'border-t-sky-500' },
  { status: 'ASSIGNED', label: 'Ditugaskan', accent: 'border-t-indigo-500' },
  { status: 'IN_PROGRESS', label: 'Dikerjakan', accent: 'border-t-amber-500' },
  { status: 'WAITING_PART', label: 'Tunggu Part', accent: 'border-t-orange-500' },
  { status: 'RESOLVED', label: 'Selesai', accent: 'border-t-emerald-500' },
]

// Form Buat Tiket
const showCreate = ref(false)
const createBusy = ref(false)
const customers = ref<{ id: number; company_name: string }[]>([])
const sites = ref<{ id: number; site_name: string }[]>([])
const serials = ref<
  { id: number; asset_tag: string; serial_number: string; product_name: string }[]
>([])

const formCustomerId = ref<number | ''>('')
const formSiteId = ref<number | ''>('')
const formSerialId = ref<number | ''>('')
const formTitle = ref('')
const formCategory = ref<TicketCategory>('HARDWARE')
const formPriority = ref<TicketPriority>('MEDIUM')
const formReportedBy = ref('')
const formDescription = ref('')

const canCreate = computed(() => auth.can('ticket.write'))

function byStatus(st: TicketStatus): MaintenanceTicket[] {
  return tickets.value.filter((t) => t.status === st)
}

function priorityClass(p: TicketPriority): string {
  switch (p) {
    case 'CRITICAL':
      return 'bg-rose-100 text-rose-800'
    case 'HIGH':
      return 'bg-orange-100 text-orange-800'
    case 'LOW':
      return 'bg-slate-100 text-slate-700'
    default:
      return 'bg-amber-100 text-amber-800'
  }
}

async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    const page = await ticketService.list({
      q: q.value.trim() || undefined,
      priority: priorityFilter.value || undefined,
      limit: 100,
    })
    tickets.value = page.items
  } catch (err) {
    error.value = userMessage(toApiError(err))
  } finally {
    loading.value = false
  }
}

async function openCreateModal(): Promise<void> {
  showCreate.value = true
  if (customers.value.length === 0) {
    try {
      const [cPage, sPage] = await Promise.all([
        customerService.list({ limit: 100 }),
        inventoryService.serials({ status: 'RENTED', limit: 100 }),
      ])
      customers.value = cPage.items.map((c) => ({ id: c.id, company_name: c.company_name }))
      serials.value = sPage.items.map((s) => ({
        id: s.id,
        asset_tag: s.asset_tag,
        serial_number: s.serial_number,
        product_name: s.product_name,
      }))
    } catch {
      // Abaikan jika gagal memuat opsi awal; pengguna masih bisa coba ulang.
    }
  }
}

async function onCustomerChange(): Promise<void> {
  formSiteId.value = ''
  sites.value = []
  if (typeof formCustomerId.value !== 'number') return
  try {
    const det = await customerService.get(formCustomerId.value)
    sites.value = det.sites.map((s) => ({ id: s.id, site_name: s.site_name }))
    if (sites.value.length === 1) {
      formSiteId.value = sites.value[0]!.id
    }
  } catch {
    sites.value = []
  }
}

async function submitCreate(): Promise<void> {
  if (
    typeof formCustomerId.value !== 'number' ||
    typeof formSiteId.value !== 'number' ||
    typeof formSerialId.value !== 'number' ||
    formTitle.value.trim().length < 3
  ) {
    toast.add({
      severity: 'warn',
      summary: 'Data belum lengkap',
      detail: 'Pilih pelanggan, cabang, unit serial, dan isi judul tiket minimal 3 karakter.',
      life: 4000,
    })
    return
  }
  createBusy.value = true
  try {
    const created = await ticketService.create({
      customer_id: formCustomerId.value,
      customer_site_id: formSiteId.value,
      product_serial_id: formSerialId.value,
      category: formCategory.value,
      priority: formPriority.value,
      title: formTitle.value.trim(),
      reported_by_name: formReportedBy.value.trim() || undefined,
      description: formDescription.value.trim() || undefined,
    })
    showCreate.value = false
    toast.add({
      severity: 'success',
      summary: 'Tiket dibuat',
      detail: `${created.ticket_number} berhasil dibuat.`,
      life: 3500,
    })
    await router.push(`/tickets/${created.id}`)
  } catch (err) {
    toast.add({
      severity: 'error',
      summary: 'Gagal membuat tiket',
      detail: userMessage(toApiError(err)),
      life: 5000,
    })
  } finally {
    createBusy.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <header class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 class="text-xl font-bold text-slate-900">Tiket Maintenance</h1>
        <p class="mt-0.5 text-sm text-slate-600">
          Kelola laporan kerusakan unit sewa, penugasan teknisi, dan pemantauan SLA.
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <div class="inline-flex rounded-lg border border-slate-200 bg-white p-1">
          <button
            type="button"
            class="rounded-md px-3 py-1.5 text-xs font-medium"
            :class="viewMode === 'kanban' ? 'bg-indigo-600 text-white' : 'text-slate-700'"
            @click="viewMode = 'kanban'"
          >
            Kanban
          </button>
          <button
            type="button"
            class="rounded-md px-3 py-1.5 text-xs font-medium"
            :class="viewMode === 'list' ? 'bg-indigo-600 text-white' : 'text-slate-700'"
            @click="viewMode = 'list'"
          >
            Tabel
          </button>
        </div>
        <button v-if="canCreate" type="button" :class="btnPrimary" @click="openCreateModal">
          <i class="pi pi-plus text-xs" aria-hidden="true" />
          Buat Tiket
        </button>
      </div>
    </header>

    <!-- Filter -->
    <form
      class="flex flex-wrap items-center gap-2 rounded-xl border border-slate-200 bg-white p-3"
      @submit.prevent="load"
    >
      <input
        v-model="q"
        type="search"
        placeholder="Cari nomor tiket, judul, pelanggan, atau asset tag…"
        class="min-h-10 min-w-60 flex-1 rounded-lg border border-slate-300 px-3 text-sm"
      />
      <select
        v-model="priorityFilter"
        class="min-h-10 rounded-lg border border-slate-300 bg-white px-3 text-sm"
        @change="load"
      >
        <option value="">Semua Prioritas</option>
        <option value="CRITICAL">CRITICAL</option>
        <option value="HIGH">HIGH</option>
        <option value="MEDIUM">MEDIUM</option>
        <option value="LOW">LOW</option>
      </select>
      <button type="submit" :class="btnSecondary" :disabled="loading">Cari</button>
    </form>

    <!-- Form Buat Tiket -->
    <section
      v-if="showCreate"
      class="space-y-3 rounded-xl border border-indigo-200 bg-white p-4 shadow-xs"
    >
      <div class="flex items-center justify-between">
        <h2 class="text-sm font-semibold text-slate-900">Buat Tiket Maintenance Baru</h2>
        <button
          type="button"
          class="text-xs text-slate-500 hover:text-slate-800"
          @click="showCreate = false"
        >
          Tutup
        </button>
      </div>
      <div class="grid gap-3 sm:grid-cols-3">
        <div>
          <label for="t-cust" class="block text-xs font-medium text-slate-700">Pelanggan *</label>
          <select
            id="t-cust"
            v-model="formCustomerId"
            class="mt-1 min-h-10 w-full rounded-lg border border-slate-300 bg-white px-3 text-sm"
            @change="onCustomerChange"
          >
            <option value="">Pilih pelanggan…</option>
            <option v-for="c in customers" :key="c.id" :value="c.id">{{ c.company_name }}</option>
          </select>
        </div>
        <div>
          <label for="t-site" class="block text-xs font-medium text-slate-700">Cabang *</label>
          <select
            id="t-site"
            v-model="formSiteId"
            class="mt-1 min-h-10 w-full rounded-lg border border-slate-300 bg-white px-3 text-sm"
          >
            <option value="">Pilih cabang…</option>
            <option v-for="s in sites" :key="s.id" :value="s.id">{{ s.site_name }}</option>
          </select>
        </div>
        <div>
          <label for="t-sn" class="block text-xs font-medium text-slate-700">Unit Serial *</label>
          <select
            id="t-sn"
            v-model="formSerialId"
            class="mt-1 min-h-10 w-full rounded-lg border border-slate-300 bg-white px-3 text-sm"
          >
            <option value="">Pilih unit…</option>
            <option v-for="s in serials" :key="s.id" :value="s.id">
              {{ s.asset_tag }} — {{ s.product_name }}
            </option>
          </select>
        </div>
      </div>
      <div class="grid gap-3 sm:grid-cols-3">
        <div class="sm:col-span-2">
          <label for="t-title" class="block text-xs font-medium text-slate-700"
            >Judul Keluhan *</label
          >
          <input
            id="t-title"
            v-model="formTitle"
            type="text"
            placeholder="Mis. Printer sering paper jam di tray 2"
            class="mt-1 min-h-10 w-full rounded-lg border border-slate-300 px-3 text-sm"
          />
        </div>
        <div>
          <label for="t-reporter" class="block text-xs font-medium text-slate-700">Pelapor</label>
          <input
            id="t-reporter"
            v-model="formReportedBy"
            type="text"
            placeholder="Nama PIC pelapor"
            class="mt-1 min-h-10 w-full rounded-lg border border-slate-300 px-3 text-sm"
          />
        </div>
      </div>
      <div class="grid gap-3 sm:grid-cols-3">
        <div>
          <label for="t-cat" class="block text-xs font-medium text-slate-700">Kategori</label>
          <select
            id="t-cat"
            v-model="formCategory"
            class="mt-1 min-h-10 w-full rounded-lg border border-slate-300 bg-white px-3 text-sm"
          >
            <option value="PAPER_JAM">PAPER_JAM</option>
            <option value="PRINT_QUALITY">PRINT_QUALITY</option>
            <option value="HARDWARE">HARDWARE</option>
            <option value="SOFTWARE">SOFTWARE</option>
            <option value="NETWORK">NETWORK</option>
            <option value="PREVENTIVE">PREVENTIVE</option>
            <option value="OTHER">OTHER</option>
          </select>
        </div>
        <div>
          <label for="t-prio" class="block text-xs font-medium text-slate-700">Prioritas</label>
          <select
            id="t-prio"
            v-model="formPriority"
            class="mt-1 min-h-10 w-full rounded-lg border border-slate-300 bg-white px-3 text-sm"
          >
            <option value="LOW">LOW (SLA 72 jam)</option>
            <option value="MEDIUM">MEDIUM (SLA 24 jam)</option>
            <option value="HIGH">HIGH (SLA 8 jam)</option>
            <option value="CRITICAL">CRITICAL (SLA 4 jam)</option>
          </select>
        </div>
        <div>
          <label for="t-desc" class="block text-xs font-medium text-slate-700">Deskripsi</label>
          <input
            id="t-desc"
            v-model="formDescription"
            type="text"
            placeholder="Detail keluhan singkat"
            class="mt-1 min-h-10 w-full rounded-lg border border-slate-300 px-3 text-sm"
          />
        </div>
      </div>
      <div class="flex justify-end gap-2">
        <button type="button" :class="btnSecondary" @click="showCreate = false">Batal</button>
        <button type="button" :class="btnPrimary" :disabled="createBusy" @click="submitCreate">
          {{ createBusy ? 'Menyimpan…' : 'Simpan Tiket' }}
        </button>
      </div>
    </section>

    <p
      v-if="error"
      role="alert"
      class="rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800"
    >
      {{ error }}
    </p>

    <div v-else-if="loading" class="py-12 text-center text-sm text-slate-500" role="status">
      Memuat tiket maintenance…
    </div>

    <!-- Mode Kanban -->
    <div v-else-if="viewMode === 'kanban'" class="grid gap-3 md:grid-cols-5">
      <div
        v-for="col in kanbanColumns"
        :key="col.status"
        class="flex flex-col rounded-xl border border-slate-200 border-t-4 bg-slate-50/70 p-3"
        :class="col.accent"
      >
        <div class="mb-2 flex items-center justify-between">
          <h2 class="text-xs font-bold uppercase tracking-wide text-slate-700">{{ col.label }}</h2>
          <span class="rounded-full bg-white px-2 py-0.5 text-xs font-semibold text-slate-700">
            {{ byStatus(col.status).length }}
          </span>
        </div>
        <div class="space-y-2">
          <RouterLink
            v-for="t in byStatus(col.status)"
            :key="t.id"
            :to="`/tickets/${t.id}`"
            class="block rounded-lg border border-slate-200 bg-white p-3 shadow-2xs transition hover:border-indigo-300"
          >
            <div class="flex items-center justify-between gap-1">
              <span class="font-mono text-[11px] font-semibold text-slate-600">{{
                t.ticket_number
              }}</span>
              <span
                class="rounded px-1.5 py-0.5 text-[10px] font-bold"
                :class="priorityClass(t.priority)"
              >
                {{ t.priority }}
              </span>
            </div>
            <p class="mt-1 line-clamp-2 text-xs font-semibold text-slate-900">{{ t.title }}</p>
            <p class="mt-1 truncate text-[11px] text-slate-600">{{ t.customer_name }}</p>
            <p class="font-mono text-[11px] text-slate-500">{{ t.asset_tag }}</p>
            <p v-if="t.sla_breached" class="mt-1 text-[10px] font-semibold text-rose-700">
              SLA terlewati
            </p>
          </RouterLink>
          <p
            v-if="byStatus(col.status).length === 0"
            class="py-4 text-center text-xs text-slate-400"
          >
            Kosong
          </p>
        </div>
      </div>
    </div>

    <!-- Mode Tabel -->
    <div v-else class="overflow-x-auto rounded-xl border border-slate-200 bg-white">
      <table v-stack-table class="min-w-full divide-y divide-slate-200 text-sm">
        <thead class="bg-slate-50 text-left text-xs font-semibold text-slate-600">
          <tr>
            <th class="px-4 py-3">Nomor</th>
            <th class="px-4 py-3">Judul &amp; Unit</th>
            <th class="px-4 py-3">Pelanggan</th>
            <th class="px-4 py-3">Prioritas</th>
            <th class="px-4 py-3">Status</th>
            <th class="px-4 py-3">Teknisi</th>
            <th class="px-4 py-3">SLA</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-100">
          <tr v-for="t in tickets" :key="t.id" class="hover:bg-slate-50">
            <td class="px-4 py-3 font-mono text-xs font-semibold">
              <RouterLink :to="`/tickets/${t.id}`" class="text-indigo-700 hover:underline">
                {{ t.ticket_number }}
              </RouterLink>
            </td>
            <td class="px-4 py-3">
              <p class="font-medium text-slate-900">{{ t.title }}</p>
              <p class="font-mono text-xs text-slate-500">
                {{ t.asset_tag }} · {{ t.product_name }}
              </p>
            </td>
            <td class="px-4 py-3 text-xs text-slate-700">
              {{ t.customer_name }} — {{ t.site_name }}
            </td>
            <td class="px-4 py-3">
              <span
                class="rounded px-2 py-0.5 text-xs font-semibold"
                :class="priorityClass(t.priority)"
              >
                {{ t.priority }}
              </span>
            </td>
            <td class="px-4 py-3"><StatusBadge :value="t.status" /></td>
            <td class="px-4 py-3 text-xs text-slate-700">
              {{ t.assigned_to_name ?? 'Belum ditugaskan' }}
            </td>
            <td
              class="px-4 py-3 text-xs"
              :class="t.sla_breached ? 'font-semibold text-rose-700' : 'text-slate-600'"
            >
              {{ t.sla_due_at ? formatDateTime(t.sla_due_at) : '—' }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
