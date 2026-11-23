<script setup lang="ts">
import { computed } from 'vue'
import { stockImpact } from '@/lib/lpb'
import type { LPBItem, LPBStatus, LPBType } from '@/types/lpb'

/** Saldo rak "sebelum → sesudah" untuk satu item LPB (angka ledger bila sudah disetujui, selain itu prakiraan). */
const props = defineProps<{ item: LPBItem; type: LPBType; status: LPBStatus }>()
const impact = computed(() => stockImpact(props.item, props.type, props.status))
</script>

<template>
  <span v-if="impact" data-impact class="whitespace-nowrap tabular-nums">
    {{ impact.before }} <span aria-hidden="true">→</span><span class="sr-only"> menjadi </span>
    <strong>{{ impact.after }}</strong>
    <span v-if="!impact.final" class="ms-1 text-xs text-slate-600">(prakiraan)</span>
  </span>
  <span v-else data-impact>—</span>
</template>
