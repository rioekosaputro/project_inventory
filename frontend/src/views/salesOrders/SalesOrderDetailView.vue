<script setup lang="ts">
// Pola halaman: Detail — Sales Order, Rincian Pajak PMK 131/2024, dan Penerbitan DO (T2.5).
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import BackLink from '@/components/BackLink.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import SlideOver from '@/components/SlideOver.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { usePermission } from '@/composables/usePermission'
import { toApiError, userMessage } from '@/lib/apiError'
import { formatDateTime, formatRupiah } from '@/lib/format'
import { btnDanger, btnPrimary, btnSecondary, rowLink } from '@/lib/ui'
import { warehouseService } from '@/services/masterService'
import { salesOrderService } from '@/services/salesOrderService'
import type { Warehouse } from '@/types/master'
import type { SalesOrderDetail } from '@/types/salesOrder'

const inputCls =
  'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 focus:border-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-500/20'

const route = useRoute()
const toast = useToast()
const { can } = usePermission()

const soId = computed(() => Number(route.params.id))
const loading = ref(true)
const loadError = ref<string | null>(null)
const so = ref<SalesOrderDetail | null>(null)
const warehouses = ref<Warehouse[]>([])

// Confirm SO
const confirmOpen = ref(false)
const confirmBusy = ref(false)

// Cancel SO
const cancelOpen = ref(false)
const cancelBusy = ref(false)
const cancelError = ref<string | null>(null)
const cancelReason = ref('')

// Issue DO
const doOpen = ref(false)
const doBusy = ref(false)
const doError = ref<string | null>(null)
const doForm = reactive({
  warehouse_id: '' as number | '',
  scheduled_date: '',
  notes: '',
})

async function load(): Promise<void> {
  loading.value = true
  loadError.value = null
  try {
    so.value = await salesOrderService.get(soId.value)
  } catch (err) {
    loadError.value = userMessage(toApiError(err))
  } finally {
    loading.value = false
  }
}

async function confirmSO(): Promise<void> {
  if (!so.value) return
  confirmBusy.value = true
  try {
    so.value = await salesOrderService.confirm(so.value.id)
    confirmOpen.value = false
    toast.add({
      severity: 'success',
      summary: 'Berhasil',
      detail: `SO ${so.value.so_number} dikonfirmasi dan siap diterbitkan DO`,
      life: 3500,
    })
  } catch (err) {
    toast.add({
      severity: 'error',
      summary: 'Gagal',
      detail: userMessage(toApiError(err)),
      life: 5000,
    })
  } finally {
    confirmBusy.value = false
  }
}

async function submitCancel(): Promise<void> {
  if (!so.value) return
  cancelError.value = null
  if (cancelReason.value.trim().length < 5) {
    cancelError.value = 'Alasan pembatalan minimal 5 karakter.'
    return
  }
  cancelBusy.value = true
  try {
    so.value = await salesOrderService.cancel(so.value.id, cancelReason.value.trim())
    cancelOpen.value = false
    toast.add({
      severity: 'success',
      summary: 'Berhasil',
      detail: `SO ${so.value.so_number} dibatalkan`,
      life: 3500,
    })
  } catch (err) {
    cancelError.value = userMessage(toApiError(err))
  } finally {
    cancelBusy.value = false
  }
}

async function openIssueDO(): Promise<void> {
  doError.value = null
  doForm.scheduled_date = so.value?.requested_date ?? ''
  doForm.notes = ''
  if (warehouses.value.length === 0) {
    try {
      const res = await warehouseService.list({ limit: 100, is_active: 'true' })
      warehouses.value = res.items
      if (warehouses.value.length === 1) {
        doForm.warehouse_id = warehouses.value[0]!.id
      }
    } catch {
      warehouses.value = []
    }
  }
  doOpen.value = true
}

async function submitIssueDO(): Promise<void> {
  if (!so.value || !doForm.warehouse_id) {
    doError.value = 'Pilih gudang pelaksana.'
    return
  }
  doError.value = null
  doBusy.value = true
  try {
    const dos = await salesOrderService.issueDeliveryOrders(so.value.id, {
      warehouse_id: Number(doForm.warehouse_id),
      scheduled_date: doForm.scheduled_date || null,
      notes: doForm.notes.trim() || null,
    })
    doOpen.value = false
    toast.add({
      severity: 'success',
      summary: 'Berhasil',
      detail: `${dos.length} Delivery Order berhasil diterbitkan ke gudang`,
      life: 3500,
    })
    await load()
  } catch (err) {
    doError.value = userMessage(toApiError(err))
  } finally {
    doBusy.value = false
  }
}

