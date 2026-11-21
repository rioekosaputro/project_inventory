<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'

/**
 * Konfirmasi aksi berisiko: judul menyebut entitas secara spesifik, tombol konfirmasi memakai
 * kata kerja aksinya dan berwarna peringatan, fokus awal di "Batal", Escape membatalkan.
 * <dialog> modal: browser mengurus inert, fokus terkurung, Escape, dan mengembalikan fokus ke pemicu.
 */
const props = defineProps<{
  open: boolean
  title: string
  message?: string
  confirmLabel: string
  busy?: boolean
}>()
const emit = defineEmits<{ confirm: []; cancel: [] }>()

const dialog = ref<HTMLDialogElement | null>(null)

watch(
  () => props.open,
  async (open) => {
    await nextTick()
    const el = dialog.value
    if (!el) return
    if (open && !el.open) el.showModal?.()
    if (!open && el.open) el.close()
  },
  { immediate: true, flush: 'post' },
)

// Escape / tombol Batal menutup <dialog> sendiri; sinkronkan status ke pemilik.
function onClose(): void {
  if (props.open) emit('cancel')
}
</script>

<template>
  <dialog ref="dialog" class="confirm-dialog" aria-labelledby="confirm-title" @close="onClose">
    <h2 id="confirm-title" class="text-lg font-semibold text-slate-900">{{ title }}</h2>
    <p v-if="message" class="mt-2 text-sm text-slate-700">{{ message }}</p>
    <div class="mt-6 flex justify-end gap-2">
      <button
        type="button"
        autofocus
        class="min-h-11 rounded-lg border border-slate-300 px-4 text-sm font-medium text-slate-800 hover:bg-slate-50"
        @click="dialog?.close()"
      >
        Batal
      </button>
      <button
        type="button"
        class="min-h-11 rounded-lg bg-red-700 px-4 text-sm font-medium text-white hover:bg-red-800 disabled:opacity-70"
        :aria-busy="busy"
        @click="emit('confirm')"
      >
        {{ busy ? 'Memproses…' : confirmLabel }}
      </button>
    </div>
  </dialog>
</template>
