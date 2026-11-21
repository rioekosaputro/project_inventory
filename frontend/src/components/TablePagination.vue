<script setup lang="ts">
import { computed } from 'vue'
import { LIMIT_CHOICES } from '@/composables/useServerTable'
import type { PageMeta } from '@/types/api'

const props = defineProps<{ meta: PageMeta | null; loading?: boolean }>()
const emit = defineEmits<{ page: [number]; limit: [number] }>()

const total = computed(() => props.meta?.total ?? 0)
const page = computed(() => props.meta?.page ?? 1)
const pages = computed(() => Math.max(props.meta?.total_pages ?? 1, 1))
const limit = computed(() => props.meta?.limit ?? 20)

const from = computed(() => (total.value === 0 ? 0 : (page.value - 1) * limit.value + 1))
const to = computed(() => Math.min(page.value * limit.value, total.value))

const btn =
  'inline-flex min-h-11 items-center gap-2 rounded-lg border border-slate-300 bg-white px-4 text-sm font-medium text-slate-800 hover:bg-slate-50 disabled:cursor-not-allowed disabled:bg-slate-50 disabled:text-slate-500'
</script>

<template>
  <!-- Selalu tampil, juga bila hanya satu halaman. Tombol nonaktif tetap berlabel. -->
  <nav
    aria-label="Paginasi"
    class="flex flex-wrap items-center justify-between gap-3 border-t border-slate-200 px-4 py-3 text-sm text-slate-700"
  >
    <p aria-live="off">
      <template v-if="total > 0">
        Menampilkan <strong>{{ from }}–{{ to }}</strong> dari <strong>{{ total }}</strong>
      </template>
      <template v-else>Tidak ada data</template>
    </p>

    <div class="flex flex-wrap items-center gap-2">
      <label class="flex items-center gap-2">
        <span>Baris per halaman</span>
        <select
          class="min-h-11 rounded-lg border border-slate-300 bg-white px-2"
          :value="limit"
          @change="emit('limit', Number(($event.target as HTMLSelectElement).value))"
        >
          <option v-for="n in LIMIT_CHOICES" :key="n" :value="n">{{ n }}</option>
        </select>
      </label>

      <button
        type="button"
        :class="btn"
        :disabled="page <= 1 || loading"
        @click="emit('page', page - 1)"
      >
        <i class="pi pi-chevron-left text-xs" aria-hidden="true" />
        Sebelumnya
      </button>
      <span class="px-1">Halaman {{ page }} dari {{ pages }}</span>
      <button
        type="button"
        :class="btn"
        :disabled="page >= pages || loading"
        @click="emit('page', page + 1)"
      >
        Berikutnya
        <i class="pi pi-chevron-right text-xs" aria-hidden="true" />
      </button>
    </div>
  </nav>
</template>
