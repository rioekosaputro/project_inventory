<script setup lang="ts">
// Pola halaman: List / Index.
import { computed } from 'vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import ListPage from '@/components/ListPage.vue'
import MasterTabs from '@/components/MasterTabs.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import type { Column } from '@/components/DataTable.vue'
import type { FilterDef } from '@/components/FilterPanel.vue'
import { useActiveToggle } from '@/composables/useActiveToggle'
import { useOptions } from '@/composables/useOptions'
import { usePermission } from '@/composables/usePermission'
import { useServerTable } from '@/composables/useServerTable'
import { btnRow, rowLink } from '@/lib/ui'
import { rackService, warehouseService } from '@/services/masterService'
import type { Rack } from '@/types/master'

const { can } = usePermission()

const table = useServerTable<Rack>({
  fetch: (p) => rackService.list(p),
  defaultSort: 'warehouse,code',
  filterKeys: ['warehouse_id', 'zone', 'is_sellable', 'is_active'],
})

const warehouses = useOptions(
  () => warehouseService.list({ limit: 100, sort: 'code' }),
  (w) => ({ value: String(w.id), label: `${w.code} — ${w.name}` }),
)

const columns: Column[] = [
  { key: 'warehouse_code', label: 'Gudang', sortKey: 'warehouse' },
  { key: 'code', label: 'Kode rak', sortKey: 'code' },
  { key: 'zone', label: 'Zona', sortKey: 'zone' },
  { key: 'is_sellable', label: 'Dapat dijual' },
  { key: 'is_active', label: 'Status' },
  { key: 'actions', label: 'Tindakan', actions: true },
]

const filterDefs = computed<FilterDef[]>(() => [
  { key: 'warehouse_id', label: 'Gudang', options: warehouses.options.value },
  { key: 'zone', label: 'Zona', type: 'text' },
  {
    key: 'is_sellable',
    label: 'Dapat dijual',
    options: [
      { value: 'true', label: 'Ya' },
      { value: 'false', label: 'Tidak' },
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
])

const toggle = useActiveToggle<Rack>({
  update: (id, body) => rackService.update(id, body),
  describe: (r) => `rak ${r.code} (${r.warehouse_code})`,
  onDone: () => table.reload(),
})
</script>

<template>
  <ListPage
    title="Rak"
    description="Posisi penyimpanan di dalam gudang. Rak non-sellable (mis. karantina) tidak dihitung sebagai stok tersedia."
    entity="rak"
    empty-value="Tambahkan rak agar stok dapat ditempatkan pada posisi yang jelas."
    caption="Daftar rak"
    search-label="Cari rak"
    :columns="columns"
    :table="table"
    :filter-defs="filterDefs"
    create-label="Tambah Rak"
    :create-to="{ path: '/master/rak/baru' }"
    :can-create="can('rack.write')"
  >
    <template #top><MasterTabs /></template>

    <template #cell-code="{ row }">
      <RouterLink v-if="can('rack.write')" :to="`/master/rak/${row.id}`" :class="rowLink">{{
        row.code
      }}</RouterLink>
      <span v-else class="font-medium">{{ row.code }}</span>
    </template>
    <template #cell-zone="{ row }">{{ row.zone ?? '—' }}</template>
    <template #cell-is_sellable="{ row }">
      <StatusBadge :value="row.is_sellable ? 'YES' : 'NO'" />
    </template>
    <template #cell-is_active="{ row }">
      <div class="flex flex-wrap items-center gap-1">
        <StatusBadge :value="row.is_active ? 'ACTIVE' : 'INACTIVE'" />
        <StatusBadge v-if="row.locked_by_opname_id" value="LOCKED" />
      </div>
    </template>
    <template #cell-actions="{ row }">
      <div v-if="can('rack.write')" class="flex justify-end">
        <button type="button" :class="btnRow" @click="toggle.request(row)">
          {{ row.is_active ? 'Nonaktifkan' : 'Aktifkan' }}
          <span class="sr-only">rak {{ row.code }} di gudang {{ row.warehouse_code }}</span>
        </button>
      </div>
    </template>
  </ListPage>

  <ConfirmDialog
    :open="toggle.pending.value !== null"
    :title="`Nonaktifkan rak '${toggle.pending.value?.code ?? ''}' di gudang ${toggle.pending.value?.warehouse_code ?? ''}?`"
    message="Rak yang masih menyimpan stok atau sedang dikunci stock opname tidak dapat dinonaktifkan. Anda dapat mengaktifkannya kembali kapan saja."
    confirm-label="Nonaktifkan Rak"
    :busy="toggle.busy.value"
    @confirm="toggle.confirm"
    @cancel="toggle.cancel"
  />
</template>
