<script setup lang="ts">
// Scanner serial number gaya POS (plan Bagian 9.3). Alat scan USB/Bluetooth bekerja sebagai papan
// ketik yang mengakhiri kode dengan Enter, jadi satu kolom isian yang selalu terfokus sudah cukup;
// kamera/ketik manual memakai kolom dan tombol yang sama.
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useScanQueue } from '@/composables/useScanQueue'
import { isMuted, setMuted, unlockAudio } from '@/lib/audio'
import { normalizeCode, scansRegisteredUnits } from '@/lib/lpb'
import { btnPrimary, btnSecondary } from '@/lib/ui'
import { lpbService } from '@/services/lpbService'
import type { AddSerialResult, LPBItem, LPBSubType, SerialCondition } from '@/types/lpb'

const props = defineProps<{
  lpbId: number
  subType: LPBSubType
  /** Item bertipe serial number pada dokumen (item lain diabaikan). */
  items: LPBItem[]
}>()
const emit = defineEmits<{ saved: [itemId: number, result: AddSerialResult] }>()

/** Penghapusan/transfer memindahkan unit yang sudah terdaftar: kode = asset tag atau SN, tanpa kondisi. */
const isRegistered = computed(() => scansRegisteredUnits(props.subType))
const serialItems = computed(() => props.items.filter((i) => i.has_serial_number))

const remaining = (item: LPBItem): number => Math.max(item.qty - item.serials.length, 0)
const nextIncomplete = (): number | null =>
  serialItems.value.find((i) => remaining(i) > 0)?.id ?? null

const targetId = ref<number | null>(nextIncomplete() ?? serialItems.value[0]?.id ?? null)
const code = ref('')
const condition = ref<SerialCondition>('GOOD')
const muted = ref(isMuted())
const input = ref<HTMLInputElement | null>(null)

const target = computed(() => serialItems.value.find((i) => i.id === targetId.value) ?? null)

/** SN dan asset tag yang sudah tercatat di dokumen ini (untuk deteksi dobel lokal). */
const known = computed(() => {
  const set = new Set<string>()
  for (const item of props.items) {
    for (const s of item.serials) {
      set.add(normalizeCode(s.serial_number))
      if (s.asset_tag) set.add(normalizeCode(s.asset_tag))
    }
  }
  return set
})

const queue = useScanQueue<SerialCondition>({
  send: (itemId, serial, cond) =>
    lpbService.addSerial(props.lpbId, itemId, {
      serial_number: serial,
      ...(isRegistered.value ? {} : { condition: cond }),
    }),
  known: () => known.value,
  capacity: (itemId) => {
    const item = serialItems.value.find((i) => i.id === itemId)
    return item ? remaining(item) : 0
  },
  onSaved: (itemId, result) => {
    emit('saved', itemId, result)
    // Item penuh → pindah otomatis ke item berikutnya yang belum lengkap.
    if (result.scanned_count >= result.qty) {
      const next = nextIncomplete()
      if (next !== null && next !== itemId) targetId.value = next
    }
  },
})

// Daftar item dapat berubah (muat ulang/ubah): jaga target tetap valid.
watch(serialItems, (list) => {
  if (!list.some((i) => i.id === targetId.value)) {
    targetId.value = nextIncomplete() ?? list[0]?.id ?? null
  }
})

function submit(): void {
  unlockAudio()
  if (targetId.value === null) return
  const raw = code.value
  code.value = ''
  queue.submit(raw, targetId.value, condition.value)
  void nextTick(() => input.value?.focus()) // scan berikutnya langsung siap, tanpa menyentuh layar
}

function toggleMute(): void {
  muted.value = !muted.value
  setMuted(muted.value)
}

onMounted(() => input.value?.focus())

const STATUS_ICON: Record<string, string> = {
  queued: 'pi pi-clock',
  sending: 'pi pi-spinner pi-spin',
  ok: 'pi pi-check-circle',
  error: 'pi pi-times-circle',
  duplicate: 'pi pi-exclamation-triangle',
}
const STATUS_TEXT: Record<string, string> = {
  queued: 'Menunggu',
  sending: 'Diperiksa',
  ok: 'Berhasil',
  error: 'Gagal',
  duplicate: 'Dobel',
}
const STATUS_CLASS: Record<string, string> = {
  queued: 'text-slate-700',
  sending: 'text-slate-700',
  ok: 'text-green-800',
  error: 'text-red-800',
  duplicate: 'text-amber-900',
}

const itemLabel = (i: LPBItem): string =>
  `${i.sku} · ${i.product_name} — rak ${i.rack_code} (${i.serials.length} dari ${i.qty})`
