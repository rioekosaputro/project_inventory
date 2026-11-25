<script setup lang="ts">
// Kartu Stok per produk/rak (plan Bagian 9.4: /stok/kartu-stok) + Export Excel.
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import BackLink from '@/components/BackLink.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { toApiError, userMessage } from '@/lib/apiError'
import { formatDateTime } from '@/lib/format'
import { subTypeLabel } from '@/lib/lpb'
import { downloadBlob } from '@/lib/pdf'
import { btnPrimary, btnSecondary, rowLink } from '@/lib/ui'
import { inventoryService } from '@/services/inventoryService'
import { productService, rackService } from '@/services/masterService'
import type { StockCard } from '@/types/inventory'
import type { Product, Rack } from '@/types/master'

const route = useRoute()
const toast = useToast()

const products = ref<Product[]>([])
const racks = ref<Rack[]>([])
const productId = ref<number | ''>('')
const rackId = ref<number | ''>('')
const dateFrom = ref('')
const dateTo = ref('')

const card = ref<StockCard | null>(null)
const loading = ref(false)
const exporting = ref(false)
const errorMsg = ref('')

onMounted(async () => {
  try {
    const [pPage, rPage] = await Promise.all([
      productService.list({ limit: 100, sort: 'sku' }),
      rackService.list({ limit: 100, sort: 'warehouse,code' }),
    ])
    products.value = pPage.items
    racks.value = rPage.items
  } catch (e) {
    errorMsg.value = userMessage(toApiError(e))
  }
  const qp = Number(route.query.product_id)
  const qr = Number(route.query.rack_id)
  if (Number.isInteger(qp) && qp > 0) {
    productId.value = qp
    if (Number.isInteger(qr) && qr > 0) rackId.value = qr
    await fetchCard()
  }
})

async function fetchCard(): Promise<void> {
  if (!productId.value) return
  loading.value = true
  errorMsg.value = ''
  try {
    card.value = await inventoryService.stockCard({
      product_id: Number(productId.value),
      rack_id: rackId.value ? Number(rackId.value) : undefined,
      date_from: dateFrom.value || undefined,
      date_to: dateTo.value || undefined,
    })
  } catch (e) {
    errorMsg.value = userMessage(toApiError(e))
    card.value = null
  } finally {
    loading.value = false
  }
}

async function exportExcel(): Promise<void> {
  if (!productId.value || exporting.value) return
  exporting.value = true
  try {
    const blob = await inventoryService.exportStockCardXlsx({
      product_id: Number(productId.value),
      rack_id: rackId.value ? Number(rackId.value) : undefined,
      date_from: dateFrom.value || undefined,
      date_to: dateTo.value || undefined,
    })
    downloadBlob(blob, `kartu-stok-${productId.value}.xlsx`)
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: 'Gagal mengekspor Kartu Stok',
      detail: userMessage(toApiError(e)),
      life: 7000,
    })
  } finally {
    exporting.value = false
  }
}
</script>

