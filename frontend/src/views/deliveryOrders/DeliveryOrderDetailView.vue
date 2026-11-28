<script setup lang="ts">
// Pola halaman: Detail — Delivery Order, Reservasi Stok/SN, Release/Cancel, Buat Draft LPB (T2.6).
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import BackLink from '@/components/BackLink.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import SlideOver from '@/components/SlideOver.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { usePermission } from '@/composables/usePermission'
import { toApiError, userMessage } from '@/lib/apiError'
import { btnDanger, btnPrimary, btnRow, btnSecondary, rowLink } from '@/lib/ui'
import { deliveryOrderService } from '@/services/deliveryOrderService'
import { inventoryService } from '@/services/inventoryService'
import { rackService } from '@/services/masterService'
import type { DeliveryOrderDetail, DOItem } from '@/types/deliveryOrder'
import type { SerialItem } from '@/types/inventory'
import type { Rack } from '@/types/master'

const inputCls =
  'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 focus:border-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-500/20'

const route = useRoute()
const router = useRouter()
const toast = useToast()
const { can } = usePermission()

const doId = computed(() => Number(route.params.id))
const loading = ref(true)
const loadError = ref<string | null>(null)
const detail = ref<DeliveryOrderDetail | null>(null)
const racks = ref<Rack[]>([])

// SlideOver reservasi 1 item OUTBOUND
const reserveOpen = ref(false)
const reserveTarget = ref<DOItem | null>(null)
const reserveBusy = ref(false)
const reserveError = ref<string | null>(null)
const availableSerials = ref<SerialItem[]>([])
const loadingSerials = ref(false)
const reserveForm = reactive({
  rack_id: '' as number | '',
  qty: 1,
  serial_ids: [] as number[],
})

// SlideOver buat LPB untuk DO INBOUND (pilih rak tujuan retur)
const inboundLpbOpen = ref(false)
const inboundRackId = ref<number | ''>('')
const lpbBusy = ref(false)
const lpbError = ref<string | null>(null)

// Konfirmasi release / cancel
const confirmAction = ref<'release' | 'cancel' | null>(null)
const actionBusy = ref(false)

async function load(): Promise<void> {
  loading.value = true
  loadError.value = null
  try {
    const d = await deliveryOrderService.get(doId.value)
    detail.value = d
    const rRes = await rackService.list({
      warehouse_id: d.warehouse_id,
      is_active: 'true',
      limit: 100,
    })
    racks.value = rRes.items
  } catch (err) {
    loadError.value = userMessage(toApiError(err))
  } finally {
    loading.value = false
  }
}

const sellableRacks = computed(() =>
  racks.value.filter((r) => r.is_sellable && !r.locked_by_opname_id),
)

async function openReserve(item: DOItem): Promise<void> {
  reserveTarget.value = item
  reserveError.value = null
  reserveForm.rack_id = sellableRacks.value[0]?.id ?? ''
  reserveForm.qty = item.remaining_qty
  reserveForm.serial_ids = []
  availableSerials.value = []
  reserveOpen.value = true
  if (item.has_serial_number && reserveForm.rack_id) {
    await loadAvailableSerials(item.product_id, Number(reserveForm.rack_id))
  }
}

async function loadAvailableSerials(productId: number, rackId: number): Promise<void> {
  loadingSerials.value = true
  try {
    const res = await inventoryService.serials({
      product_id: productId,
      rack_id: rackId,
      status: 'AVAILABLE',
      limit: 100,
    })
    availableSerials.value = res.items
  } catch {
    availableSerials.value = []
  } finally {
    loadingSerials.value = false
  }
}

watch(
  () => reserveForm.rack_id,
  (rid) => {
    reserveForm.serial_ids = []
    if (reserveTarget.value?.has_serial_number && rid) {
      void loadAvailableSerials(reserveTarget.value.product_id, Number(rid))
    }
  },
)

function toggleSerial(id: number): void {
  const idx = reserveForm.serial_ids.indexOf(id)
  if (idx >= 0) {
    reserveForm.serial_ids.splice(idx, 1)
  } else if (
    reserveTarget.value &&
    reserveForm.serial_ids.length < reserveTarget.value.remaining_qty
  ) {
    reserveForm.serial_ids.push(id)
  }
}

