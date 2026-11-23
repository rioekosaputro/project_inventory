<script setup lang="ts">
// Pola halaman: List / Index (antrean kerja). Keputusan dibuat di halaman rincian LPB — di sana
// saldo sebelum → sesudah per item terlihat sebelum menyetujui.
import ListPage from '@/components/ListPage.vue'
import type { Column } from '@/components/DataTable.vue'
import type { FilterDef } from '@/components/FilterPanel.vue'
import { useServerTable } from '@/composables/useServerTable'
import { formatDate, formatDateTime } from '@/lib/format'
import { SUB_TYPE_LABELS, subTypeLabel } from '@/lib/lpb'
import { btnPrimary, rowLink } from '@/lib/ui'
import { lpbService } from '@/services/lpbService'
import type { LPBListItem } from '@/types/lpb'

// Tertua dulu: yang paling lama menunggu dikerjakan lebih dulu.
const table = useServerTable<LPBListItem>({
  fetch: (p) => lpbService.approvals(p),
  defaultSort: 'submitted_at',
  filterKeys: ['sub_type'],
})

const columns: Column[] = [
  { key: 'lpb_number', label: 'Nomor LPB', sortKey: 'lpb_number' },
  { key: 'submitted_at', label: 'Diajukan', sortKey: 'submitted_at' },
  { key: 'sub_type', label: 'Jenis' },
  { key: 'warehouse_code', label: 'Gudang', sortKey: 'warehouse' },
  { key: 'created_by_name', label: 'Pengaju' },
  { key: 'total_qty', label: 'Isi' },
  { key: 'actions', label: 'Tindakan', actions: true },
]

const filterDefs: FilterDef[] = [
  {
    key: 'sub_type',
    label: 'Jenis',
    options: Object.entries(SUB_TYPE_LABELS).map(([value, label]) => ({ value, label })),
  },
]
</script>

<template>
  <ListPage
    title="Persetujuan LPB"
    description="LPB yang menunggu keputusan Anda, yang paling lama menunggu di atas. Periksa dampak stoknya sebelum menyetujui."
    entity="LPB"
    empty-value="Tidak ada LPB yang menunggu persetujuan. Dokumen baru muncul di sini setelah staf gudang mengajukannya."
    caption="Antrean persetujuan LPB"
    search-label="Cari nomor LPB, referensi, atau supplier"
    :columns="columns"
    :table="table"
    :filter-defs="filterDefs"
  >
    <template #cell-lpb_number="{ row }">
      <RouterLink :to="`/lpb/${row.id}`" :class="rowLink">{{ row.lpb_number }}</RouterLink>
      <span class="block text-xs text-slate-600">{{ formatDate(row.doc_date) }}</span>
    </template>
    <template #cell-submitted_at="{ row }">{{ formatDateTime(row.submitted_at) }}</template>
    <template #cell-sub_type="{ row }">{{ subTypeLabel(row.sub_type) }}</template>
    <template #cell-total_qty="{ row }">
      {{ row.item_count }} item · {{ row.total_qty }} unit
    </template>
    <template #cell-actions="{ row }">
      <div class="flex justify-end">
        <RouterLink :to="`/lpb/${row.id}`" :class="btnPrimary">
          Tinjau<span class="sr-only"> LPB {{ row.lpb_number }}</span>
        </RouterLink>
      </div>
    </template>
  </ListPage>
</template>