<template>
  <div class="mx-auto max-w-6xl">
    <BackLink to="/stok/buku-besar" label="Kembali ke Buku Besar Stok" />

    <header class="mt-3 flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 class="text-xl font-semibold text-slate-900">Kartu Stok Produk</h1>
        <p class="mt-1 text-sm text-slate-600">
          Saldo awal, rincian mutasi kronologis, dan saldo akhir per produk atau rak.
        </p>
      </div>
      <button
        v-if="card"
        type="button"
        :class="btnSecondary"
        :aria-busy="exporting"
        @click="exportExcel"
      >
        <i class="pi pi-download text-xs" aria-hidden="true" />
        {{ exporting ? 'Menyiapkan Excel…' : 'Export Excel' }}
      </button>
    </header>

    <form
      class="mt-4 grid gap-3 rounded-xl border border-slate-200 bg-white p-4 sm:grid-cols-2 lg:grid-cols-5"
      @submit.prevent="fetchCard"
    >
      <div class="lg:col-span-2">
        <label for="sc-product" class="block text-xs font-medium text-slate-600">Produk</label>
        <select
          id="sc-product"
          v-model="productId"
          required
          class="mt-1 w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm"
        >
          <option value="">Pilih produk…</option>
          <option v-for="p in products" :key="p.id" :value="p.id">
            {{ p.sku }} — {{ p.name }}
          </option>
        </select>
      </div>
      <div>
        <label for="sc-rack" class="block text-xs font-medium text-slate-600">
          Rak (opsional)
        </label>
        <select
          id="sc-rack"
          v-model="rackId"
          class="mt-1 w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm"
        >
          <option value="">Semua rak</option>
          <option v-for="r in racks" :key="r.id" :value="r.id">
            {{ r.warehouse_code }} / {{ r.code }}
          </option>
        </select>
      </div>
      <div>
        <label for="sc-from" class="block text-xs font-medium text-slate-600">Dari tanggal</label>
        <input
          id="sc-from"
          v-model="dateFrom"
          type="date"
          class="mt-1 w-full rounded-lg border border-slate-300 px-3 py-2 text-sm"
        />
      </div>
      <div class="flex items-end gap-2">
        <div class="flex-1">
          <label for="sc-to" class="block text-xs font-medium text-slate-600">Sampai</label>
          <input
            id="sc-to"
            v-model="dateTo"
            type="date"
            class="mt-1 w-full rounded-lg border border-slate-300 px-3 py-2 text-sm"
          />
        </div>
        <button type="submit" :class="btnPrimary" :aria-busy="loading">Tampilkan</button>
      </div>
    </form>

    <div
      v-if="errorMsg"
      class="mt-4 rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800"
      role="alert"
    >
      {{ errorMsg }}
    </div>

    <template v-if="card">
      <div class="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
        <div class="rounded-xl border border-slate-200 bg-white p-4">
          <div class="text-xs text-slate-500">Saldo Awal</div>
          <div class="mt-1 font-mono text-xl font-bold text-slate-900">
            {{ card.opening_balance }}
          </div>
        </div>
        <div class="rounded-xl border border-slate-200 bg-white p-4">
          <div class="text-xs text-slate-500">Total Masuk</div>
          <div class="mt-1 font-mono text-xl font-bold text-emerald-700">+{{ card.total_in }}</div>
        </div>
        <div class="rounded-xl border border-slate-200 bg-white p-4">
          <div class="text-xs text-slate-500">Total Keluar</div>
          <div class="mt-1 font-mono text-xl font-bold text-indigo-700">-{{ card.total_out }}</div>
        </div>
        <div class="rounded-xl border border-slate-200 bg-white p-4">
          <div class="text-xs text-slate-500">Saldo Akhir</div>
          <div class="mt-1 font-mono text-xl font-bold text-slate-900">
            {{ card.closing_balance }}
          </div>
        </div>
      </div>

      <div class="mt-4 overflow-x-auto rounded-xl border border-slate-200 bg-white">
        <table class="min-w-full divide-y divide-slate-200 text-sm">
          <thead class="bg-slate-50 text-left text-xs font-semibold text-slate-600">
            <tr>
              <th class="px-4 py-3">Waktu</th>
              <th class="px-4 py-3">Dokumen LPB</th>
              <th class="px-4 py-3">Rak</th>
              <th class="px-4 py-3">Mutasi</th>
              <th class="px-4 py-3 text-right">Saldo Rak</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            <tr v-if="card.movements.length === 0">
              <td colspan="5" class="px-4 py-8 text-center text-slate-500">
                Tidak ada mutasi pada periode ini.
              </td>
            </tr>
            <tr v-for="m in card.movements" :key="m.id">
              <td class="px-4 py-2.5 text-xs text-slate-700">
                {{ formatDateTime(m.occurred_at) }}
              </td>
              <td class="px-4 py-2.5">
                <RouterLink :to="`/lpb/${m.lpb_id}`" :class="rowLink" class="font-mono text-xs">
                  {{ m.lpb_number }}
                </RouterLink>
                <div class="text-xs text-slate-500">{{ subTypeLabel(m.movement_type) }}</div>
              </td>
              <td class="px-4 py-2.5 font-mono text-xs">
                {{ m.warehouse_code }} / {{ m.rack_code }}
              </td>
              <td class="px-4 py-2.5">
                <StatusBadge :value="m.direction" />
                <span class="ml-1.5 font-mono font-semibold">
                  {{ m.direction === 'IN' ? `+${m.qty}` : `-${m.qty}` }}
                </span>
              </td>
              <td class="px-4 py-2.5 text-right font-mono text-xs">
                {{ m.balance_before }} → <strong>{{ m.balance_after }}</strong>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </div>
</template>
