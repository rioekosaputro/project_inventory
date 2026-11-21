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
import { warehouseService } from '@/services/masterService'
import type { Warehouse } from '@/types/master'

const { can } = usePermission()

const table = useServerTable<Warehouse>({
  fetch: (p) => warehouseService.list(p),
  defaultSort: 'code',
  filterKeys: ['is_active'],
})

const columns: Column[] = [
  { key: 'code', label: 'Kode', sortKey: 'code' },
  { key: 'name', label: 'Nama', sortKey: 'name' },
  { key: 'address', label: 'Alamat' },
  { key: 'phone', label: 'Telepon' },
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

const toggle = useActiveToggle<Warehouse>({
  update: (id, body) => warehouseService.update(id, body),
  describe: (w) => `gudang ${w.code}`,
  onDone: () => table.reload(),
})
</script>

<template>
  <ListPage
    title="Gudang"
    description="Lokasi penyimpanan fisik. Kode gudang dipakai pada nomor dokumen dan tidak dapat diubah."
    entity="gudang"
    empty-value="Tambahkan gudang pertama agar rak dan stok dapat dicatat."
    caption="Daftar gudang"
    search-label="Cari gudang"
    :columns="columns"
    :table="table"
    :filter-defs="filterDefs"
    create-label="Tambah Gudang"
    :create-to="{ path: '/master/gudang/baru' }"
    :can-create="can('master.write')"
  >
    <template #top><MasterTabs /></template>

    <template #cell-code="{ row }">
      <RouterLink v-if="can('master.write')" :to="`/master/gudang/${row.id}`" :class="rowLink">{{
        row.code
      }}</RouterLink>
      <span v-else class="font-medium">{{ row.code }}</span>
    </template>
    <template #cell-address="{ row }">
      <span class="block max-w-64 truncate" :title="row.address ?? undefined">{{
        row.address ?? '—'
      }}</span>
    </template>
    <template #cell-phone="{ row }">{{ row.phone ?? '—' }}</template>
    <template #cell-is_active="{ row }">
      <StatusBadge :value="row.is_active ? 'ACTIVE' : 'INACTIVE'" />
    </template>
    <template #cell-actions="{ row }">
      <div v-if="can('master.write')" class="flex justify-end">
        <button type="button" :class="btnRow" @click="toggle.request(row)">
          {{ row.is_active ? 'Nonaktifkan' : 'Aktifkan' }}
          <span class="sr-only">gudang {{ row.code }}</span>
        </button>
      </div>
    </template>
  </ListPage>

  <ConfirmDialog
    :open="toggle.pending.value !== null"
    :title="`Nonaktifkan gudang '${toggle.pending.value?.code ?? ''}'?`"
    message="Gudang yang nonaktif tidak dapat dipilih untuk dokumen baru. Gudang yang masih menyimpan stok tidak dapat dinonaktifkan. Anda dapat mengaktifkannya kembali kapan saja."
    confirm-label="Nonaktifkan Gudang"
    :busy="toggle.busy.value"
    @confirm="toggle.confirm"
    @cancel="toggle.cancel"
  />
</template>
