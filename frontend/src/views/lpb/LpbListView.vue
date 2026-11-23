<script setup lang="ts">
// Pola halaman: List / Index.
import ListPage from '@/components/ListPage.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import type { Column } from '@/components/DataTable.vue'
import type { FilterDef } from '@/components/FilterPanel.vue'
import { usePermission } from '@/composables/usePermission'
import { useServerTable } from '@/composables/useServerTable'
import { formatDate, formatDateTime } from '@/lib/format'
import { STATUS_LABELS, SUB_TYPE_LABELS, subTypeLabel } from '@/lib/lpb'
import { rowLink } from '@/lib/ui'
import { lpbService } from '@/services/lpbService'
import type { LPBListItem } from '@/types/lpb'

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
</template>
