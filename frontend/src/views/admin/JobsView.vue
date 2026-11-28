<script setup lang="ts">
import { vStackTable } from '@/directives/stackTable'
// Halaman Riwayat Eksekusi Job Terjadwal untuk Admin (plan Bagian 6.5 & T4.3).
import { computed, onMounted, ref } from 'vue'
import { toApiError, userMessage } from '@/lib/apiError'
import { formatDateTime } from '@/lib/format'
import { btnPrimary } from '@/lib/ui'
import { adminService, type JobRun } from '@/services/adminService'

const runs = ref<JobRun[]>([])
const loading = ref(true)
const error = ref('')
const limit = ref(50)
const filterJob = ref('')
const filterStatus = ref('')
const expandedId = ref<number | null>(null)

const jobNames = computed(() => {
  const set = new Set<string>()
  for (const r of runs.value) set.add(r.job_name)
  return Array.from(set).sort()
})

const filteredRuns = computed(() =>
  runs.value.filter((r) => {
    if (filterJob.value && r.job_name !== filterJob.value) return false
    if (filterStatus.value && r.status !== filterStatus.value) return false
    return true
  }),
)

const stats = computed(() => {
  let success = 0
  let failed = 0
  let running = 0
  for (const r of runs.value) {
    if (r.status === 'SUCCESS') success++
    else if (r.status === 'FAILED') failed++
    else if (r.status === 'RUNNING') running++
  }
  return { total: runs.value.length, success, failed, running }
})

async function loadJobs(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    runs.value = await adminService.jobs(limit.value)
  } catch (err) {
    error.value = userMessage(toApiError(err))
  } finally {
    loading.value = false
  }
}

function statusClass(status: string): string {
  if (status === 'SUCCESS') return 'bg-emerald-50 text-emerald-700 border-emerald-200'
  if (status === 'FAILED') return 'bg-rose-50 text-rose-700 border-rose-200'
  return 'bg-amber-50 text-amber-700 border-amber-200'
}

