<script setup lang="ts">
// Daftar Stock Opname + panel mulai opname baru (kunci rak & snapshot).
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import type { Column } from '@/components/DataTable.vue'
import type { FilterDef } from '@/components/FilterPanel.vue'
import ListPage from '@/components/ListPage.vue'
import SlideOver from '@/components/SlideOver.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { usePermission } from '@/composables/usePermission'
import { useServerTable } from '@/composables/useServerTable'
import { toApiError, userMessage } from '@/lib/apiError'
import { formatDateTime } from '@/lib/format'
import { btnPrimary, btnSecondary, rowLink } from '@/lib/ui'
import { rackService, warehouseService } from '@/services/masterService'
import { opnameService } from '@/services/opnameService'
import { useAuthStore } from '@/stores/auth'
import type { Rack, Warehouse } from '@/types/master'
import type { OpnameListItem } from '@/types/opname'

const router = useRouter()
const toast = useToast()
const auth = useAuthStore()
const { can } = usePermission()

const table = useServerTable<OpnameListItem>({
  fetch: (p) => opnameService.list(p),
  defaultSort: '-started_at',
  filterKeys: ['status'],
})

const columns: Column[] = [
  { key: 'opname_number', label: 'Nomor Opname', sortKey: 'opname_number' },
  { key: 'warehouse_code', label: 'Gudang', sortKey: 'warehouse' },
  { key: 'status', label: 'Status', sortKey: 'status' },
  { key: 'progress', label: 'Progres' },
  { key: 'started_at', label: 'Dimulai', sortKey: 'started_at' },
]

const filterDefs: FilterDef[] = [
  {
    key: 'status',
    label: 'Status',
    options: [
      { value: 'IN_PROGRESS', label: 'Sedang berjalan' },
      { value: 'SUBMITTED', label: 'Diajukan' },
      { value: 'FINALIZED', label: 'Difinalisasi' },
      { value: 'CANCELLED', label: 'Dibatalkan' },
    ],
  },
]

// ---- Panel mulai opname baru
const startOpen = ref(false)
const warehouses = ref<Warehouse[]>([])
const racks = ref<Rack[]>([])
const loadingRacks = ref(false)
const warehouseId = ref<number | ''>('')
const selectedRacks = ref<number[]>([])
const isBlindCount = ref(true)
const notes = ref('')
const formError = ref('')
const starting = ref(false)

const availableRacks = computed(() =>
  racks.value.filter((r) => r.is_active && r.locked_by_opname_id === null),
)

async function openStart(): Promise<void> {
  formError.value = ''
  selectedRacks.value = []
  isBlindCount.value = true
  notes.value = ''
  startOpen.value = true
  if (warehouses.value.length === 0) {
    try {
      const page = await warehouseService.list({ limit: 100, is_active: true, sort: 'code' })
      warehouses.value = page.items
    } catch (e) {
      formError.value = userMessage(toApiError(e))
    }
  }
  if (auth.user?.warehouse_id) {
    warehouseId.value = auth.user.warehouse_id
  } else if (warehouses.value.length === 1) {
    warehouseId.value = warehouses.value[0]!.id
  }
}

watch(warehouseId, async (wid) => {
  selectedRacks.value = []
  racks.value = []
  if (!wid) return
  loadingRacks.value = true
  try {
    const page = await rackService.list({
      limit: 100,
      warehouse_id: wid,
      is_active: true,
      sort: 'code',
    })
    racks.value = page.items
  } catch (e) {
    formError.value = userMessage(toApiError(e))
  } finally {
    loadingRacks.value = false
  }
})

function toggleRack(id: number): void {
  const idx = selectedRacks.value.indexOf(id)
  if (idx >= 0) selectedRacks.value.splice(idx, 1)
  else selectedRacks.value.push(id)
}

function selectAllRacks(): void {
  selectedRacks.value = availableRacks.value.map((r) => r.id)
}

async function submitStart(): Promise<void> {
  formError.value = ''
  if (!warehouseId.value) {
    formError.value = 'Pilih gudang terlebih dahulu.'
    return
  }
  if (selectedRacks.value.length === 0) {
    formError.value = 'Pilih minimal satu rak untuk di-opname.'
    return
  }
  starting.value = true
  try {
    const created = await opnameService.start({
      warehouse_id: Number(warehouseId.value),
      rack_ids: [...selectedRacks.value],
      is_blind_count: isBlindCount.value,
      notes: notes.value.trim() || null,
    })
    startOpen.value = false
    toast.add({
      severity: 'success',
      summary: `Stock Opname ${created.opname_number} dimulai`,
      life: 4000,
    })
    await router.push(`/opname/${created.id}`)
  } catch (e) {
    formError.value = userMessage(toApiError(e))
  } finally {
    starting.value = false
  }
}
</script>

