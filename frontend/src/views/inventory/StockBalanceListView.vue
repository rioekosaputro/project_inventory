<script setup lang="ts">
// Saldo stok per produk & rak (plan Bagian 9.4: /stok).
import { computed } from 'vue'
import type { Column } from '@/components/DataTable.vue'
import type { FilterDef } from '@/components/FilterPanel.vue'
import ListPage from '@/components/ListPage.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { useOptions } from '@/composables/useOptions'
import { usePermission } from '@/composables/usePermission'
import { useServerTable } from '@/composables/useServerTable'
import { rowLink } from '@/lib/ui'
import { inventoryService } from '@/services/inventoryService'
import { warehouseService } from '@/services/masterService'
import type { StockBalance } from '@/types/inventory'

const { can } = usePermission()

const table = useServerTable<StockBalance>({
  fetch: (p) => inventoryService.balances(p),
  defaultSort: 'sku,warehouse,rack',
  filterKeys: ['warehouse_id', 'low_stock'],
})

const warehouses = useOptions(
  () => warehouseService.list({ limit: 100, sort: 'code' }),
  (w) => ({ value: String(w.id), label: `${w.code} — ${w.name}` }),
)

const columns: Column[] = [
  { key: 'product', label: 'Produk', sortKey: 'sku' },
  { key: 'location', label: 'Gudang / Rak', sortKey: 'warehouse' },
  { key: 'qty_on_hand', label: 'Fisik', sortKey: 'qty_on_hand' },
  { key: 'qty_reserved', label: 'Direservasi' },
  { key: 'qty_available', label: 'Tersedia', sortKey: 'qty_available' },
  { key: 'status', label: 'Keterangan' },
]

const filterDefs = computed<FilterDef[]>(() => [
  { key: 'warehouse_id', label: 'Gudang', options: warehouses.options.value },
  {
    key: 'low_stock',
    label: 'Stok menipis',
    options: [{ value: 'true', label: 'Hanya stok menipis' }],
  },
])
</script>

<template>
  <ListPage
    title="Saldo Stok"
    description="Saldo stok fisik, reservasi, dan tersedia per produk dan rak. Rak non-sellable tidak dihitung tersedia."
    entity="saldo stok"
    empty-value="Saldo stok akan muncul setelah LPB masuk pertama disetujui."
    caption="Daftar saldo stok"
    search-label="Cari SKU atau nama produk"
    :columns="columns"
    :table="table"
    :filter-defs="filterDefs"
    :can-create="false"
  >
    <template #cell-product="{ row }">
      <RouterLink
        v-if="can('ledger.read')"
        :to="`/stok/kartu-stok?product_id=${row.product_id}&rack_id=${row.rack_id}`"
        :class="rowLink"
      >
        {{ row.sku }} — {{ row.product_name }}
      </RouterLink>
      <span v-else class="font-medium text-slate-900">{{ row.sku }} — {{ row.product_name }}</span>
      <div class="text-xs text-slate-500">
        {{ row.has_serial_number ? 'Serial Number' : 'Non-SN' }} · {{ row.unit }}
      </div>
    </template>
    <template #cell-location="{ row }">
      <div class="font-mono text-xs font-semibold text-slate-900">
        {{ row.warehouse_code }} / {{ row.rack_code }}
      </div>
      <div class="text-xs text-slate-500">
        {{ row.zone ?? '—' }}{{ row.is_sellable ? '' : ' (Non-sellable)' }}
      </div>
    </template>
    <template #cell-qty_on_hand="{ row }">
      <span class="font-mono font-semibold">{{ row.qty_on_hand }}</span>
    </template>
    <template #cell-qty_reserved="{ row }">
      <span class="font-mono text-slate-600">{{ row.qty_reserved }}</span>
    </template>
    <template #cell-qty_available="{ row }">
      <span class="font-mono font-semibold text-emerald-700">{{ row.qty_available }}</span>
    </template>
    <template #cell-status="{ row }">
      <StatusBadge v-if="row.is_low_stock" value="LOW_STOCK" />
      <span v-else class="text-xs text-slate-500">Aman</span>
    </template>
  </ListPage>
</template>
