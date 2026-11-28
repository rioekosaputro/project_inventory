<script setup lang="ts">
// Pola halaman: Detail — Invoice, Issue, Void, Pembayaran + PPh 23, Cetak PDF (T2.9).
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import BackLink from '@/components/BackLink.vue'
import SlideOver from '@/components/SlideOver.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { usePermission } from '@/composables/usePermission'
import { toApiError, userMessage } from '@/lib/apiError'
import { formatDateTime, formatRupiah } from '@/lib/format'
import { openPdfBlob } from '@/lib/pdf'
import { btnDanger, btnPrimary, btnSecondary, rowLink } from '@/lib/ui'
import { invoiceService } from '@/services/invoiceService'
import type { InvoiceDetail, PaymentMethod } from '@/types/invoice'

const inputCls =
  'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 focus:border-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-500/20'

const route = useRoute()
const toast = useToast()
const { can } = usePermission()

const invoiceId = computed(() => Number(route.params.id))
const loading = ref(true)
const loadError = ref<string | null>(null)
const inv = ref<InvoiceDetail | null>(null)
const printing = ref(false)

// Issue
const issueOpen = ref(false)
const issueBusy = ref(false)
const issueError = ref<string | null>(null)
const fakturPajak = ref('')

// Void
const voidOpen = ref(false)
const voidBusy = ref(false)
const voidError = ref<string | null>(null)
const voidReason = ref('')

// Payment
const payOpen = ref(false)
const payBusy = ref(false)
const payError = ref<string | null>(null)
const payKey = ref('')
const payForm = reactive({
  payment_date: new Date().toISOString().slice(0, 10),
  amount_received: '',
  pph23_amount: '0.00',
  pph23_slip_number: '',
  method: 'TRANSFER' as PaymentMethod,
  reference_number: '',
  notes: '',
})

async function load(): Promise<void> {
  loading.value = true
  loadError.value = null
  try {
    inv.value = await invoiceService.get(invoiceId.value)
  } catch (err) {
    loadError.value = userMessage(toApiError(err))
  } finally {
    loading.value = false
  }
}

async function printPdf(): Promise<void> {
  if (!inv.value) return
  printing.value = true
  try {
    const blob = await invoiceService.fetchPdf(inv.value.id)
    await openPdfBlob(blob, `${inv.value.invoice_number.replace(/\//g, '-')}.pdf`)
  } catch (err) {
    toast.add({
      severity: 'error',
      summary: 'Gagal',
      detail: userMessage(toApiError(err)),
      life: 5000,
    })
  } finally {
    printing.value = false
  }
}

async function submitIssue(): Promise<void> {
  if (!inv.value) return
  issueBusy.value = true
  issueError.value = null
  try {
    inv.value = await invoiceService.issue(inv.value.id, fakturPajak.value.trim() || null)
    issueOpen.value = false
    toast.add({
      severity: 'success',
      summary: 'Berhasil',
      detail: `Invoice ${inv.value.invoice_number} telah diterbitkan`,
      life: 3500,
    })
  } catch (err) {
    issueError.value = userMessage(toApiError(err))
  } finally {
    issueBusy.value = false
  }
}

async function submitVoid(): Promise<void> {
  if (!inv.value) return
  voidError.value = null
  if (voidReason.value.trim().length < 5) {
    voidError.value = 'Alasan pembatalan minimal 5 karakter.'
    return
  }
  voidBusy.value = true
  try {
    inv.value = await invoiceService.void(inv.value.id, voidReason.value.trim())
    voidOpen.value = false
    toast.add({
      severity: 'success',
      summary: 'Berhasil',
      detail: `Invoice ${inv.value.invoice_number} di-VOID`,
      life: 3500,
    })
  } catch (err) {
    voidError.value = userMessage(toApiError(err))
  } finally {
    voidBusy.value = false
  }
}

