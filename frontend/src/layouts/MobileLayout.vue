<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

// Layout satu kolom untuk driver (PWA, Fase 3): tombol besar, tanpa sidebar.
const auth = useAuthStore()
const router = useRouter()

async function logout(): Promise<void> {
  await auth.logout()
  await router.replace({ name: 'login' })
}
</script>

<template>
  <a href="#main" class="skip-link">Lewati ke konten utama</a>

  <div class="flex min-h-dvh flex-col">
    <header
      class="sticky top-0 z-20 flex h-14 items-center justify-between border-b border-slate-200 bg-white px-4"
    >
      <p class="text-base font-semibold">Rental ERP</p>
      <button
        type="button"
        class="inline-flex min-h-11 items-center gap-2 rounded-lg px-3 text-sm font-medium text-slate-700 hover:bg-slate-100"
        @click="logout"
      >
        <i class="pi pi-sign-out" aria-hidden="true" />
        Keluar
      </button>
    </header>
    <main id="main" tabindex="-1" class="mx-auto w-full max-w-xl flex-1 p-4 pb-8 outline-none">
      <slot />
    </main>
  </div>
</template>