</script>

<template>
  <section
    aria-labelledby="scanner-title"
    class="rounded-xl border border-slate-200 bg-white p-4 shadow-sm"
    @pointerdown.once="unlockAudio"
  >
    <div class="flex flex-wrap items-start justify-between gap-2">
      <div>
        <h2 id="scanner-title" class="text-base font-semibold text-slate-900">
          {{
            props.subType === 'DISPOSAL'
              ? 'Scan unit yang dihapus'
              : props.subType === 'TRANSFER_OUT'
                ? 'Scan unit yang dipindahkan'
                : 'Scan serial number'
          }}
        </h2>
        <p class="mt-1 text-sm text-slate-600">
          Arahkan alat scan ke barcode — setiap Enter menambahkan satu unit. Scan berikutnya tidak
          perlu menunggu hasil sebelumnya.
        </p>
      </div>
      <button type="button" :class="btnSecondary" :aria-pressed="muted" @click="toggleMute">
        <i :class="muted ? 'pi pi-volume-off' : 'pi pi-volume-up'" aria-hidden="true" />
        {{ muted ? 'Suara mati' : 'Suara aktif' }}
      </button>
    </div>

    <p v-if="serialItems.length === 0" class="mt-4 text-sm text-slate-700">
      Dokumen ini tidak memiliki item berserial number, jadi tidak ada yang perlu di-scan.
    </p>

    <form v-else id="scan-form" class="mt-4 space-y-3" novalidate @submit.prevent="submit">
      <div class="grid gap-3 sm:grid-cols-2">
        <div class="sm:col-span-2">
          <label for="scan-item" class="block text-sm font-medium text-slate-800">
            Item yang sedang di-scan
          </label>
          <select
            id="scan-item"
            v-model="targetId"
            class="mt-1 min-h-11 w-full rounded-md border border-slate-300 bg-white px-3 text-base"
          >
            <option v-for="i in serialItems" :key="i.id" :value="i.id">
              {{ itemLabel(i) }}{{ remaining(i) === 0 ? ' — lengkap' : '' }}
            </option>
          </select>
        </div>

        <div :class="isRegistered ? 'sm:col-span-2' : ''">
          <label for="scan-code" class="block text-sm font-medium text-slate-800">
            {{ isRegistered ? 'Asset tag atau serial number' : 'Serial number' }}
          </label>
          <input
            id="scan-code"
            ref="input"
            v-model="code"
            type="text"
            class="mt-1 min-h-11 w-full rounded-md border border-slate-300 bg-white px-3 font-mono text-base"
            autocomplete="off"
            autocapitalize="characters"
            spellcheck="false"
            enterkeyhint="done"
            aria-describedby="scan-hint"
            @keydown="unlockAudio"
          />
        </div>

        <div v-if="!isRegistered">
          <label for="scan-condition" class="block text-sm font-medium text-slate-800">
            Kondisi unit
          </label>
          <select
            id="scan-condition"
            v-model="condition"
            class="mt-1 min-h-11 w-full rounded-md border border-slate-300 bg-white px-3 text-base"
          >
            <option value="GOOD">Baik</option>
            <option value="MINOR_DEFECT">Cacat ringan</option>
            <option value="BROKEN">Rusak (hanya untuk rak non-jual)</option>
          </select>
        </div>
      </div>

      <p id="scan-hint" class="text-sm text-slate-600">
        <template v-if="target">
          Sisa untuk item ini: <strong>{{ remaining(target) }}</strong> dari {{ target.qty }} unit.
        </template>
      </p>

      <button type="submit" :class="btnPrimary">
        <i class="pi pi-plus text-xs" aria-hidden="true" />Tambah Serial Number
      </button>
    </form>

    <!-- Hasil terakhir: ikon + teks (bukan hanya warna). role=log → dibacakan pembaca layar. -->
    <ul
      v-if="queue.entries.value.length > 0"
      role="log"
      aria-label="Hasil scan terakhir"
      class="mt-4 divide-y divide-slate-100 rounded-lg border border-slate-200"
    >
      <li
        v-for="e in queue.entries.value"
        :key="e.key"
        class="flex items-start gap-2 px-3 py-2 text-sm"
        :class="STATUS_CLASS[e.status]"
      >
        <i :class="STATUS_ICON[e.status]" class="mt-0.5" aria-hidden="true" />
        <span class="min-w-0 break-words">
          <span class="font-medium">{{ STATUS_TEXT[e.status] }}:</span> {{ e.message }}
        </span>
      </li>
    </ul>
  </section>
</template>
