<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const auth = useAuthStore()

const ALL = [
  { label: 'Produk', to: '/master/produk', permission: 'product.read' },
  { label: 'Kategori', to: '/master/kategori', permission: 'master.read' },
  { label: 'Gudang', to: '/master/gudang', permission: 'master.read' },
  { label: 'Rak', to: '/master/rak', permission: 'master.read' },
  { label: 'Supplier', to: '/master/supplier', permission: 'master.read' },
]

const tabs = computed(() => ALL.filter((t) => auth.can(t.permission)))
const isCurrent = (to: string) => route.path === to || route.path.startsWith(`${to}/`)
</script>

<template>
  <!-- Navigasi antar daftar master (tautan rute, bukan ARIA tabs: tiap item membuka halaman sendiri). -->
  <nav aria-label="Data master" class="overflow-x-auto">
    <ul class="flex min-w-max gap-1 border-b border-slate-200">
      <li v-for="t in tabs" :key="t.to">
        <RouterLink
          :to="t.to"
          class="-mb-px inline-flex min-h-11 items-center border-b-2 px-4 text-sm font-medium"
          :class="
            isCurrent(t.to)
              ? 'border-indigo-700 text-indigo-800'
              : 'border-transparent text-slate-700 hover:border-slate-300 hover:text-slate-900'
          "
          :aria-current="isCurrent(t.to) ? 'page' : undefined"
          >{{ t.label }}</RouterLink
        >
      </li>
    </ul>
  </nav>
</template>
