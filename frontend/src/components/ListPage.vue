<script setup lang="ts" generic="T extends object">
// Pola halaman: List / Index. Dipakai semua daftar master (dan daftar lain di fase berikutnya).
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { RouteLocationRaw } from 'vue-router'
import DataTable, { type Column } from '@/components/DataTable.vue'
import FilterPanel, { type FilterDef } from '@/components/FilterPanel.vue'
import TablePagination from '@/components/TablePagination.vue'
import { announce } from '@/composables/useAnnouncer'
import type { ServerTable } from '@/composables/useServerTable'
import { btnPrimary, btnSecondary } from '@/lib/ui'

const props = defineProps<{
  title: string
  description: string
  /** Kata benda untuk teks kosong/pengumuman, mis. "gudang". */
  entity: string
  /** Satu kalimat nilai bila daftar masih kosong ("Tambahkan gudang agar ..."). */
  emptyValue: string
  caption: string
  searchLabel: string
  columns: Column[]
  table: ServerTable<T>
  filterDefs?: FilterDef[]
  /** Teks tombol buat; diawali kata kerja ("Tambah Gudang"). */
  createLabel?: string
  createTo?: RouteLocationRaw
  /** Tombol buat hanya bila pengguna berhak (otorisasi sebenarnya tetap di backend). */
  canCreate?: boolean
}>()
const emit = defineEmits<{ create: [] }>()

defineSlots<{ top?: () => unknown; [key: `cell-${string}`]: (props: { row: T }) => unknown }>()

const defs = computed(() => props.filterDefs ?? [])
const rows = computed(() => props.table.rows.value)
const showEmpty = computed(
  () => props.table.loaded.value && rows.value.length === 0 && !props.table.error.value,
)
const firstUse = computed(() => showEmpty.value && !props.table.hasCriteria.value)

const chips = computed(() =>
  props.table.activeFilters.value.map(([key, value]) => {
    const def = defs.value.find((d) => d.key === key)
    const option = def?.options?.find((o) => o.value === value)
    return { key, text: `${def?.label ?? key}: ${option?.label ?? value}` }
  }),
)

// ---- "/" memfokuskan pencarian (bila fokus tidak sedang di kolom isian)
const searchEl = ref<HTMLInputElement | null>(null)
function onKey(e: KeyboardEvent): void {
  if (e.key !== '/' || e.ctrlKey || e.metaKey || e.altKey) return
  const t = e.target as HTMLElement | null
  if (t && (t.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(t.tagName))) return
  e.preventDefault()
  searchEl.value?.focus()
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))

// ---- pengumuman untuk pembaca layar (satu live region halaman)
watch(
  () => props.table.meta.value?.total,
  (total, before) => {
    if (total === undefined || before === undefined) return
    if (props.table.hasCriteria.value) announce(`${total} ${props.entity} ditemukan`)
  },
)

// Sel tanpa slot khusus menampilkan nilai kolomnya (kosong → tanda strip).
function cellValue(row: T, key: string): unknown {
  return (row as Record<string, unknown>)[key] ?? '—'
}

function onSort(sortKey: string): void {
  // Arah baru dihitung dari status sebelum klik (URL baru diperbarui setelah navigasi selesai).
  const toDescending =
    props.table.sortColumn.value === sortKey && props.table.sortDirection.value === 'ascending'
  props.table.toggleSort(sortKey)
  const label = props.columns.find((c) => c.sortKey === sortKey)?.label ?? sortKey
  announce(`Diurutkan menurut ${label}, ${toDescending ? 'turun' : 'naik'}`)
}
</script>

