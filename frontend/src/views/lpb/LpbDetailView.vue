<script setup lang="ts">
// Pola halaman: Detail / Record. Satu dokumen LPB: ringkasan, item + dampak stok, scan serial number
// (selama draft), keputusan persetujuan, dan riwayat.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Message from 'primevue/message'
import { useToast } from 'primevue/usetoast'
import BackLink from '@/components/BackLink.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import DataTable, { type Column } from '@/components/DataTable.vue'
import SerialScannerPOS from '@/components/SerialScannerPOS.vue'
import SlideOver from '@/components/SlideOver.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import StockImpactCell from '@/components/StockImpactCell.vue'
import { usePermission } from '@/composables/usePermission'
import { toApiError, userMessage } from '@/lib/apiError'
import { focusFirstInvalid, isProgrammaticFocus } from '@/lib/focus'
import { formatDate, formatDateTime, formatRupiah } from '@/lib/format'
import { eventLabel, remainingScans, subTypeLabel } from '@/lib/lpb'
import { openPdfBlob } from '@/lib/pdf'
import { btnDanger, btnPrimary, btnSecondary } from '@/lib/ui'
import { lpbService } from '@/services/lpbService'
import { useAuthStore } from '@/stores/auth'
import type { AddSerialResult, LPBDetail, LPBItem } from '@/types/lpb'

const route = useRoute()
const toast = useToast()
const auth = useAuthStore()
const { can } = usePermission()

const id = computed(() => Number(route.params.id))
const lpb = ref<LPBDetail | null>(null)
const loading = ref(true)
const loadError = ref('')

async function load(): Promise<void> {
  loading.value = true
  loadError.value = ''
  try {
    lpb.value = await lpbService.get(id.value)
    document.title = `${lpb.value.lpb_number} — Rental ERP-Lite`
  } catch (e) {
    loadError.value = toApiError(e).message
  } finally {
    loading.value = false
  }
}
onMounted(load)
// Pindah ke LPB lain (mis. tautan pasangan transfer) memakai komponen yang sama: muat ulang saat id berubah.
watch(id, (next, prev) => {
  if (Number.isInteger(next) && next > 0 && next !== prev) void load()
})

// ---- hak aksi (kenyamanan UI; otorisasi sebenarnya di backend)
const status = computed(() => lpb.value?.status)
/** Draft milik pembuatnya; Kepala Gudang dan ADMIN boleh membantu (sama dengan aturan backend). */
const canModify = computed(() => {
  const d = lpb.value
  const u = auth.user
  if (!d || !u || !can('lpb.create')) return false
  return u.id === d.created_by || u.role === 'ADMIN' || u.role === 'HEAD_WAREHOUSE'
})
const canEdit = computed(() => canModify.value && status.value === 'DRAFT')
const canSubmit = computed(() => canEdit.value && can('lpb.submit'))
const canRevise = computed(() => canModify.value && status.value === 'REJECTED')
const canDecide = computed(() => can('lpb.approve') && status.value === 'PENDING_APPROVAL')
/** Transfer masuk tidak dapat ditolak: barang sudah keluar dari gudang asal (aturan backend). */
const canReject = computed(() => canDecide.value && lpb.value?.sub_type !== 'TRANSFER_IN')
const canPrint = computed(() => can('lpb.print') && status.value === 'APPROVED')
const registeredSerialIds = computed(() =>
  (lpb.value?.items ?? [])
    .flatMap((i) => i.serials)
    .map((s) => s.product_serial_id)
    .filter((sid): sid is number => sid !== null),
)
const canPrintAssetLabels = computed(
  () =>
    can('inventory.read') && status.value === 'APPROVED' && registeredSerialIds.value.length > 0,
)
const canDeliverPOD = computed(
  () =>
    can('lpb.approve') &&
    status.value === 'APPROVED' &&
    (lpb.value?.sub_type === 'RENTAL_DELIVERY' || lpb.value?.sub_type === 'CONSUMABLE_DELIVERY') &&
    lpb.value?.delivery_status !== 'DELIVERED',
)
const canPickupReturn = computed(
  () =>
    can('lpb.approve') &&
    lpb.value?.sub_type === 'RENTAL_RETURN' &&
    (status.value === 'DRAFT' || status.value === 'PENDING_APPROVAL') &&
    lpb.value?.delivery_status !== 'PICKED_UP',
)

