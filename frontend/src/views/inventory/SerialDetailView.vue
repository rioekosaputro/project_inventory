<script setup lang="ts">
// Detail unit SN + timeline riwayat perubahan status/lokasi (plan Bagian 9.4: /stok/serial/:id).
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import BackLink from '@/components/BackLink.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { toApiError, userMessage } from '@/lib/apiError'
import { formatDateTime } from '@/lib/format'
import { openPdfBlob } from '@/lib/pdf'
import { btnSecondary } from '@/lib/ui'
import { inventoryService } from '@/services/inventoryService'
import type { SerialDetail } from '@/types/inventory'

const route = useRoute()
const toast = useToast()

const id = computed(() => Number(route.params.id))
const unit = ref<SerialDetail | null>(null)
const loading = ref(true)
const loadError = ref('')
const printing = ref(false)

async function load(): Promise<void> {
  loading.value = true
  loadError.value = ''
  try {
    unit.value = await inventoryService.serialHistory(id.value)
    document.title = `${unit.value.asset_tag} — Riwayat Unit`
  } catch (e) {
    loadError.value = userMessage(toApiError(e))
  } finally {
    loading.value = false
  }
}

onMounted(load)
watch(id, (next, prev) => {
  if (Number.isInteger(next) && next > 0 && next !== prev) void load()
})

async function printLabel(): Promise<void> {
  if (!unit.value || printing.value) return
  printing.value = true
  try {
    const blob = await inventoryService.fetchSerialLabelsPdf([unit.value.id])
    openPdfBlob(blob, `label-${unit.value.asset_tag}.pdf`)
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: 'Gagal mencetak label aset',
      detail: userMessage(toApiError(e)),
      life: 7000,
    })
  } finally {
    printing.value = false
  }
}
</script>

<template>
  <div class="w-full">
    <BackLink to="/stok/serial" label="Kembali ke daftar Serial Number" />

    <div v-if="loading" class="mt-6 py-12 text-center text-sm text-slate-600" role="status">
      Memuat riwayat unit…
    </div>
    <div
      v-else-if="loadError || !unit"
      class="mt-6 rounded-xl border border-slate-200 bg-white p-8 text-center"
      role="alert"
    >
      <h1 class="text-base font-semibold text-slate-900">Unit tidak dapat ditampilkan</h1>
      <p class="mt-1 text-sm text-slate-600">{{ loadError }}</p>
      <button type="button" :class="[btnSecondary, 'mt-4']" @click="load">Coba lagi</button>
    </div>

    <template v-else>
      <header class="mt-3 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 class="flex flex-wrap items-center gap-3 font-mono text-xl font-semibold text-slate-900">
            {{ unit.asset_tag }} <StatusBadge :value="unit.status" />
          </h1>
          <p class="mt-1 text-sm text-slate-600">
            SN: <strong class="font-mono">{{ unit.serial_number }}</strong> · {{ unit.sku }} —
            {{ unit.product_name }}
          </p>
        </div>
        <button type="button" :class="btnSecondary" :aria-busy="printing" @click="printLabel">
          <i class="pi pi-qrcode text-xs" aria-hidden="true" />
          {{ printing ? 'Menyiapkan…' : 'Cetak Label Aset' }}
        </button>
      </header>

      <dl
        class="mt-4 grid gap-4 rounded-xl border border-slate-200 bg-white p-5 sm:grid-cols-3"
      >
        <div>
          <dt class="text-xs font-medium text-slate-500">Lokasi Saat Ini</dt>
          <dd class="mt-1 font-mono text-sm font-semibold text-slate-900">
            {{ unit.rack_code ? `${unit.warehouse_code} / ${unit.rack_code}` : 'Di luar gudang' }}
          </dd>
        </div>
        <div>
          <dt class="text-xs font-medium text-slate-500">Kondisi</dt>
          <dd class="mt-1 text-sm font-medium text-slate-900">{{ unit.condition }}</dd>
        </div>
        <div>
          <dt class="text-xs font-medium text-slate-500">Terakhir Diperbarui</dt>
          <dd class="mt-1 text-sm text-slate-900">{{ formatDateTime(unit.updated_at) }}</dd>
        </div>
      </dl>

      <section class="mt-6">
        <h2 class="text-base font-semibold text-slate-900">Riwayat Perubahan Status &amp; Lokasi</h2>
        <div
          v-if="unit.history.length === 0"
          class="mt-3 rounded-xl border border-slate-200 bg-white p-6 text-center text-sm text-slate-600"
        >
          Belum ada catatan riwayat.
        </div>
        <ol
          v-else
          class="mt-3 divide-y divide-slate-100 rounded-xl border border-slate-200 bg-white"
        >
          <li
            v-for="h in unit.history"
            :key="h.id"
            class="flex flex-wrap items-start justify-between gap-2 px-4 py-3 text-sm"
          >
            <div>
              <div class="flex flex-wrap items-center gap-2">
                <span class="font-medium text-slate-900">{{ h.event_type }}</span>
                <template v-if="h.from_status">
                  <StatusBadge :value="h.from_status" />
                  <span class="text-slate-400">→</span>
                </template>
                <StatusBadge :value="h.to_status" />
              </div>
              <div class="mt-1 text-xs text-slate-600">
                <span v-if="h.from_rack_code || h.to_rack_code" class="font-mono">
                  Rak: {{ h.from_rack_code ?? '—' }} → {{ h.to_rack_code ?? '—' }} ·
                </span>
                {{ h.user_name ?? 'Sistem' }}
                <template v-if="h.notes"> · {{ h.notes }}</template>
              </div>
            </div>
            <div class="text-xs text-slate-500">{{ formatDateTime(h.created_at) }}</div>
          </li>
        </ol>
      </section>
    </template>
  </div>
</template>