<template>
  <section>
    <slot name="top" />

    <header class="mt-4 flex flex-wrap items-start justify-between gap-3">
      <div class="min-w-0">
        <h1 class="text-xl font-semibold text-slate-900">{{ title }}</h1>
        <p class="mt-1 text-sm text-slate-600">{{ description }}</p>
      </div>
      <template v-if="canCreate && createLabel">
        <RouterLink v-if="createTo" :to="createTo" :class="btnPrimary">
          <i class="pi pi-plus text-xs" aria-hidden="true" />{{ createLabel }}
        </RouterLink>
        <button v-else type="button" :class="btnPrimary" @click="emit('create')">
          <i class="pi pi-plus text-xs" aria-hidden="true" />{{ createLabel }}
        </button>
      </template>
    </header>

    <div class="mt-4 flex flex-wrap items-end gap-3">
      <form
        role="search"
        class="min-w-56 flex-1 sm:max-w-md"
        @submit.prevent="table.submitSearch()"
      >
        <label for="list-search" class="block text-sm font-medium text-slate-800">
          {{ searchLabel }}
        </label>
        <div class="relative mt-1">
          <span
            class="pointer-events-none absolute inset-y-0 start-3 flex items-center text-sm text-slate-500"
            aria-hidden="true"
          >
            <i class="pi pi-search" />
          </span>
          <input
            id="list-search"
            ref="searchEl"
            type="search"
            class="min-h-11 w-full rounded-lg border border-slate-300 bg-white ps-9 pe-10 text-sm"
            autocomplete="off"
            spellcheck="false"
            enterkeyhint="search"
            :value="table.search.value"
            @input="table.setSearch(($event.target as HTMLInputElement).value)"
          />
          <kbd
            class="pointer-events-none absolute inset-y-0 end-3 my-auto hidden h-6 items-center rounded border border-slate-300 bg-slate-50 px-1.5 text-xs text-slate-600 md:inline-flex"
            aria-hidden="true"
            >/</kbd
          >
        </div>
      </form>

      <FilterPanel
        v-if="defs.length > 0"
        :defs="defs"
        :values="table.filters.value"
        @change="table.setFilter"
        @clear="table.clearFilters"
      />
    </div>

    <!-- Nilai filter aktif sebagai chip; setiap chip dapat dilepas sendiri-sendiri. -->
    <ul
      v-if="chips.length > 0"
      class="mt-3 flex flex-wrap items-center gap-2"
      aria-label="Filter aktif"
    >
      <li v-for="c in chips" :key="c.key">
        <button
          type="button"
          class="inline-flex min-h-11 items-center gap-2 rounded-full border border-indigo-200 bg-indigo-50 ps-3 pe-2 text-sm font-medium text-indigo-900 hover:bg-indigo-100"
          :aria-label="`Hapus filter ${c.text}`"
          @click="table.setFilter(c.key, '')"
        >
          {{ c.text }}<i class="pi pi-times text-xs" aria-hidden="true" />
        </button>
      </li>
      <li>
        <button
          type="button"
          class="min-h-11 rounded-lg px-3 text-sm font-medium text-indigo-800 hover:bg-indigo-50"
          @click="table.clearFilters"
        >
          Hapus semua filter
        </button>
      </li>
    </ul>

    <!-- Bingkai tabel: permukaan putih + garis tipis. Status kosong/galat tetap di dalam bingkai. -->
    <div class="mt-4 overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
      <DataTable
        :columns="columns"
        :rows="rows"
        :caption="caption"
        :loading="table.loading.value"
        :sort-column="table.sortColumn.value"
        :sort-direction="table.sortDirection.value"
        @sort="onSort"
      >
        <template v-for="col in columns" :key="col.key" #[`cell-${col.key}`]="{ row }">
          <slot :name="`cell-${col.key}`" :row="row">{{ cellValue(row, col.key) }}</slot>
        </template>
      </DataTable>

      <div v-if="table.error.value" class="px-6 py-10 text-center" role="alert">
        <h2 class="text-base font-semibold text-slate-900">Data gagal dimuat</h2>
        <p class="mt-1 text-sm text-slate-700">{{ table.error.value }}</p>
        <button type="button" :class="[btnSecondary, 'mt-4']" @click="table.reload()">
          Coba lagi
        </button>
      </div>

      <div v-else-if="firstUse" class="px-6 py-10 text-center">
        <h2 class="text-base font-semibold text-slate-900">Belum ada {{ entity }}</h2>
        <p class="mx-auto mt-1 max-w-md text-sm text-slate-700">{{ emptyValue }}</p>
        <template v-if="canCreate && createLabel">
          <RouterLink v-if="createTo" :to="createTo" :class="[btnPrimary, 'mt-4']">
            {{ createLabel }}
          </RouterLink>
          <button v-else type="button" :class="[btnPrimary, 'mt-4']" @click="emit('create')">
            {{ createLabel }}
          </button>
        </template>
      </div>

      <div v-else-if="showEmpty" class="px-6 py-10 text-center">
        <h2 class="text-base font-semibold text-slate-900">Tidak ada {{ entity }} yang cocok</h2>
        <p class="mt-1 text-sm text-slate-700">
          Coba kata kunci lain, atau atur ulang pencarian dan filter.
        </p>
        <button type="button" :class="[btnSecondary, 'mt-4']" @click="table.clearFilters">
          Hapus semua filter
        </button>
      </div>

      <TablePagination
        :meta="table.meta.value"
        :loading="table.loading.value"
        @page="table.setPage"
        @limit="table.setLimit"
      />
    </div>
  </section>
</template>
