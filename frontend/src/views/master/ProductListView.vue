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
import { formatRupiah } from '@/lib/format'
import { btnRow, rowLink } from '@/lib/ui'
import { categoryService, productService } from '@/services/masterService'
import type { Product } from '@/types/master'

const { can } = usePermission()
const canWrite = computed(() => can('master.write'))

const table = useServerTable<Product>({
  fetch: (p) => productService.list(p),
  defaultSort: 'name',
  filterKeys: ['category_id', 'item_type', 'has_serial_number', 'is_active'],
})

// Teknisi hanya boleh membaca produk (tanpa master.read): daftar kategori tidak tersedia baginya.
const categories = useOptions(
  () =>
    can('master.read')
      ? categoryService.list({ limit: 100, sort: 'name' })
      : Promise.reject(new Error('skip')),
  (c) => ({ value: String(c.id), label: c.name }),
)

const columns: Column[] = [
  { key: 'sku', label: 'SKU', sortKey: 'sku' },
  { key: 'name', label: 'Nama', sortKey: 'name' },
  { key: 'category_name', label: 'Kategori', sortKey: 'category' },
  { key: 'item_type', label: 'Jenis' },
  { key: 'has_serial_number', label: 'Serial number' },
  { key: 'min_stock_alert', label: 'Min. stok', class: 'text-end' },
  { key: 'default_monthly_rate', label: 'Tarif sewa/bulan', class: 'text-end' },
  { key: 'is_active', label: 'Status' },
  { key: 'actions', label: 'Tindakan', actions: true },
]

const filterDefs = computed<FilterDef[]>(() => [
  ...(categories.options.value.length > 0
    ? [{ key: 'category_id', label: 'Kategori', options: categories.options.value }]
    : []),
  {
    key: 'item_type',
    label: 'Jenis barang',
    options: [
      { value: 'ASSET', label: 'Aset' },
      { value: 'CONSUMABLE', label: 'Habis pakai' },
    ],
  },
  {
    key: 'has_serial_number',
    label: 'Serial number',
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

const toggle = useActiveToggle<Product>({
  update: (id, body) => productService.update(id, body),
  describe: (p) => `produk ${p.sku}`,
  onDone: () => table.reload(),
})
</script>

<template>
  <ListPage
    title="Produk"
    description="Barang yang disimpan dan disewakan. Produk ber-serial number dilacak per unit; lainnya per jumlah."
    entity="produk"
    empty-value="Tambahkan produk pertama agar stok dan LPB dapat dicatat."
    caption="Daftar produk"
    search-label="Cari produk"
    :columns="columns"
    :table="table"
    :filter-defs="filterDefs"
    create-label="Tambah Produk"
    :create-to="{ path: '/master/produk/baru' }"
    :can-create="canWrite"
  >
    <template #top><MasterTabs /></template>

    <template #cell-sku="{ row }">
      <RouterLink v-if="canWrite" :to="`/master/produk/${row.id}`" :class="rowLink">{{
        row.sku
      }}</RouterLink>
      <span v-else class="font-medium">{{ row.sku }}</span>
    </template>
    <template #cell-item_type="{ row }"><StatusBadge :value="row.item_type" /></template>
    <template #cell-has_serial_number="{ row }">
      <StatusBadge :value="row.has_serial_number ? 'YES' : 'NO'" />
    </template>
    <template #cell-default_monthly_rate="{ row }">{{
      formatRupiah(row.default_monthly_rate)
    }}</template>
    <template #cell-is_active="{ row }">
      <StatusBadge :value="row.is_active ? 'ACTIVE' : 'INACTIVE'" />
    </template>
    <template #cell-actions="{ row }">
      <div v-if="canWrite" class="flex justify-end">
        <button type="button" :class="btnRow" @click="toggle.request(row)">
          {{ row.is_active ? 'Nonaktifkan' : 'Aktifkan' }}
          <span class="sr-only">produk {{ row.sku }}</span>
        </button>
      </div>
    </template>
  </ListPage>

  <ConfirmDialog
    :open="toggle.pending.value !== null"
    :title="`Nonaktifkan produk '${toggle.pending.value?.sku ?? ''}'?`"
    message="Produk yang nonaktif tidak dapat dipilih untuk dokumen baru. Stok dan riwayatnya tetap tersimpan. Anda dapat mengaktifkannya kembali kapan saja."
    confirm-label="Nonaktifkan Produk"
    :busy="toggle.busy.value"
    @confirm="toggle.confirm"
    @cancel="toggle.cancel"
  />
</template>
