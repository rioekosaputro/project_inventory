<script setup lang="ts">
import { computed } from 'vue'

/**
 * Satu sumber label & warna untuk semua status/jenis di aplikasi (plan Bagian 9.2).
 * Makna tidak hanya lewat warna: selalu ada teks label.
 */
const props = defineProps<{ value: string }>()

type Tone = 'green' | 'slate' | 'indigo' | 'amber' | 'red' | 'sky'

const STATUS: Record<string, { label: string; tone: Tone }> = {
  ACTIVE: { label: 'Aktif', tone: 'green' },
  INACTIVE: { label: 'Nonaktif', tone: 'slate' },
  ASSET: { label: 'Aset', tone: 'indigo' },
  CONSUMABLE: { label: 'Habis pakai', tone: 'amber' },
  YES: { label: 'Ya', tone: 'sky' },
  NO: { label: 'Tidak', tone: 'slate' },
  LOCKED: { label: 'Dikunci opname', tone: 'red' },
}

const TONES: Record<Tone, string> = {
  green: 'bg-green-100 text-green-900',
  slate: 'bg-slate-100 text-slate-800',
  indigo: 'bg-indigo-100 text-indigo-900',
  amber: 'bg-amber-100 text-amber-900',
  red: 'bg-red-100 text-red-900',
  sky: 'bg-sky-100 text-sky-900',
}

const view = computed(() => STATUS[props.value] ?? { label: props.value, tone: 'slate' as Tone })
</script>

<template>
  <span
    class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium whitespace-nowrap"
    :class="TONES[view.tone]"
    >{{ view.label }}</span
  >
</template>
