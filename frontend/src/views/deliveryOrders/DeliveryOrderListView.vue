<script setup lang="ts">
// Pola halaman: List / Index — Antrean Delivery Order Gudang (T2.6).
import ListPage from '@/components/ListPage.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import type { Column } from '@/components/DataTable.vue'
import type { FilterDef } from '@/components/FilterPanel.vue'
import { useServerTable } from '@/composables/useServerTable'
import { btnRow, rowLink } from '@/lib/ui'
import { deliveryOrderService } from '@/services/deliveryOrderService'
import type { DeliveryOrder } from '@/types/deliveryOrder'

const table = useServerTable<DeliveryOrder>({
  fetch: (p) => deliveryOrderService.list(p),
  defaultSort: '-created_at',
  filterKeys: ['direction', 'status'],
})

const columns: Column[] = [
  { key: 'do_number', label: 'Nomor DO', sortKey: 'do_number' },
  { key: 'direction', label: 'Arah' },
  { key: 'so_number', label: 'Sales Order' },
  { key: 'customer_name', label: 'Pelanggan / Site' },
  { key: 'warehouse_code', label: 'Gudang' },
  { key: 'progress', label: 'Reservasi' },
  { key: 'status', label: 'Status' },
  { key: 'actions', label: 'Tindakan', actions: true },
]

const filterDefs: FilterDef[] = [
  {
    key: 'direction',
    label: 'Arah',
    options: [
      { value: 'OUTBOUND', label: 'OUTBOUND (Kirim)' },
      { value: 'INBOUND', label: 'INBOUND (Tarik)' },
    ],
  },
  {
    key: 'status',
    label: 'Status',
    options: [
      { value: 'OPEN', label: 'Terbuka' },
      { value: 'RESERVED', label: 'Tereservasi' },
      { value: 'LPB_CREATED', label: 'LPB Dibuat' },
      { value: 'COMPLETED', label: 'Selesai' },
      { value: 'CANCELLED', label: 'Dibatalkan' },
    ],
  },
]
</script>

<template>
  <ListPage
    title="Delivery Order"
    description="Antrean persiapan barang di gudang: reservasi stok/SN dan pembuatan draft LPB."
    entity="delivery order"
    empty-value="Delivery Order akan muncul otomatis setelah Sales Order dikonfirmasi dan diterbitkan ke gudang."
    caption="Daftar Delivery Order"
    search-label="Cari nomor DO, nomor SO, pelanggan, atau site"
    :columns="columns"
    :table="table"
    :filter-defs="filterDefs"
    :can-create="false"
  >
    <template #cell-do_number="{ row }">
      <RouterLink :to="`/delivery-orders/${row.id}`" :class="rowLink" class="font-mono text-xs">
        {{ row.do_number }}
      </RouterLink>
    </template>
    <template #cell-direction="{ row }">
      <span
        class="inline-flex rounded-full px-2 py-0.5 text-xs font-medium"
        :class="
          row.direction === 'OUTBOUND'
            ? 'bg-indigo-50 text-indigo-700'
            : 'bg-amber-50 text-amber-800'
        "
      >
        {{ row.direction }}
      </span>
    </template>
    <template #cell-so_number="{ row }">
      <span class="font-mono text-xs text-slate-700">{{ row.so_number }}</span>
    </template>
    <template #cell-customer_name="{ row }">
      <div class="font-medium text-slate-900">{{ row.customer_name }}</div>
      <div class="text-xs text-slate-500">{{ row.site_name }} · {{ row.site_city }}</div>
    </template>
    <template #cell-warehouse_code="{ row }">
      <span class="font-mono text-xs text-slate-700">{{ row.warehouse_code }}</span>
    </template>
    <template #cell-progress="{ row }">
      <span v-if="row.direction === 'OUTBOUND'" class="text-xs font-medium text-slate-800">
        {{ row.reserved_qty }} / {{ row.total_qty }}
      </span>
      <span v-else class="text-xs text-slate-500">{{ row.total_qty }} unit</span>
    </template>
    <template #cell-status="{ row }">
      <StatusBadge :value="row.status" />
    </template>
    <template #cell-actions="{ row }">
      <div class="flex justify-end">
        <RouterLink :to="`/delivery-orders/${row.id}`" :class="btnRow">Proses</RouterLink>
      </div>
    </template>
  </ListPage>
</template>
