<script setup lang="ts">
import { vStackTable } from '@/directives/stackTable'
// Halaman Admin Rekonsiliasi Invarian Stok I1–I6 (plan Bagian 5.9 & T4.2).
import { onMounted, ref } from 'vue'
import { toApiError, userMessage } from '@/lib/apiError'
import { formatDateTime } from '@/lib/format'
import { btnPrimary } from '@/lib/ui'
import { adminService, type ReconcileReport } from '@/services/adminService'

const report = ref<ReconcileReport | null>(null)
const loading = ref(true)
const error = ref('')

const invariantChecklist = [
  { code: 'I1', label: 'Saldo rak produk SN = jumlah unit fisik di rak' },
  { code: 'I1b', label: 'Tidak ada unit fisik SN di rak tanpa baris saldo' },
  { code: 'I2', label: 'qty_reserved produk SN = jumlah unit RESERVED di rak' },
  { code: 'I3', label: 'Saldo rak = Σ ledger IN − Σ ledger OUT' },
  { code: 'I3b', label: 'Tidak ada baris ledger tanpa baris saldo' },
  { code: 'I4', label: 'Tidak ada saldo negatif atau reserved > on_hand' },
  { code: 'I5', label: 'Setiap baris ledger memenuhi balance_after = balance_before ± qty' },
  { code: 'I5b', label: 'Rantai ledger per (produk, rak) berkesinambungan tanpa celah' },
  { code: 'I6', label: 'Unit di gudang punya rak; unit tersewa punya pelanggan' },
]

async function runCheck(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    report.value = await adminService.reconcile()
  } catch (err) {
    error.value = userMessage(toApiError(err))
  } finally {
    loading.value = false
  }
}

onMounted(runCheck)
</script>

<template>
  <div class="space-y-5">
    <header class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 class="text-xl font-bold text-slate-900">Rekonsiliasi Invarian Stok</h1>
        <p class="mt-0.5 text-sm text-slate-600">
          Verifikasi konsistensi saldo rak, serial number, reservasi DO, dan buku besar stok
          (I1–I6).
        </p>
      </div>
      <button type="button" :class="btnPrimary" :disabled="loading" @click="runCheck">
        <i class="pi pi-sync text-xs" :class="{ 'pi-spin': loading }" aria-hidden="true" />
        {{ loading ? 'Memeriksa…' : 'Jalankan Pemeriksaan' }}
      </button>
    </header>

    <div
      v-if="error"
      role="alert"
      class="rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800"
    >
      {{ error }}
    </div>

    <div
      v-else-if="loading && !report"
      class="py-12 text-center text-sm text-slate-500"
      role="status"
    >
      Menjalankan 9 pemeriksaan invarian stok…
    </div>

    <template v-else-if="report">
      <section
        class="rounded-xl border p-5"
        :class="
          report.healthy
            ? 'border-emerald-200 bg-emerald-50 text-emerald-900'
            : 'border-rose-200 bg-rose-50 text-rose-900'
        "
      >
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div class="flex items-center gap-2.5">
            <i
              class="pi text-xl"
              :class="
                report.healthy
                  ? 'pi-check-circle text-emerald-600'
                  : 'pi-exclamation-triangle text-rose-600'
              "
              aria-hidden="true"
            />
            <div>
              <p class="font-semibold">
                {{
                  report.healthy
                    ? `Semua ${report.total_checks} invarian stok konsisten (0 anomali)`
                    : `Ditemukan ${report.violation_count} pelanggaran invarian stok`
                }}
              </p>
              <p class="text-xs opacity-80">
                Terakhir diperiksa: {{ formatDateTime(report.checked_at) }}
              </p>
            </div>
          </div>
        </div>
      </section>

      <section
        v-if="report.violations.length > 0"
        class="rounded-xl border border-rose-200 bg-white p-5"
      >
        <h2 class="text-sm font-semibold text-rose-800">Detail Anomali</h2>
        <div class="mt-3 overflow-x-auto">
          <table v-stack-table class="min-w-full divide-y divide-slate-200 text-xs">
            <thead>
              <tr class="text-left text-slate-500">
                <th class="py-2 pr-3">Kode</th>
                <th class="py-2 pr-3">Pemeriksaan</th>
                <th class="py-2">Baris Data</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-100">
              <tr v-for="(v, idx) in report.violations" :key="idx">
                <td class="py-2 pr-3 font-mono font-bold text-rose-700">{{ v.code }}</td>
                <td class="py-2 pr-3 text-slate-900">{{ v.check }}</td>
                <td class="py-2 font-mono text-slate-600">{{ v.detail }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section class="rounded-xl border border-slate-200 bg-white p-5">
        <h2 class="text-sm font-semibold text-slate-900">Daftar Invarian yang Diperiksa</h2>
        <ul class="mt-3 divide-y divide-slate-100 text-sm">
          <li
            v-for="item in invariantChecklist"
            :key="item.code"
            class="flex items-center justify-between gap-3 py-2.5"
          >
            <div class="flex items-center gap-2.5">
              <span
                class="rounded bg-slate-100 px-2 py-0.5 font-mono text-xs font-bold text-slate-700"
              >
                {{ item.code }}
              </span>
              <span class="text-slate-700">{{ item.label }}</span>
            </div>
            <span
              class="text-xs font-semibold"
              :class="
                report.violations.some((v) => v.code === item.code)
                  ? 'text-rose-700'
                  : 'text-emerald-700'
              "
            >
              {{ report.violations.some((v) => v.code === item.code) ? 'Anomali' : 'OK' }}
            </span>
          </li>
        </ul>
      </section>
    </template>
  </div>
</template>
