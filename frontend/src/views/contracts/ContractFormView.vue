<script setup lang="ts">
// Pola halaman: Form — Buat / Ubah Draft Kontrak Sewa (T2.3).
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import BackLink from '@/components/BackLink.vue'
import { toApiError, userMessage } from '@/lib/apiError'
import { formatRupiah } from '@/lib/format'
import { btnDanger, btnPrimary, btnSecondary } from '@/lib/ui'
import { contractService } from '@/services/contractService'
import { customerService } from '@/services/customerService'
import { productService } from '@/services/masterService'
import type { BillingMode, ContractItemPayload } from '@/types/contract'
import type { Customer, CustomerSite } from '@/types/customer'
import type { Product } from '@/types/master'

const inputCls =
  'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 focus:border-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-500/20 disabled:bg-slate-100'

const route = useRoute()
const router = useRouter()
const toast = useToast()

const editId = computed(() => (route.params.id ? Number(route.params.id) : null))
const isEdit = computed(() => editId.value !== null)

const loading = ref(true)
const loadError = ref<string | null>(null)
const submitting = ref(false)
const formError = ref<string | null>(null)

const customers = ref<Customer[]>([])
const sites = ref<CustomerSite[]>([])
const products = ref<Product[]>([])

interface ItemRow {
  product_id: number | ''
  customer_site_id: number | ''
  qty: number
  monthly_rate: string
  notes: string
}

const todayISO = new Date().toISOString().slice(0, 10)

const form = reactive({
  customer_id: '' as number | '',
  start_date: todayISO,
  end_date: '',
  duration_months: 12,
  billing_day_of_month: 1,
  billing_mode: 'PRORATA' as BillingMode,
  is_ppn: true,
  payment_term_days: 30,
  deposit_amount: '0.00',
  notes: '',
})

const items = ref<ItemRow[]>([
  { product_id: '', customer_site_id: '', qty: 1, monthly_rate: '', notes: '' },
])

function computeEndDate(startIso: string, months: number): string {
  if (!startIso || !Number.isInteger(months) || months < 1) return ''
  const parts = startIso.split('-').map(Number)
  const y = parts[0]
  const m = parts[1]
  const d = parts[2]
  if (!y || !m || !d) return ''
  const dt = new Date(Date.UTC(y, m - 1 + months, d - 1))
  return dt.toISOString().slice(0, 10)
}

function recalcEndDate(): void {
  const next = computeEndDate(form.start_date, Number(form.duration_months))
  if (next) form.end_date = next
}

function addItem(): void {
  items.value.push({ product_id: '', customer_site_id: '', qty: 1, monthly_rate: '', notes: '' })
}

function removeItem(idx: number): void {
  if (items.value.length <= 1) return
  items.value.splice(idx, 1)
}

function onProductChange(row: ItemRow): void {
  if (!row.product_id) return
  const p = products.value.find((x) => x.id === Number(row.product_id))
  if (p?.default_monthly_rate && !row.monthly_rate) {
    row.monthly_rate = p.default_monthly_rate
  }
}

const estimatedMonthly = computed(() => {
  let total = 0
  for (const r of items.value) {
    const rate = Number(r.monthly_rate)
    if (Number.isFinite(rate) && r.qty > 0) {
      total += rate * r.qty
    }
  }
  return total.toFixed(2)
})

watch(
  () => form.customer_id,
  async (cid) => {
    if (!cid) {
      sites.value = []
      return
    }
    try {
      sites.value = await customerService.listSites(Number(cid), true)
      if (!isEdit.value) {
        const cust = customers.value.find((c) => c.id === Number(cid))
        if (cust) {
          form.is_ppn = cust.is_pkp
          form.payment_term_days = cust.payment_term_days
        }
      }
    } catch {
      sites.value = []
    }
  },
)

async function init(): Promise<void> {
  loading.value = true
  loadError.value = null
  try {
    const [custRes, prodRes] = await Promise.all([
      customerService.list({ limit: 100, is_active: 'true' }),
      productService.list({ limit: 100, is_active: 'true', item_type: 'ASSET' }),
    ])
    customers.value = custRes.items
    products.value = prodRes.items

    if (editId.value !== null) {
      const detail = await contractService.get(editId.value)
      if (detail.status !== 'DRAFT') {
        loadError.value = 'Hanya kontrak berstatus DRAFT yang dapat diubah.'
        return
      }
      form.customer_id = detail.customer_id
      form.start_date = detail.start_date
      form.end_date = detail.end_date
      form.duration_months = detail.duration_months
      form.billing_day_of_month = detail.billing_day_of_month
      form.billing_mode = detail.billing_mode
      form.is_ppn = detail.is_ppn
      form.payment_term_days = detail.payment_term_days
      form.deposit_amount = detail.deposit_amount
      form.notes = detail.notes ?? ''
      sites.value = await customerService.listSites(detail.customer_id, true)
      items.value = detail.items.map((it) => ({
        product_id: it.product_id,
        customer_site_id: it.customer_site_id ?? '',
        qty: it.qty,
        monthly_rate: it.monthly_rate,
        notes: it.notes ?? '',
      }))
    } else {
      recalcEndDate()
    }
  } catch (err) {
    loadError.value = userMessage(toApiError(err))
  } finally {
    loading.value = false
  }
}

