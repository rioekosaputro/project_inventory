<script setup lang="ts">
// Halaman verifikasi QR publik Surat Jalan / LPB (plan Bagian 7.6 & 9.4).
// Tidak memerlukan login; hanya menampilkan ringkasan dokumen tanpa harga/SN lengkap.
import { onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import StatusBadge from '@/components/StatusBadge.vue'
import { toApiError } from '@/lib/apiError'
import { formatDate, formatDateTime } from '@/lib/format'
import { subTypeLabel } from '@/lib/lpb'
import { btnSecondary } from '@/lib/ui'
import { lpbService } from '@/services/lpbService'
import type { LPBVerifySummary } from '@/types/lpb'

const route = useRoute()
const summary = ref<LPBVerifySummary | null>(null)
const loading = ref(true)
const errorMessage = ref('')
const notFound = ref(false)

async function load(): Promise<void> {
  const token = String(route.params.token ?? '').trim()
  loading.value = true
  errorMessage.value = ''
  notFound.value = false
  summary.value = null
  if (!token) {
    notFound.value = true
    loading.value = false
    return
  }
  try {
    summary.value = await lpbService.verifyPublic(token)
    document.title = `Verifikasi ${summary.value.lpb_number} — Rental ERP-Lite`
  } catch (e) {
    const apiErr = toApiError(e)
    if (apiErr.status === 404) {
      notFound.value = true
    } else {
      errorMessage.value = apiErr.message
    }
  } finally {
    loading.value = false
  }
}

onMounted(load)
watch(
  () => route.params.token,
  (next, prev) => {
    if (next !== prev) void load()
  },
)
</script>

<template>
  <div class="w-full max-w-lg rounded-2xl border border-slate-200 bg-white p-6 shadow-sm sm:p-8">
    <div class="text-center">
      <p class="text-xs font-semibold uppercase tracking-wider text-slate-500">
        Verifikasi Dokumen · Rental ERP-Lite
      </p>
    </div>

    <div v-if="loading" class="mt-6 py-8 text-center text-sm text-slate-600" role="status">
      Memeriksa keaslian dokumen…
    </div>

    <div v-else-if="notFound" class="mt-6 text-center" role="alert">
      <div
        class="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-rose-100 text-rose-700"
      >
        <i class="pi pi-times-circle text-2xl" aria-hidden="true" />
      </div>
      <h1 class="mt-3 text-lg font-semibold text-slate-900">Dokumen tidak ditemukan</h1>
      <p class="mt-1 text-sm text-slate-600">
        Kode verifikasi tidak valid atau dokumen belum disetujui. Pastikan Anda memindai QR dari
        Surat Jalan / LPB resmi.
      </p>
    </div>

    <div v-else-if="errorMessage" class="mt-6 text-center" role="alert">
      <h1 class="text-lg font-semibold text-slate-900">Gagal memverifikasi dokumen</h1>
      <p class="mt-1 text-sm text-slate-600">{{ errorMessage }}</p>
      <button type="button" :class="[btnSecondary, 'mt-4']" @click="load">Coba lagi</button>
    </div>

    <template v-else-if="summary">
      <div class="mt-5 text-center">
        <div
          class="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-emerald-100 text-emerald-700"
        >
          <i class="pi pi-verified text-2xl" aria-hidden="true" />
        </div>
        <h1 class="mt-3 text-lg font-semibold text-slate-900">Dokumen Asli &amp; Terverifikasi</h1>
        <p class="mt-1 font-mono text-sm font-semibold text-slate-800">{{ summary.lpb_number }}</p>
        <div class="mt-2">
          <StatusBadge :value="summary.status" />
        </div>
      </div>

      <dl class="mt-6 divide-y divide-slate-100 rounded-xl border border-slate-200 bg-slate-50/70">
        <div class="flex justify-between gap-4 px-4 py-3 text-sm">
          <dt class="text-slate-600">Jenis Dokumen</dt>
          <dd class="text-right font-medium text-slate-900">
            {{ subTypeLabel(summary.sub_type) }}
          </dd>
        </div>
        <div class="flex justify-between gap-4 px-4 py-3 text-sm">
          <dt class="text-slate-600">Tanggal Dokumen</dt>
          <dd class="text-right font-medium text-slate-900">{{ formatDate(summary.doc_date) }}</dd>
        </div>
        <div class="flex justify-between gap-4 px-4 py-3 text-sm">
          <dt class="text-slate-600">Gudang</dt>
          <dd class="text-right font-medium text-slate-900">
            {{ summary.warehouse_code }} — {{ summary.warehouse_name }}
          </dd>
        </div>
        <div v-if="summary.supplier_name" class="flex justify-between gap-4 px-4 py-3 text-sm">
          <dt class="text-slate-600">Supplier</dt>
          <dd class="text-right font-medium text-slate-900">{{ summary.supplier_name }}</dd>
        </div>
        <div class="flex justify-between gap-4 px-4 py-3 text-sm">
          <dt class="text-slate-600">Jumlah Item</dt>
          <dd class="text-right font-medium text-slate-900">
            {{ summary.item_count }} baris ({{ summary.total_qty }} unit/pcs)
          </dd>
        </div>
        <div class="flex justify-between gap-4 px-4 py-3 text-sm">
          <dt class="text-slate-600">Disetujui Oleh</dt>
          <dd class="text-right font-medium text-slate-900">
            {{ summary.approved_by_name ?? '—' }}
          </dd>
        </div>
        <div class="flex justify-between gap-4 px-4 py-3 text-sm">
          <dt class="text-slate-600">Waktu Persetujuan</dt>
          <dd class="text-right font-medium text-slate-900">
            {{ summary.approved_at ? formatDateTime(summary.approved_at) : '—' }}
          </dd>
        </div>
      </dl>

      <p class="mt-4 text-center text-xs text-slate-500">
        Demi keamanan, rincian harga dan daftar serial number lengkap tidak ditampilkan pada halaman
        verifikasi publik.
      </p>
    </template>
  </div>
</template>
