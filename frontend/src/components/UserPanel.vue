<script setup lang="ts">
import { useRouter } from 'vue-router'
import { roleLabel } from '@/lib/labels'
import { useAuthStore } from '@/stores/auth'

const props = defineProps<{
  /**
   * Satu baris untuk header (mis. saat wajib ganti password dan sidebar tidak ada): nama di
   * layar ≥ sm, selalu ada tombol Keluar. Tanpa ini: panel bertumpuk untuk sidebar/laci.
   */
  compact?: boolean
  /** Mode ciut (ikon saja) saat sidebar diciutkan. */
  collapsed?: boolean
}>()

const auth = useAuthStore()
const router = useRouter()

async function logout(): Promise<void> {
  await auth.logout()
  await router.replace({ name: 'login' })
}
</script>

<template>
  <div v-if="auth.user && props.compact" class="flex items-center gap-3">
    <div class="hidden min-w-0 text-end sm:block">
      <p class="truncate text-sm font-semibold text-slate-900">{{ auth.user.name }}</p>
      <p class="truncate text-xs text-slate-600">{{ roleLabel(auth.user.role) }}</p>
    </div>
    <button
      type="button"
      class="inline-flex min-h-11 items-center gap-2 rounded-lg px-3 text-sm font-medium text-slate-700 hover:bg-slate-100"
      @click="logout"
    >
      <i class="pi pi-sign-out" aria-hidden="true" />
      Keluar
    </button>
  </div>

  <div v-else-if="auth.user && props.collapsed" class="flex flex-col items-center gap-1">
    <div
      class="flex h-9 w-9 items-center justify-center rounded-full bg-indigo-100 text-xs font-semibold text-indigo-800"
      :title="`${auth.user.name} (${roleLabel(auth.user.role)})`"
    >
      {{ auth.user.name.slice(0, 2).toUpperCase() }}
    </div>
    <RouterLink
      :to="{ name: 'change-password' }"
      class="flex min-h-11 w-full items-center justify-center rounded-lg text-slate-700 hover:bg-slate-100"
      aria-label="Ganti Password"
      title="Ganti Password"
    >
      <i class="pi pi-key w-5 text-center" aria-hidden="true" />
    </RouterLink>
    <button
      type="button"
      class="flex min-h-11 w-full items-center justify-center rounded-lg text-slate-700 hover:bg-slate-100"
      aria-label="Keluar"
      title="Keluar"
      @click="logout"
    >
      <i class="pi pi-sign-out w-5 text-center" aria-hidden="true" />
    </button>
  </div>

  <div v-else-if="auth.user" class="space-y-1">
    <div class="px-3 py-2">
      <p class="truncate text-sm font-semibold text-slate-900">{{ auth.user.name }}</p>
      <p class="truncate text-xs text-slate-600">{{ roleLabel(auth.user.role) }}</p>
    </div>
    <RouterLink
      :to="{ name: 'change-password' }"
      class="flex min-h-11 items-center gap-3 rounded-lg px-3 text-sm font-medium text-slate-700 hover:bg-slate-100"
    >
      <i class="pi pi-key w-5 text-center" aria-hidden="true" />
      Ganti Password
    </RouterLink>
    <button
      type="button"
      class="flex min-h-11 w-full items-center gap-3 rounded-lg px-3 text-start text-sm font-medium text-slate-700 hover:bg-slate-100"
      @click="logout"
    >
      <i class="pi pi-sign-out w-5 text-center" aria-hidden="true" />
      Keluar
    </button>
  </div>
</template>
