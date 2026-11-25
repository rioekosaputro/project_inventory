<script setup lang="ts">
// Pola halaman: List / Index — Sales Order (T2.5).
import ListPage from '@/components/ListPage.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import type { Column } from '@/components/DataTable.vue'
import type { FilterDef } from '@/components/FilterPanel.vue'
import { usePermission } from '@/composables/usePermission'
import { useServerTable } from '@/composables/useServerTable'
import { formatRupiah } from '@/lib/format'
import { btnRow, rowLink } from '@/lib/ui'
import { salesOrderService } from '@/services/salesOrderService'
import type { OrderType, SalesOrder } from '@/types/salesOrder'

const { can } = usePermission()

const table = useServerTable<SalesOrder>({
  fetch: (p) => salesOrderService.list(p),
  defaultSort: '-created_at',
  filterKeys: ['order_type', 'status'],
})

const columns: Column[] = [
  { key: 'so_number', label: 'Nomor SO', sortKey: 'so_number' },
  { key: 'order_type', label: 'Tipe' },
  { key: 'customer_name', label: 'Pelanggan / Site' },
  { key: 'order_date', label: 'Tanggal', sortKey: 'order_date' },
  { key: 'grand_total', label: 'Grand Total', sortKey: 'grand_total' },
  { key: 'do_count', label: 'DO' },
  { key: 'status', label: 'Status' },
  { key: 'actions', label: 'Tindakan', actions: true },
]

const filterDefs: FilterDef[] = [
  {
    key: 'order_type',
    label: 'Tipe SO',
    options: [
      { value: 'NEW_DELIVERY', label: 'Pengiriman Baru' },
      { value: 'WITHDRAWAL', label: 'Penarikan Unit' },
      { value: 'UNIT_SWAP', label: 'Tukar Unit' },
      { value: 'CONSUMABLE', label: 'Penjualan Consumable' },
    ],
  },
  {
    key: 'status',
    label: 'Status',
    options: [
      { value: 'DRAFT', label: 'Draft' },
      { value: 'CONFIRMED', label: 'Dikonfirmasi' },
      { value: 'IN_PROGRESS', label: 'Diproses' },
      { value: 'COMPLETED', label: 'Selesai' },
      { value: 'CANCELLED', label: 'Dibatalkan' },
    ],
  },
]

const typeLabels: Record<OrderType, string> = {
  NEW_DELIVERY: 'Pengiriman Baru',
  WITHDRAWAL: 'Penarikan',
  UNIT_SWAP: 'Tukar Unit',
  CONSUMABLE: 'Consumable',
}
</script>

<template>
  <ListPage
    title="Sales Order"
    description="Pesanan pengiriman sewa baru, penarikan, tukar unit, dan penjualan consumable beserta penerbitan DO."
    entity="sales order"
    empty-value="Buat Sales Order pertama untuk memulai proses pengiriman atau penarikan unit."
    caption="Daftar Sales Order"
    search-label="Cari nomor SO, pelanggan, atau site"
    :columns="columns"
    :table="table"
    :filter-defs="filterDefs"
    create-label="Buat Sales Order"
    :create-to="{ path: '/sales-orders/baru' }"
    :can-create="can('so.write')"
  >
    <template #cell-so_number="{ row }">
      <RouterLink :to="`/sales-orders/${row.id}`" :class="rowLink" class="font-mono text-xs">
        {{ row.so_number }}
      </RouterLink>
    </template>
    <template #cell-order_type="{ row }">
      <span
        class="inline-flex items-center rounded-full bg-slate-100 px-2.5 py-0.5 text-xs font-medium text-slate-700"
      >
        {{ typeLabels[row.order_type] }}
      </span>
    </template>
    <template #cell-customer_name="{ row }">
      <div class="font-medium text-slate-900">{{ row.customer_name }}</div>
      <div class="text-xs text-slate-500">{{ row.site_name }} · {{ row.site_city }}</div>
    </template>
    <template #cell-order_date="{ row }">{{ row.order_date }}</template>
    <template #cell-grand_total="{ row }">
      <span class="font-medium text-slate-900">{{ formatRupiah(row.grand_total) }}</span>
    </template>
    <template #cell-do_count="{ row }">{{ row.do_count }}</template>
    <template #cell-status="{ row }">
      <StatusBadge :value="row.status" />
    </template>
    <template #cell-actions="{ row }">
      <div class="flex justify-end gap-2">
        <RouterLink :to="`/sales-orders/${row.id}`" :class="btnRow">Detail</RouterLink>
        <RouterLink
          v-if="row.status === 'DRAFT' && can('so.write')"
          :to="`/sales-orders/${row.id}/ubah`"
          :class="btnRow"
        >
          Ubah
        </RouterLink>
      </div>
    </template>
  </ListPage>
</template>
