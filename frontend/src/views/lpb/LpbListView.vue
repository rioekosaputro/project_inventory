<script setup lang="ts">
// Pola halaman: List / Index + slide-over Import Stok Awal (Excel/CSV → draft LPB INITIAL_STOCK).
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import type { Column } from '@/components/DataTable.vue'
import type { FilterDef } from '@/components/FilterPanel.vue'
import ListPage from '@/components/ListPage.vue'
import SlideOver from '@/components/SlideOver.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { usePermission } from '@/composables/usePermission'
import { useServerTable } from '@/composables/useServerTable'
import { toApiError, userMessage } from '@/lib/apiError'
import { formatDate, formatDateTime } from '@/lib/format'
import { STATUS_LABELS, SUB_TYPE_LABELS, subTypeLabel } from '@/lib/lpb'
import { downloadBlob } from '@/lib/pdf'
import { btnPrimary, btnSecondary, rowLink } from '@/lib/ui'
import { lpbService } from '@/services/lpbService'
import { warehouseService } from '@/services/masterService'
import { useAuthStore } from '@/stores/auth'
import type { FieldError } from '@/types/api'
import type { LPBListItem } from '@/types/lpb'
import type { Warehouse } from '@/types/master'

const router = useRouter()
const toast = useToast()
const auth = useAuthStore()
const { can } = usePermission()

const table = useServerTable<LPBListItem>({
  fetch: (p) => lpbService.list(p),
  defaultSort: '-created_at',
  filterKeys: ['status', 'sub_type', 'mine'],
})

const columns: Column[] = [
  { key: 'lpb_number', label: 'Nomor LPB', sortKey: 'lpb_number' },
  { key: 'doc_date', label: 'Tanggal', sortKey: 'doc_date' },
  { key: 'sub_type', label: 'Jenis' },
  { key: 'warehouse_code', label: 'Gudang', sortKey: 'warehouse' },
  { key: 'supplier_name', label: 'Supplier' },
  { key: 'total_qty', label: 'Isi' },
  { key: 'created_at', label: 'Dibuat', sortKey: 'created_at' },
  { key: 'status', label: 'Status', sortKey: 'status' },
]

const filterDefs: FilterDef[] = [
  {
    key: 'status',
    label: 'Status',
    options: Object.entries(STATUS_LABELS).map(([value, label]) => ({ value, label })),
  },
  {
    key: 'sub_type',
    label: 'Jenis',
    options: Object.entries(SUB_TYPE_LABELS).map(([value, label]) => ({ value, label })),
  },
  { key: 'mine', label: 'Pembuat', options: [{ value: 'true', label: 'Dokumen saya' }] },
]

// ---- Import Stok Awal
const importOpen = ref(false)
const warehouses = ref<Warehouse[]>([])
const warehouseId = ref<number | ''>('')
const docDate = ref('')
const notes = ref('')
const selectedFile = ref<File | null>(null)
const importing = ref(false)
const downloadingTmpl = ref(false)
const importError = ref('')
const rowErrors = ref<FieldError[]>([])

async function openImport(): Promise<void> {
  importError.value = ''
  rowErrors.value = []
  selectedFile.value = null
  notes.value = ''
  importOpen.value = true
  if (warehouses.value.length === 0) {
    try {
      warehouses.value = (
        await warehouseService.list({ limit: 100, is_active: true, sort: 'code' })
      ).items
    } catch (e) {
      importError.value = userMessage(toApiError(e))
    }
  }
  if (auth.user?.warehouse_id) {
    warehouseId.value = auth.user.warehouse_id
  } else if (warehouses.value.length === 1) {
    warehouseId.value = warehouses.value[0]!.id
  }
}

function onFileChange(ev: Event): void {
  const input = ev.target as HTMLInputElement
  selectedFile.value = input.files?.[0] ?? null
}

async function downloadTemplate(): Promise<void> {
  if (downloadingTmpl.value) return
  downloadingTmpl.value = true
  try {
    const blob = await lpbService.downloadInitialStockTemplate()
    downloadBlob(blob, 'template-stok-awal.xlsx')
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: 'Gagal mengunduh template',
      detail: userMessage(toApiError(e)),
      life: 7000,
    })
  } finally {
    downloadingTmpl.value = false
  }
}

async function submitImport(): Promise<void> {
  importError.value = ''
  rowErrors.value = []
  if (!warehouseId.value) {
    importError.value = 'Pilih gudang tujuan terlebih dahulu.'
    return
  }
  if (!selectedFile.value) {
    importError.value = 'Pilih berkas Excel (.xlsx) atau CSV terlebih dahulu.'
    return
  }
  importing.value = true
  try {
    const created = await lpbService.importInitialStock(
      Number(warehouseId.value),
      selectedFile.value,
      { docDate: docDate.value || undefined, notes: notes.value.trim() || undefined },
    )
    importOpen.value = false
    toast.add({
      severity: 'success',
      summary: `Draft ${created.lpb_number} berhasil dibuat dari impor stok awal`,
      life: 5000,
    })
    await router.push(`/lpb/${created.id}`)
  } catch (e) {
    const err = toApiError(e)
    importError.value = err.message
    rowErrors.value = err.fieldErrors
  } finally {
    importing.value = false
  }
}
</script>

