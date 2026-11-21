<script setup lang="ts">
// Pola halaman: Form (buat/ubah satu data). Tombol Simpan tidak pernah dinonaktifkan.
import BackLink from '@/components/BackLink.vue'
import Message from 'primevue/message'
import { btnPrimary, btnSecondary } from '@/lib/ui'
import type { RouteLocationRaw } from 'vue-router'

defineProps<{
  title: string
  description?: string
  backTo: RouteLocationRaw
  backLabel: string
  submitLabel: string
  submitting?: boolean
  /** Galat tingkat form (bukan milik satu field). */
  formError?: string
  /** Memuat data awal (mode ubah). */
  loading?: boolean
  /** Gagal memuat data awal (mis. tidak ditemukan). */
  loadError?: string
}>()
const emit = defineEmits<{ submit: [] }>()
</script>

<template>
  <section class="max-w-2xl">
    <BackLink :to="backTo" :label="backLabel" />
    <h1 class="mt-3 text-xl font-semibold text-slate-900">{{ title }}</h1>
    <p v-if="description" class="mt-1 text-sm text-slate-600">{{ description }}</p>

    <div
      v-if="loading"
      class="mt-6 space-y-4 rounded-xl border border-slate-200 bg-white p-6"
      aria-busy="true"
    >
      <span class="sr-only">Memuat data…</span>
      <div
        v-for="n in 4"
        :key="n"
        class="h-11 animate-pulse rounded-lg bg-slate-200"
        aria-hidden="true"
      />
    </div>

    <div
      v-else-if="loadError"
      class="mt-6 rounded-xl border border-slate-200 bg-white px-6 py-10 text-center"
      role="alert"
    >
      <h2 class="text-base font-semibold text-slate-900">Data tidak dapat ditampilkan</h2>
      <p class="mt-1 text-sm text-slate-700">{{ loadError }}</p>
      <RouterLink :to="backTo" :class="[btnSecondary, 'mt-4']">{{ backLabel }}</RouterLink>
    </div>

    <form
      v-else
      id="entity-form"
      class="mt-6 space-y-4 rounded-xl border border-slate-200 bg-white p-6 shadow-sm"
      novalidate
      @submit.prevent="emit('submit')"
    >
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>

      <slot />

      <div class="flex flex-wrap items-center gap-3 pt-2">
        <button type="submit" :class="btnPrimary" :aria-busy="submitting">
          <i v-if="submitting" class="pi pi-spinner pi-spin" aria-hidden="true" />
          {{ submitting ? 'Menyimpan…' : submitLabel }}
        </button>
        <RouterLink :to="backTo" :class="btnSecondary">Batal</RouterLink>
      </div>
    </form>
  </section>
</template>
