<script setup lang="ts">
// Dashboard Gudang (plan Bagian 9.4 & 9.5: /dashboard).
import { onMounted, ref } from 'vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { usePermission } from '@/composables/usePermission'
import { toApiError, userMessage } from '@/lib/apiError'
import { formatDateTime, formatRupiah } from '@/lib/format'
import { subTypeLabel } from '@/lib/lpb'
import { btnSecondary, rowLink } from '@/lib/ui'
import { dashboardService, type WarehouseDashboardSummary } from '@/services/dashboardService'
import { warehouseService } from '@/services/masterService'
import { useAuthStore } from '@/stores/auth'
import type { Warehouse } from '@/types/master'

const auth = useAuthStore()
const { can } = usePermission()

const summary = ref<WarehouseDashboardSummary | null>(null)
const warehouses = ref<Warehouse[]>([])
const selectedWh = ref<number | ''>('')
const loading = ref(true)
const errorMsg = ref('')

async function load(): Promise<void> {
  loading.value = true
  errorMsg.value = ''
  try {
    summary.value = await dashboardService.summary(
      selectedWh.value ? Number(selectedWh.value) : undefined,
    )
  } catch (e) {
    errorMsg.value = userMessage(toApiError(e))
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  if (!auth.user?.warehouse_id && can('master.read')) {
    try {
      warehouses.value = (await warehouseService.list({ limit: 100, is_active: true, sort: 'code' }))
        .items
    } catch {
      // opsional
    }
  }
  await load()
})
</script>

