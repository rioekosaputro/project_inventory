<script setup lang="ts" generic="T extends object">
import { computed } from 'vue'

export interface Column {
  key: string
  label: string
  /** Kunci urutan di API; kosong = kolom tidak dapat diurutkan. */
  sortKey?: string
  /** Kolom tindakan: label disembunyikan secara visual (tetap dibaca pembaca layar). */
  actions?: boolean
  class?: string
}

const props = withDefaults(
  defineProps<{
    columns: Column[]
    rows: T[]
    rowKey?: string
    /** Nama tabel: dipakai sebagai nama wilayah gulir (pembaca layar). */
    caption: string
    loading?: boolean
    sortColumn?: string
    sortDirection?: 'ascending' | 'descending'
  }>(),
  { rowKey: 'id', loading: false, sortColumn: '', sortDirection: 'ascending' },
)

const emit = defineEmits<{ sort: [sortKey: string] }>()

defineSlots<{ [key: `cell-${string}`]: (props: { row: T }) => unknown }>()

const captionId = `tbl-${Math.random().toString(36).slice(2, 8)}`
const showSkeleton = computed(() => props.loading && props.rows.length === 0)

function cell(row: T, key: string): unknown {
  return (row as Record<string, unknown>)[key] ?? '—'
}

function ariaSort(col: Column): 'ascending' | 'descending' | undefined {
  // aria-sort hanya pada kolom yang sedang diurutkan (tidak memakai "none").
  return col.sortKey && col.sortKey === props.sortColumn ? props.sortDirection : undefined
}
</script>

<template>
  <!-- Wilayah gulir: kolom lebar bergulir di dalam kotak, bukan di halaman. -->
  <div
    role="region"
    tabindex="0"
    :aria-labelledby="captionId"
    class="data-table-wrapper"
    :aria-busy="loading"
  >
    <table class="data-table">
      <caption :id="captionId" class="sr-only">
        {{
          caption
        }}
      </caption>
      <thead>
        <tr>
          <th v-for="col in columns" :key="col.key" scope="col" :aria-sort="ariaSort(col)">
            <button
              v-if="col.sortKey"
              type="button"
              class="sort-button"
              @click="emit('sort', col.sortKey)"
            >
              <span>{{ col.label }}</span>
              <i
                class="pi sort-icon"
                :class="
                  ariaSort(col) === 'ascending'
                    ? 'pi-sort-amount-up-alt'
                    : ariaSort(col) === 'descending'
                      ? 'pi-sort-amount-down'
                      : 'pi-sort-alt'
                "
                aria-hidden="true"
              />
            </button>
            <span v-else :class="{ 'sr-only': col.actions }">{{ col.label }}</span>
          </th>
        </tr>
      </thead>
      <tbody :class="{ 'opacity-60 transition-opacity': loading && !showSkeleton }">
        <template v-if="showSkeleton">
          <tr v-for="n in 5" :key="`sk-${n}`" aria-hidden="true">
            <td v-for="col in columns" :key="col.key">
              <span class="block h-4 animate-pulse rounded bg-slate-200" />
            </td>
          </tr>
        </template>
        <template v-else>
          <tr v-for="row in rows" :key="String((row as Record<string, unknown>)[rowKey])">
            <td v-for="col in columns" :key="col.key" :class="col.class">
              <slot :name="`cell-${col.key}`" :row="row">{{ cell(row, col.key) }}</slot>
            </td>
          </tr>
        </template>
      </tbody>
    </table>
  </div>
</template>
