<script setup lang="ts">
// Pola halaman: List / Index — Pelanggan (T2.2).
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import ListPage from '@/components/ListPage.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import type { Column } from '@/components/DataTable.vue'
import type { FilterDef } from '@/components/FilterPanel.vue'
import { useActiveToggle } from '@/composables/useActiveToggle'
import { usePermission } from '@/composables/usePermission'
import { useServerTable } from '@/composables/useServerTable'
import { btnRow, rowLink } from '@/lib/ui'
import { customerService } from '@/services/customerService'
import type { Customer } from '@/types/customer'

const { can } = usePermission()

const table = useServerTable<Customer>({
  fetch: (p) => customerService.list(p),
  defaultSort: 'company_name',
  filterKeys: ['is_pkp', 'is_active'],
})

const columns: Column[] = [
  { key: 'customer_code', label: 'Kode', sortKey: 'customer_code' },
  { key: 'company_name', label: 'Perusahaan', sortKey: 'company_name' },
  { key: 'pic_name', label: 'PIC' },
  { key: 'phone', label: 'Telepon' },
  { key: 'is_pkp', label: 'Pajak' },
  { key: 'site_count', label: 'Site' },
  { key: 'active_asset_count', label: 'Unit Sewa' },
  { key: 'is_active', label: 'Status' },
  { key: 'actions', label: 'Tindakan', actions: true },
]

const filterDefs: FilterDef[] = [
  {
    key: 'is_pkp',
    label: 'Status PKP',
    options: [
      { value: 'true', label: 'PKP' },
      { value: 'false', label: 'Non-PKP' },
    ],
  },
  {
    key: 'is_active',
    label: 'Status',
    options: [
      { value: 'true', label: 'Aktif' },
      { value: 'false', label: 'Nonaktif' },
    ],
  },
]

const toggle = useActiveToggle<Customer>({
  update: (id, body) => customerService.update(id, body),
  describe: (c) => `pelanggan ${c.customer_code}`,
  onDone: () => table.reload(),
})
</script>

<template>
  <ListPage
    title="Pelanggan"
    description="Daftar perusahaan pelanggan beserta cabang/site dan unit sewa terpasang."
    entity="pelanggan"
    empty-value="Tambahkan pelanggan pertama agar cabang/site dan kontrak sewa dapat dikelola."
    caption="Daftar pelanggan"
    search-label="Cari kode, nama perusahaan, PIC, atau NPWP"
    :columns="columns"
    :table="table"
    :filter-defs="filterDefs"
    create-label="Tambah Pelanggan"
    :create-to="{ path: '/pelanggan/baru' }"
    :can-create="can('customer.write')"
  >
    <template #cell-customer_code="{ row }">
      <RouterLink :to="`/pelanggan/${row.id}`" :class="rowLink">
        {{ row.customer_code }}
      </RouterLink>
    </template>
    <template #cell-company_name="{ row }">
      <RouterLink :to="`/pelanggan/${row.id}`" class="font-medium text-slate-900 hover:underline">
        {{ row.company_name }}
      </RouterLink>
    </template>
    <template #cell-is_pkp="{ row }">
      <span
        class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
        :class="
          row.is_pkp ? 'bg-blue-50 text-blue-700' : 'bg-slate-100 text-slate-600'
        "
      >
        {{ row.is_pkp ? 'PKP' : 'Non-PKP' }}
      </span>
    </template>
    <template #cell-site_count="{ row }">{{ row.site_count }}</template>
    <template #cell-active_asset_count="{ row }">{{ row.active_asset_count }}</template>
    <template #cell-is_active="{ row }">
      <StatusBadge :value="row.is_active ? 'ACTIVE' : 'INACTIVE'" />
    </template>
    <template #cell-actions="{ row }">
      <div class="flex justify-end gap-2">
        <RouterLink :to="`/pelanggan/${row.id}`" :class="btnRow">Detail</RouterLink>
        <button
          v-if="can('customer.write')"
          type="button"
          :class="btnRow"
          @click="toggle.request(row)"
        >
          {{ row.is_active ? 'Nonaktifkan' : 'Aktifkan' }}
          <span class="sr-only">pelanggan {{ row.customer_code }}</span>
        </button>
      </div>
    </template>
  </ListPage>

  <ConfirmDialog
    :open="toggle.pending.value !== null"
    :title="`Nonaktifkan pelanggan '${toggle.pending.value?.customer_code ?? ''}'?`"
    message="Pelanggan yang masih memiliki unit sewa aktif tidak dapat dinonaktifkan. Riwayat tetap tersimpan."
    confirm-label="Nonaktifkan Pelanggan"
    :busy="toggle.busy.value"
    @confirm="toggle.confirm"
    @cancel="toggle.cancel"
  />
</template>