async function submitReserve(): Promise<void> {
  if (!detail.value || !reserveTarget.value || !reserveForm.rack_id) {
    reserveError.value = 'Pilih rak terlebih dahulu.'
    return
  }
  reserveError.value = null
  const isSN = reserveTarget.value.has_serial_number
  const qty = isSN ? reserveForm.serial_ids.length : Number(reserveForm.qty)
  if (qty < 1) {
    reserveError.value = isSN ? 'Pilih minimal 1 serial number.' : 'Qty minimal 1.'
    return
  }

  reserveBusy.value = true
  try {
    detail.value = await deliveryOrderService.reserve(detail.value.id, [
      {
        do_item_id: reserveTarget.value.id,
        rack_id: Number(reserveForm.rack_id),
        qty,
        serial_ids: isSN ? [...reserveForm.serial_ids] : undefined,
      },
    ])
    reserveOpen.value = false
    toast.add({
      severity: 'success',
      summary: 'Berhasil',
      detail: `Reservasi ${reserveTarget.value.product_sku} (${qty} unit) berhasil`,
      life: 3500,
    })
  } catch (err) {
    reserveError.value = userMessage(toApiError(err))
  } finally {
    reserveBusy.value = false
  }
}

async function handleCreateLPB(): Promise<void> {
  if (!detail.value) return
  if (detail.value.direction === 'INBOUND') {
    lpbError.value = null
    inboundRackId.value = racks.value[0]?.id ?? ''
    inboundLpbOpen.value = true
    return
  }
  await executeCreateLPB(null)
}

async function executeCreateLPB(rackId: number | null): Promise<void> {
  if (!detail.value) return
  lpbBusy.value = true
  lpbError.value = null
  try {
    const created = await deliveryOrderService.createLPB(detail.value.id, { rack_id: rackId })
    inboundLpbOpen.value = false
    toast.add({
      severity: 'success',
      summary: 'Berhasil',
      detail: `Draft LPB ${created.lpb_number} berhasil dibuat`,
      life: 3500,
    })
    await router.push(`/lpb/${created.id}`)
  } catch (err) {
    const msg = userMessage(toApiError(err))
    if (inboundLpbOpen.value) lpbError.value = msg
    else toast.add({ severity: 'error', summary: 'Gagal', detail: msg, life: 5000 })
  } finally {
    lpbBusy.value = false
  }
}

async function runConfirmAction(): Promise<void> {
  if (!detail.value || !confirmAction.value) return
  actionBusy.value = true
  try {
    if (confirmAction.value === 'release') {
      detail.value = await deliveryOrderService.release(detail.value.id)
      toast.add({
        severity: 'success',
        summary: 'Berhasil',
        detail: 'Semua reservasi pada DO ini telah dilepas',
        life: 3500,
      })
    } else {
      detail.value = await deliveryOrderService.cancel(detail.value.id)
      toast.add({
        severity: 'success',
        summary: 'Berhasil',
        detail: 'Delivery Order dibatalkan dan reservasi dikembalikan',
        life: 3500,
      })
    }
    confirmAction.value = null
  } catch (err) {
    toast.add({
      severity: 'error',
      summary: 'Gagal',
      detail: userMessage(toApiError(err)),
      life: 5000,
    })
  } finally {
    actionBusy.value = false
  }
}

