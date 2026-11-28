<script setup lang="ts">
// Detail Stock Opname — mendukung mode HP/tablet untuk input hitungan non-SN & scan barcode SN.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Message from 'primevue/message'
import { useToast } from 'primevue/usetoast'
import BackLink from '@/components/BackLink.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { usePermission } from '@/composables/usePermission'
import { toApiError, userMessage } from '@/lib/apiError'
import { beep } from '@/lib/audio'
import { formatDateTime } from '@/lib/format'
import { btnDanger, btnPrimary, btnSecondary } from '@/lib/ui'
import { opnameService } from '@/services/opnameService'
import type { OpnameDetail, OpnameSerial } from '@/types/opname'

const route = useRoute()
const toast = useToast()
const { can } = usePermission()

const id = computed(() => Number(route.params.id))
const opname = ref<OpnameDetail | null>(null)
const loading = ref(true)
const loadError = ref('')
const activeTab = ref<'counts' | 'serials'>('counts')

// Draft hitungan non-SN: key = `${product_id}:${rack_id}`
const draftCounts = ref<Record<string, string>>({})
const savingCounts = ref(false)

// Scanner SN
const scanRackId = ref<number | ''>('')
const scanCode = ref('')
const scanning = ref(false)
const lastScan = ref<OpnameSerial | null>(null)
const scanInput = ref<HTMLInputElement | null>(null)

// Konfirmasi aksi
const busy = ref(false)
const confirmFinalize = ref(false)
const confirmCancel = ref(false)

const isInProgress = computed(() => opname.value?.status === 'IN_PROGRESS')
const canCount = computed(() => isInProgress.value && can('opname.create'))
const canFinalize = computed(() => opname.value?.status === 'SUBMITTED' && can('opname.finalize'))
const canCancel = computed(
  () =>
    (opname.value?.status === 'IN_PROGRESS' || opname.value?.status === 'SUBMITTED') &&
    can('opname.finalize'),
)

const serialStats = computed(() => {
  const list = opname.value?.serials ?? []
  return {
    total: list.length,
    found: list.filter((s) => s.result === 'FOUND').length,
    missing: list.filter((s) => s.result === 'MISSING').length,
    wrongRack: list.filter((s) => s.result === 'WRONG_RACK').length,
    unexpected: list.filter((s) => s.result === 'UNEXPECTED').length,
  }
})

function syncDrafts(d: OpnameDetail): void {
  const next: Record<string, string> = {}
  for (const it of d.items) {
    next[`${it.product_id}:${it.rack_id}`] = it.counted_qty !== null ? String(it.counted_qty) : ''
  }
  draftCounts.value = next
  if (!scanRackId.value && d.racks.length > 0) {
    scanRackId.value = d.racks[0]!.rack_id
  }
}

async function load(): Promise<void> {
  loading.value = true
  loadError.value = ''
  try {
    const d = await opnameService.get(id.value)
    opname.value = d
    syncDrafts(d)
    document.title = `${d.opname_number} — Stock Opname`
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

async function saveCounts(): Promise<void> {
  if (!opname.value || savingCounts.value) return
  const payload = opname.value.items
    .map((it) => {
      const raw = (draftCounts.value[`${it.product_id}:${it.rack_id}`] ?? '').trim()
      if (raw === '') return null
      const qty = Number(raw)
      if (!Number.isInteger(qty) || qty < 0) return null
      return { product_id: it.product_id, rack_id: it.rack_id, counted_qty: qty, notes: it.notes }
    })
    .filter((x): x is NonNullable<typeof x> => x !== null)

  if (payload.length === 0) {
    toast.add({ severity: 'warn', summary: 'Isi minimal satu jumlah hitungan', life: 4000 })
    return
  }
  savingCounts.value = true
  try {
    const d = await opnameService.saveCounts(opname.value.id, payload)
    opname.value = d
    syncDrafts(d)
    toast.add({ severity: 'success', summary: 'Hitungan non-SN tersimpan', life: 3000 })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: 'Gagal menyimpan hitungan',
      detail: userMessage(toApiError(e)),
      life: 7000,
    })
  } finally {
    savingCounts.value = false
  }
}