<template>
  <ListPage
    title="LPB (Lembar Penerimaan Barang)"
    description="Dokumen barang masuk dan keluar gudang. Stok baru berubah setelah LPB disetujui."
    entity="LPB"
    empty-value="Buat LPB pertama untuk mencatat barang yang masuk ke gudang. Stok baru bertambah setelah kepala gudang menyetujuinya."
    caption="Daftar LPB"
    search-label="Cari nomor LPB, referensi, atau supplier"
    :columns="columns"
    :table="table"
    :filter-defs="filterDefs"
    create-label="Buat LPB"
    :create-to="{ path: '/lpb/baru' }"
    :can-create="can('lpb.create')"
  >
    <template v-if="can('lpb.create')" #actions>
      <button type="button" :class="btnSecondary" @click="openImport">
        <i class="pi pi-upload text-xs" aria-hidden="true" />Import Stok Awal (Excel/CSV)
      </button>
    </template>

    <template #cell-lpb_number="{ row }">
      <RouterLink :to="`/lpb/${row.id}`" :class="rowLink">{{ row.lpb_number }}</RouterLink>
    </template>
    <template #cell-doc_date="{ row }">{{ formatDate(row.doc_date) }}</template>
    <template #cell-sub_type="{ row }">{{ subTypeLabel(row.sub_type) }}</template>
    <template #cell-supplier_name="{ row }">{{ row.supplier_name ?? '—' }}</template>
    <template #cell-total_qty="{ row }">
      {{ row.item_count }} item · {{ row.total_qty }} unit
    </template>
    <template #cell-created_at="{ row }">
      {{ formatDateTime(row.created_at) }}
      <span class="block text-xs text-slate-600">{{ row.created_by_name }}</span>
    </template>
    <template #cell-status="{ row }"><StatusBadge :value="row.status" /></template>
  </ListPage>

  <SlideOver :open="importOpen" title="Import Stok Awal (Excel / CSV)" @close="importOpen = false">
    <form class="flex flex-col gap-4" @submit.prevent="submitImport">
      <div class="rounded-lg border border-slate-200 bg-slate-50 p-3 text-xs text-slate-700">
        <p>
          Impor akan membuat <strong>Draft LPB Stok Awal</strong>. Stok baru bertambah setelah
          dokumen diajukan dan disetujui Kepala Gudang.
        </p>
        <button
          type="button"
          class="mt-2 font-medium text-indigo-700 hover:underline"
          :aria-busy="downloadingTmpl"
          @click="downloadTemplate"
        >
          <i class="pi pi-download mr-1 text-xs" aria-hidden="true" />
          {{ downloadingTmpl ? 'Menyiapkan template…' : 'Unduh Template Excel (.xlsx)' }}
        </button>
      </div>

      <div
        v-if="importError"
        class="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-800"
        role="alert"
      >
        <div class="font-medium">{{ importError }}</div>
        <ul v-if="rowErrors.length > 0" class="mt-1.5 list-disc pl-5 text-xs">
          <li v-for="(fe, idx) in rowErrors" :key="idx">
            <span class="font-mono">{{ fe.field }}</span
            >: {{ fe.message }}
          </li>
        </ul>
      </div>

      <div>
        <label for="imp-wh" class="block text-sm font-medium text-slate-800">Gudang Tujuan</label>
        <select
          id="imp-wh"
          v-model="warehouseId"
          class="mt-1 w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm"
        >
          <option value="">Pilih gudang…</option>
          <option v-for="w in warehouses" :key="w.id" :value="w.id">
            {{ w.code }} — {{ w.name }}
          </option>
        </select>
      </div>

      <div>
        <label for="imp-date" class="block text-sm font-medium text-slate-800">
          Tanggal Dokumen (opsional)
        </label>
        <input
          id="imp-date"
          v-model="docDate"
          type="date"
          class="mt-1 w-full rounded-lg border border-slate-300 px-3 py-2 text-sm"
        />
      </div>

      <div>
        <label for="imp-file" class="block text-sm font-medium text-slate-800">
          Berkas Excel (.xlsx) atau CSV
        </label>
        <input
          id="imp-file"
          type="file"
          accept=".xlsx,.csv,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
          class="mt-1 block w-full text-sm text-slate-700"
          @change="onFileChange"
        />
      </div>

      <div>
        <label for="imp-notes" class="block text-sm font-medium text-slate-800">
          Catatan (opsional)
        </label>
        <textarea
          id="imp-notes"
          v-model="notes"
          rows="2"
          class="mt-1 w-full rounded-lg border border-slate-300 px-3 py-2 text-sm"
          placeholder="Mis. Stok awal hasil opname pembukaan"
        />
      </div>

      <div class="mt-2 flex justify-end gap-2">
        <button type="button" :class="btnSecondary" @click="importOpen = false">Batal</button>
        <button type="submit" :class="btnPrimary" :aria-busy="importing">
          {{ importing ? 'Mengimpor…' : 'Impor ke Draft LPB' }}
        </button>
      </div>
    </form>
  </SlideOver>
</template>
