<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'

/**
 * Panel samping untuk tugas singkat (maks. 2 field). <dialog> modal dari tepi kanan: Escape menutup,
 * klik backdrop menutup, fokus kembali ke pemicu. Panel penuh lebar pada layar sempit.
 */
const props = defineProps<{ open: boolean; title: string }>()
const emit = defineEmits<{ close: [] }>()

const dialog = ref<HTMLDialogElement | null>(null)

// Isi tetap dirender selama animasi keluar (200 ms) agar panel tidak mengosong saat bergeser.
const rendered = ref(props.open)
let unmountTimer: ReturnType<typeof setTimeout> | undefined
watch(
  () => props.open,
  (open) => {
    clearTimeout(unmountTimer)
    if (open) rendered.value = true
    else unmountTimer = setTimeout(() => (rendered.value = false), 250)
  },
)

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

function onClose(): void {
  if (props.open) emit('close')
}
// Konten mengisi seluruh panel, jadi klik yang mengenai <dialog> itu sendiri = backdrop.
function onClick(e: MouseEvent): void {
  if (e.target === dialog.value) dialog.value?.close()
}
</script>

<template>
  <dialog
    ref="dialog"
    class="slide-over"
    aria-labelledby="slide-over-title"
    @close="onClose"
    @click="onClick"
  >
    <div v-if="rendered" class="flex h-full flex-col">
      <div class="flex min-h-14 items-center justify-between border-b border-slate-200 ps-5 pe-2">
        <h2 id="slide-over-title" class="text-base font-semibold text-slate-900">{{ title }}</h2>
        <button
          type="button"
          class="inline-flex min-h-11 min-w-11 items-center justify-center rounded-lg text-slate-800 hover:bg-slate-100"
          aria-label="Tutup panel"
          @click="dialog?.close()"
        >
          <i class="pi pi-times" aria-hidden="true" />
        </button>
      </div>
      <div class="flex-1 overflow-y-auto p-5"><slot /></div>
    </div>
  </dialog>
</template>