function durationText(run: JobRun): string {
  if (!run.finished_at) return 'Sedang berjalan…'
  const ms = new Date(run.finished_at).getTime() - new Date(run.started_at).getTime()
  if (Number.isNaN(ms) || ms < 0) return '—'
  if (ms < 1000) return `${ms} ms`
  return `${(ms / 1000).toFixed(2)} dtk`
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

onMounted(() => void loadJobs())
</script>

<template>
  <div class="space-y-5">
    <header class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 class="text-xl font-bold text-slate-900">Riwayat Jobs Terjadwal</h1>
        <p class="mt-0.5 text-sm text-slate-600">
          Status eksekusi idempoten billing bulanan, kontrak berakhir, jatuh tempo invoice, stok
          minimum, dan rekonsiliasi stok.
        </p>
      </div>
      <button type="button" :class="btnPrimary" :disabled="loading" @click="loadJobs">
        <i class="pi pi-refresh text-xs" :class="{ 'pi-spin': loading }" aria-hidden="true" />
        Muat Ulang
      </button>
    </header>

    <section class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
      <div class="rounded-xl border border-slate-200 bg-white p-4">
        <p class="text-xs font-medium text-slate-500">Total Eksekusi</p>
        <p class="mt-1 text-2xl font-bold text-slate-900">{{ stats.total }}</p>
      </div>
      <div class="rounded-xl border border-emerald-200 bg-emerald-50/50 p-4">
        <p class="text-xs font-medium text-emerald-700">Sukses</p>
        <p class="mt-1 text-2xl font-bold text-emerald-800">{{ stats.success }}</p>
      </div>
      <div class="rounded-xl border border-rose-200 bg-rose-50/50 p-4">
        <p class="text-xs font-medium text-rose-700">Gagal</p>
        <p class="mt-1 text-2xl font-bold text-rose-800">{{ stats.failed }}</p>
      </div>
      <div class="rounded-xl border border-amber-200 bg-amber-50/50 p-4">
        <p class="text-xs font-medium text-amber-700">Berjalan</p>
        <p class="mt-1 text-2xl font-bold text-amber-800">{{ stats.running }}</p>
      </div>
    </section>

    <div class="flex flex-wrap items-end gap-3 rounded-xl border border-slate-200 bg-white p-4">
      <div>
        <label for="job-filter-name" class="block text-xs font-medium text-slate-600"
          >Nama Job</label
        >
        <select
          id="job-filter-name"
          v-model="filterJob"
          class="mt-1 rounded-lg border border-slate-300 px-2.5 py-1.5 text-sm focus:border-blue-500 focus:outline-none"
        >
          <option value="">Semua job</option>
          <option v-for="name in jobNames" :key="name" :value="name">{{ name }}</option>
        </select>
      </div>
      <div>
        <label for="job-filter-status" class="block text-xs font-medium text-slate-600"
          >Status</label
        >
        <select
          id="job-filter-status"
          v-model="filterStatus"
          class="mt-1 rounded-lg border border-slate-300 px-2.5 py-1.5 text-sm focus:border-blue-500 focus:outline-none"
        >
          <option value="">Semua status</option>
          <option value="SUCCESS">SUCCESS</option>
          <option value="FAILED">FAILED</option>
          <option value="RUNNING">RUNNING</option>
        </select>
      </div>
      <div>
        <label for="job-limit" class="block text-xs font-medium text-slate-600">Tampilkan</label>
        <select
          id="job-limit"
          v-model.number="limit"
          class="mt-1 rounded-lg border border-slate-300 px-2.5 py-1.5 text-sm focus:border-blue-500 focus:outline-none"
          @change="loadJobs"
        >
          <option :value="50">50 terbaru</option>
          <option :value="100">100 terbaru</option>
          <option :value="200">200 terbaru</option>
        </select>
      </div>
    </div>

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
              <th class="px-4 py-3">Nama Job</th>
              <th class="px-4 py-3">Run Key</th>
              <th class="px-4 py-3">Status</th>
              <th class="px-4 py-3">Mulai</th>
              <th class="px-4 py-3">Durasi</th>
              <th class="px-4 py-3 text-right">Ringkasan</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            <tr v-if="loading && runs.length === 0">
              <td colspan="6" class="px-4 py-10 text-center text-slate-500">
                Memuat riwayat jobs…
              </td>
            </tr>
            <tr v-else-if="filteredRuns.length === 0">
              <td colspan="6" class="px-4 py-10 text-center text-slate-500">
                Belum ada riwayat eksekusi job.
              </td>
            </tr>
            <template v-for="run in filteredRuns" :key="run.id">
              <tr class="hover:bg-slate-50/80">
                <td class="px-4 py-2.5 font-mono text-xs font-semibold text-slate-900">
                  {{ run.job_name }}
                </td>
                <td class="whitespace-nowrap px-4 py-2.5 font-mono text-xs text-slate-700">
                  {{ run.run_key }}
                </td>
                <td class="px-4 py-2.5">
                  <span
                    class="inline-flex rounded-full border px-2 py-0.5 text-xs font-semibold"
                    :class="statusClass(run.status)"
                  >
                    {{ run.status }}
                  </span>
                </td>
                <td class="whitespace-nowrap px-4 py-2.5 text-xs text-slate-600">
                  {{ formatDateTime(run.started_at) }}
                </td>
                <td class="whitespace-nowrap px-4 py-2.5 text-xs text-slate-600">
                  {{ durationText(run) }}
                </td>
                <td class="px-4 py-2.5 text-right">
                  <button
                    v-if="run.summary || run.error"
                    type="button"
                    class="text-xs font-medium text-blue-600 hover:underline"
                    @click="toggleExpand(run.id)"
                  >
                    {{ expandedId === run.id ? 'Tutup Detail' : 'Lihat Detail' }}
                  </button>
                  <span v-else class="text-xs text-slate-400">—</span>
                </td>
              </tr>
              <tr v-if="expandedId === run.id" class="bg-slate-50">
                <td colspan="6" class="space-y-2 px-4 py-3">
                  <div
                    v-if="run.error"
                    class="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-800"
                  >
                    <span class="font-semibold">Error:</span> {{ run.error }}
                  </div>
                  <div v-if="run.summary">
                    <p class="mb-1 text-xs font-semibold text-slate-600">Summary JSON</p>
                    <pre
                      class="max-h-60 overflow-auto rounded-lg border border-slate-200 bg-white p-2.5 font-mono text-xs text-slate-700"
                      >{{ formatJson(run.summary) }}</pre>
                  </div>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>