watch(soId, () => {
  void load()
})

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="space-y-6">
    <div class="space-y-2">
      <BackLink to="/sales-orders" label="Daftar Sales Order" />
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div class="flex flex-wrap items-center gap-3">
            <h1 class="font-mono text-2xl font-semibold text-slate-900">
              {{ so ? so.so_number : 'Detail Sales Order' }}
            </h1>
            <StatusBadge v-if="so" :value="so.status" />
            <span
              v-if="so"
              class="inline-flex items-center rounded-full bg-slate-100 px-2.5 py-0.5 text-xs font-medium text-slate-700"
            >
              {{ so.order_type }}
            </span>
          </div>
          <p v-if="so" class="mt-1 text-sm text-slate-600">
            <RouterLink :to="`/pelanggan/${so.customer_id}`" :class="rowLink">
              {{ so.customer_code }} — {{ so.customer_name }}
            </RouterLink>
            · Site: {{ so.site_name }} ({{ so.site_city }})
            <template v-if="so.contract_id && so.contract_number">
              · Kontrak:
              <RouterLink :to="`/kontrak/${so.contract_id}`" :class="rowLink">
                {{ so.contract_number }}
              </RouterLink>
            </template>
          </p>
        </div>

        <div v-if="so" class="flex flex-wrap items-center gap-2">
          <RouterLink
            v-if="so.status === 'DRAFT' && can('so.write')"
            :to="`/sales-orders/${so.id}/ubah`"
            :class="btnSecondary"
          >
            Ubah Draft
          </RouterLink>
          <button
            v-if="so.status === 'DRAFT' && can('so.approve')"
            type="button"
            :class="btnPrimary"
            @click="confirmOpen = true"
          >
            Konfirmasi SO
          </button>
          <button
            v-if="so.status === 'CONFIRMED' && can('do.create')"
            type="button"
            :class="btnPrimary"
            @click="openIssueDO"
          >
            Terbitkan DO ke Gudang
          </button>
          <button
            v-if="(so.status === 'DRAFT' || so.status === 'CONFIRMED') && can('so.approve')"
            type="button"
            :class="btnDanger"
            @click="cancelOpen = true"
          >
            Batalkan SO
          </button>
        </div>
      </div>
    </div>

    <div
      v-if="loading"
      class="rounded-xl border border-slate-200 bg-white p-8 text-center text-sm text-slate-500"
    >
      Memuat detail Sales Order…
    </div>
    <div
      v-else-if="loadError"
      class="rounded-xl border border-rose-200 bg-rose-50 p-6 text-sm text-rose-800"
      role="alert"
    >
      {{ loadError }}
    </div>

    <template v-else-if="so">
      <div
        v-if="so.status === 'CANCELLED'"
        class="rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-900"
      >
        <div class="font-semibold">
          SO dibatalkan oleh {{ so.cancelled_by_name ?? '—' }}
          <span v-if="so.cancelled_at">pada {{ formatDateTime(so.cancelled_at) }}</span>
        </div>
        <p class="mt-1">Alasan: {{ so.cancel_reason ?? '—' }}</p>
      </div>

      <!-- Informasi & Kalkulasi Pajak -->
      <div class="grid gap-6 md:grid-cols-2">
        <div class="space-y-3 rounded-xl border border-slate-200 bg-white p-5">
          <h2 class="text-sm font-semibold text-slate-900">Informasi Pesanan</h2>
          <dl class="divide-y divide-slate-100 text-sm">
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">Tanggal Order</dt>
              <dd class="text-slate-900">{{ so.order_date }}</dd>
            </div>
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">Target Tanggal</dt>
              <dd class="text-slate-900">{{ so.requested_date ?? '—' }}</dd>
            </div>
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">Dibuat oleh</dt>
              <dd class="text-slate-900">{{ so.created_by_name }}</dd>
            </div>
            <div v-if="so.confirmed_at" class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">Dikonfirmasi oleh</dt>
              <dd class="text-slate-900">
                {{ so.confirmed_by_name ?? '—' }} · {{ formatDateTime(so.confirmed_at) }}
              </dd>
            </div>
            <div v-if="so.notes" class="py-2">
              <dt class="text-slate-500">Catatan</dt>
              <dd class="mt-1 text-slate-900">{{ so.notes }}</dd>
            </div>
          </dl>
        </div>

        <div class="space-y-3 rounded-xl border border-slate-200 bg-white p-5">
          <h2 class="text-sm font-semibold text-slate-900">Ringkasan Nilai &amp; Pajak</h2>
          <dl class="divide-y divide-slate-100 text-sm">
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">Subtotal</dt>
              <dd class="font-mono text-slate-900">{{ formatRupiah(so.subtotal) }}</dd>
            </div>
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">Diskon</dt>
              <dd class="font-mono text-slate-900">{{ formatRupiah(so.discount_amount) }}</dd>
            </div>
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">DPP</dt>
              <dd class="font-mono text-slate-900">{{ formatRupiah(so.dpp) }}</dd>
            </div>
            <div v-if="so.is_ppn" class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">DPP Nilai Lain (11/12)</dt>
              <dd class="font-mono text-slate-900">{{ formatRupiah(so.dpp_other_value) }}</dd>
            </div>
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">
                PPN {{ so.is_ppn ? `(${so.tax_rate}%)` : '(Non-PPN)' }}
              </dt>
              <dd class="font-mono text-slate-900">{{ formatRupiah(so.tax_amount) }}</dd>
            </div>
            <div class="flex justify-between gap-4 py-2 font-semibold">
              <dt class="text-slate-900">Grand Total</dt>
              <dd class="font-mono text-slate-900">{{ formatRupiah(so.grand_total) }}</dd>
            </div>
          </dl>
        </div>
      </div>

      <!-- Daftar Item -->
      <section class="overflow-x-auto rounded-xl border border-slate-200 bg-white">
        <div class="border-b border-slate-200 px-5 py-3">
          <h2 class="text-sm font-semibold text-slate-900">Item Pesanan ({{ so.items.length }})</h2>
        </div>
        <table class="min-w-full divide-y divide-slate-200 text-sm">
          <thead
            class="bg-slate-50 text-left text-xs font-semibold tracking-wide text-slate-600 uppercase"
          >
            <tr>
              <th class="px-4 py-3">Arah</th>
              <th class="px-4 py-3">Produk</th>
              <th class="px-4 py-3">Serial / Asset Tag</th>
              <th class="px-4 py-3 text-right">Qty</th>
              <th class="px-4 py-3 text-right">Harga Satuan</th>
              <th class="px-4 py-3 text-right">Total Baris</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            <tr v-for="it in so.items" :key="it.id">
              <td class="px-4 py-3">
                <span
                  class="inline-flex rounded-full px-2 py-0.5 text-xs font-medium"
                  :class="
                    it.direction === 'OUT'
                      ? 'bg-indigo-50 text-indigo-700'
                      : 'bg-amber-50 text-amber-800'
                  "
                >
                  {{ it.direction }}
                </span>
              </td>
              <td class="px-4 py-3">
                <div class="font-medium text-slate-900">{{ it.product_name }}</div>
                <div class="font-mono text-xs text-slate-500">{{ it.product_sku }}</div>
              </td>
              <td class="px-4 py-3 font-mono text-xs text-slate-700">
                <template v-if="it.serial_number"
                  >{{ it.serial_number }} ({{ it.asset_tag }})</template
                >
                <span v-else class="text-slate-400">—</span>
              </td>
              <td class="px-4 py-3 text-right">{{ it.qty }}</td>
              <td class="px-4 py-3 text-right font-mono text-xs">
                {{ formatRupiah(it.unit_price) }}
              </td>
              <td class="px-4 py-3 text-right font-medium text-slate-900">
                {{ formatRupiah(it.line_total) }}
              </td>
            </tr>
          </tbody>
        </table>
      </section>

      <!-- Delivery Orders Terbit -->
      <section class="overflow-x-auto rounded-xl border border-slate-200 bg-white">
        <div class="border-b border-slate-200 px-5 py-3">
          <h2 class="text-sm font-semibold text-slate-900">
            Delivery Order ({{ so.delivery_orders.length }})
          </h2>
        </div>
        <div
          v-if="so.delivery_orders.length === 0"
          class="px-5 py-6 text-center text-sm text-slate-500"
        >
          Belum ada Delivery Order diterbitkan untuk SO ini.
        </div>
        <table v-else class="min-w-full divide-y divide-slate-200 text-sm">
          <thead
            class="bg-slate-50 text-left text-xs font-semibold tracking-wide text-slate-600 uppercase"
          >
            <tr>
              <th class="px-4 py-3">Nomor DO</th>
              <th class="px-4 py-3">Arah</th>
              <th class="px-4 py-3">Gudang</th>
              <th class="px-4 py-3">Jadwal</th>
              <th class="px-4 py-3 text-right">Qty</th>
              <th class="px-4 py-3">Status</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            <tr v-for="d in so.delivery_orders" :key="d.id">
              <td class="px-4 py-3 font-mono text-xs font-medium text-slate-900">
                <RouterLink :to="`/delivery-orders/${d.id}`" :class="rowLink">
                  {{ d.do_number }}
                </RouterLink>
              </td>
              <td class="px-4 py-3">
                <span
                  class="inline-flex rounded-full px-2 py-0.5 text-xs font-medium"
                  :class="
                    d.direction === 'OUTBOUND'
                      ? 'bg-indigo-50 text-indigo-700'
                      : 'bg-amber-50 text-amber-800'
                  "
                >
                  {{ d.direction }}
                </span>
              </td>
              <td class="px-4 py-3 text-slate-700">
                {{ d.warehouse_code }} — {{ d.warehouse_name }}
              </td>
              <td class="px-4 py-3 text-slate-700">{{ d.scheduled_date ?? '—' }}</td>
              <td class="px-4 py-3 text-right">{{ d.total_qty }}</td>
              <td class="px-4 py-3"><StatusBadge :value="d.status" /></td>
            </tr>
          </tbody>
        </table>
      </section>
    </template>

    <ConfirmDialog
      :open="confirmOpen"
      :title="`Konfirmasi Sales Order '${so?.so_number ?? ''}'?`"
      message="Setelah dikonfirmasi, isi pesanan tidak dapat diubah lagi dan siap diterbitkan Delivery Order ke gudang."
      confirm-label="Konfirmasi SO"
      :busy="confirmBusy"
      @confirm="confirmSO"
      @cancel="confirmOpen = false"
    />

    <SlideOver :open="cancelOpen" title="Batalkan Sales Order" @close="cancelOpen = false">
      <form class="space-y-4" @submit.prevent="submitCancel">
        <div
          v-if="cancelError"
          class="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-800"
          role="alert"
        >
          {{ cancelError }}
        </div>
        <div>
          <label class="block text-xs font-medium text-slate-700" for="so-cancel-reason">
            Alasan Pembatalan <span class="text-rose-600">*</span>
          </label>
          <textarea
            id="so-cancel-reason"
            v-model="cancelReason"
            rows="3"
            required
            minlength="5"
            maxlength="500"
            class="mt-1"
            :class="inputCls"
          />
        </div>
        <div class="flex justify-end gap-2">
          <button type="button" :class="btnSecondary" @click="cancelOpen = false">Tutup</button>
          <button type="submit" :disabled="cancelBusy" :class="btnDanger">
            {{ cancelBusy ? 'Memproses…' : 'Batalkan SO' }}
          </button>
        </div>
      </form>
    </SlideOver>

    <SlideOver :open="doOpen" title="Terbitkan Delivery Order ke Gudang" @close="doOpen = false">
      <form class="space-y-4" @submit.prevent="submitIssueDO">
        <p class="text-xs text-slate-600">
          Untuk SO <strong>UNIT_SWAP</strong>, sistem otomatis menerbitkan 1 DO
          <strong>OUTBOUND</strong> dan 1 DO <strong>INBOUND</strong> ke gudang terpilih.
        </p>
        <div
          v-if="doError"
          class="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-800"
          role="alert"
        >
          {{ doError }}
        </div>
        <div>
          <label class="block text-xs font-medium text-slate-700" for="do-wh">
            Gudang Pelaksana <span class="text-rose-600">*</span>
          </label>
          <select id="do-wh" v-model="doForm.warehouse_id" required class="mt-1" :class="inputCls">
            <option value="">Pilih gudang aktif…</option>
            <option v-for="w in warehouses" :key="w.id" :value="w.id">
              {{ w.code }} — {{ w.name }}
            </option>
          </select>
        </div>
        <div>
          <label class="block text-xs font-medium text-slate-700" for="do-sched">
            Tanggal Jadwal (opsional)
          </label>
          <input
            id="do-sched"
            v-model="doForm.scheduled_date"
            type="date"
            class="mt-1"
            :class="inputCls"
          />
        </div>
        <div>
          <label class="block text-xs font-medium text-slate-700" for="do-notes">Catatan</label>
          <textarea
            id="do-notes"
            v-model="doForm.notes"
            rows="2"
            maxlength="1000"
            class="mt-1"
            :class="inputCls"
          />
        </div>
        <div class="flex justify-end gap-2">
          <button type="button" :class="btnSecondary" @click="doOpen = false">Batal</button>
          <button type="submit" :disabled="doBusy" :class="btnPrimary">
            {{ doBusy ? 'Menerbitkan…' : 'Terbitkan DO' }}
          </button>
        </div>
      </form>
    </SlideOver>
  </div>
</template>
