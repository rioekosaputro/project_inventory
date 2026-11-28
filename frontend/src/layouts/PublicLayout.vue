<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const fullBleed = computed(() => Boolean(route.meta.fullBleed))
</script>

<template>
  <a href="#main" class="skip-link">Lewati ke konten utama</a>

  <main v-if="fullBleed" id="main" tabindex="-1" class="min-h-dvh w-full outline-none">
    <slot />
  </main>

  <div v-else class="flex min-h-dvh flex-col items-center justify-center px-4 py-8">
    <p class="mb-6 flex items-center gap-2 text-lg font-semibold">
      <i class="pi pi-box text-indigo-700" aria-hidden="true" />
      Rental ERP
    </p>
    <main id="main" tabindex="-1" class="w-full max-w-md outline-none">
      <slot />
    </main>
  </div>
</template>
