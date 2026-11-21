<script setup lang="ts">
import { computed } from 'vue'

export interface FilterDef {
  key: string
  label: string
  /** select (pilihan tetap) atau text (teks bebas). */
  type?: 'select' | 'text'
  options?: { value: string; label: string }[]
}

const props = defineProps<{
  defs: FilterDef[]
  values: Record<string, string>
}>()
const emit = defineEmits<{ change: [key: string, value: string]; clear: [] }>()

const panelId = 'filter-panel'
const activeCount = computed(() => props.defs.filter((d) => props.values[d.key]).length)
</script>

<template>
  <div>
    <!-- Satu kontrol Filter; semua dimensi ada di panelnya. Nilai aktif muncul sebagai chip di bawah toolbar. -->
    <button
      type="button"
      class="filter-trigger inline-flex min-h-11 items-center gap-2 rounded-lg border border-slate-300 bg-white px-4 text-sm font-medium text-slate-800 hover:bg-slate-50"
      :popovertarget="panelId"
      aria-haspopup="true"
    >
      <i class="pi pi-filter text-xs" aria-hidden="true" />
      Filter
      <span
        v-if="activeCount > 0"
        class="inline-flex min-w-5 items-center justify-center rounded-full bg-indigo-700 px-1.5 text-xs font-semibold text-white"
        >{{ activeCount }}<span class="sr-only"> filter aktif</span></span
      >
    </button>

    <div :id="panelId" popover="auto" class="filter-panel" aria-label="Filter data" role="group">
      <div class="space-y-3">
        <div v-for="def in defs" :key="def.key">
          <label :for="`filter-${def.key}`" class="block text-sm font-medium text-slate-800">{{
            def.label
          }}</label>
          <select
            v-if="def.type !== 'text'"
            :id="`filter-${def.key}`"
            class="mt-1 min-h-11 w-full rounded-lg border border-slate-300 bg-white px-3 text-sm"
            :value="values[def.key] ?? ''"
            @change="emit('change', def.key, ($event.target as HTMLSelectElement).value)"
          >
            <option value="">Semua</option>
            <option v-for="o in def.options ?? []" :key="o.value" :value="o.value">
              {{ o.label }}
            </option>
          </select>
          <input
            v-else
            :id="`filter-${def.key}`"
            type="text"
            class="mt-1 min-h-11 w-full rounded-lg border border-slate-300 bg-white px-3 text-sm"
            :value="values[def.key] ?? ''"
            @change="emit('change', def.key, ($event.target as HTMLInputElement).value.trim())"
          />
        </div>
      </div>
      <div class="mt-4 flex items-center justify-between gap-2">
        <button
          type="button"
          class="min-h-11 rounded-lg px-3 text-sm font-medium text-indigo-800 hover:bg-indigo-50 disabled:cursor-not-allowed disabled:text-slate-500 disabled:hover:bg-transparent"
          :disabled="activeCount === 0"
          @click="emit('clear')"
        >
          Hapus semua filter
        </button>
        <button
          type="button"
          class="min-h-11 rounded-lg border border-slate-300 px-4 text-sm font-medium text-slate-800 hover:bg-slate-50"
          :popovertarget="panelId"
          popovertargetaction="hide"
        >
          Tutup
        </button>
      </div>
    </div>
  </div>
</template>
