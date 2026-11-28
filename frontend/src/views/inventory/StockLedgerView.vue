<script setup lang="ts">
// Buku Besar Mutasi Stok (plan Bagian 9.4: /stok/buku-besar) + Export Excel.
import { computed, ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import type { Column } from '@/components/DataTable.vue'
import type { FilterDef } from '@/components/FilterPanel.vue'
import ListPage from '@/components/ListPage.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { useOptions } from '@/composables/useOptions'
import { useServerTable } from '@/composables/useServerTable'
import { toApiError, userMessage } from '@/lib/apiError'
import { formatDateTime } from '@/lib/format'
import { subTypeLabel } from '@/lib/lpb'
import { downloadBlob } from '@/lib/pdf'
import { btnSecondary, rowLink } from '@/lib/ui'
import { inventoryService } from '@/services/inventoryService'
import { warehouseService } from '@/services/masterService'
import type { LedgerRow } from '@/types/inventory'

const toast = useToast()
const exporting = ref(false)

const table = useServerTable<LedgerRow>({
  fetch: (p) => inventoryService.ledgers(p),
  defaultSort: '-occurred_at,-id',
  filterKeys: ['direction', 'warehouse_id', 'date_from', 'date_to'],
})

const warehouses = useOptions(
  () => warehouseService.list({ limit: 100, sort: 'code' }),
  (w) => ({ value: String(w.id), label: `${w.code} — ${w.name}` }),
)

const columns: Column[] = [
  { key: 'occurred_at', label: 'Waktu', sortKey: 'occurred_at' },
  { key: 'lpb_number', label: 'Dokumen LPB' },
  { key: 'product', label: 'Produk' },
  { key: 'location', label: 'Gudang / Rak' },
  { key: 'direction', label: 'Mutasi', sortKey: 'qty' },
  { key: 'balance', label: 'Saldo Rak' },
]

const filterDefs = computed<FilterDef[]>(() => [
  {
    key: 'direction',
    label: 'Arah',
    options: [
      { value: 'IN', label: 'Masuk (IN)' },
      { value: 'OUT', label: 'Keluar (OUT)' },
    ],
  },
  { key: 'warehouse_id', label: 'Gudang', options: warehouses.options.value },
  { key: 'date_from', label: 'Dari tanggal (YYYY-MM-DD)', type: 'text' },
  { key: 'date_to', label: 'Sampai tanggal (YYYY-MM-DD)', type: 'text' },
])

async function exportExcel(): Promise<void> {
  if (exporting.value) return
  exporting.value = true
  try {
    const blob = await inventoryService.exportLedgersXlsx({
      q: table.q.value || undefined,
      ...table.filters.value,
    })
    downloadBlob(blob, 'buku-besar-stok.xlsx')
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: 'Gagal mengekspor Excel',
      detail: userMessage(toApiError(e)),
      life: 7000,
    })
  } finally {
    exporting.value = false
  }
}
</script>

<template>
  <ListPage
    title="Buku Besar Stok"
    description="Catatan mutasi stok append-only dari setiap LPB yang disetujui."
    entity="mutasi stok"
    empty-value="Mutasi stok akan tercatat otomatis setiap kali LPB disetujui."
    caption="Daftar buku besar mutasi stok"
    search-label="Cari SKU, produk, atau nomor LPB"
    :columns="columns"
    :table="table"
    :filter-defs="filterDefs"
    :can-create="false"
  >
    <template #actions>
      <RouterLink to="/stok/kartu-stok" :class="btnSecondary">
        <i class="pi pi-file-excel text-xs" aria-hidden="true" />Buka Kartu Stok
      </RouterLink>
      <button type="button" :class="btnSecondary" :aria-busy="exporting" @click="exportExcel">
        <i class="pi pi-download text-xs" aria-hidden="true" />
        {{ exporting ? 'Menyiapkan Excel…' : 'Export Excel' }}
      </button>
    </template>

    <template #cell-occurred_at="{ row }">
      <div class="text-xs text-slate-900">{{ formatDateTime(row.occurred_at) }}</div>
      <div class="text-xs text-slate-500">{{ row.created_by_name }}</div>
    </template>
    <template #cell-lpb_number="{ row }">
      <RouterLink :to="`/lpb/${row.lpb_id}`" :class="rowLink" class="font-mono text-xs">
        {{ row.lpb_number }}
      </RouterLink>
      <div class="text-xs text-slate-500">{{ subTypeLabel(row.movement_type) }}</div>
    </template>
    <template #cell-product="{ row }">
      <RouterLink
        :to="`/stok/kartu-stok?product_id=${row.product_id}&rack_id=${row.rack_id}`"
        :class="rowLink"
      >
        {{ row.sku }} — {{ row.product_name }}
      </RouterLink>
    </template>
    <template #cell-location="{ row }">
      <span class="font-mono text-xs">{{ row.warehouse_code }} / {{ row.rack_code }}</span>
    </template>
    <template #cell-direction="{ row }">
      <div class="flex items-center gap-1.5">
        <StatusBadge :value="row.direction" />
        <span class="font-mono text-sm font-semibold">
          {{ row.direction === 'IN' ? `+${row.qty}` : `-${row.qty}` }}
        </span>
      </div>
    </template>
    <template #cell-balance="{ row }">
      <span class="font-mono text-xs text-slate-700">
        {{ row.balance_before }} → <strong>{{ row.balance_after }}</strong>
      </span>
    </template>
  </ListPage>
</template>