<template>
  <ListPage
    title="Stock Opname"
    description="Penghitungan fisik stok per rak. Rak yang sedang opname dikunci sementara dari transaksi LPB hingga difinalisasi."
    entity="stock opname"
    empty-value="Mulai stock opname untuk mencocokkan stok fisik gudang dengan sistem."
    caption="Daftar stock opname"
    search-label="Cari nomor opname atau gudang"
    :columns="columns"
    :table="table"
    :filter-defs="filterDefs"
    create-label="Mulai Opname"
    :can-create="can('opname.create')"
    @create="openStart"
  >
    <template #cell-opname_number="{ row }">
      <RouterLink :to="`/opname/${row.id}`" :class="rowLink">{{ row.opname_number }}</RouterLink>
    </template>
    <template #cell-warehouse_code="{ row }">
      {{ row.warehouse_code }} — {{ row.warehouse_name }}
    </template>
    <template #cell-status="{ row }">
      <StatusBadge :value="row.status" />
    </template>
    <template #cell-progress="{ row }">
      <span class="text-xs text-slate-700">
        {{ row.rack_count }} rak · Non-SN {{ row.counted_count }}/{{ row.item_count }} · SN
        {{ row.found_count }}/{{ row.serial_count }}
      </span>
    </template>
    <template #cell-started_at="{ row }">
      <div class="text-xs">
        <div class="text-slate-900">{{ formatDateTime(row.started_at) }}</div>
        <div class="text-slate-500">{{ row.started_by_name }}</div>
      </div>
    </template>
  </ListPage>

  <SlideOver :open="startOpen" title="Mulai Stock Opname" @close="startOpen = false">
    <form class="flex flex-col gap-4" @submit.prevent="submitStart">
      <div
        v-if="formError"
        class="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-800"
        role="alert"
      >
        {{ formError }}
      </div>

      <div>
        <label for="opname-wh" class="block text-sm font-medium text-slate-800">Gudang</label>
        <select
          id="opname-wh"
          v-model="warehouseId"
          class="mt-1 w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm"
        >
          <option value="">Pilih gudang…</option>
          <option v-for="w in warehouses" :key="w.id" :value="w.id">
            {{ w.code }} — {{ w.name }}
          </option>
        </select>
      </div>

      <div>
        <div class="flex items-center justify-between">
          <span class="text-sm font-medium text-slate-800">Rak yang dihitung</span>
          <button
            v-if="availableRacks.length > 0"
            type="button"
            class="text-xs font-medium text-indigo-700 hover:underline"
            @click="selectAllRacks"
          >
            Pilih semua ({{ availableRacks.length }})
          </button>
        </div>
        <p v-if="loadingRacks" class="mt-2 text-xs text-slate-500">Memuat daftar rak…</p>
        <p v-else-if="warehouseId && racks.length === 0" class="mt-2 text-xs text-slate-500">
          Belum ada rak aktif di gudang ini.
        </p>
        <div v-else class="mt-2 grid max-h-56 grid-cols-2 gap-2 overflow-y-auto">
          <label
            v-for="rk in racks"
            :key="rk.id"
            class="flex cursor-pointer items-center gap-2 rounded-lg border px-3 py-2 text-sm"
            :class="
              rk.locked_by_opname_id
                ? 'cursor-not-allowed border-slate-200 bg-slate-50 text-slate-400'
                : selectedRacks.includes(rk.id)
                  ? 'border-indigo-500 bg-indigo-50 text-indigo-950'
                  : 'border-slate-200 bg-white text-slate-800'
            "
          >
            <input
              type="checkbox"
              :disabled="rk.locked_by_opname_id !== null"
              :checked="selectedRacks.includes(rk.id)"
              @change="toggleRack(rk.id)"
            />
            <span class="font-mono text-xs font-semibold">{{ rk.code }}</span>
          </label>
        </div>
      </div>

      <label class="flex items-start gap-2.5 text-sm text-slate-800">
        <input v-model="isBlindCount" type="checkbox" class="mt-0.5" />
        <span>
          <strong>Blind count</strong> — sembunyikan jumlah sistem dari staf gudang selama proses
          hitung agar hasil objektif.
        </span>
      </label>

      <div>
        <label for="opname-notes" class="block text-sm font-medium text-slate-800">
          Catatan (opsional)
        </label>
        <textarea
          id="opname-notes"
          v-model="notes"
          rows="2"
          class="mt-1 w-full rounded-lg border border-slate-300 px-3 py-2 text-sm"
          placeholder="Mis. Opname akhir bulan zona printer"
        />
      </div>

      <div class="mt-2 flex justify-end gap-2">
        <button type="button" :class="btnSecondary" @click="startOpen = false">Batal</button>
        <button type="submit" :class="btnPrimary" :aria-busy="starting">
          {{ starting ? 'Memulai…' : 'Mulai & Kunci Rak' }}
        </button>
      </div>
    </form>
  </SlideOver>
</template>
