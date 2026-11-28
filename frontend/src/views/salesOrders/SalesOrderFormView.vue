<script setup lang="ts">
// Pola halaman: Form — Buat / Ubah Draft Sales Order (4 tipe) (T2.5).
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import BackLink from '@/components/BackLink.vue'
import { toApiError, userMessage } from '@/lib/apiError'
import { btnDanger, btnPrimary, btnSecondary } from '@/lib/ui'
import { contractService } from '@/services/contractService'
import { customerService } from '@/services/customerService'
import { productService } from '@/services/masterService'
import { salesOrderService } from '@/services/salesOrderService'
import type { RentalContract } from '@/types/contract'
import type { Customer, CustomerAsset, CustomerSite } from '@/types/customer'
import type { Product } from '@/types/master'
import type { ItemDirection, OrderType, SOItemPayload } from '@/types/salesOrder'

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
const contracts = ref<RentalContract[]>([])
const customerAssets = ref<CustomerAsset[]>([])
const allProducts = ref<Product[]>([])

interface ItemRow {
  product_id: number | ''
  direction: ItemDirection
  qty: number
  unit_price: string
  product_serial_id: number | ''
  notes: string
}

const todayISO = new Date().toISOString().slice(0, 10)

const form = reactive({
  order_type: 'NEW_DELIVERY' as OrderType,
  customer_id: '' as number | '',
  customer_site_id: '' as number | '',
  contract_id: '' as number | '',
  order_date: todayISO,
  requested_date: '',
  is_ppn: true,
  discount_amount: '0.00',
  notes: '',
})

const items = ref<ItemRow[]>([
  {
    product_id: '',
    direction: 'OUT',
    qty: 1,
    unit_price: '0.00',
    product_serial_id: '',
    notes: '',
  },
])

const eligibleProducts = computed(() =>
  allProducts.value.filter((p) =>
    form.order_type === 'CONSUMABLE' ? p.item_type === 'CONSUMABLE' : p.item_type === 'ASSET',
  ),
)

function defaultDir(t: OrderType): ItemDirection {
  return t === 'WITHDRAWAL' ? 'IN' : 'OUT'
}

function addItem(): void {
  items.value.push({
    product_id: '',
    direction: defaultDir(form.order_type),
    qty: 1,
    unit_price: '0.00',
    product_serial_id: '',
    notes: '',
  })
}

function removeItem(idx: number): void {
  if (items.value.length <= 1) return
  items.value.splice(idx, 1)
}

function onProductChange(row: ItemRow): void {
  const prod = allProducts.value.find((p) => p.id === Number(row.product_id))
  if (!prod) return
  if (row.direction === 'IN') {
    row.unit_price = '0.00'
  } else if (form.order_type === 'CONSUMABLE' && prod.default_sell_price) {
    row.unit_price = prod.default_sell_price
  } else if (form.order_type === 'NEW_DELIVERY' && prod.default_monthly_rate) {
    row.unit_price = prod.default_monthly_rate
  }
}

watch(
  () => form.order_type,
  (next) => {
    if (isEdit.value) return
    if (next === 'UNIT_SWAP') {
      items.value = [
        {
          product_id: '',
          direction: 'OUT',
          qty: 1,
          unit_price: '0.00',
          product_serial_id: '',
          notes: '',
        },
        {
          product_id: '',
          direction: 'IN',
          qty: 1,
          unit_price: '0.00',
          product_serial_id: '',
          notes: '',
        },
      ]
    } else {
      const dir = defaultDir(next)
      for (const r of items.value) {
        r.direction = dir
        r.product_id = ''
        r.product_serial_id = ''
      }
    }
    if (next === 'CONSUMABLE') {
      form.contract_id = ''
    }
  },
)

watch(
  () => form.customer_id,
  async (cid) => {
    if (!cid) {
      sites.value = []
      contracts.value = []
      customerAssets.value = []
      return
    }
    const idNum = Number(cid)
    try {
      const [sList, cPage, aList] = await Promise.all([
        customerService.listSites(idNum, true),
        contractService.list({ customer_id: idNum, limit: 100 }),
        customerService.listAssets(idNum),
      ])
      sites.value = sList
      contracts.value = cPage.items.filter((c) => c.status !== 'DRAFT')
      customerAssets.value = aList
      if (!isEdit.value) {
        const primary = sList.find((s) => s.is_primary) ?? sList[0]
        form.customer_site_id = primary ? primary.id : ''
        const cust = customers.value.find((c) => c.id === idNum)
        if (cust) form.is_ppn = cust.is_pkp
      }
    } catch {
      sites.value = []
      contracts.value = []
      customerAssets.value = []
    }
  },
)

