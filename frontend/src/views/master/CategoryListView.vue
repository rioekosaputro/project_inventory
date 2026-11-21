<script setup lang="ts">
// Pola halaman: List / Index. Buat/ubah (2 field) memakai panel samping, bukan halaman baru.
import { ref } from 'vue'
import CategoryForm from '@/components/CategoryForm.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import ListPage from '@/components/ListPage.vue'
import MasterTabs from '@/components/MasterTabs.vue'
import SlideOver from '@/components/SlideOver.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import type { Column } from '@/components/DataTable.vue'
import type { FilterDef } from '@/components/FilterPanel.vue'
import { useActiveToggle } from '@/composables/useActiveToggle'
import { usePermission } from '@/composables/usePermission'
import { useServerTable } from '@/composables/useServerTable'
import { btnRow, rowLink } from '@/lib/ui'
import { categoryService } from '@/services/masterService'
import type { Category } from '@/types/master'

const { can } = usePermission()

const table = useServerTable<Category>({
  fetch: (p) => categoryService.list(p),
  defaultSort: 'name',
  filterKeys: ['item_type', 'is_active'],
})

const columns: Column[] = [
  { key: 'name', label: 'Nama', sortKey: 'name' },
  { key: 'item_type', label: 'Jenis barang', sortKey: 'item_type' },
  { key: 'is_active', label: 'Status' },
  { key: 'actions', label: 'Tindakan', actions: true },
]

const filterDefs: FilterDef[] = [
  {
    key: 'item_type',
    label: 'Jenis barang',
    options: [
      { value: 'ASSET', label: 'Aset' },
      { value: 'CONSUMABLE', label: 'Habis pakai' },
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

// ---- panel samping: buat (null) / ubah (kategori)
const panelOpen = ref(false)
const editing = ref<Category | null>(null)
function openCreate(): void {
  editing.value = null
  panelOpen.value = true
}
function openEdit(c: Category): void {
  editing.value = c
  panelOpen.value = true
}
function onSaved(): void {
  panelOpen.value = false
  void table.reload()
}

const toggle = useActiveToggle<Category>({
  update: (id, body) => categoryService.update(id, body),
  describe: (c) => `kategori ${c.name}`,
  onDone: () => table.reload(),
})
</script>

<template>
  <ListPage
    title="Kategori"
    description="Pengelompokan produk. Jenis barang menentukan apakah produk dilacak per unit (aset) atau per jumlah (habis pakai)."
    entity="kategori"
    empty-value="Tambahkan kategori pertama, lalu kelompokkan produk ke dalamnya."
    caption="Daftar kategori"
    search-label="Cari kategori"
    :columns="columns"
    :table="table"
    :filter-defs="filterDefs"
    create-label="Tambah Kategori"
    :can-create="can('master.write')"
    @create="openCreate"
  >
    <template #top><MasterTabs /></template>

    <template #cell-name="{ row }">
      <button v-if="can('master.write')" type="button" :class="rowLink" @click="openEdit(row)">
        {{ row.name }}
      </button>
      <span v-else class="font-medium">{{ row.name }}</span>
    </template>
    <template #cell-item_type="{ row }"><StatusBadge :value="row.item_type" /></template>
    <template #cell-is_active="{ row }">
      <StatusBadge :value="row.is_active ? 'ACTIVE' : 'INACTIVE'" />
    </template>
    <template #cell-actions="{ row }">
      <div v-if="can('master.write')" class="flex justify-end">
        <button type="button" :class="btnRow" @click="toggle.request(row)">
          {{ row.is_active ? 'Nonaktifkan' : 'Aktifkan' }}
          <span class="sr-only">kategori {{ row.name }}</span>
        </button>
      </div>
    </template>
  </ListPage>

  <SlideOver
    :open="panelOpen"
    :title="editing ? `Ubah kategori ${editing.name}` : 'Tambah kategori'"
    @close="panelOpen = false"
  >
    <CategoryForm
      :key="editing?.id ?? 'new'"
      :category="editing"
      @saved="onSaved"
      @cancel="panelOpen = false"
    />
  </SlideOver>

  <ConfirmDialog
    :open="toggle.pending.value !== null"
    :title="`Nonaktifkan kategori '${toggle.pending.value?.name ?? ''}'?`"
    message="Kategori yang nonaktif tidak dapat dipilih untuk produk baru. Produk yang sudah ada tidak berubah. Anda dapat mengaktifkannya kembali kapan saja."
    confirm-label="Nonaktifkan Kategori"
    :busy="toggle.busy.value"
    @confirm="toggle.confirm"
    @cancel="toggle.cancel"
  />
</template>
