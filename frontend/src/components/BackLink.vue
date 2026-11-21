<script setup lang="ts">
import type { RouteLocationRaw } from 'vue-router'

defineProps<{ to: RouteLocationRaw; label: string }>()
</script>

<template>
  <RouterLink
    :to="to"
    class="inline-flex min-h-11 items-center gap-2 text-sm font-medium text-indigo-800 hover:underline"
  >
    <i class="pi pi-arrow-left text-xs" aria-hidden="true" />{{ label }}
  </RouterLink>
</template>