async function submitScan(): Promise<void> {
  if (!opname.value || !scanRackId.value || scanning.value) return
  const code = scanCode.value.trim()
  if (!code) return
  scanning.value = true
  try {
    const res = await opnameService.scanSerial(opname.value.id, Number(scanRackId.value), code)
    lastScan.value = res
    scanCode.value = ''
    beep(res.result === 'FOUND' ? 'success' : 'error')
    // Muat ulang daftar serial agar progres langsung akurat.
    opname.value = await opnameService.get(opname.value.id)
  } catch (e) {
    beep('error')
    toast.add({
      severity: 'error',
      summary: 'Scan gagal',
      detail: userMessage(toApiError(e)),
      life: 6000,
    })
  } finally {
    scanning.value = false
    scanInput.value?.focus()
  }
}

async function runAction(fn: () => Promise<OpnameDetail>, msg: string): Promise<boolean> {
  if (busy.value) return false
  busy.value = true
  try {
    const d = await fn()
    opname.value = d
    syncDrafts(d)
    toast.add({ severity: 'success', summary: msg, life: 4000 })
    return true
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: 'Aksi gagal',
      detail: userMessage(toApiError(e)),
      life: 9000,
    })
    return false
  } finally {
    busy.value = false
  }
}

const submitOpname = () =>
  runAction(() => opnameService.submit(id.value), 'Stock opname diajukan untuk finalisasi')

async function finalizeOpname(): Promise<void> {
  const ok = await runAction(
    () => opnameService.finalize(id.value),
    'Stock opname difinalisasi; stok disesuaikan dan rak dibuka',
  )
  if (ok) confirmFinalize.value = false
}

async function cancelOpname(): Promise<void> {
  const ok = await runAction(
    () => opnameService.cancel(id.value),
    'Stock opname dibatalkan; kunci rak dibuka',
  )
  if (ok) confirmCancel.value = false
}
</script>

