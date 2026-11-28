<script setup lang="ts">
// Pola halaman: List / Index — Daftar Tagihan, Generate Bulanan (Preview/Run), Export Excel (T2.9).
import { ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import ListPage from '@/components/ListPage.vue'
import SlideOver from '@/components/SlideOver.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import type { Column } from '@/components/DataTable.vue'
import type { FilterDef } from '@/components/FilterPanel.vue'
import { usePermission } from '@/composables/usePermission'
import { useServerTable } from '@/composables/useServerTable'
import { toApiError, userMessage } from '@/lib/apiError'
import { formatRupiah } from '@/lib/format'
import { btnPrimary, btnRow, btnSecondary, rowLink } from '@/lib/ui'
import { invoiceService } from '@/services/invoiceService'
import type { BillingRunResult, Invoice } from '@/types/invoice'

const { can } = usePermission()
const toast = useToast()

const table = useServerTable<Invoice>({
  fetch: (p) => invoiceService.list(p),
  defaultSort: '-invoice_date',
  filterKeys: ['status', 'invoice_type', 'overdue'],
})

const columns: Column[] = [
  { key: 'invoice_number', label: 'No. Invoice', sortKey: 'invoice_number' },
  { key: 'customer_name', label: 'Pelanggan' },
  { key: 'invoice_date', label: 'Tgl / Jatuh Tempo', sortKey: 'due_date' },
  { key: 'grand_total', label: 'Grand Total', sortKey: 'grand_total' },
  { key: 'remaining_amount', label: 'Sisa Piutang' },
  { key: 'status', label: 'Status' },
  { key: 'actions', label: 'Tindakan', actions: true },
]

const filterDefs: FilterDef[] = [
  {
    key: 'status',
    label: 'Status',
    options: [
      { value: 'DRAFT', label: 'Draft' },
      { value: 'ISSUED', label: 'Diterbitkan' },
      { value: 'PARTIALLY_PAID', label: 'Dibayar Sebagian' },
      { value: 'PAID', label: 'Lunas' },
      { value: 'VOID', label: 'Void' },
    ],
  },
  {
    key: 'invoice_type',
    label: 'Jenis',
    options: [
      { value: 'RENTAL', label: 'Sewa Bulanan' },
      { value: 'CONSUMABLE', label: 'Consumable' },
      { value: 'SERVICE', label: 'Service' },
    ],
  },
  {
    key: 'overdue',
    label: 'Jatuh Tempo',
    options: [{ value: 'true', label: 'Lewat Jatuh Tempo' }],
  },
]

// Generate billing slide-over
const genOpen = ref(false)
const genDate = ref(new Date().toISOString().slice(0, 10))
const genBusy = ref(false)
const genError = ref<string | null>(null)
const genResult = ref<BillingRunResult | null>(null)

function openGenerate(): void {
  genError.value = null
  genResult.value = null
  genOpen.value = true
}

async function runGenerate(preview: boolean): Promise<void> {
  genBusy.value = true
  genError.value = null
  try {
    const res = await invoiceService.generate({ run_date: genDate.value || null }, preview)
    genResult.value = res
    if (!preview) {
      toast.add({
        severity: 'success',
        summary: 'Berhasil',
        detail: `${res.invoices_created} invoice draft dibuat (${res.skipped} dilewati)`,
        life: 4000,
      })
      await table.reload()
    }
  } catch (err) {
    genError.value = userMessage(toApiError(err))
  } finally {
    genBusy.value = false
  }
}

const exporting = ref(false)
async function exportExcel(): Promise<void> {
  exporting.value = true
  try {
    const blob = await invoiceService.exportExcel()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `tagihan-${new Date().toISOString().slice(0, 10)}.xlsx`
    a.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  } catch (err) {
    toast.add({
      severity: 'error',
      summary: 'Gagal',
      detail: userMessage(toApiError(err)),
      life: 5000,
    })
  } finally {
    exporting.value = false
  }
}
</script>

<template>
  <ListPage
    title="Tagihan & Invoice"
    description="Daftar tagihan sewa bulanan & penjualan, penerbitan faktur, pembayaran + PPh 23, dan rekap PPN."
    entity="invoice"
    empty-value="Jalankan Generate Tagihan Bulanan untuk membuat draft invoice dari kontrak aktif."
    caption="Daftar invoice"
    search-label="Cari nomor invoice, pelanggan, atau nomor faktur pajak"
    :columns="columns"
    :table="table"
    :filter-defs="filterDefs"
    :can-create="false"
  >
    <template #top>
      <div class="flex flex-wrap items-center justify-end gap-2">
        <button type="button" :disabled="exporting" :class="btnSecondary" @click="exportExcel">
          <i class="pi pi-file-excel text-xs" aria-hidden="true" />
          {{ exporting ? 'Menyiapkan Excel…' : 'Export Excel (Rekap PPN)' }}
        </button>
        <button
          v-if="can('invoice.create')"
          type="button"
          :class="btnPrimary"
          @click="openGenerate"
        >
          <i class="pi pi-bolt text-xs" aria-hidden="true" />
          Generate Tagihan Bulanan
        </button>
      </div>
    </template>

    <template #cell-invoice_number="{ row }">
      <RouterLink :to="`/invoices/${row.id}`" :class="rowLink" class="font-mono text-xs">
        {{ row.invoice_number }}
      </RouterLink>
    </template>
    <template #cell-customer_name="{ row }">
      <div class="font-medium text-slate-900">{{ row.customer_name }}</div>
      <div class="font-mono text-xs text-slate-500">
        {{ row.customer_code }}
        <template v-if="row.contract_number"> · {{ row.contract_number }}</template>
      </div>
    </template>
    <template #cell-invoice_date="{ row }">
      <div class="text-xs text-slate-800">{{ row.invoice_date }}</div>
      <div class="text-xs text-slate-500">JT: {{ row.due_date }}</div>
    </template>
    <template #cell-grand_total="{ row }">
      <span class="font-medium text-slate-900">{{ formatRupiah(row.grand_total) }}</span>
    </template>
    <template #cell-remaining_amount="{ row }">
      <span
        class="font-mono text-xs font-medium"
        :class="row.remaining_amount === '0.00' ? 'text-emerald-700' : 'text-amber-700'"
      >
        {{ formatRupiah(row.remaining_amount) }}
      </span>
    </template>
    <template #cell-status="{ row }">
      <StatusBadge :value="row.status" />
    </template>
    <template #cell-actions="{ row }">
      <div class="flex justify-end">
        <RouterLink :to="`/invoices/${row.id}`" :class="btnRow">Detail</RouterLink>
      </div>
    </template>
  </ListPage>

  <SlideOver :open="genOpen" title="Generate Tagihan Bulanan" @close="genOpen = false">
    <div class="space-y-4">
      <p class="text-xs text-slate-600">
        Menghitung tagihan sewa bulanan (arrears, PRORATA / FULL_MONTH, PPN PMK 131/2024) untuk
        kontrak yang tanggal tagihnya cocok dengan tanggal jalan. Gunakan <strong>Preview</strong>
        untuk simulasi tanpa menyimpan.
      </p>
      <div
        v-if="genError"
        class="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-800"
        role="alert"
      >
        {{ genError }}
      </div>
      <div>
        <label for="gen-date" class="block text-xs font-medium text-slate-700">
          Tanggal Jalan (WIB)
        </label>
        <input
          id="gen-date"
          v-model="genDate"
          type="date"
          class="mt-1 w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm"
        />
      </div>
      <div class="flex flex-wrap gap-2">
        <button type="button" :disabled="genBusy" :class="btnSecondary" @click="runGenerate(true)">
          {{ genBusy ? 'Memproses…' : 'Preview (Dry-Run)' }}
        </button>
        <button type="button" :disabled="genBusy" :class="btnPrimary" @click="runGenerate(false)">
          {{ genBusy ? 'Memproses…' : 'Jalankan Generate' }}
        </button>
      </div>

      <div v-if="genResult" class="space-y-3 rounded-lg border border-slate-200 bg-slate-50 p-3">
        <div class="text-xs font-semibold text-slate-800">
          Hasil {{ genResult.preview ? 'Preview' : 'Generate' }} ({{ genResult.run_date }}):
          {{ genResult.invoices_created }} dibuat · {{ genResult.skipped }} dilewati ·
          {{ genResult.errors }} error
        </div>
        <div v-if="genResult.results.length === 0" class="text-xs text-slate-500">
          Tidak ada kontrak yang jatuh hari tagih pada tanggal ini.
        </div>
        <div v-else class="space-y-2">
          <div
            v-for="r in genResult.results"
            :key="r.contract_id"
            class="rounded border border-slate-200 bg-white p-2.5 text-xs"
          >
            <div class="flex items-center justify-between gap-2">
              <span class="font-mono font-medium">{{ r.contract_number }}</span>
              <StatusBadge :value="r.status" />
            </div>
            <div class="text-slate-600">
              {{ r.customer_name }} · {{ r.period_start }} s/d {{ r.period_end }}
            </div>
            <div v-if="r.grand_total !== '0.00'" class="mt-1 font-medium text-slate-900">
              Grand Total: {{ formatRupiah(r.grand_total) }} (PPN: {{ formatRupiah(r.tax_amount) }})
            </div>
          </div>
        </div>
      </div>
    </div>
  </SlideOver>
</template>