async function submit(): Promise<void> {
  formError.value = null
  if (!form.customer_id) {
    formError.value = 'Pilih pelanggan terlebih dahulu.'
    return
  }
  if (!form.start_date || !form.end_date || form.end_date <= form.start_date) {
    formError.value = 'Tanggal berakhir harus setelah tanggal mulai.'
    return
  }

  const mappedItems: ContractItemPayload[] = []
  for (let i = 0; i < items.value.length; i++) {
    const r = items.value[i]!
    if (!r.product_id) {
      formError.value = `Baris #${i + 1}: pilih produk aset.`
      return
    }
    if (!r.qty || r.qty < 1) {
      formError.value = `Baris #${i + 1}: qty minimal 1.`
      return
    }
    mappedItems.push({
      product_id: Number(r.product_id),
      customer_site_id: r.customer_site_id ? Number(r.customer_site_id) : null,
      qty: Number(r.qty),
      monthly_rate: r.monthly_rate.trim() || null,
      notes: r.notes.trim() || null,
    })
  }

  submitting.value = true
  try {
    const common = {
      start_date: form.start_date,
      end_date: form.end_date,
      duration_months: Number(form.duration_months),
      billing_day_of_month: Number(form.billing_day_of_month),
      billing_mode: form.billing_mode,
      is_ppn: form.is_ppn,
      payment_term_days: Number(form.payment_term_days),
      deposit_amount: form.deposit_amount.trim() || '0.00',
      notes: form.notes.trim() || null,
      items: mappedItems,
    }
    const saved =
      editId.value !== null
        ? await contractService.update(editId.value, common)
        : await contractService.create({ customer_id: Number(form.customer_id), ...common })
    toast.add({
      severity: 'success',
      summary: 'Berhasil',
      detail: `Kontrak ${saved.contract_number} ${isEdit.value ? 'diperbarui' : 'dibuat'}`,
      life: 3500,
    })
    await router.push(`/kontrak/${saved.id}`)
  } catch (err) {
    formError.value = userMessage(toApiError(err))
  } finally {
    submitting.value = false
  }
}

onMounted(() => {
  void init()
})
</script>