function openPayment(): void {
  if (!inv.value) return
  payError.value = null
  payKey.value = crypto.randomUUID ? crypto.randomUUID() : `pay-${Date.now()}`
  payForm.payment_date = new Date().toISOString().slice(0, 10)
  payForm.amount_received = inv.value.remaining_amount
  payForm.pph23_amount = '0.00'
  payForm.pph23_slip_number = ''
  payForm.method = 'TRANSFER'
  payForm.reference_number = ''
  payForm.notes = ''
  payOpen.value = true
}

async function submitPayment(): Promise<void> {
  if (!inv.value) return
  payBusy.value = true
  payError.value = null
  try {
    inv.value = await invoiceService.recordPayment(
      inv.value.id,
      {
        payment_date: payForm.payment_date || null,
        amount_received: payForm.amount_received.trim(),
        pph23_amount: payForm.pph23_amount.trim() || '0.00',
        pph23_slip_number: payForm.pph23_slip_number.trim() || null,
        method: payForm.method,
        reference_number: payForm.reference_number.trim() || null,
        notes: payForm.notes.trim() || null,
      },
      payKey.value,
    )
    payOpen.value = false
    toast.add({
      severity: 'success',
      summary: 'Berhasil',
      detail: `Pembayaran dicatat — status invoice: ${inv.value.status}`,
      life: 3500,
    })
  } catch (err) {
    payError.value = userMessage(toApiError(err))
  } finally {
    payBusy.value = false
  }
}

