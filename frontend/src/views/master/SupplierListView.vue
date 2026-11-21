<script setup lang="ts">
// Pola halaman: List / Index.
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import ListPage from '@/components/ListPage.vue'
import MasterTabs from '@/components/MasterTabs.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import type { Column } from '@/components/DataTable.vue'
import type { FilterDef } from '@/components/FilterPanel.vue'
import { useActiveToggle } from '@/composables/useActiveToggle'
import { usePermission } from '@/composables/usePermission'
import { useServerTable } from '@/composables/useServerTable'
import { btnRow, rowLink } from '@/lib/ui'
import { supplierService } from '@/services/masterService'
import type { Supplier } from '@/types/master'

const { can } = usePermission()

const table = useServerTable<Supplier>({
  fetch: (p) => supplierService.list(p),
  defaultSort: 'name',
  filterKeys: ['is_active'],
})

const columns: Column[] = [
  { key: 'code', label: 'Kode', sortKey: 'code' },
  { key: 'name', label: 'Nama', sortKey: 'name' },
  { key: 'pic_name', label: 'PIC' },
  { key: 'phone', label: 'Telepon' },
  { key: 'email', label: 'Email' },
  { key: 'is_active', label: 'Status' },
  { key: 'actions', label: 'Tindakan', actions: true },
]

const filterDefs: FilterDef[] = [
  {
    key: 'is_active',
    label: 'Status',
    options: [
      { value: 'true', label: 'Aktif' },
      { value: 'false', label: 'Nonaktif' },
    ],
  },
]

const toggle = useActiveToggle<Supplier>({
  update: (id, body) => supplierService.update(id, body),
  describe: (s) => `supplier ${s.code}`,
  onDone: () => table.reload(),
})
</script>

<template>
  <ListPage
    title="Supplier"
    description="Pemasok barang. Dipilih saat mencatat penerimaan barang (LPB pembelian)."
    entity="supplier"
    empty-value="Tambahkan supplier pertama agar penerimaan barang dapat dikaitkan dengan pemasoknya."
    caption="Daftar supplier"
    search-label="Cari supplier"
    :columns="columns"
    :table="table"
    :filter-defs="filterDefs"
    create-label="Tambah Supplier"
    :create-to="{ path: '/master/supplier/baru' }"
    :can-create="can('master.write')"
  >
    <template #top><MasterTabs /></template>

    <template #cell-code="{ row }">
      <RouterLink v-if="can('master.write')" :to="`/master/supplier/${row.id}`" :class="rowLink">{{
        row.code
      }}</RouterLink>
      <span v-else class="font-medium">{{ row.code }}</span>
    </template>
    <template #cell-pic_name="{ row }">{{ row.pic_name ?? '—' }}</template>
    <template #cell-phone="{ row }">{{ row.phone ?? '—' }}</template>
    <template #cell-email="{ row }">{{ row.email ?? '—' }}</template>
    <template #cell-is_active="{ row }">
      <StatusBadge :value="row.is_active ? 'ACTIVE' : 'INACTIVE'" />
    </template>
    <template #cell-actions="{ row }">
      <div v-if="can('master.write')" class="flex justify-end">
        <button type="button" :class="btnRow" @click="toggle.request(row)">
          {{ row.is_active ? 'Nonaktifkan' : 'Aktifkan' }}
          <span class="sr-only">supplier {{ row.code }}</span>
        </button>
      </div>
    </template>
  </ListPage>

  <ConfirmDialog
    :open="toggle.pending.value !== null"
    :title="`Nonaktifkan supplier '${toggle.pending.value?.code ?? ''}'?`"
    message="Supplier yang nonaktif tidak dapat dipilih untuk penerimaan baru. Riwayat tetap tersimpan. Anda dapat mengaktifkannya kembali kapan saja."
    confirm-label="Nonaktifkan Supplier"
    :busy="toggle.busy.value"
    @confirm="toggle.confirm"
    @cancel="toggle.cancel"
  />
</template>
