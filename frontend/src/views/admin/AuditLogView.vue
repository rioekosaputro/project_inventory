<script setup lang="ts">
import { vStackTable } from '@/directives/stackTable'
// Halaman Audit Log Viewer untuk Admin & Manajemen (plan Bagian 5.1 & T4.3).
import { onMounted, ref } from 'vue'
import { toApiError, userMessage } from '@/lib/apiError'
import { formatDateTime } from '@/lib/format'
import { btnPrimary, btnSecondary } from '@/lib/ui'
import { adminService, type AuditLogRow } from '@/services/adminService'

const rows = ref<AuditLogRow[]>([])
const loading = ref(true)
const error = ref('')
const page = ref(1)
const limit = ref(25)
const total = ref(0)
const totalPages = ref(1)

const search = ref('')
const action = ref('')
const entityType = ref('')
const dateFrom = ref('')
const dateTo = ref('')
const expandedId = ref<number | null>(null)

const entityOptions = [
  '',
  'user',
  'product',
  'warehouse',
  'rack',
  'lpb',
  'stock_opname',
  'customer',
  'rental_contract',
  'sales_order',
  'delivery_order',
  'invoice',
  'maintenance_ticket',
  'job_run',
]

async function fetchLogs(targetPage = page.value): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const res = await adminService.auditLogs({
      q: search.value.trim() || undefined,
      action: action.value.trim() || undefined,
      entity_type: entityType.value || undefined,
      date_from: dateFrom.value || undefined,
      date_to: dateTo.value || undefined,
      page: targetPage,
      limit: limit.value,
    })
    rows.value = res.items
    page.value = res.meta.page
    total.value = res.meta.total
    totalPages.value = Math.max(1, res.meta.total_pages)
  } catch (err) {
    error.value = userMessage(toApiError(err))
  } finally {
    loading.value = false
  }
}

function applyFilters(): void {
  expandedId.value = null
  void fetchLogs(1)
}

function resetFilters(): void {
  search.value = ''
  action.value = ''
  entityType.value = ''
  dateFrom.value = ''
  dateTo.value = ''
  expandedId.value = null
  void fetchLogs(1)
}

function toggleExpand(id: number): void {
  expandedId.value = expandedId.value === id ? null : id
}

function formatJson(val: unknown): string {
  if (val === undefined || val === null) return '—'
  try {
    return JSON.stringify(val, null, 2)
  } catch {
    return String(val)
  }
}

onMounted(() => void fetchLogs(1))
</script>

