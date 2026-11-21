<script setup lang="ts">
import { announcement } from '@/composables/useAnnouncer'
</script>

<template>
  <div class="sr-only" role="status" aria-live="polite" aria-atomic="true">{{ announcement }}</div>
</template>