watch(doId, () => {
  void load()
})

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="space-y-6">
    <div class="space-y-2">
      <BackLink to="/delivery-orders" label="Daftar Delivery Order" />
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div class="flex flex-wrap items-center gap-3">
            <h1 class="font-mono text-2xl font-semibold text-slate-900">
              {{ detail ? detail.do_number : 'Detail Delivery Order' }}
            </h1>
            <StatusBadge v-if="detail" :value="detail.status" />
            <span
              v-if="detail"
              class="inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium"
              :class="
                detail.direction === 'OUTBOUND'
                  ? 'bg-indigo-50 text-indigo-700'
                  : 'bg-amber-50 text-amber-800'
              "
            >
              {{ detail.direction }}
            </span>
          </div>
          <p v-if="detail" class="mt-1 text-sm text-slate-600">
            SO:
            <RouterLink :to="`/sales-orders/${detail.so_id}`" :class="rowLink">
              {{ detail.so_number }}
            </RouterLink>
            · {{ detail.customer_name }} ({{ detail.site_name }}) · Gudang
            <strong>{{ detail.warehouse_code }}</strong>
          </p>
        </div>

        <div v-if="detail" class="flex flex-wrap items-center gap-2">
          <RouterLink
            v-if="detail.lpb_id && detail.lpb_number"
            :to="`/lpb/${detail.lpb_id}`"
            :class="btnPrimary"
          >
            Buka LPB {{ detail.lpb_number }}
          </RouterLink>

          <template v-if="can('do.reserve')">
            <button
              v-if="
                (detail.direction === 'OUTBOUND' && detail.status === 'RESERVED') ||
                (detail.direction === 'INBOUND' && detail.status === 'OPEN')
              "
              type="button"
              :disabled="lpbBusy"
              :class="btnPrimary"
              @click="handleCreateLPB"
            >
              {{ lpbBusy ? 'Membuat LPB…' : 'Buat Draft LPB' }}
            </button>
            <button
              v-if="
                detail.direction === 'OUTBOUND' &&
                detail.reserved_qty > 0 &&
                (detail.status === 'OPEN' || detail.status === 'RESERVED')
              "
              type="button"
              :class="btnSecondary"
              @click="confirmAction = 'release'"
            >
              Lepas Reservasi
            </button>
            <button
              v-if="detail.status === 'OPEN' || detail.status === 'RESERVED'"
              type="button"
              :class="btnDanger"
              @click="confirmAction = 'cancel'"
            >
              Batalkan DO
            </button>
          </template>
        </div>
      </div>
    </div>

    <div
      v-if="loading"
      class="rounded-xl border border-slate-200 bg-white p-8 text-center text-sm text-slate-500"
    >
      Memuat detail Delivery Order…
    </div>
    <div
      v-else-if="loadError"
      class="rounded-xl border border-rose-200 bg-rose-50 p-6 text-sm text-rose-800"
      role="alert"
    >
      {{ loadError }}
    </div>

    <template v-else-if="detail">
      <section class="overflow-x-auto rounded-xl border border-slate-200 bg-white">
        <div class="border-b border-slate-200 px-5 py-3">
          <h2 class="text-sm font-semibold text-slate-900">
            Item &amp; Reservasi Gudang ({{ detail.items.length }})
          </h2>
        </div>
        <table class="min-w-full divide-y divide-slate-200 text-sm">
          <thead
            class="bg-slate-50 text-left text-xs font-semibold tracking-wide text-slate-600 uppercase"
          >
            <tr>
              <th class="px-4 py-3">Produk</th>
              <th class="px-4 py-3 text-right">Diminta</th>
              <th class="px-4 py-3 text-right">Tereservasi</th>
              <th class="px-4 py-3">Detail Reservasi / Serial</th>
              <th class="px-4 py-3 text-right">Tindakan</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            <tr v-for="it in detail.items" :key="it.id">
              <td class="px-4 py-3">
                <div class="font-medium text-slate-900">{{ it.product_name }}</div>
                <div class="font-mono text-xs text-slate-500">
                  {{ it.product_sku }} · {{ it.has_serial_number ? 'Berserial' : 'Non-SN' }}
                </div>
              </td>
              <td class="px-4 py-3 text-right font-medium">{{ it.qty }}</td>
              <td class="px-4 py-3 text-right">
                <span
                  class="font-medium"
                  :class="it.reserved_qty >= it.qty ? 'text-emerald-700' : 'text-amber-700'"
                >
                  {{ it.reserved_qty }}
                </span>
              </td>
              <td class="px-4 py-3 text-xs text-slate-700">
                <div v-if="it.reservations.length > 0" class="space-y-1">
                  <div v-for="rs in it.reservations" :key="rs.id" class="font-mono">
                    Rak {{ rs.rack_code }} ·
                    <template v-if="rs.serial_number">
                      {{ rs.serial_number }} ({{ rs.asset_tag }})
                    </template>
                    <template v-else>{{ rs.qty }} unit</template>
                  </div>
                </div>
                <span v-else-if="it.so_serial_number" class="font-mono text-slate-600">
                  Target tarik: {{ it.so_serial_number }}
                </span>
                <span v-else class="text-slate-400">Belum direservasi</span>
              </td>
              <td class="px-4 py-3 text-right">
                <button
                  v-if="
                    detail.direction === 'OUTBOUND' &&
                    (detail.status === 'OPEN' || detail.status === 'RESERVED') &&
                    it.remaining_qty > 0 &&
                    can('do.reserve')
                  "
                  type="button"
                  :class="btnRow"
                  @click="openReserve(it)"
                >
                  Reservasi
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </section>
    </template>

    <!-- SlideOver Reservasi Item -->
    <SlideOver
      :open="reserveOpen"
      :title="`Reservasi: ${reserveTarget?.product_name ?? ''}`"
      @close="reserveOpen = false"
    >
      <form v-if="reserveTarget" class="space-y-4" @submit.prevent="submitReserve">
        <div
          v-if="reserveError"
          class="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-800"
          role="alert"
        >
          {{ reserveError }}
        </div>
        <p class="text-xs text-slate-600">
          Sisa kebutuhan: <strong>{{ reserveTarget.remaining_qty }} unit</strong>
        </p>

        <div>
          <label class="block text-xs font-medium text-slate-700" for="res-rack">
            Rak Asal (Sellable) <span class="text-rose-600">*</span>
          </label>
          <select
            id="res-rack"
            v-model="reserveForm.rack_id"
            required
            class="mt-1"
            :class="inputCls"
          >
            <option value="">Pilih rak…</option>
            <option v-for="r in sellableRacks" :key="r.id" :value="r.id">
              {{ r.code }} {{ r.zone ? `(${r.zone})` : '' }}
            </option>
          </select>
        </div>

        <div v-if="reserveTarget.has_serial_number" class="space-y-2">
          <div class="text-xs font-medium text-slate-700">
            Pilih Serial Number AVAILABLE ({{ reserveForm.serial_ids.length }} /
            {{ reserveTarget.remaining_qty }})
          </div>
          <div v-if="loadingSerials" class="text-xs text-slate-500">Memuat daftar unit…</div>
          <div
            v-else-if="availableSerials.length === 0"
            class="rounded-lg border border-dashed border-slate-300 p-3 text-xs text-slate-500"
          >
            Tidak ada unit AVAILABLE pada rak ini.
          </div>
          <div
            v-else
            class="max-h-60 space-y-1.5 overflow-y-auto rounded-lg border border-slate-200 p-2"
          >
            <label
              v-for="s in availableSerials"
              :key="s.id"
              class="flex cursor-pointer items-center gap-2 rounded px-2 py-1 text-xs hover:bg-slate-50"
            >
              <input
                type="checkbox"
                :checked="reserveForm.serial_ids.includes(s.id)"
                :disabled="
                  !reserveForm.serial_ids.includes(s.id) &&
                  reserveForm.serial_ids.length >= reserveTarget.remaining_qty
                "
                class="h-4 w-4 rounded border-slate-300 text-indigo-600"
                @change="toggleSerial(s.id)"
              />
              <span class="font-mono font-medium">{{ s.serial_number }}</span>
              <span class="font-mono text-slate-500">({{ s.asset_tag }})</span>
            </label>
          </div>
        </div>

        <div v-else>
          <label class="block text-xs font-medium text-slate-700" for="res-qty">
            Qty Reservasi <span class="text-rose-600">*</span>
          </label>
          <input
            id="res-qty"
            v-model.number="reserveForm.qty"
            type="number"
            min="1"
            :max="reserveTarget.remaining_qty"
            required
            class="mt-1"
            :class="inputCls"
          />
        </div>

        <div class="flex justify-end gap-2 pt-2">
          <button type="button" :class="btnSecondary" @click="reserveOpen = false">Batal</button>
          <button type="submit" :disabled="reserveBusy" :class="btnPrimary">
            {{ reserveBusy ? 'Menyimpan…' : 'Simpan Reservasi' }}
          </button>
        </div>
      </form>
    </SlideOver>

    <!-- SlideOver Buat LPB INBOUND -->
    <SlideOver
      :open="inboundLpbOpen"
      title="Buat Draft LPB Retur (INBOUND)"
      @close="inboundLpbOpen = false"
    >
      <form
        class="space-y-4"
        @submit.prevent="executeCreateLPB(inboundRackId ? Number(inboundRackId) : null)"
      >
        <div
          v-if="lpbError"
          class="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-800"
          role="alert"
        >
          {{ lpbError }}
        </div>
        <div>
          <label class="block text-xs font-medium text-slate-700" for="inb-rack">
            Rak Tujuan Penerimaan <span class="text-rose-600">*</span>
          </label>
          <select id="inb-rack" v-model="inboundRackId" required class="mt-1" :class="inputCls">
            <option value="">Pilih rak…</option>
            <option v-for="r in racks" :key="r.id" :value="r.id">
              {{ r.code }} {{ r.is_sellable ? '' : '(Karantina/Non-Sellable)' }}
            </option>
          </select>
        </div>
        <div class="flex justify-end gap-2">
          <button type="button" :class="btnSecondary" @click="inboundLpbOpen = false">Batal</button>
          <button type="submit" :disabled="lpbBusy" :class="btnPrimary">
            {{ lpbBusy ? 'Membuat…' : 'Buat Draft LPB' }}
          </button>
        </div>
      </form>
    </SlideOver>

    <ConfirmDialog
      :open="confirmAction !== null"
      :title="
        confirmAction === 'release'
          ? `Lepas semua reservasi DO '${detail?.do_number ?? ''}'?`
          : `Batalkan DO '${detail?.do_number ?? ''}'?`
      "
      :message="
        confirmAction === 'release'
          ? 'Seluruh stok dan serial yang direservasi akan dikembalikan ke status AVAILABLE.'
          : 'DO akan dibatalkan dan seluruh reservasi stok/serial akan dilepas otomatis.'
      "
      :confirm-label="confirmAction === 'release' ? 'Lepas Reservasi' : 'Batalkan DO'"
      :busy="actionBusy"
      @confirm="runConfirmAction"
      @cancel="confirmAction = null"
    />
  </div>
</template>
