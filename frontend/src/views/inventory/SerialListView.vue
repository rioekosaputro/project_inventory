<script setup lang="ts">
// Daftar unit ber-Serial Number (plan Bagian 9.4: /stok/serial) + cetak label satuan.
import { computed, ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import type { Column } from '@/components/DataTable.vue'
import type { FilterDef } from '@/components/FilterPanel.vue'
import ListPage from '@/components/ListPage.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { useOptions } from '@/composables/useOptions'
import { useServerTable } from '@/composables/useServerTable'
import { toApiError, userMessage } from '@/lib/apiError'
import { openPdfBlob } from '@/lib/pdf'
import { btnRow, rowLink } from '@/lib/ui'
import { inventoryService } from '@/services/inventoryService'
import { warehouseService } from '@/services/masterService'
import type { SerialItem } from '@/types/inventory'

const toast = useToast()
const printingId = ref<number | null>(null)

const table = useServerTable<SerialItem>({
  fetch: (p) => inventoryService.serials(p),
  defaultSort: 'asset_tag',
  filterKeys: ['status', 'warehouse_id'],
})

const warehouses = useOptions(
  () => warehouseService.list({ limit: 100, sort: 'code' }),
  (w) => ({ value: String(w.id), label: `${w.code} — ${w.name}` }),
)

const columns: Column[] = [
  { key: 'asset_tag', label: 'Asset Tag', sortKey: 'asset_tag' },
  { key: 'serial_number', label: 'Serial Number', sortKey: 'serial_number' },
  { key: 'product', label: 'Produk', sortKey: 'sku' },
  { key: 'location', label: 'Lokasi' },
  { key: 'status', label: 'Status', sortKey: 'status' },
  { key: 'actions', label: 'Tindakan', actions: true },
]

const filterDefs = computed<FilterDef[]>(() => [
  {
    key: 'status',
    label: 'Status',
    options: [
      { value: 'AVAILABLE', label: 'Tersedia' },
      { value: 'RESERVED', label: 'Direservasi' },
      { value: 'IN_TRANSIT', label: 'Dalam perjalanan' },
      { value: 'RENTED', label: 'Disewa' },
      { value: 'MAINTENANCE', label: 'Perawatan' },
      { value: 'BROKEN', label: 'Rusak' },
      { value: 'LOST', label: 'Hilang' },
      { value: 'RETIRED', label: 'Pensiun' },
    ],
  },
  { key: 'warehouse_id', label: 'Gudang', options: warehouses.options.value },
])

async function printLabel(s: SerialItem): Promise<void> {
  if (printingId.value !== null) return
  printingId.value = s.id
  try {
    const blob = await inventoryService.fetchSerialLabelsPdf([s.id])
    await openPdfBlob(blob, `label-${s.asset_tag}.pdf`)
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: 'Gagal mencetak label aset',
      detail: userMessage(toApiError(e)),
      life: 7000,
    })
  } finally {
    printingId.value = null
  }
}
</script>

<template>
  <ListPage
    title="Serial Number"
    description="Daftar seluruh unit fisik ber-asset tag dan serial number beserta status dan lokasi terakhirnya."
    entity="unit serial"
    empty-value="Unit serial number akan terdaftar otomatis saat LPB pembelian atau stok awal disetujui."
    caption="Daftar unit serial number"
    search-label="Cari asset tag, SN, SKU, atau nama produk"
    :columns="columns"
    :table="table"
    :filter-defs="filterDefs"
    :can-create="false"
  >
    <template #cell-asset_tag="{ row }">
      <RouterLink :to="`/stok/serial/${row.id}`" :class="rowLink" class="font-mono">
        {{ row.asset_tag }}
      </RouterLink>
    </template>
    <template #cell-serial_number="{ row }">
      <span class="font-mono text-xs text-slate-800">{{ row.serial_number }}</span>
    </template>
    <template #cell-product="{ row }">
      <div class="font-medium text-slate-900">{{ row.product_name }}</div>
      <div class="font-mono text-xs text-slate-500">{{ row.sku }}</div>
    </template>
    <template #cell-location="{ row }">
      <span v-if="row.rack_code" class="font-mono text-xs text-slate-800">
        {{ row.warehouse_code }} / {{ row.rack_code }}
      </span>
      <span v-else class="text-xs text-slate-500">Di luar gudang</span>
    </template>
    <template #cell-status="{ row }">
      <StatusBadge :value="row.status" />
    </template>
    <template #cell-actions="{ row }">
      <div class="flex justify-end">
        <button
          type="button"
          :class="btnRow"
          :aria-busy="printingId === row.id"
          @click="printLabel(row)"
        >
          {{ printingId === row.id ? 'Menyiapkan…' : 'Cetak Label' }}
        </button>
      </div>
    </template>
  </ListPage>
</template>