async function init(): Promise<void> {
  loading.value = true
  loadError.value = null
  try {
    const [custRes, prodRes] = await Promise.all([
      customerService.list({ limit: 100, is_active: 'true' }),
      productService.list({ limit: 100, is_active: 'true' }),
    ])
    customers.value = custRes.items
    allProducts.value = prodRes.items

    if (editId.value !== null) {
      const detail = await salesOrderService.get(editId.value)
      if (detail.status !== 'DRAFT') {
        loadError.value = 'Hanya Sales Order berstatus DRAFT yang dapat diubah.'
        return
      }
      form.order_type = detail.order_type
      form.customer_id = detail.customer_id
      form.customer_site_id = detail.customer_site_id
      form.contract_id = detail.contract_id ?? ''
      form.order_date = detail.order_date
      form.requested_date = detail.requested_date ?? ''
      form.is_ppn = detail.is_ppn
      form.discount_amount = detail.discount_amount
      form.notes = detail.notes ?? ''
      items.value = detail.items.map((it) => ({
        product_id: it.product_id,
        direction: it.direction,
        qty: it.qty,
        unit_price: it.unit_price,
        product_serial_id: it.product_serial_id ?? '',
        notes: it.notes ?? '',
      }))
    }
  } catch (err) {
    loadError.value = userMessage(toApiError(err))
  } finally {
    loading.value = false
  }
}