<template>
  <div class="space-y-6">
    <div class="space-y-2">
      <BackLink
        :to="isEdit && editId ? `/kontrak/${editId}` : '/kontrak'"
        :label="isEdit ? 'Detail kontrak' : 'Daftar kontrak'"
      />
      <h1 class="text-2xl font-semibold text-slate-900">
        {{ isEdit ? 'Ubah Draft Kontrak' : 'Buat Kontrak Sewa' }}
      </h1>
      <p class="text-sm text-slate-600">
        Tentukan pelanggan, periode sewa, aturan penagihan, dan daftar produk aset beserta tarif
        bulanannya.
      </p>
    </div>

    <div
      v-if="loading"
      class="rounded-xl border border-slate-200 bg-white p-8 text-center text-sm text-slate-500"
    >
      Memuat data form kontrak…
    </div>
    <div
      v-else-if="loadError"
      class="rounded-xl border border-rose-200 bg-rose-50 p-6 text-sm text-rose-800"
      role="alert"
    >
      {{ loadError }}
    </div>

    <form v-else class="space-y-6" @submit.prevent="submit">
      <div
        v-if="formError"
        class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-800"
        role="alert"
      >
        {{ formError }}
      </div>

      <!-- Header Kontrak -->
      <section class="space-y-4 rounded-xl border border-slate-200 bg-white p-5">
        <h2 class="text-sm font-semibold text-slate-900">Informasi Kontrak &amp; Periode</h2>
        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <label class="block text-xs font-medium text-slate-700" for="ctr-customer">
              Pelanggan <span class="text-rose-600">*</span>
            </label>
            <select
              id="ctr-customer"
              v-model="form.customer_id"
              required
              :disabled="isEdit"
              class="mt-1"
              :class="inputCls"
            >
              <option value="">Pilih pelanggan aktif…</option>
              <option v-for="c in customers" :key="c.id" :value="c.id">
                {{ c.customer_code }} — {{ c.company_name }}
              </option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-700" for="ctr-mode">
              Mode Tagihan
            </label>
            <select id="ctr-mode" v-model="form.billing_mode" class="mt-1" :class="inputCls">
              <option value="PRORATA">PRORATA (proporsional hari aktif)</option>
              <option value="FULL_MONTH">FULL_MONTH (penuh per bulan)</option>
            </select>
          </div>
        </div>

        <div class="grid gap-4 sm:grid-cols-3">
          <div>
            <label class="block text-xs font-medium text-slate-700" for="ctr-start">
              Tanggal Mulai <span class="text-rose-600">*</span>
            </label>
            <input
              id="ctr-start"
              v-model="form.start_date"
              type="date"
              required
              class="mt-1"
              :class="inputCls"
              @change="recalcEndDate"
            />
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-700" for="ctr-duration">
              Durasi (bulan) <span class="text-rose-600">*</span>
            </label>
            <input
              id="ctr-duration"
              v-model.number="form.duration_months"
              type="number"
              min="1"
              max="120"
              required
              class="mt-1"
              :class="inputCls"
              @change="recalcEndDate"
            />
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-700" for="ctr-end">
              Tanggal Berakhir <span class="text-rose-600">*</span>
            </label>
            <input
              id="ctr-end"
              v-model="form.end_date"
              type="date"
              required
              class="mt-1"
              :class="inputCls"
            />
          </div>
        </div>

        <div class="grid gap-4 sm:grid-cols-3">
          <div>
            <label class="block text-xs font-medium text-slate-700" for="ctr-bill-day">
              Tanggal Tagih (1–28)
            </label>
            <input
              id="ctr-bill-day"
              v-model.number="form.billing_day_of_month"
              type="number"
              min="1"
              max="28"
              required
              class="mt-1"
              :class="inputCls"
            />
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-700" for="ctr-term">
              Termin Pembayaran (hari)
            </label>
            <input
              id="ctr-term"
              v-model.number="form.payment_term_days"
              type="number"
              min="0"
              max="365"
              required
              class="mt-1"
              :class="inputCls"
            />
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-700" for="ctr-deposit">
              Deposit (Rp)
            </label>
            <input
              id="ctr-deposit"
              v-model="form.deposit_amount"
              type="text"
              inputmode="decimal"
              placeholder="0.00"
              class="mt-1 font-mono"
              :class="inputCls"
            />
          </div>
        </div>

        <label class="flex items-center gap-2 text-sm text-slate-800">
          <input
            v-model="form.is_ppn"
            type="checkbox"
            class="h-4 w-4 rounded border-slate-300 text-indigo-600 focus:ring-indigo-500"
          />
          Kenakan PPN (PMK 131/2024: DPP Nilai Lain 11/12 × 12%)
        </label>

        <div>
          <label class="block text-xs font-medium text-slate-700" for="ctr-notes">Catatan</label>
          <textarea
            id="ctr-notes"
            v-model="form.notes"
            rows="2"
            maxlength="1000"
            class="mt-1"
            :class="inputCls"
          />
        </div>
      </section>

      <!-- Item Tarif Kontrak -->
      <section class="space-y-4 rounded-xl border border-slate-200 bg-white p-5">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div>
            <h2 class="text-sm font-semibold text-slate-900">Item Tarif Sewa (Produk Aset)</h2>
            <p class="text-xs text-slate-500">
              Estimasi tagihan bulanan:
              <strong class="text-slate-900">{{ formatRupiah(estimatedMonthly) }}</strong>
            </p>
          </div>
          <button type="button" :class="btnSecondary" @click="addItem">+ Tambah Baris</button>
        </div>

        <div class="space-y-3">
          <div
            v-for="(row, idx) in items"
            :key="idx"
            class="grid gap-3 rounded-lg border border-slate-200 bg-slate-50/70 p-3 md:grid-cols-12"
          >
            <div class="md:col-span-4">
              <label class="block text-xs font-medium text-slate-700">Produk Aset *</label>
              <select
                v-model="row.product_id"
                required
                class="mt-1"
                :class="inputCls"
                @change="onProductChange(row)"
              >
                <option value="">Pilih produk…</option>
                <option v-for="p in products" :key="p.id" :value="p.id">
                  {{ p.sku }} — {{ p.name }}
                </option>
              </select>
            </div>
            <div class="md:col-span-3">
              <label class="block text-xs font-medium text-slate-700">Cabang / Site</label>
              <select v-model="row.customer_site_id" class="mt-1" :class="inputCls">
                <option value="">Semua / fleksibel</option>
                <option v-for="s in sites" :key="s.id" :value="s.id">{{ s.site_name }}</option>
              </select>
            </div>
            <div class="md:col-span-1">
              <label class="block text-xs font-medium text-slate-700">Qty *</label>
              <input
                v-model.number="row.qty"
                type="number"
                min="1"
                required
                class="mt-1"
                :class="inputCls"
              />
            </div>
            <div class="md:col-span-3">
              <label class="block text-xs font-medium text-slate-700">Tarif / Bln (Rp) *</label>
              <input
                v-model="row.monthly_rate"
                type="text"
                inputmode="decimal"
                required
                placeholder="500000.00"
                class="mt-1 font-mono"
                :class="inputCls"
              />
            </div>
            <div class="flex items-end justify-end md:col-span-1">
              <button
                type="button"
                :disabled="items.length <= 1"
                :class="btnDanger"
                @click="removeItem(idx)"
              >
                Hapus
              </button>
            </div>
          </div>
        </div>
      </section>

      <div class="flex justify-end gap-3">
        <RouterLink
          :to="isEdit && editId ? `/kontrak/${editId}` : '/kontrak'"
          :class="btnSecondary"
        >
          Batal
        </RouterLink>
        <button type="submit" :disabled="submitting" :class="btnPrimary">
          {{ submitting ? 'Menyimpan…' : isEdit ? 'Simpan Perubahan' : 'Simpan Draft Kontrak' }}
        </button>
      </div>
    </form>
  </div>
</template>