<template>
  <div class="space-y-5">
    <header class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 class="text-xl font-bold text-slate-900">Log Audit</h1>
        <p class="mt-0.5 text-sm text-slate-600">
          Catatan aktivitas sistem bersifat append-only (autentikasi, mutasi dokumen, dan eksekusi
          job).
        </p>
      </div>
      <button type="button" :class="btnPrimary" :disabled="loading" @click="fetchLogs(page)">
        <i class="pi pi-refresh text-xs" :class="{ 'pi-spin': loading }" aria-hidden="true" />
        Muat Ulang
      </button>
    </header>

    <form
      class="grid gap-3 rounded-xl border border-slate-200 bg-white p-4 sm:grid-cols-2 lg:grid-cols-6"
      @submit.prevent="applyFilters"
    >
      <div class="lg:col-span-2">
        <label for="audit-q" class="block text-xs font-medium text-slate-600">Cari</label>
        <input
          id="audit-q"
          v-model="search"
          type="text"
          placeholder="Nama/email pelaku, aksi, request ID…"
          class="mt-1 w-full rounded-lg border border-slate-300 px-3 py-1.5 text-sm focus:border-blue-500 focus:outline-none"
        />
      </div>
      <div>
        <label for="audit-action" class="block text-xs font-medium text-slate-600">Aksi</label>
        <input
          id="audit-action"
          v-model="action"
          type="text"
          placeholder="mis. LPB_APPROVE"
          class="mt-1 w-full rounded-lg border border-slate-300 px-3 py-1.5 font-mono text-sm uppercase focus:border-blue-500 focus:outline-none"
        />
      </div>
      <div>
        <label for="audit-entity" class="block text-xs font-medium text-slate-600">Entitas</label>
        <select
          id="audit-entity"
          v-model="entityType"
          class="mt-1 w-full rounded-lg border border-slate-300 px-2.5 py-1.5 text-sm focus:border-blue-500 focus:outline-none"
        >
          <option v-for="opt in entityOptions" :key="opt" :value="opt">
            {{ opt || 'Semua entitas' }}
          </option>
        </select>
      </div>
      <div>
        <label for="audit-from" class="block text-xs font-medium text-slate-600">Dari</label>
        <input
          id="audit-from"
          v-model="dateFrom"
          type="date"
          class="mt-1 w-full rounded-lg border border-slate-300 px-2.5 py-1.5 text-sm focus:border-blue-500 focus:outline-none"
        />
      </div>
      <div>
        <label for="audit-to" class="block text-xs font-medium text-slate-600">Sampai</label>
        <input
          id="audit-to"
          v-model="dateTo"
          type="date"
          class="mt-1 w-full rounded-lg border border-slate-300 px-2.5 py-1.5 text-sm focus:border-blue-500 focus:outline-none"
        />
      </div>
      <div class="flex items-end gap-2 sm:col-span-2 lg:col-span-6">
        <button type="submit" :class="btnPrimary" :disabled="loading">Terapkan Filter</button>
        <button type="button" :class="btnSecondary" :disabled="loading" @click="resetFilters">
          Reset
        </button>
        <span class="ml-auto text-xs text-slate-500">Total {{ total }} catatan</span>
      </div>
    </form>

    <div
      v-if="error"
      role="alert"
      class="rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800"
    >
      {{ error }}
    </div>

    <div v-else class="overflow-hidden rounded-xl border border-slate-200 bg-white">
      <div class="overflow-x-auto">
        <table v-stack-table class="min-w-full divide-y divide-slate-200 text-sm">
          <thead class="bg-slate-50 text-left text-xs font-semibold text-slate-600">
            <tr>
              <th class="px-4 py-3">Waktu</th>
              <th class="px-4 py-3">Pelaku</th>
              <th class="px-4 py-3">Aksi</th>
              <th class="px-4 py-3">Entitas</th>
              <th class="px-4 py-3">IP / Request ID</th>
              <th class="px-4 py-3 text-right">Detail</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            <tr v-if="loading && rows.length === 0">
              <td colspan="6" class="px-4 py-10 text-center text-slate-500">Memuat log audit…</td>
            </tr>
            <tr v-else-if="rows.length === 0">
              <td colspan="6" class="px-4 py-10 text-center text-slate-500">
                Tidak ada log audit yang cocok dengan filter.
              </td>
            </tr>
            <template v-for="row in rows" :key="row.id">
              <tr class="hover:bg-slate-50/80">
                <td class="whitespace-nowrap px-4 py-2.5 text-xs text-slate-600">
                  {{ formatDateTime(row.created_at) }}
                </td>
                <td class="px-4 py-2.5">
                  <div v-if="row.user_name || row.user_email" class="text-xs">
                    <p class="font-medium text-slate-900">{{ row.user_name || '—' }}</p>
                    <p class="text-slate-500">{{ row.user_email }}</p>
                  </div>
                  <span v-else class="text-xs italic text-slate-400">Sistem</span>
                </td>
                <td class="px-4 py-2.5">
                  <span
                    class="inline-block rounded bg-slate-100 px-2 py-0.5 font-mono text-xs font-semibold text-slate-800"
                  >
                    {{ row.action }}
                  </span>
                </td>
                <td class="whitespace-nowrap px-4 py-2.5 font-mono text-xs text-slate-700">
                  <span v-if="row.entity_type">
                    {{ row.entity_type
                    }}<template v-if="row.entity_id"> #{{ row.entity_id }}</template>
                  </span>
                  <span v-else class="text-slate-400">—</span>
                </td>
                <td class="px-4 py-2.5 font-mono text-xs text-slate-500">
                  <div>{{ row.ip_address || '—' }}</div>
                  <div v-if="row.request_id" class="truncate max-w-[160px]" :title="row.request_id">
                    {{ row.request_id }}
                  </div>
                </td>
                <td class="px-4 py-2.5 text-right">
                  <button
                    v-if="row.before_data !== undefined || row.after_data !== undefined"
                    type="button"
                    class="text-xs font-medium text-blue-600 hover:underline"
                    @click="toggleExpand(row.id)"
                  >
                    {{ expandedId === row.id ? 'Tutup JSON' : 'Lihat JSON' }}
                  </button>
                  <span v-else class="text-xs text-slate-400">—</span>
                </td>
              </tr>
              <tr v-if="expandedId === row.id" class="bg-slate-50">
                <td colspan="6" class="px-4 py-3">
                  <div class="grid gap-3 md:grid-cols-2">
                    <div>
                      <p class="mb-1 text-xs font-semibold text-slate-600">Before Data</p>
                      <pre
                        class="max-h-60 overflow-auto rounded-lg border border-slate-200 bg-white p-2.5 font-mono text-xs text-slate-700"
                        >{{ formatJson(row.before_data) }}</pre>
                    </div>
                    <div>
                      <p class="mb-1 text-xs font-semibold text-slate-600">After Data</p>
                      <pre
                        class="max-h-60 overflow-auto rounded-lg border border-slate-200 bg-white p-2.5 font-mono text-xs text-slate-700"
                        >{{ formatJson(row.after_data) }}</pre>
                    </div>
                  </div>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>

      <footer
        class="flex items-center justify-between border-t border-slate-200 bg-slate-50 px-4 py-2.5 text-xs text-slate-600"
      >
        <span>Halaman {{ page }} dari {{ totalPages }}</span>
        <div class="flex gap-2">
          <button
            type="button"
            :class="btnSecondary"
            :disabled="loading || page <= 1"
            @click="fetchLogs(page - 1)"
          >
            Sebelumnya
          </button>
          <button
            type="button"
            :class="btnSecondary"
            :disabled="loading || page >= totalPages"
            @click="fetchLogs(page + 1)"
          >
            Berikutnya
          </button>
        </div>
      </footer>
    </div>
  </div>
</template>