watch(invoiceId, () => {
  void load()
})

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="space-y-6">
    <div class="space-y-2">
      <BackLink to="/invoices" label="Daftar Tagihan" />
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div class="flex flex-wrap items-center gap-3">
            <h1 class="font-mono text-2xl font-semibold text-slate-900">
              {{ inv ? inv.invoice_number : 'Detail Invoice' }}
            </h1>
            <StatusBadge v-if="inv" :value="inv.status" />
          </div>
          <p v-if="inv" class="mt-1 text-sm text-slate-600">
            <RouterLink :to="`/pelanggan/${inv.customer_id}`" :class="rowLink">
              {{ inv.customer_code }} — {{ inv.customer_name }}
            </RouterLink>
            <template v-if="inv.contract_id && inv.contract_number">
              · Kontrak:
              <RouterLink :to="`/kontrak/${inv.contract_id}`" :class="rowLink">
                {{ inv.contract_number }}
              </RouterLink>
            </template>
          </p>
        </div>

        <div v-if="inv" class="flex flex-wrap items-center gap-2">
          <button type="button" :disabled="printing" :class="btnSecondary" @click="printPdf">
            <i class="pi pi-print text-xs" aria-hidden="true" />
            {{ printing ? 'Menyiapkan PDF…' : 'Cetak PDF' }}
          </button>
          <button
            v-if="inv.status === 'DRAFT' && can('invoice.approve')"
            type="button"
            :class="btnPrimary"
            @click="issueOpen = true"
          >
            Terbitkan Invoice
          </button>
          <button
            v-if="
              (inv.status === 'ISSUED' || inv.status === 'PARTIALLY_PAID') && can('invoice.pay')
            "
            type="button"
            :class="btnPrimary"
            @click="openPayment"
          >
            Catat Pembayaran
          </button>
          <button
            v-if="
              (inv.status === 'DRAFT' || inv.status === 'ISSUED') &&
              inv.paid_amount === '0.00' &&
              can('invoice.approve')
            "
            type="button"
            :class="btnDanger"
            @click="voidOpen = true"
          >
            Void
          </button>
        </div>
      </div>
    </div>

    <div
      v-if="loading"
      class="rounded-xl border border-slate-200 bg-white p-8 text-center text-sm text-slate-500"
    >
      Memuat detail invoice…
    </div>
    <div
      v-else-if="loadError"
      class="rounded-xl border border-rose-200 bg-rose-50 p-6 text-sm text-rose-800"
      role="alert"
    >
      {{ loadError }}
    </div>

    <template v-else-if="inv">
      <div
        v-if="inv.status === 'VOID'"
        class="rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-900"
      >
        <strong>Invoice VOID</strong>
        <span v-if="inv.voided_at"> pada {{ formatDateTime(inv.voided_at) }}</span>
        — Alasan: {{ inv.void_reason ?? '—' }}
      </div>

      <div class="grid gap-6 md:grid-cols-2">
        <div class="space-y-3 rounded-xl border border-slate-200 bg-white p-5">
          <h2 class="text-sm font-semibold text-slate-900">Informasi Tagihan</h2>
          <dl class="divide-y divide-slate-100 text-sm">
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">Jenis</dt>
              <dd class="font-medium text-slate-900">{{ inv.invoice_type }}</dd>
            </div>
            <div v-if="inv.billing_period_start" class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">Periode Sewa</dt>
              <dd class="text-slate-900">
                {{ inv.billing_period_start }} s/d {{ inv.billing_period_end }}
              </dd>
            </div>
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">Tanggal / Jatuh Tempo</dt>
              <dd class="text-slate-900">{{ inv.invoice_date }} · JT {{ inv.due_date }}</dd>
            </div>
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">No. Faktur Pajak</dt>
              <dd class="font-mono text-slate-900">{{ inv.faktur_pajak_number ?? '—' }}</dd>
            </div>
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">NPWP Pelanggan</dt>
              <dd class="font-mono text-slate-900">{{ inv.customer_npwp ?? '—' }}</dd>
            </div>
          </dl>
        </div>

        <div class="space-y-3 rounded-xl border border-slate-200 bg-white p-5">
          <h2 class="text-sm font-semibold text-slate-900">Rincian Pajak &amp; Pembayaran</h2>
          <dl class="divide-y divide-slate-100 text-sm">
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">DPP</dt>
              <dd class="font-mono text-slate-900">{{ formatRupiah(inv.dpp) }}</dd>
            </div>
            <div v-if="inv.is_ppn" class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">DPP Nilai Lain (11/12)</dt>
              <dd class="font-mono text-slate-900">{{ formatRupiah(inv.dpp_other_value) }}</dd>
            </div>
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">
                PPN {{ inv.is_ppn ? `(${inv.tax_rate}%)` : '(Non-PPN)' }}
              </dt>
              <dd class="font-mono text-slate-900">{{ formatRupiah(inv.tax_amount) }}</dd>
            </div>
            <div class="flex justify-between gap-4 py-2 font-semibold">
              <dt class="text-slate-900">Grand Total</dt>
              <dd class="font-mono text-slate-900">{{ formatRupiah(inv.grand_total) }}</dd>
            </div>
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">Terbayar (incl. PPh 23)</dt>
              <dd class="font-mono text-emerald-700">{{ formatRupiah(inv.paid_amount) }}</dd>
            </div>
            <div class="flex justify-between gap-4 py-2 font-semibold">
              <dt class="text-slate-900">Sisa Piutang</dt>
              <dd class="font-mono text-amber-700">{{ formatRupiah(inv.remaining_amount) }}</dd>
            </div>
          </dl>
        </div>
      </div>

      <!-- Baris Item -->
      <section class="overflow-x-auto rounded-xl border border-slate-200 bg-white">
        <div class="border-b border-slate-200 px-5 py-3">
          <h2 class="text-sm font-semibold text-slate-900">Rincian Item ({{ inv.items.length }})</h2>
        </div>
        <table class="min-w-full divide-y divide-slate-200 text-sm">
          <thead
            class="bg-slate-50 text-left text-xs font-semibold tracking-wide text-slate-600 uppercase"
          >
            <tr>
              <th class="px-4 py-3">Deskripsi</th>
              <th class="px-4 py-3 text-right">Qty</th>
              <th class="px-4 py-3 text-right">Tarif / Harga</th>
              <th class="px-4 py-3 text-right">Total</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            <tr v-for="it in inv.items" :key="it.id">
              <td class="px-4 py-3 text-slate-800">{{ it.description }}</td>
              <td class="px-4 py-3 text-right">{{ it.qty }}</td>
              <td class="px-4 py-3 text-right font-mono text-xs">{{ formatRupiah(it.unit_price) }}</td>
              <td class="px-4 py-3 text-right font-medium text-slate-900">
                {{ formatRupiah(it.line_total) }}
              </td>
            </tr>
          </tbody>
        </table>
      </section>

      <!-- Riwayat Pembayaran -->
      <section class="overflow-x-auto rounded-xl border border-slate-200 bg-white">
        <div class="border-b border-slate-200 px-5 py-3">
          <h2 class="text-sm font-semibold text-slate-900">
            Riwayat Pembayaran &amp; PPh 23 ({{ inv.payments.length }})
          </h2>
        </div>
        <div v-if="inv.payments.length === 0" class="px-5 py-6 text-center text-sm text-slate-500">
          Belum ada pembayaran tercatat.
        </div>
        <table v-else class="min-w-full divide-y divide-slate-200 text-sm">
          <thead
            class="bg-slate-50 text-left text-xs font-semibold tracking-wide text-slate-600 uppercase"
          >
            <tr>
              <th class="px-4 py-3">Tanggal</th>
              <th class="px-4 py-3">Metode / Ref</th>
              <th class="px-4 py-3 text-right">Diterima</th>
              <th class="px-4 py-3 text-right">PPh 23</th>
              <th class="px-4 py-3">Bukti Potong</th>
              <th class="px-4 py-3 text-right">Total Pelunasan</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            <tr v-for="p in inv.payments" :key="p.id">
              <td class="px-4 py-3 text-slate-800">{{ p.payment_date }}</td>
              <td class="px-4 py-3 text-slate-700">
                {{ p.method }}
                <span v-if="p.reference_number" class="block font-mono text-xs text-slate-500">
                  {{ p.reference_number }}
                </span>
              </td>
              <td class="px-4 py-3 text-right font-mono text-xs">
                {{ formatRupiah(p.amount_received) }}
              </td>
              <td class="px-4 py-3 text-right font-mono text-xs">
                {{ formatRupiah(p.pph23_amount) }}
              </td>
              <td class="px-4 py-3 font-mono text-xs text-slate-700">
                {{ p.pph23_slip_number ?? '—' }}
              </td>
              <td class="px-4 py-3 text-right font-medium text-emerald-700">
                {{ formatRupiah(p.total_applied) }}
              </td>
            </tr>
          </tbody>
        </table>
      </section>
    </template>

    <!-- SlideOver Issue -->
    <SlideOver :open="issueOpen" title="Terbitkan Invoice" @close="issueOpen = false">
      <form class="space-y-4" @submit.prevent="submitIssue">
        <div v-if="issueError" class="rounded bg-rose-50 p-2 text-xs text-rose-800" role="alert">
          {{ issueError }}
        </div>
        <div>
          <label for="faktur-pajak" class="block text-xs font-medium text-slate-700">
            Nomor Faktur Pajak Coretax (opsional)
          </label>
          <input
            id="faktur-pajak"
            v-model="fakturPajak"
            type="text"
            maxlength="40"
            placeholder="040.000-26.00000001"
            class="mt-1 font-mono"
            :class="inputCls"
          />
        </div>
        <div class="flex justify-end gap-2">
          <button type="button" :class="btnSecondary" @click="issueOpen = false">Batal</button>
          <button type="submit" :disabled="issueBusy" :class="btnPrimary">
            {{ issueBusy ? 'Menerbitkan…' : 'Terbitkan Invoice' }}
          </button>
        </div>
      </form>
    </SlideOver>

    <!-- SlideOver Void -->
    <SlideOver :open="voidOpen" title="Batalkan (VOID) Invoice" @close="voidOpen = false">
      <form class="space-y-4" @submit.prevent="submitVoid">
        <div v-if="voidError" class="rounded bg-rose-50 p-2 text-xs text-rose-800" role="alert">
          {{ voidError }}
        </div>
        <div>
          <label for="void-reason" class="block text-xs font-medium text-slate-700">
            Alasan Pembatalan <span class="text-rose-600">*</span>
          </label>
          <textarea
            id="void-reason"
            v-model="voidReason"
            rows="3"
            required
            minlength="5"
            maxlength="500"
            class="mt-1"
            :class="inputCls"
          />
        </div>
        <div class="flex justify-end gap-2">
          <button type="button" :class="btnSecondary" @click="voidOpen = false">Batal</button>
          <button type="submit" :disabled="voidBusy" :class="btnDanger">
            {{ voidBusy ? 'Memproses…' : 'Konfirmasi VOID' }}
          </button>
        </div>
      </form>
    </SlideOver>

    <!-- SlideOver Catat Pembayaran + PPh 23 -->
    <SlideOver :open="payOpen" title="Catat Pembayaran (+ PPh 23)" @close="payOpen = false">
      <form class="space-y-4" @submit.prevent="submitPayment">
        <div v-if="payError" class="rounded bg-rose-50 p-2 text-xs text-rose-800" role="alert">
          {{ payError }}
        </div>
        <div class="grid gap-3 sm:grid-cols-2">
          <div>
            <label for="pay-date" class="block text-xs font-medium text-slate-700">Tanggal Bayar</label>
            <input id="pay-date" v-model="payForm.payment_date" type="date" required class="mt-1" :class="inputCls" />
          </div>
          <div>
            <label for="pay-method" class="block text-xs font-medium text-slate-700">Metode</label>
            <select id="pay-method" v-model="payForm.method" class="mt-1" :class="inputCls">
              <option value="TRANSFER">TRANSFER</option>
              <option value="CASH">CASH</option>
              <option value="GIRO">GIRO</option>
              <option value="OTHER">OTHER</option>
            </select>
          </div>
        </div>
        <div class="grid gap-3 sm:grid-cols-2">
          <div>
            <label for="pay-amt" class="block text-xs font-medium text-slate-700">
              Nominal Diterima (Rp) *
            </label>
            <input
              id="pay-amt"
              v-model="payForm.amount_received"
              type="text"
              inputmode="decimal"
              required
              class="mt-1 font-mono"
              :class="inputCls"
            />
          </div>
          <div>
            <label for="pay-pph23" class="block text-xs font-medium text-slate-700">
              Potongan PPh 23 (Rp)
            </label>
            <input
              id="pay-pph23"
              v-model="payForm.pph23_amount"
              type="text"
              inputmode="decimal"
              class="mt-1 font-mono"
              :class="inputCls"
            />
          </div>
        </div>
        <div class="grid gap-3 sm:grid-cols-2">
          <div>
            <label for="pay-slip" class="block text-xs font-medium text-slate-700">
              No. Bukti Potong PPh 23
            </label>
            <input id="pay-slip" v-model="payForm.pph23_slip_number" type="text" maxlength="60" class="mt-1" :class="inputCls" />
          </div>
          <div>
            <label for="pay-ref" class="block text-xs font-medium text-slate-700">No. Referensi</label>
            <input id="pay-ref" v-model="payForm.reference_number" type="text" maxlength="80" class="mt-1" :class="inputCls" />
          </div>
        </div>
        <div>
          <label for="pay-notes" class="block text-xs font-medium text-slate-700">Catatan</label>
          <textarea id="pay-notes" v-model="payForm.notes" rows="2" maxlength="500" class="mt-1" :class="inputCls" />
        </div>
        <div class="flex justify-end gap-2">
          <button type="button" :class="btnSecondary" @click="payOpen = false">Batal</button>
          <button type="submit" :disabled="payBusy" :class="btnPrimary">
            {{ payBusy ? 'Menyimpan…' : 'Simpan Pembayaran' }}
          </button>
        </div>
      </form>
    </SlideOver>
  </div>
</template>
