<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import Toast from 'primevue/toast'
import LiveRegion from '@/components/LiveRegion.vue'
import AppLayout from '@/layouts/AppLayout.vue'
import MobileLayout from '@/layouts/MobileLayout.vue'
import PublicLayout from '@/layouts/PublicLayout.vue'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const auth = useAuthStore()

const layouts = { app: AppLayout, mobile: MobileLayout, public: PublicLayout } as const

const layout = computed(() => {
  const wanted = route.meta.layout ?? 'auto'
  const key = wanted === 'auto' ? (auth.isAuthenticated ? 'app' : 'public') : wanted
  return layouts[key]
})
</script>

<template>
  <!-- Selama sesi dipulihkan (cookie refresh) jangan tampilkan layout apa pun agar tidak berkedip. -->
  <div v-if="!auth.ready" class="grid min-h-dvh place-items-center text-slate-600" role="status">
    <span><i class="pi pi-spinner pi-spin" aria-hidden="true" /> Memuat…</span>
  </div>
  <component :is="layout" v-else>
    <RouterView />
  </component>

  <Toast position="top-right" />
  <LiveRegion />
</template>