// ---- POD manual & pickup penarikan (T2.7)
const podOpen = ref(false)
const podBusy = ref(false)
const podError = ref('')
const podReceiver = ref('')
const podDate = ref('')
const podNotes = ref('')
const podFile = ref<File | null>(null)
const pickupConfirmOpen = ref(false)

function openPod(): void {
  podError.value = ''
  podReceiver.value = ''
  podDate.value = ''
  podNotes.value = ''
  podFile.value = null
  podOpen.value = true
}

function onPodFileChange(ev: Event): void {
  const input = ev.target as HTMLInputElement
  podFile.value = input.files?.[0] ?? null
}

async function submitPod(): Promise<void> {
  if (!lpb.value) return
  podError.value = ''
  if (podReceiver.value.trim().length < 2) {
    podError.value = 'Nama penerima wajib diisi minimal 2 karakter.'
    return
  }
  podBusy.value = true
  try {
    lpb.value = await lpbService.deliver(lpb.value.id, {
      receiverName: podReceiver.value.trim(),
      deliveredDate: podDate.value || undefined,
      notes: podNotes.value.trim() || undefined,
      photo: podFile.value,
    })
    podOpen.value = false
    toast.add({
      severity: 'success',
      summary: 'Berhasil',
      detail: `Pengiriman ${lpb.value.lpb_number} ditandai DELIVERED (POD)`,
      life: 3500,
    })
  } catch (e) {
    podError.value = userMessage(toApiError(e))
  } finally {
    podBusy.value = false
  }
}

async function confirmPickup(): Promise<void> {
  if (!lpb.value) return
  podBusy.value = true
  try {
    lpb.value = await lpbService.pickup(lpb.value.id)
    pickupConfirmOpen.value = false
    toast.add({
      severity: 'success',
      summary: 'Berhasil',
      detail: `Unit penarikan ${lpb.value.lpb_number} ditandai PICKED_UP`,
      life: 3500,
    })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: 'Gagal',
      detail: userMessage(toApiError(e)),
      life: 5000,
    })
  } finally {
    podBusy.value = false
  }
}

const approveMessage = computed(() => {
  switch (lpb.value?.sub_type) {
    case 'TRANSFER_OUT':
      return 'Stok rak asal berkurang dan unit berstatus dalam perjalanan. LPB masuk pasangannya dibuat otomatis: bila tujuannya satu gudang langsung disetujui, bila antar gudang menunggu Kepala Gudang tujuan. Tindakan ini tidak dapat diurungkan — koreksi dilakukan dengan dokumen baru.'
    case 'TRANSFER_IN':
      return 'Stok rak tujuan bertambah dan unit kembali tersedia di gudang ini, dicatat di buku besar. Pastikan barang sudah tiba. Tindakan ini tidak dapat diurungkan — koreksi dilakukan dengan dokumen baru.'
    case 'DISPOSAL':
      return 'Stok rak berkurang dan unit yang dihapus berstatus pensiun (tidak lagi ada di gudang), dicatat di buku besar. Tindakan ini tidak dapat diurungkan — koreksi dilakukan dengan dokumen baru.'
    default:
      return 'Stok rak akan berubah sesuai tabel dampak stok, unit baru dibuatkan asset tag, dan semuanya dicatat di buku besar. Tindakan ini tidak dapat diurungkan — koreksi dilakukan dengan dokumen baru.'
  }
})

const serialItems = computed(() => lpb.value?.items.filter((i) => i.has_serial_number) ?? [])
const missingScans = computed(() => serialItems.value.reduce((n, i) => n + remainingScans(i), 0))

// ---- tabel item
const columns = computed<Column[]>(() => {
  const cols: Column[] = [
    { key: 'product', label: 'Produk' },
    { key: 'rack_code', label: 'Rak' },
    { key: 'qty', label: 'Jumlah' },
  ]
  if (lpb.value?.items.some((i) => i.unit_cost !== null)) {
    cols.push({ key: 'unit_cost', label: 'Biaya satuan' })
  }
  cols.push({ key: 'scan', label: 'Serial number' })
  if (status.value !== 'CANCELLED') {
    cols.push({
      key: 'impact',
      label:
        status.value === 'APPROVED'
          ? 'Saldo rak (sebelum → sesudah)'
          : 'Prakiraan saldo rak (sekarang → setelah disetujui)',
    })
  }
  return cols
})