<template>
  <div class="w-full">
    <BackLink to="/opname" label="Kembali ke daftar Stock Opname" />

    <div v-if="loading" class="mt-6 py-12 text-center text-sm text-slate-600" role="status">
      Memuat data stock opname…
    </div>
    <div
      v-else-if="loadError || !opname"
      class="mt-6 rounded-xl border border-slate-200 bg-white p-8 text-center"
      role="alert"
    >
      <h1 class="text-base font-semibold text-slate-900">Opname tidak dapat ditampilkan</h1>
      <p class="mt-1 text-sm text-slate-600">{{ loadError }}</p>
      <button type="button" :class="[btnSecondary, 'mt-4']" @click="load">Coba lagi</button>
    </div>

    <template v-else>
      <header class="mt-3 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 class="flex flex-wrap items-center gap-3 text-xl font-semibold text-slate-900">
            {{ opname.opname_number }} <StatusBadge :value="opname.status" />
          </h1>
          <p class="mt-1 text-sm text-slate-600">
            Gudang {{ opname.warehouse_code }} — {{ opname.warehouse_name }} · Dimulai
            {{ formatDateTime(opname.started_at) }} oleh {{ opname.started_by_name }}
          </p>
          <div class="mt-2 flex flex-wrap gap-1.5">
            <span
              v-for="rk in opname.racks"
              :key="rk.rack_id"
              class="rounded-md border border-slate-200 bg-slate-100 px-2 py-0.5 font-mono text-xs font-medium text-slate-700"
            >
              {{ rk.rack_code }}
            </span>
          </div>
        </div>

        <div class="flex flex-wrap items-center gap-2">
          <button
            v-if="canCount"
            type="button"
            :class="btnPrimary"
            :aria-busy="busy"
            @click="submitOpname"
          >
            Ajukan Opname
          </button>
          <button
            v-if="canFinalize"
            type="button"
            :class="btnPrimary"
            :aria-busy="busy"
            @click="confirmFinalize = true"
          >
            Finalisasi &amp; Sesuaikan Stok
          </button>
          <button v-if="canCancel" type="button" :class="btnDanger" @click="confirmCancel = true">
            Batalkan Opname
          </button>
        </div>
      </header>

      <Message
        v-if="opname.adjustment_in_lpb_id || opname.adjustment_out_lpb_id"
        severity="success"
        :closable="false"
        class="mt-4"
      >
        LPB penyesuaian otomatis:
        <RouterLink
          v-if="opname.adjustment_in_lpb_id"
          :to="`/lpb/${opname.adjustment_in_lpb_id}`"
          class="ml-1 font-mono font-semibold underline"
        >
          {{ opname.adjustment_in_lpb_number }}
        </RouterLink>
        <RouterLink
          v-if="opname.adjustment_out_lpb_id"
          :to="`/lpb/${opname.adjustment_out_lpb_id}`"
          class="ml-2 font-mono font-semibold underline"
        >
          {{ opname.adjustment_out_lpb_number }}
        </RouterLink>
      </Message>

      <!-- Tabs -->
      <div class="mt-5 flex gap-2 border-b border-slate-200">
        <button
          type="button"
          class="min-h-[44px] border-b-2 px-4 text-sm font-medium"
          :class="
            activeTab === 'counts'
              ? 'border-indigo-600 text-indigo-700'
              : 'border-transparent text-slate-600 hover:text-slate-900'
          "
          @click="activeTab = 'counts'"
        >
          Hitung Non-SN ({{ opname.items.length }})
        </button>
        <button
          type="button"
          class="min-h-[44px] border-b-2 px-4 text-sm font-medium"
          :class="
            activeTab === 'serials'
              ? 'border-indigo-600 text-indigo-700'
              : 'border-transparent text-slate-600 hover:text-slate-900'
          "
          @click="activeTab = 'serials'"
        >
          Scan Serial Number ({{ serialStats.found }}/{{ serialStats.total }})
        </button>
      </div>

      <!-- Tab Non-SN -->
      <section v-if="activeTab === 'counts'" class="mt-4">
        <div
          v-if="opname.items.length === 0"
          class="rounded-xl border border-slate-200 bg-white p-6 text-center text-sm text-slate-600"
        >
          Tidak ada saldo barang non-SN pada rak terpilih.
        </div>
        <div v-else class="space-y-3">
          <div
            v-for="it in opname.items"
            :key="it.id"
            class="flex flex-col gap-3 rounded-xl border border-slate-200 bg-white p-4 sm:flex-row sm:items-center sm:justify-between"
          >
            <div>
              <div class="font-medium text-slate-900">
                <span class="font-mono text-xs text-slate-500">{{ it.sku }}</span> ·
                {{ it.product_name }}
              </div>
              <div class="mt-0.5 text-xs text-slate-600">
                Rak <strong class="font-mono">{{ it.rack_code }}</strong> · Satuan {{ it.unit }}
                <template v-if="it.system_qty !== null">
                  · Sistem: <strong>{{ it.system_qty }}</strong>
                  <template v-if="it.diff_qty !== null">
                    · Selisih:
                    <strong
                      :class="
                        it.diff_qty === 0
                          ? 'text-emerald-700'
                          : it.diff_qty > 0
                            ? 'text-sky-700'
                            : 'text-rose-700'
                      "
                    >
                      {{ it.diff_qty > 0 ? `+${it.diff_qty}` : it.diff_qty }}
                    </strong>
                  </template>
                </template>
                <template v-else> · <em>Blind count</em></template>
              </div>
            </div>

            <div class="flex items-center gap-2">
              <label :for="`cnt-${it.id}`" class="text-xs font-medium text-slate-600">Fisik:</label>
              <input
                v-if="canCount"
                :id="`cnt-${it.id}`"
                v-model="draftCounts[`${it.product_id}:${it.rack_id}`]"
                type="number"
                min="0"
                inputmode="numeric"
                class="min-h-[44px] w-28 rounded-lg border border-slate-300 px-3 text-right text-base font-semibold"
                placeholder="0"
              />
              <span v-else class="font-mono text-base font-semibold text-slate-900">
                {{ it.counted_qty ?? '—' }}
              </span>
            </div>
          </div>

          <div v-if="canCount" class="flex justify-end pt-2">
            <button type="button" :class="btnPrimary" :aria-busy="savingCounts" @click="saveCounts">
              {{ savingCounts ? 'Menyimpan…' : 'Simpan Hitungan' }}
            </button>
          </div>
        </div>
      </section>

      <!-- Tab Serials -->
      <section v-else class="mt-4 space-y-4">
        <form
          v-if="canCount"
          class="flex flex-col gap-3 rounded-xl border border-slate-200 bg-white p-4 sm:flex-row sm:items-end"
          @submit.prevent="submitScan"
        >
          <div class="sm:w-44">
            <label for="scan-rack" class="block text-xs font-medium text-slate-600">Rak</label>
            <select
              id="scan-rack"
              v-model="scanRackId"
              class="mt-1 min-h-[44px] w-full rounded-lg border border-slate-300 bg-white px-3 text-sm"
            >
              <option v-for="rk in opname.racks" :key="rk.rack_id" :value="rk.rack_id">
                {{ rk.rack_code }}
              </option>
            </select>
          </div>
          <div class="flex-1">
            <label for="scan-code" class="block text-xs font-medium text-slate-600">
              Scan Asset Tag / Serial Number (Tekan Enter)
            </label>
            <input
              id="scan-code"
              ref="scanInput"
              v-model="scanCode"
              type="text"
              autocomplete="off"
              class="mt-1 min-h-[44px] w-full rounded-lg border border-slate-300 px-3 font-mono text-sm"
              placeholder="AST-2611-00001 atau SN…"
            />
          </div>
          <button type="submit" :class="btnPrimary" :aria-busy="scanning">Catat Scan</button>
        </form>

        <div
          v-if="lastScan"
          class="flex items-center justify-between rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm"
          role="status"
        >
          <span>
            Scan terakhir:
            <strong class="font-mono">{{
              lastScan.asset_tag ?? lastScan.scanned_code ?? lastScan.serial_number
            }}</strong>
            di rak <strong class="font-mono">{{ lastScan.rack_code }}</strong>
          </span>
          <StatusBadge :value="lastScan.result" />
        </div>

        <div class="grid grid-cols-2 gap-2 sm:grid-cols-4">
          <div class="rounded-lg border border-slate-200 bg-white p-3 text-center">
            <div class="text-xs text-slate-500">Ditemukan</div>
            <div class="text-lg font-bold text-emerald-700">{{ serialStats.found }}</div>
          </div>
          <div class="rounded-lg border border-slate-200 bg-white p-3 text-center">
            <div class="text-xs text-slate-500">Belum Ditemukan</div>
            <div class="text-lg font-bold text-rose-700">{{ serialStats.missing }}</div>
          </div>
          <div class="rounded-lg border border-slate-200 bg-white p-3 text-center">
            <div class="text-xs text-slate-500">Salah Rak</div>
            <div class="text-lg font-bold text-indigo-700">{{ serialStats.wrongRack }}</div>
          </div>
          <div class="rounded-lg border border-slate-200 bg-white p-3 text-center">
            <div class="text-xs text-slate-500">Tak Terduga</div>
            <div class="text-lg font-bold text-amber-700">{{ serialStats.unexpected }}</div>
          </div>
        </div>

        <div
          v-if="opname.serials.length === 0"
          class="rounded-xl border border-slate-200 bg-white p-6 text-center text-sm text-slate-600"
        >
          Belum ada unit berserial pada rak terpilih.
        </div>
        <div v-else class="divide-y divide-slate-100 rounded-xl border border-slate-200 bg-white">
          <div
            v-for="s in opname.serials"
            :key="s.id"
            class="flex flex-wrap items-center justify-between gap-2 px-4 py-3 text-sm"
          >
            <div>
              <div class="font-mono font-semibold text-slate-900">
                {{ s.asset_tag ?? s.scanned_code ?? '—' }}
                <span v-if="s.serial_number" class="font-normal text-slate-500">
                  ({{ s.serial_number }})
                </span>
              </div>
              <div class="text-xs text-slate-600">
                Rak <strong class="font-mono">{{ s.rack_code }}</strong>
                <template v-if="s.product_name"> · {{ s.sku }} — {{ s.product_name }}</template>
              </div>
            </div>
            <StatusBadge :value="s.result" />
          </div>
        </div>
      </section>
    </template>

    <ConfirmDialog
      :open="confirmFinalize"
      :title="`Finalisasi Stock Opname ${opname?.opname_number ?? ''}?`"
      message="Sistem akan membuat & menyetujui LPB penyesuaian secara otomatis untuk semua selisih stok dan unit hilang/ditemukan, lalu membuka kunci rak. Tindakan ini tidak dapat diurungkan."
      confirm-label="Finalisasi Opname"
      tone="primary"
      :busy="busy"
      @confirm="finalizeOpname"
      @cancel="confirmFinalize = false"
    />

    <ConfirmDialog
      :open="confirmCancel"
      :title="`Batalkan Stock Opname ${opname?.opname_number ?? ''}?`"
      message="Kunci rak akan segera dibuka kembali dan tidak ada perubahan stok yang dilakukan."
      confirm-label="Batalkan Opname"
      :busy="busy"
      @confirm="cancelOpname"
      @cancel="confirmCancel = false"
    />
  </div>
</template>
