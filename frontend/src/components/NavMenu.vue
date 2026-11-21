<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { navGroups, visibleNav } from '@/config/navigation'
import { routeExists } from '@/router/guards'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const auth = useAuthStore()

// Menu dinamis: hanya item yang permission-nya dimiliki pengguna dan rutenya sudah ada.
const groups = computed(() =>
  visibleNav(navGroups, { can: auth.can, routeExists: (path) => routeExists(router, path) }),
)
</script>

<template>
  <nav aria-label="Menu utama">
    <div v-for="group in groups" :key="group.label" class="mb-2">
      <p class="px-3 pt-3 pb-1 text-xs font-semibold tracking-wide text-slate-600 uppercase">
        {{ group.label }}
      </p>
      <ul>
        <li v-for="item in group.items" :key="item.to">
          <RouterLink
            :to="item.to"
            class="flex min-h-11 items-center gap-3 rounded-lg px-3 text-sm font-medium text-slate-700 hover:bg-slate-100"
            active-class="!bg-indigo-50 !text-indigo-800"
          >
            <i :class="item.icon" class="w-5 text-center" aria-hidden="true" />
            <span>{{ item.label }}</span>
          </RouterLink>
        </li>
      </ul>
    </div>
  </nav>
</template>