// ---- aksi
const busy = ref(false)
const printing = ref(false)
const printingLabels = ref(false)

async function printPdf(): Promise<void> {
  if (printing.value || !lpb.value) return
  printing.value = true
  try {
    const blob = await lpbService.fetchPdf(lpb.value.id)
    await openPdfBlob(blob, `${lpb.value.lpb_number.replace(/\//g, '-')}.pdf`)
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: 'Gagal mencetak Surat Jalan',
      detail: userMessage(toApiError(e)),
      life: 9000,
    })
  } finally {
    printing.value = false
  }
}

async function printAssetLabels(): Promise<void> {
  if (printingLabels.value || registeredSerialIds.value.length === 0) return
  printingLabels.value = true
  try {
    const blob = await lpbService.fetchSerialLabelsPdf(registeredSerialIds.value)
    await openPdfBlob(
      blob,
      `label-aset-${lpb.value?.lpb_number.replace(/\//g, '-') ?? 'batch'}.pdf`,
    )
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: 'Gagal mencetak label aset',
      detail: userMessage(toApiError(e)),
      life: 9000,
    })
  } finally {
    printingLabels.value = false
  }
}

async function run(action: () => Promise<LPBDetail>, success: string): Promise<boolean> {
  if (busy.value) return false
  busy.value = true
  try {
    lpb.value = await action()
    toast.add({ severity: 'success', summary: success, life: 4000 })
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

const submit = () =>
  run(() => lpbService.submit(id.value), `LPB ${lpb.value?.lpb_number} diajukan untuk persetujuan`)
const revise = () =>
  run(() => lpbService.revise(id.value), 'LPB dibuka kembali sebagai draft untuk direvisi')

// Batalkan draft (aksi yang tidak dapat diurungkan → konfirmasi yang menyebut dokumennya).
const confirmCancel = ref(false)
async function cancel(): Promise<void> {
  const ok = await run(() => lpbService.cancel(id.value), `LPB ${lpb.value?.lpb_number} dibatalkan`)
  if (ok) confirmCancel.value = false
}

// Setujui: kunci idempotensi dibuat SEKALI per dialog dan dipakai ulang bila pengguna mengulang
// setelah galat jaringan, sehingga stok tidak pernah bertambah dua kali.
const confirmApprove = ref(false)
let approveKey = ''
function askApprove(): void {
  approveKey = crypto.randomUUID()
  confirmApprove.value = true
}
async function approve(): Promise<void> {
  const ok = await run(
    () => lpbService.approve(id.value, approveKey),
    `LPB ${lpb.value?.lpb_number} disetujui; stok diperbarui`,
  )
  if (ok) confirmApprove.value = false
}

// Tolak: satu isian (alasan) → panel samping.
const rejecting = ref(false)
const reason = ref('')
const reasonTouched = ref(false)
const reasonError = computed(() => {
  if (!reasonTouched.value) return ''
  const n = reason.value.trim().length
  if (n < 3)
    return 'Tuliskan alasan penolakan, minimal 3 karakter, agar pembuat tahu yang harus diperbaiki.'
  if (n > 500) return 'Alasan maksimal 500 karakter.'
  return ''
})
function openReject(): void {
  reason.value = ''
  reasonTouched.value = false
  rejecting.value = true
}
// Fokus pengguna menghapus galat seketika; fokus yang dipindahkan program (setelah submit gagal) tidak.
function onReasonFocus(): void {
  if (!isProgrammaticFocus()) reasonTouched.value = false
}
async function reject(): Promise<void> {
  reasonTouched.value = true
  if (reasonError.value) {
    await focusFirstInvalid(document.getElementById('reject-form'))
    return
  }
  const ok = await run(
    () => lpbService.reject(id.value, reason.value.trim()),
    `LPB ${lpb.value?.lpb_number} ditolak`,
  )
  if (ok) rejecting.value = false
}

// ---- scan serial number
function onSaved(itemId: number, result: AddSerialResult): void {
  const item = lpb.value?.items.find((i) => i.id === itemId)
  if (!item) return
  item.serials.push({
    id: result.id,
    product_serial_id: result.product_serial_id,
    serial_number: result.serial_number,
    asset_tag: result.asset_tag,
    condition: result.condition,
    scanned_by: result.scanned_by,
    scanned_at: result.scanned_at,
  })
  item.scanned_count = result.scanned_count
}

const removing = ref<number | null>(null)
async function removeSerial(item: LPBItem, serialId: number): Promise<void> {
  if (removing.value !== null) return
  removing.value = serialId
  try {
    await lpbService.removeSerial(id.value, item.id, serialId)
    item.serials = item.serials.filter((s) => s.id !== serialId)
    item.scanned_count = item.serials.length
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: 'Serial number gagal dihapus',
      detail: userMessage(toApiError(e)),
      life: 8000,
    })
  } finally {
    removing.value = null
  }
}