async function submit(): Promise<void> {
  formError.value = null
  if (!form.customer_id || !form.customer_site_id) {
    formError.value = 'Pilih pelanggan dan site tujuan terlebih dahulu.'
    return
  }
  if (form.order_type !== 'CONSUMABLE' && !form.contract_id) {
    formError.value = 'Kontrak wajib dipilih untuk jenis SO ini.'
    return
  }

  const mapped: SOItemPayload[] = []
  for (let i = 0; i < items.value.length; i++) {
    const r = items.value[i]!
    if (!r.product_id) {
      formError.value = `Baris #${i + 1}: pilih produk.`
      return
    }
    mapped.push({
      product_id: Number(r.product_id),
      direction: r.direction,
      qty: Number(r.qty),
      unit_price: r.unit_price.trim() || '0.00',
      product_serial_id: r.product_serial_id ? Number(r.product_serial_id) : null,
      notes: r.notes.trim() || null,
    })
  }

  submitting.value = true
  try {
    const saved =
      editId.value !== null
        ? await salesOrderService.update(editId.value, {
            customer_site_id: Number(form.customer_site_id),
            order_date: form.order_date,
            requested_date: form.requested_date || null,
            is_ppn: form.is_ppn,
            discount_amount: form.discount_amount.trim() || '0.00',
            notes: form.notes.trim() || null,
            items: mapped,
          })
        : await salesOrderService.create({
            order_type: form.order_type,
            customer_id: Number(form.customer_id),
            customer_site_id: Number(form.customer_site_id),
            contract_id: form.contract_id ? Number(form.contract_id) : null,
            order_date: form.order_date,
            requested_date: form.requested_date || null,
            is_ppn: form.is_ppn,
            discount_amount: form.discount_amount.trim() || '0.00',
            notes: form.notes.trim() || null,
            items: mapped,
          })
    toast.add({
      severity: 'success',
      summary: 'Berhasil',
      detail: `Sales Order ${saved.so_number} ${isEdit.value ? 'diperbarui' : 'dibuat'}`,
      life: 3500,
    })
    await router.push(`/sales-orders/${saved.id}`)
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
        :to="isEdit && editId ? `/sales-orders/${editId}` : '/sales-orders'"
        :label="isEdit ? 'Detail SO' : 'Daftar Sales Order'"
      />
      <h1 class="text-2xl font-semibold text-slate-900">
        {{ isEdit ? 'Ubah Draft Sales Order' : 'Buat Sales Order' }}
      </h1>
      <p class="text-sm text-slate-600">
        Kalkulasi DPP, DPP Nilai Lain (11/12), dan PPN 12% dihitung otomatis oleh server sesuai
        aturan PMK 131/2024.
      </p>
    </div>

    <div
      v-if="loading"
      class="rounded-xl border border-slate-200 bg-white p-8 text-center text-sm text-slate-500"
    >
      Memuat form Sales Order…
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

      <section class="space-y-4 rounded-xl border border-slate-200 bg-white p-5">
        <h2 class="text-sm font-semibold text-slate-900">Header Sales Order</h2>
        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <label class="block text-xs font-medium text-slate-700" for="so-type">
              Tipe Sales Order <span class="text-rose-600">*</span>
            </label>
            <select
              id="so-type"
              v-model="form.order_type"
              :disabled="isEdit"
              class="mt-1"
              :class="inputCls"
            >
              <option value="NEW_DELIVERY">NEW_DELIVERY — Pengiriman Sewa Baru</option>
              <option value="WITHDRAWAL">WITHDRAWAL — Penarikan Unit</option>
              <option value="UNIT_SWAP">UNIT_SWAP — Tukar Unit (OUT + IN)</option>
              <option value="CONSUMABLE">CONSUMABLE — Penjualan Consumable</option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-700" for="so-customer">
              Pelanggan <span class="text-rose-600">*</span>
            </label>
            <select
              id="so-customer"
              v-model="form.customer_id"
              required
              :disabled="isEdit"
              class="mt-1"
              :class="inputCls"
            >
              <option value="">Pilih pelanggan…</option>
              <option v-for="c in customers" :key="c.id" :value="c.id">
                {{ c.customer_code }} — {{ c.company_name }}
              </option>
            </select>
          </div>
        </div>

        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <label class="block text-xs font-medium text-slate-700" for="so-site">
              Cabang / Site Tujuan <span class="text-rose-600">*</span>
            </label>
            <select
              id="so-site"
              v-model="form.customer_site_id"
              required
              class="mt-1"
              :class="inputCls"
            >
              <option value="">Pilih site aktif…</option>
              <option v-for="s in sites" :key="s.id" :value="s.id">
                {{ s.site_name }} ({{ s.city }})
              </option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-700" for="so-contract">
              Kontrak Sewa
              <span v-if="form.order_type !== 'CONSUMABLE'" class="text-rose-600">*</span>
            </label>
            <select
              id="so-contract"
              v-model="form.contract_id"
              :required="form.order_type !== 'CONSUMABLE'"
              :disabled="isEdit"
              class="mt-1"
              :class="inputCls"
            >
              <option value="">
                {{
                  form.order_type === 'CONSUMABLE' ? 'Tanpa kontrak (opsional)' : 'Pilih kontrak…'
                }}
              </option>
              <option v-for="ct in contracts" :key="ct.id" :value="ct.id">
                {{ ct.contract_number }} ({{ ct.status }})
              </option>
            </select>
          </div>
        </div>

        <div class="grid gap-4 sm:grid-cols-3">
          <div>
            <label class="block text-xs font-medium text-slate-700" for="so-date">
              Tanggal Order <span class="text-rose-600">*</span>
            </label>
            <input
              id="so-date"
              v-model="form.order_date"
              type="date"
              required
              class="mt-1"
              :class="inputCls"
            />
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-700" for="so-req-date">
              Target Tanggal Kirim/Tarik
            </label>
            <input
              id="so-req-date"
              v-model="form.requested_date"
              type="date"
              class="mt-1"
              :class="inputCls"
            />
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-700" for="so-discount">
              Diskon (Rp)
            </label>
            <input
              id="so-discount"
              v-model="form.discount_amount"
              type="text"
              inputmode="decimal"
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
          <label class="block text-xs font-medium text-slate-700" for="so-notes">Catatan</label>
          <textarea
            id="so-notes"
            v-model="form.notes"
            rows="2"
            maxlength="1000"
            class="mt-1"
            :class="inputCls"
          />
        </div>
      </section>

      <section class="space-y-4 rounded-xl border border-slate-200 bg-white p-5">
        <div class="flex items-center justify-between gap-2">
          <h2 class="text-sm font-semibold text-slate-900">Item Sales Order</h2>
          <button type="button" :class="btnSecondary" @click="addItem">+ Tambah Baris</button>
        </div>

        <div class="space-y-3">
          <div
            v-for="(row, idx) in items"
            :key="idx"
            class="grid gap-3 rounded-lg border border-slate-200 bg-slate-50/70 p-3 md:grid-cols-12"
          >
            <div class="md:col-span-2">
              <label class="block text-xs font-medium text-slate-700">Arah</label>
              <select
                v-model="row.direction"
                :disabled="form.order_type !== 'UNIT_SWAP'"
                class="mt-1"
                :class="inputCls"
              >
                <option value="OUT">OUT (Keluar)</option>
                <option value="IN">IN (Tarik)</option>
              </select>
            </div>
            <div class="md:col-span-4">
              <label class="block text-xs font-medium text-slate-700">Produk *</label>
              <select
                v-model="row.product_id"
                required
                class="mt-1"
                :class="inputCls"
                @change="onProductChange(row)"
              >
                <option value="">Pilih produk…</option>
                <option v-for="p in eligibleProducts" :key="p.id" :value="p.id">
                  {{ p.sku }} — {{ p.name }}
                </option>
              </select>
            </div>
            <div v-if="row.direction === 'IN'" class="md:col-span-3">
              <label class="block text-xs font-medium text-slate-700"
                >Unit Terpasang (opsional)</label
              >
              <select v-model="row.product_serial_id" class="mt-1" :class="inputCls">
                <option value="">Pilih SN spesifik…</option>
                <option
                  v-for="a in customerAssets.filter(
                    (x) => !row.product_id || x.product_id === Number(row.product_id),
                  )"
                  :key="a.serial_id"
                  :value="a.serial_id"
                >
                  {{ a.serial_number }} ({{ a.asset_tag }})
                </option>
              </select>
            </div>
            <div v-else class="md:col-span-3">
              <label class="block text-xs font-medium text-slate-700">Harga / Tarif (Rp)</label>
              <input
                v-model="row.unit_price"
                type="text"
                inputmode="decimal"
                class="mt-1 font-mono"
                :class="inputCls"
              />
            </div>
            <div class="md:col-span-2">
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
          :to="isEdit && editId ? `/sales-orders/${editId}` : '/sales-orders'"
          :class="btnSecondary"
        >
          Batal
        </RouterLink>
        <button type="submit" :disabled="submitting" :class="btnPrimary">
          {{ submitting ? 'Menyimpan…' : isEdit ? 'Simpan Perubahan' : 'Simpan Draft SO' }}
        </button>
      </div>
    </form>
  </div>
</template>
