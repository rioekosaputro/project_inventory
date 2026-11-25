<script setup lang="ts">
// Pola halaman: List / Index — Kontrak Sewa (T2.3).
import ListPage from '@/components/ListPage.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import type { Column } from '@/components/DataTable.vue'
import type { FilterDef } from '@/components/FilterPanel.vue'
import { usePermission } from '@/composables/usePermission'
import { useServerTable } from '@/composables/useServerTable'
import { formatRupiah } from '@/lib/format'
import { btnRow, rowLink } from '@/lib/ui'
import { contractService } from '@/services/contractService'
import type { RentalContract } from '@/types/contract'

const { can } = usePermission()

const table = useServerTable<RentalContract>({
  fetch: (p) => contractService.list(p),
  defaultSort: '-created_at',
  filterKeys: ['status', 'expiring_in'],
})

const columns: Column[] = [
  { key: 'contract_number', label: 'Nomor Kontrak', sortKey: 'contract_number' },
  { key: 'customer_name', label: 'Pelanggan' },
  { key: 'period', label: 'Periode', sortKey: 'end_date' },
  { key: 'total_qty', label: 'Qty' },
  { key: 'estimated_monthly', label: 'Estimasi / Bln' },
  { key: 'status', label: 'Status' },
  { key: 'actions', label: 'Tindakan', actions: true },
]

const filterDefs: FilterDef[] = [
  {
    key: 'status',
    label: 'Status',
    options: [
      { value: 'DRAFT', label: 'Draft' },
      { value: 'ACTIVE', label: 'Aktif' },
      { value: 'EXPIRED', label: 'Berakhir' },
      { value: 'TERMINATED', label: 'Diterminasi' },
    ],
  },
  {
    key: 'expiring_in',
    label: 'Akan Berakhir',
    options: [
      { value: '30', label: '≤ 30 hari' },
      { value: '60', label: '≤ 60 hari' },
    ],
  },
]
</script>

<template>
  <ListPage
    title="Kontrak Sewa"
    description="Daftar perjanjian sewa B2B beserta periode, tarif bulanan, dan status aktivasi."
    entity="kontrak"
    empty-value="Buat kontrak sewa pertama untuk menentukan tarif unit dan jadwal penagihan bulanan."
    caption="Daftar kontrak sewa"
    search-label="Cari nomor kontrak atau nama pelanggan"
    :columns="columns"
    :table="table"
    :filter-defs="filterDefs"
    create-label="Buat Kontrak"
    :create-to="{ path: '/kontrak/baru' }"
    :can-create="can('contract.write')"
  >
    <template #cell-contract_number="{ row }">
      <RouterLink :to="`/kontrak/${row.id}`" :class="rowLink" class="font-mono text-xs">
        {{ row.contract_number }}
      </RouterLink>
    </template>
    <template #cell-customer_name="{ row }">
      <div class="font-medium text-slate-900">{{ row.customer_name }}</div>
      <div class="font-mono text-xs text-slate-500">{{ row.customer_code }}</div>
    </template>
    <template #cell-period="{ row }">
      <div class="text-xs text-slate-800">{{ row.start_date }} s/d {{ row.end_date }}</div>
      <div class="text-xs text-slate-500">{{ row.duration_months }} bulan · {{ row.billing_mode }}</div>
    </template>
    <template #cell-total_qty="{ row }">{{ row.total_qty }}</template>
    <template #cell-estimated_monthly="{ row }">
      <span class="font-medium text-slate-900">{{ formatRupiah(row.estimated_monthly) }}</span>
    </template>
    <template #cell-status="{ row }">
      <StatusBadge :value="row.status" />
    </template>
    <template #cell-actions="{ row }">
      <div class="flex justify-end gap-2">
        <RouterLink :to="`/kontrak/${row.id}`" :class="btnRow">Detail</RouterLink>
        <RouterLink
          v-if="row.status === 'DRAFT' && can('contract.write')"
          :to="`/kontrak/${row.id}/ubah`"
          :class="btnRow"
        >
          Ubah
        </RouterLink>
      </div>
    </template>
  </ListPage>
</template>
