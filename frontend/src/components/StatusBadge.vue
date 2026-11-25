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
  // LPB (plan Bagian 5.2)
  DRAFT: { label: 'Draft', tone: 'slate' },
  PENDING_APPROVAL: { label: 'Menunggu persetujuan', tone: 'amber' },
  APPROVED: { label: 'Disetujui', tone: 'green' },
  REJECTED: { label: 'Ditolak', tone: 'red' },
  CANCELLED: { label: 'Dibatalkan', tone: 'slate' },
  INBOUND: { label: 'Masuk', tone: 'sky' },
  OUTBOUND: { label: 'Keluar', tone: 'indigo' },
  // Stock Opname (T1.12)
  IN_PROGRESS: { label: 'Sedang berjalan', tone: 'sky' },
  SUBMITTED: { label: 'Diajukan', tone: 'amber' },
  FINALIZED: { label: 'Difinalisasi', tone: 'green' },
  FOUND: { label: 'Ditemukan', tone: 'green' },
  MISSING: { label: 'Belum ditemukan', tone: 'red' },
  UNEXPECTED: { label: 'Tak terduga', tone: 'amber' },
  WRONG_RACK: { label: 'Salah rak', tone: 'indigo' },
  // Inventory & Serial (T1.13)
  AVAILABLE: { label: 'Tersedia', tone: 'green' },
  RESERVED: { label: 'Direservasi', tone: 'amber' },
  IN_TRANSIT: { label: 'Dalam perjalanan', tone: 'sky' },
  RENTED: { label: 'Disewa', tone: 'indigo' },
  MAINTENANCE: { label: 'Perawatan', tone: 'amber' },
  BROKEN: { label: 'Rusak', tone: 'red' },
  LOST: { label: 'Hilang', tone: 'red' },
  RETIRED: { label: 'Pensiun', tone: 'slate' },
  IN: { label: 'Masuk', tone: 'green' },
  OUT: { label: 'Keluar', tone: 'indigo' },
  LOW_STOCK: { label: 'Menipis', tone: 'red' },
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