const CONDITION_LABELS = { GOOD: 'Baik', MINOR_DEFECT: 'Cacat ringan', BROKEN: 'Rusak' } as const

function reasonOf(data: Record<string, unknown> | null): string | null {
  return data && typeof data.reason === 'string' ? data.reason : null
}
</script>

<template>
  <section>
    <BackLink to="/lpb" label="Daftar LPB" />

    <div v-if="loading" class="mt-6 space-y-4" aria-busy="true">
      <span class="sr-only">Memuat LPB…</span>
      <div
        v-for="n in 4"
        :key="n"
        class="h-16 animate-pulse rounded-lg bg-slate-200"
        aria-hidden="true"
      />
    </div>

    <div
      v-else-if="loadError || !lpb"
      class="mt-6 rounded-xl border border-slate-200 bg-white px-6 py-10 text-center"
      role="alert"
    >
      <h1 class="text-base font-semibold text-slate-900">LPB tidak dapat ditampilkan</h1>
      <p class="mt-1 text-sm text-slate-700">{{ loadError }}</p>
      <button type="button" :class="[btnSecondary, 'mt-4']" @click="load">Coba lagi</button>
    </div>

    <template v-else>
      <header class="mt-3 flex flex-wrap items-start justify-between gap-3">
        <div class="min-w-0">
          <h1 class="flex flex-wrap items-center gap-3 text-xl font-semibold text-slate-900">
            {{ lpb.lpb_number }} <StatusBadge :value="lpb.status" />
          </h1>
          <p class="mt-1 text-sm text-slate-600">
            {{ subTypeLabel(lpb.sub_type) }} · Gudang {{ lpb.warehouse_code }} ·
            {{ formatDate(lpb.doc_date) }}
          </p>
        </div>

        <div class="flex flex-wrap items-center gap-2">
          <template v-if="canEdit">
            <RouterLink :to="`/lpb/${lpb.id}/ubah`" :class="btnSecondary">
              <i class="pi pi-pencil text-xs" aria-hidden="true" />Ubah Draft
            </RouterLink>
            <button type="button" :class="btnDanger" @click="confirmCancel = true">
              Batalkan Draft
            </button>
            <button
              v-if="canSubmit"
              type="button"
              :class="btnPrimary"
              :aria-busy="busy"
              @click="submit"
            >
              Ajukan untuk Persetujuan
            </button>
          </template>
          <button
            v-if="canRevise"
            type="button"
            :class="btnPrimary"
            :aria-busy="busy"
            @click="revise"
          >
            Revisi LPB
          </button>
          <template v-if="canDecide">
            <button v-if="canReject" type="button" :class="btnDanger" @click="openReject">
              Tolak LPB
            </button>
            <button type="button" :class="btnPrimary" @click="askApprove">Setujui LPB</button>
          </template>
          <button
            v-if="canPrint"
            type="button"
            :class="btnSecondary"
            :aria-busy="printing"
            @click="printPdf"
          >
            <i class="pi pi-print text-xs" aria-hidden="true" />
            {{ printing ? 'Menyiapkan PDF…' : 'Cetak Surat Jalan' }}
          </button>
          <button
            v-if="canPrintAssetLabels"
            type="button"
            :class="btnSecondary"
            :aria-busy="printingLabels"
            @click="printAssetLabels"
          >
            <i class="pi pi-qrcode text-xs" aria-hidden="true" />
            {{ printingLabels ? 'Menyiapkan Label…' : 'Cetak Label Aset' }}
          </button>
          <button v-if="canDeliverPOD" type="button" :class="btnPrimary" @click="openPod">
            <i class="pi pi-check-circle text-xs" aria-hidden="true" />
            Tandai Terkirim (POD)
          </button>
          <button
            v-if="canPickupReturn"
            type="button"
            :class="btnPrimary"
            @click="pickupConfirmOpen = true"
          >
            <i class="pi pi-truck text-xs" aria-hidden="true" />
            Tandai Dijemput (Pickup)
          </button>
        </div>
      </header>

      <Message
        v-if="lpb.delivery_status === 'DELIVERED'"
        severity="success"
        :closable="false"
        class="mt-4"
      >
        <strong>Terkirim (POD)</strong>
        <template v-if="lpb.delivered_at"> pada {{ formatDateTime(lpb.delivered_at) }}</template>
        · Penerima: <strong>{{ lpb.receiver_name ?? '—' }}</strong>
      </Message>
      <Message
        v-else-if="lpb.delivery_status === 'PICKED_UP'"
        severity="info"
        :closable="false"
        class="mt-4"
      >
        <strong>Unit sudah dijemput (PICKED_UP)</strong> dan dalam perjalanan kembali ke gudang.
        Lanjutkan ajukan &amp; setujui LPB saat barang tiba di rak.
      </Message>

      <Message
        v-if="lpb.rejection_reason && (lpb.status === 'REJECTED' || lpb.status === 'DRAFT')"
        severity="error"
        :closable="false"
        class="mt-4"
      >
        <strong>Ditolak {{ formatDateTime(lpb.rejected_at) }}.</strong>
        Alasan: {{ lpb.rejection_reason }}
        <template v-if="lpb.status === 'REJECTED'">
          Buka kembali dengan “Revisi LPB”, perbaiki, lalu ajukan ulang.</template
        >
      </Message>

      <Message
        v-if="lpb.sub_type === 'TRANSFER_IN' && lpb.status === 'PENDING_APPROVAL'"
        severity="info"
        :closable="false"
        class="mt-4"
      >
        Barang dari transfer keluar {{ lpb.counterpart_number }} sedang dalam perjalanan. Setujui
        setelah barang tiba dan diperiksa; transfer masuk tidak dapat ditolak karena stok asal sudah
        berkurang.
      </Message>

      <Message v-if="canEdit && missingScans > 0" severity="warn" :closable="false" class="mt-4">
        Serial number belum lengkap: masih kurang {{ missingScans }} unit. LPB baru dapat diajukan
        setelah jumlah scan sama dengan jumlah item.
      </Message>

      <!-- Ringkasan dokumen -->
      <dl
        class="mt-4 grid gap-x-6 gap-y-3 rounded-xl border border-slate-200 bg-white p-5 shadow-sm sm:grid-cols-2 lg:grid-cols-3"
      >
        <div>
          <dt class="text-xs font-medium text-slate-600">Supplier</dt>
          <dd class="mt-0.5 text-sm text-slate-900">{{ lpb.supplier_name ?? '—' }}</dd>
        </div>
        <div v-if="lpb.counterpart_id">
          <dt class="text-xs font-medium text-slate-600">
            {{ lpb.sub_type === 'TRANSFER_IN' ? 'Transfer keluar asal' : 'LPB masuk pasangan' }}
          </dt>
          <dd class="mt-0.5 text-sm">
            <RouterLink
              :to="`/lpb/${lpb.counterpart_id}`"
              class="inline-flex min-h-11 items-center font-medium text-indigo-800 hover:underline"
              >{{ lpb.counterpart_number }}</RouterLink
            >
          </dd>
        </div>
        <div>
          <dt class="text-xs font-medium text-slate-600">Nomor referensi</dt>
          <dd class="mt-0.5 text-sm text-slate-900">{{ lpb.reference_number ?? '—' }}</dd>
        </div>
        <div>
          <dt class="text-xs font-medium text-slate-600">Dibuat</dt>
          <dd class="mt-0.5 text-sm text-slate-900">
            {{ lpb.created_by_name }} · {{ formatDateTime(lpb.created_at) }}
          </dd>
        </div>
        <div>
          <dt class="text-xs font-medium text-slate-600">Diajukan</dt>
          <dd class="mt-0.5 text-sm text-slate-900">{{ formatDateTime(lpb.submitted_at) }}</dd>
        </div>
        <div v-if="lpb.status === 'APPROVED'">
          <dt class="text-xs font-medium text-slate-600">Disetujui</dt>
          <dd class="mt-0.5 text-sm text-slate-900">{{ formatDateTime(lpb.approved_at) }}</dd>
        </div>
        <div class="sm:col-span-2 lg:col-span-3">
          <dt class="text-xs font-medium text-slate-600">Catatan</dt>
          <dd class="mt-0.5 text-sm whitespace-pre-line text-slate-900">{{ lpb.notes ?? '—' }}</dd>
        </div>
      </dl>

      <!-- Scanner hanya saat draft yang boleh diubah pengguna ini -->
      <div v-if="canEdit && serialItems.length > 0" class="mt-6">
        <SerialScannerPOS
          :lpb-id="lpb.id"
          :sub-type="lpb.sub_type"
          :items="lpb.items"
          @saved="onSaved"
        />
      </div>

      <!-- Item + dampak stok -->
      <h2 class="mt-8 text-base font-semibold text-slate-900">
        {{ canDecide ? 'Periksa dampak stok sebelum memutuskan' : 'Item' }}
      </h2>
      <p v-if="canDecide" class="mt-1 text-sm text-slate-600">
        Saldo rak berubah sesuai kolom terakhir bila LPB disetujui. Angka itu prakiraan dari saldo
        saat ini; angka final dicatat saat persetujuan.
      </p>
      <div
        class="mt-3 hidden overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm md:block"
      >
        <DataTable :columns="columns" :rows="lpb.items" caption="Item LPB">
          <template #cell-rack_code="{ row }">
            {{ row.rack_code }}
            <span v-if="row.dest_rack_id" class="block text-sm whitespace-nowrap text-slate-700">
              <span aria-hidden="true">→</span><span class="sr-only">ke </span>
              {{ row.dest_warehouse_code }} / {{ row.dest_rack_code }}
            </span>
          </template>
          <template #cell-product="{ row }">
            <span class="font-medium">{{ row.sku }}</span>
            <span class="block text-sm text-slate-700">{{ row.product_name }}</span>
          </template>
          <template #cell-qty="{ row }">{{ row.qty }} {{ row.unit }}</template>
          <template #cell-unit_cost="{ row }">{{ formatRupiah(row.unit_cost) }}</template>
          <template #cell-scan="{ row }">
            <template v-if="row.has_serial_number">
              <span :class="remainingScans(row) > 0 ? 'text-amber-900' : 'text-green-800'">
                {{ row.serials.length }} dari {{ row.qty }} ter-scan
              </span>
              <span v-if="remainingScans(row) === 0" class="sr-only"> (lengkap)</span>
            </template>
            <template v-else>Tanpa serial</template>
          </template>
          <template #cell-impact="{ row }">
            <StockImpactCell :item="row" :type="lpb.type" :status="lpb.status" />
          </template>
        </DataTable>
      </div>

      <!-- Layar sempit: satu kartu per item agar saldo sebelum → sesudah tidak tersembunyi di luar tabel. -->
      <ul class="mt-3 space-y-3 md:hidden" aria-label="Item LPB">
        <li
          v-for="item in lpb.items"
          :key="item.id"
          class="rounded-xl border border-slate-200 bg-white p-4 shadow-sm"
        >
          <p class="text-sm font-semibold text-slate-900">
            {{ item.sku }}
            <span class="block font-normal text-slate-700">{{ item.product_name }}</span>
          </p>
          <dl class="mt-2 grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
            <div>
              <dt class="text-xs text-slate-600">Rak</dt>
              <dd>
                {{ item.rack_code }}
                <span v-if="item.dest_rack_id" class="block text-slate-700">
                  <span aria-hidden="true">→</span><span class="sr-only">ke </span>
                  {{ item.dest_warehouse_code }} / {{ item.dest_rack_code }}
                </span>
              </dd>
            </div>
            <div>
              <dt class="text-xs text-slate-600">Jumlah</dt>
              <dd>{{ item.qty }} {{ item.unit }}</dd>
            </div>
            <div v-if="item.unit_cost !== null">
              <dt class="text-xs text-slate-600">Biaya satuan</dt>
              <dd>{{ formatRupiah(item.unit_cost) }}</dd>
            </div>
            <div>
              <dt class="text-xs text-slate-600">Serial number</dt>
              <dd v-if="item.has_serial_number">
                <span :class="remainingScans(item) > 0 ? 'text-amber-900' : 'text-green-800'"
                  >{{ item.serials.length }} dari {{ item.qty }} ter-scan</span
                >
              </dd>
              <dd v-else>Tanpa serial</dd>
            </div>
            <div v-if="lpb.status !== 'CANCELLED'" class="col-span-2">
              <dt class="text-xs text-slate-600">
                {{
                  lpb.status === 'APPROVED'
                    ? 'Saldo rak (sebelum → sesudah)'
                    : 'Prakiraan saldo rak (sekarang → setelah disetujui)'
                }}
              </dt>
              <dd class="text-base">
                <StockImpactCell :item="item" :type="lpb.type" :status="lpb.status" />
              </dd>
            </div>
          </dl>
        </li>
      </ul>

      <!-- Serial number per item -->
      <template v-if="serialItems.some((i) => i.serials.length > 0)">
        <h2 class="mt-8 text-base font-semibold text-slate-900">Serial number</h2>
        <div class="mt-3 space-y-3">
          <div
            v-for="item in serialItems.filter((i) => i.serials.length > 0)"
            :key="item.id"
            class="rounded-xl border border-slate-200 bg-white p-4 shadow-sm"
          >
            <h3 class="text-sm font-semibold text-slate-900">
              {{ item.sku }} · {{ item.product_name }}
              <span class="font-normal text-slate-600"
                >— rak {{ item.rack_code }}, {{ item.serials.length }} dari
                {{ item.qty }} unit</span
              >
            </h3>
            <ul class="mt-2 divide-y divide-slate-100">
              <li
                v-for="s in item.serials"
                :key="s.id"
                class="flex flex-wrap items-center justify-between gap-2 py-1"
              >
                <span class="min-w-0 text-sm">
                  <span class="font-mono">{{ s.serial_number }}</span>
                  <span v-if="s.asset_tag" class="ms-2 font-mono text-slate-700">{{
                    s.asset_tag
                  }}</span>
                  <span class="ms-2 text-slate-600">{{ CONDITION_LABELS[s.condition] }}</span>
                </span>
                <button
                  v-if="canEdit"
                  type="button"
                  :class="btnSecondary"
                  :aria-busy="removing === s.id"
                  @click="removeSerial(item, s.id)"
                >
                  <i class="pi pi-times text-xs" aria-hidden="true" />Hapus<span class="sr-only">
                    serial number {{ s.serial_number }}</span
                  >
                </button>
              </li>
            </ul>
          </div>
        </div>
      </template>

      <!-- Riwayat dokumen -->
      <h2 class="mt-8 text-base font-semibold text-slate-900">Riwayat</h2>
      <ol
        class="mt-3 space-y-0 rounded-xl border border-slate-200 bg-white p-5 shadow-sm"
        aria-label="Riwayat LPB"
      >
        <li
          v-for="(e, i) in lpb.timeline"
          :key="i"
          class="relative border-s-2 border-slate-200 pb-4 ps-4 last:pb-0"
        >
          <span
            class="absolute -start-[5px] top-1.5 size-2 rounded-full bg-indigo-600"
            aria-hidden="true"
          />
          <p class="text-sm font-medium text-slate-900">{{ eventLabel(e.action) }}</p>
          <p class="text-sm text-slate-600">
            {{ e.user_name ?? 'Sistem' }} · {{ formatDateTime(e.created_at) }}
          </p>
          <p v-if="reasonOf(e.data)" class="mt-1 text-sm text-slate-800">
            Alasan: {{ reasonOf(e.data) }}
          </p>
        </li>
        <li v-if="lpb.timeline.length === 0" class="text-sm text-slate-600">Belum ada riwayat.</li>
      </ol>
    </template>
  </section>

  <ConfirmDialog
    :open="confirmCancel"
    :title="`Batalkan draft ${lpb?.lpb_number ?? ''}?`"
    message="Draft yang dibatalkan tidak dapat dibuka lagi dan nomornya tidak dipakai ulang. Stok tidak terpengaruh."
    confirm-label="Batalkan Draft"
    :busy="busy"
    @confirm="cancel"
    @cancel="confirmCancel = false"
  />

  <ConfirmDialog
    :open="confirmApprove"
    :title="`Setujui LPB ${lpb?.lpb_number ?? ''}?`"
    :message="approveMessage"
    confirm-label="Setujui LPB"
    tone="primary"
    :busy="busy"
    @confirm="approve"
    @cancel="confirmApprove = false"
  />

  <SlideOver
    :open="rejecting"
    :title="`Tolak LPB ${lpb?.lpb_number ?? ''}`"
    @close="rejecting = false"
  >
    <form id="reject-form" class="space-y-4" novalidate @submit.prevent="reject">
      <p class="text-sm text-slate-700">
        Stok tidak berubah. Pembuat dapat merevisi dokumen lalu mengajukannya kembali.
      </p>
      <div>
        <label for="reject-reason" class="block text-sm font-medium text-slate-800">
          Alasan penolakan <span class="text-red-700" aria-hidden="true">*</span>
        </label>
        <textarea
          id="reject-reason"
          v-model="reason"
          rows="4"
          required
          class="mt-1 w-full rounded-md border bg-white px-3 py-2 text-base"
          :class="reasonError ? 'border-red-600' : 'border-slate-300'"
          :aria-invalid="reasonError ? 'true' : 'false'"
          :aria-describedby="reasonError ? 'reject-reason-error' : undefined"
          @blur="reasonTouched = true"
          @focus="onReasonFocus"
        />
        <p v-if="reasonError" id="reject-reason-error" class="mt-1 text-sm text-red-700">
          {{ reasonError }}
        </p>
      </div>
      <button type="submit" :class="btnDanger" :aria-busy="busy">
        {{ busy ? 'Memproses…' : 'Tolak LPB' }}
      </button>
    </form>
  </SlideOver>

  <SlideOver
    :open="podOpen"
    :title="`POD Pengiriman ${lpb?.lpb_number ?? ''}`"
    @close="podOpen = false"
  >
    <form class="space-y-4" @submit.prevent="submitPod">
      <p class="text-sm text-slate-700">
        Menandai pengiriman selesai akan memindahkan unit sewa ke status <strong>RENTED</strong>,
        mencatat <code>rental_placements</code> mulai tanggal POD, dan menandai DO/SO selesai.
      </p>
      <div
        v-if="podError"
        class="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-800"
        role="alert"
      >
        {{ podError }}
      </div>
      <div>
        <label for="pod-receiver" class="block text-sm font-medium text-slate-800">
          Nama Penerima <span class="text-red-700" aria-hidden="true">*</span>
        </label>
        <input
          id="pod-receiver"
          v-model="podReceiver"
          type="text"
          required
          minlength="2"
          maxlength="100"
          placeholder="Nama PIC / penerima di lokasi pelanggan"
          class="mt-1 w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm"
        />
      </div>
      <div>
        <label for="pod-date" class="block text-sm font-medium text-slate-800">
          Tanggal Diterima (opsional, default hari ini WIB)
        </label>
        <input
          id="pod-date"
          v-model="podDate"
          type="date"
          class="mt-1 w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm"
        />
      </div>
      <div>
        <label for="pod-photo" class="block text-sm font-medium text-slate-800">
          Foto Surat Jalan Bertanda Tangan (opsional)
        </label>
        <input
          id="pod-photo"
          type="file"
          accept="image/*"
          class="mt-1 block w-full text-sm text-slate-700"
          @change="onPodFileChange"
        />
      </div>
      <div>
        <label for="pod-notes" class="block text-sm font-medium text-slate-800">Catatan</label>
        <textarea
          id="pod-notes"
          v-model="podNotes"
          rows="2"
          maxlength="500"
          class="mt-1 w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm"
        />
      </div>
      <div class="flex justify-end gap-2">
        <button type="button" :class="btnSecondary" @click="podOpen = false">Batal</button>
        <button type="submit" :disabled="podBusy" :class="btnPrimary">
          {{ podBusy ? 'Menyimpan…' : 'Konfirmasi Terkirim (POD)' }}
        </button>
      </div>
    </form>
  </SlideOver>

  <ConfirmDialog
    :open="pickupConfirmOpen"
    :title="`Tandai unit pada ${lpb?.lpb_number ?? ''} sudah dijemput?`"
    message="Unit akan berubah dari RENTED ke IN_TRANSIT dan periode tagihan (rental_placements) ditutup per hari ini (WIB)."
    confirm-label="Konfirmasi Dijemput"
    tone="primary"
    :busy="podBusy"
    @confirm="confirmPickup"
    @cancel="pickupConfirmOpen = false"
  />
</template>
