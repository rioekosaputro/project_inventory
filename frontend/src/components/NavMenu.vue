<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { navGroups, visibleNav, type NavItem } from '@/config/navigation'
import { routeExists } from '@/router/guards'
import { useAuthStore } from '@/stores/auth'

const props = defineProps<{
  /** Mode ciut (hanya ikon dengan tooltip) pada sidebar layar lebar. */
  collapsed?: boolean
}>()

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()

// Menu dinamis: hanya item yang permission-nya dimiliki pengguna dan rutenya sudah ada.
const groups = computed(() =>
  visibleNav(navGroups, { can: auth.can, routeExists: (path) => routeExists(router, path) }),
)

// Item aktif: path persis sama atau berada di bawah awalan (mis. /master/rak di bawah /master).
// Bila beberapa item cocok (mis. /lpb dan /lpb/approvals untuk /lpb/approvals), hanya yang awalannya
// paling spesifik (terpanjang) yang aktif.
function matchLength(item: NavItem): number {
  const prefix = item.match ?? item.to
  const hit = route.path === item.to || route.path === prefix || route.path.startsWith(`${prefix}/`)
  return hit ? Math.max(prefix.length, item.to.length) : -1
}
const activeTo = computed(() => {
  let best: { to: string; len: number } | null = null
  for (const g of groups.value) {
    for (const item of g.items) {
      const len = matchLength(item)
      if (len >= 0 && (!best || len > best.len)) best = { to: item.to, len }
    }
  }
  return best?.to ?? null
})
const isActive = (item: NavItem): boolean => item.to === activeTo.value
</script>

<template>
  <nav aria-label="Menu utama">
    <div
      v-for="(group, idx) in groups"
      :key="group.label"
      class="mb-2"
      :class="props.collapsed && idx > 0 ? 'border-t border-slate-100 pt-2' : ''"
    >
      <p
        v-if="!props.collapsed"
        class="px-3 pt-3 pb-1 text-xs font-semibold tracking-wide text-slate-600 uppercase"
      >
        {{ group.label }}
      </p>
      <ul class="space-y-0.5">
        <li v-for="item in group.items" :key="item.to">
          <RouterLink
            :to="item.to"
            class="flex min-h-11 items-center rounded-lg text-sm font-medium hover:bg-slate-100"
            :class="[
              props.collapsed ? 'justify-center px-0' : 'gap-3 px-3',
              isActive(item) ? 'bg-indigo-50 text-indigo-800' : 'text-slate-700',
            ]"
            :aria-current="isActive(item) ? 'page' : undefined"
            :aria-label="props.collapsed ? item.label : undefined"
            :title="props.collapsed ? item.label : undefined"
          >
            <i :class="item.icon" class="w-5 shrink-0 text-center" aria-hidden="true" />
            <span v-if="!props.collapsed" class="truncate">{{ item.label }}</span>
          </RouterLink>
        </li>
      </ul>
    </div>
  </nav>
</template>