<template>
  <div class="w-full min-w-0">
    <header class="flex flex-wrap items-center justify-between gap-3">
      <div class="min-w-0">
        <h1 class="text-xl font-semibold text-slate-900">
          {{ summary?.commercial ? 'Dashboard Komersial & Gudang' : 'Dashboard Gudang' }}
        </h1>
        <p class="mt-1 text-sm text-slate-600">
          {{
            summary?.commercial
              ? 'Ringkasan utilisasi aset sewa, omzet, piutang jatuh tempo, kontrak berakhir, dan kondisi gudang.'
              : 'Ringkasan kondisi stok, antrean persetujuan LPB, stock opname berjalan, dan mutasi terbaru.'
          }}
        </p>
      </div>
      <div class="flex w-full flex-wrap items-center gap-2 sm:w-auto">
        <select
          v-if="warehouses.length > 0"
          v-model="selectedWh"
          aria-label="Filter gudang"
          class="min-w-0 flex-1 rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm sm:flex-initial"
          @change="load"
        >
          <option value="">Semua gudang</option>
          <option v-for="w in warehouses" :key="w.id" :value="w.id">
            {{ w.code }} — {{ w.name }}
          </option>
        </select>
        <button
          type="button"
          :class="[btnSecondary, 'shrink-0 whitespace-nowrap']"
          :aria-busy="loading"
          @click="load"
        >
          <i class="pi pi-refresh text-xs" aria-hidden="true" />Muat Ulang
        </button>
      </div>
    </header>

    <div v-if="loading && !summary" class="mt-8 py-12 text-center text-sm text-slate-600">
      Memuat metrik gudang…
    </div>
    <div
      v-else-if="errorMsg"
      class="mt-6 rounded-xl border border-rose-200 bg-rose-50 p-6 text-center text-sm text-rose-800"
      role="alert"
    >
      <p>{{ errorMsg }}</p>
      <button type="button" :class="[btnSecondary, 'mt-3']" @click="load">Coba lagi</button>
    </div>

    <template v-else-if="summary">
      <!-- Ringkasan Komersial (T2.11, plan Bagian 9.5) -->
      <template v-if="summary.commercial">
        <div class="mt-5 flex items-center justify-between">
          <h2 class="text-sm font-semibold tracking-wide text-slate-700 uppercase">Komersial</h2>
        </div>

        <section class="mt-2 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <RouterLink
            to="/stok/serial?status=RENTED"
            class="rounded-xl border border-slate-200 bg-white p-5 shadow-sm transition hover:border-slate-300"
          >
            <div class="text-xs font-medium text-slate-500">Utilisasi Aset Sewa</div>
            <div class="mt-2 font-mono text-2xl font-bold text-indigo-700">
              {{ summary.commercial.asset_utilization_pct }}%
            </div>
            <div class="mt-1 text-xs text-slate-600">
              {{ summary.commercial.rented_units }} unit RENTED dari
              {{ summary.commercial.total_active_assets }} aset aktif
            </div>
          </RouterLink>

          <RouterLink
            to="/invoices"
            class="rounded-xl border border-slate-200 bg-white p-5 shadow-sm transition hover:border-slate-300"
          >
            <div class="text-xs font-medium text-slate-500">Omzet Bulan Berjalan</div>
            <div class="mt-2 font-mono text-xl font-bold text-emerald-700">
              {{ formatRupiah(summary.commercial.monthly_revenue) }}
            </div>
            <div class="mt-1 text-xs text-slate-600">
              Tertagih: {{ formatRupiah(summary.commercial.monthly_collected) }}
            </div>
          </RouterLink>

          <RouterLink
            to="/invoices?overdue=true"
            class="rounded-xl border border-slate-200 bg-white p-5 shadow-sm transition hover:border-slate-300"
          >
            <div class="text-xs font-medium text-slate-500">Piutang &amp; Overdue</div>
            <div
              class="mt-2 font-mono text-xl font-bold"
              :class="
                summary.commercial.overdue_invoice_count > 0 ? 'text-rose-700' : 'text-slate-900'
              "
            >
              {{ formatRupiah(summary.commercial.outstanding_receivables) }}
            </div>
            <div class="mt-1 text-xs text-slate-600">
              {{ summary.commercial.overdue_invoice_count }} overdue ·
              {{ summary.commercial.due_soon_invoice_count }} jatuh tempo ≤7 hari
            </div>
          </RouterLink>

          <RouterLink
            to="/kontrak?expiring_days=30"
            class="rounded-xl border border-slate-200 bg-white p-5 shadow-sm transition hover:border-slate-300"
          >
            <div class="text-xs font-medium text-slate-500">Kontrak Berakhir ≤30 Hari</div>
            <div
              class="mt-2 font-mono text-2xl font-bold"
              :class="
                summary.commercial.expiring_contracts_count > 0
                  ? 'text-amber-700'
                  : 'text-emerald-700'
              "
            >
              {{ summary.commercial.expiring_contracts_count }}
            </div>
            <div class="mt-1 text-xs text-slate-600">
              dari {{ summary.commercial.active_contracts }} kontrak aktif ·
              {{ summary.commercial.active_customers }} pelanggan
            </div>
          </RouterLink>
        </section>

        <div class="mt-4 grid gap-6 lg:grid-cols-2">
          <!-- Invoice Overdue -->
          <section class="rounded-xl border border-slate-200 bg-white p-5 shadow-sm">
            <div class="flex items-center justify-between">
              <h3 class="text-sm font-semibold text-slate-900">Invoice Melewati Jatuh Tempo</h3>
              <RouterLink
                to="/invoices?overdue=true"
                class="text-xs font-medium text-indigo-700 hover:underline"
              >
                Lihat tagihan
              </RouterLink>
            </div>
            <p
              v-if="summary.commercial.overdue_invoices.length === 0"
              class="mt-4 text-sm text-slate-500"
            >
              Tidak ada invoice yang melewati jatuh tempo saat ini.
            </p>
            <ul v-else class="mt-3 divide-y divide-slate-100">
              <li
                v-for="inv in summary.commercial.overdue_invoices"
                :key="inv.invoice_id"
                class="flex items-center justify-between gap-3 py-2.5 text-sm"
              >
                <div class="min-w-0 flex-1">
                  <RouterLink :to="`/invoices/${inv.invoice_id}`" :class="rowLink" class="block truncate">
                    {{ inv.invoice_number }}
                  </RouterLink>
                  <div class="truncate text-xs text-slate-500">
                    {{ inv.customer_name }} · Jatuh tempo {{ inv.due_date }}
                  </div>
                </div>
                <div class="shrink-0 text-right">
                  <div class="font-mono text-xs font-bold whitespace-nowrap text-rose-700">
                    {{ formatRupiah(inv.remaining_amount) }}
                  </div>
                  <div class="text-xs whitespace-nowrap text-rose-600">
                    Terlambat {{ inv.days_overdue }} hari
                  </div>
                </div>
              </li>
            </ul>
          </section>

          <!-- Kontrak Berakhir <= 30 Hari -->
          <section class="rounded-xl border border-slate-200 bg-white p-5 shadow-sm">
            <div class="flex items-center justify-between">
              <h3 class="text-sm font-semibold text-slate-900">Kontrak Akan Berakhir (≤30 Hari)</h3>
              <RouterLink
                to="/kontrak?expiring_days=30"
                class="text-xs font-medium text-indigo-700 hover:underline"
              >
                Lihat kontrak
              </RouterLink>
            </div>
            <p
              v-if="summary.commercial.expiring_contracts.length === 0"
              class="mt-4 text-sm text-slate-500"
            >
              Tidak ada kontrak aktif yang berakhir dalam 30 hari ke depan.
            </p>
            <ul v-else class="mt-3 divide-y divide-slate-100">
              <li
                v-for="c in summary.commercial.expiring_contracts"
                :key="c.contract_id"
                class="flex items-center justify-between gap-3 py-2.5 text-sm"
              >
                <div class="min-w-0 flex-1">
                  <RouterLink :to="`/kontrak/${c.contract_id}`" :class="rowLink" class="block truncate">
                    {{ c.contract_number }}
                  </RouterLink>
                  <div class="truncate text-xs text-slate-500">
                    {{ c.customer_name }} · Berakhir {{ c.end_date }}
                  </div>
                </div>
                <span
                  class="shrink-0 rounded-full px-2.5 py-0.5 text-xs font-semibold whitespace-nowrap"
                  :class="
                    c.days_remaining <= 7
                      ? 'bg-rose-100 text-rose-800'
                      : 'bg-amber-100 text-amber-800'
                  "
                >
                  Sisa {{ c.days_remaining }} hari
                </span>
              </li>
            </ul>
          </section>
        </div>

        <div class="mt-6 flex items-center justify-between">
          <h2 class="text-sm font-semibold tracking-wide text-slate-700 uppercase">Gudang</h2>
        </div>
      </template>

      <!-- KPI Cards Gudang -->
      <section class="mt-2 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <RouterLink
          to="/stok?low_stock=true"
          class="rounded-xl border border-slate-200 bg-white p-5 shadow-sm transition hover:border-slate-300"
        >
          <div class="text-xs font-medium text-slate-500">Produk Stok Menipis</div>
          <div
            class="mt-2 font-mono text-2xl font-bold"
            :class="summary.low_stock_products > 0 ? 'text-rose-700' : 'text-emerald-700'"
          >
            {{ summary.low_stock_products }}
          </div>
          <div class="mt-1 text-xs text-slate-600">
            {{ summary.safe_stock_products }} produk aman dari {{ summary.total_active_products }}
            aktif
          </div>
        </RouterLink>

        <RouterLink
          :to="can('lpb.approve') ? '/lpb/approvals' : '/lpb?status=PENDING_APPROVAL'"
          class="rounded-xl border border-slate-200 bg-white p-5 shadow-sm transition hover:border-slate-300"
        >
          <div class="text-xs font-medium text-slate-500">LPB Menunggu Persetujuan</div>
          <div
            class="mt-2 font-mono text-2xl font-bold"
            :class="summary.pending_lpb_approvals > 0 ? 'text-amber-700' : 'text-slate-900'"
          >
            {{ summary.pending_lpb_approvals }}
          </div>
          <div class="mt-1 text-xs text-slate-600">Antrean persetujuan Kepala Gudang</div>
        </RouterLink>

        <RouterLink
          :to="can('do.read') ? '/delivery-orders' : '/opname'"
          class="rounded-xl border border-slate-200 bg-white p-5 shadow-sm transition hover:border-slate-300"
        >
          <div class="text-xs font-medium text-slate-500">Opname &amp; Antrean DO</div>
          <div class="mt-2 font-mono text-2xl font-bold text-sky-700">
            {{ summary.active_opnames }}
          </div>
          <div class="mt-1 text-xs text-slate-600">
            Opname berjalan · {{ summary.pending_do_reservations }} DO menunggu reservasi
          </div>
        </RouterLink>

        <RouterLink
          to="/stok/serial"
          class="rounded-xl border border-slate-200 bg-white p-5 shadow-sm transition hover:border-slate-300"
        >
          <div class="text-xs font-medium text-slate-500">Unit SN Tersedia</div>
          <div class="mt-2 font-mono text-2xl font-bold text-emerald-700">
            {{ summary.serial_units_available }}
          </div>
          <div class="mt-1 text-xs text-slate-600">
            dari {{ summary.serial_units_total }} unit aktif · Total tersedia:
            {{ summary.total_qty_available }}
          </div>
        </RouterLink>
      </section>

      <!-- Detail Grids -->
      <div class="mt-6 grid gap-6 lg:grid-cols-2">
        <!-- Peringatan Stok Menipis -->
        <section class="rounded-xl border border-slate-200 bg-white p-5 shadow-sm">
          <div class="flex items-center justify-between">
            <h2 class="text-sm font-semibold text-slate-900">Peringatan Stok Menipis</h2>
            <RouterLink to="/stok?low_stock=true" class="text-xs font-medium text-indigo-700 hover:underline">
              Lihat semua
            </RouterLink>
          </div>
          <p v-if="summary.low_stock_items.length === 0" class="mt-4 text-sm text-slate-500">
            Semua produk berada di atas batas stok minimum.
          </p>
          <ul v-else class="mt-3 divide-y divide-slate-100">
            <li
              v-for="it in summary.low_stock_items"
              :key="it.product_id"
              class="flex items-center justify-between gap-2 py-2.5 text-sm"
            >
              <div>
                <div class="font-medium text-slate-900">{{ it.product_name }}</div>
                <div class="font-mono text-xs text-slate-500">{{ it.sku }}</div>
              </div>
              <div class="text-right font-mono text-xs">
                <span class="font-bold text-rose-700">{{ it.qty_available }}</span>
                <span class="text-slate-500"> / min {{ it.min_stock_alert }} {{ it.unit }}</span>
              </div>
            </li>
          </ul>
        </section>

        <!-- Antrean LPB Menunggu Persetujuan -->
        <section class="rounded-xl border border-slate-200 bg-white p-5 shadow-sm">
          <div class="flex items-center justify-between">
            <h2 class="text-sm font-semibold text-slate-900">Antrean Persetujuan LPB</h2>
            <RouterLink
              :to="can('lpb.approve') ? '/lpb/approvals' : '/lpb'"
              class="text-xs font-medium text-indigo-700 hover:underline"
            >
              Buka antrean
            </RouterLink>
          </div>
          <p v-if="summary.recent_pending_lpbs.length === 0" class="mt-4 text-sm text-slate-500">
            Tidak ada LPB yang menunggu persetujuan saat ini.
          </p>
          <ul v-else class="mt-3 divide-y divide-slate-100">
            <li
              v-for="p in summary.recent_pending_lpbs"
              :key="p.id"
              class="flex items-center justify-between gap-2 py-2.5 text-sm"
            >
              <div>
                <RouterLink :to="`/lpb/${p.id}`" :class="rowLink" class="font-mono text-xs">
                  {{ p.lpb_number }}
                </RouterLink>
                <div class="text-xs text-slate-500">
                  {{ subTypeLabel(p.sub_type) }} · {{ p.warehouse_code }} · {{ p.created_by_name }}
                </div>
              </div>
              <span class="text-xs text-slate-500">
                {{ p.submitted_at ? formatDateTime(p.submitted_at) : '—' }}
              </span>
            </li>
          </ul>
        </section>
      </div>

      <!-- Mutasi Terbaru -->
      <section class="mt-6 rounded-xl border border-slate-200 bg-white p-5 shadow-sm">
        <div class="flex items-center justify-between">
          <h2 class="text-sm font-semibold text-slate-900">Mutasi Stok Terbaru</h2>
          <RouterLink
            v-if="can('ledger.read')"
            to="/stok/buku-besar"
            class="text-xs font-medium text-indigo-700 hover:underline"
          >
            Lihat buku besar
          </RouterLink>
        </div>
        <p v-if="summary.recent_movements.length === 0" class="mt-4 text-sm text-slate-500">
          Belum ada mutasi stok tercatat.
        </p>
        <div v-else class="mt-3 overflow-x-auto">
          <table class="min-w-full divide-y divide-slate-100 text-sm">
            <thead class="text-left text-xs text-slate-500">
              <tr>
                <th class="py-2 pr-3">Waktu</th>
                <th class="py-2 pr-3">LPB</th>
                <th class="py-2 pr-3">Produk</th>
                <th class="py-2 pr-3">Rak</th>
                <th class="py-2 text-right">Mutasi</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-100">
              <tr v-for="m in summary.recent_movements" :key="m.id">
                <td class="py-2 pr-3 text-xs text-slate-600">
                  {{ formatDateTime(m.occurred_at) }}
                </td>
                <td class="py-2 pr-3 font-mono text-xs">
                  <RouterLink :to="`/lpb/${m.lpb_id}`" :class="rowLink">
                    {{ m.lpb_number }}
                  </RouterLink>
                </td>
                <td class="py-2 pr-3 text-xs font-medium text-slate-900">
                  {{ m.sku }} — {{ m.product_name }}
                </td>
                <td class="py-2 pr-3 font-mono text-xs text-slate-600">
                  {{ m.warehouse_code }} / {{ m.rack_code }}
                </td>
                <td class="py-2 text-right">
                  <StatusBadge :value="m.direction" />
                  <span class="ml-1.5 font-mono text-xs font-semibold">
                    {{ m.direction === 'IN' ? `+${m.qty}` : `-${m.qty}` }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </template>
  </div>
</template>
