<script setup lang="ts">
// Pola halaman: Detail — Kontrak Sewa + Aksi Activate / Terminate / Renew (T2.3).
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import BackLink from '@/components/BackLink.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import SlideOver from '@/components/SlideOver.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { usePermission } from '@/composables/usePermission'
import { toApiError, userMessage } from '@/lib/apiError'
import { formatDateTime, formatRupiah } from '@/lib/format'
import { btnDanger, btnPrimary, btnSecondary, rowLink } from '@/lib/ui'
import { contractService } from '@/services/contractService'
import type { RentalContractDetail } from '@/types/contract'

const inputCls =
  'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 focus:border-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-500/20'

const route = useRoute()
const router = useRouter()
const toast = useToast()
const { can } = usePermission()

const contractId = computed(() => Number(route.params.id))
const loading = ref(true)
const loadError = ref<string | null>(null)
const contract = ref<RentalContractDetail | null>(null)

// Dialog aktivasi
const activateOpen = ref(false)
const activateBusy = ref(false)

// Slide-over terminasi
const terminateOpen = ref(false)
const terminateBusy = ref(false)
const terminateError = ref<string | null>(null)
const terminateForm = reactive({
  terminated_at: new Date().toISOString().slice(0, 10),
  termination_reason: '',
})

// Slide-over perpanjangan
const renewOpen = ref(false)
const renewBusy = ref(false)
const renewError = ref<string | null>(null)
const renewForm = reactive({
  duration_months: 12,
  start_date: '',
  end_date: '',
  notes: '',
})

async function load(): Promise<void> {
  loading.value = true
  loadError.value = null
  try {
    contract.value = await contractService.get(contractId.value)
  } catch (err) {
    loadError.value = userMessage(toApiError(err))
  } finally {
    loading.value = false
  }
}

async function confirmActivate(): Promise<void> {
  if (!contract.value) return
  activateBusy.value = true
  try {
    contract.value = await contractService.activate(contract.value.id)
    activateOpen.value = false
    toast.add({
      severity: 'success',
      summary: 'Berhasil',
      detail: `Kontrak ${contract.value.contract_number} telah diaktifkan`,
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
    activateBusy.value = false
  }
}

function openTerminate(): void {
  terminateError.value = null
  terminateForm.terminated_at = new Date().toISOString().slice(0, 10)
  terminateForm.termination_reason = ''
  terminateOpen.value = true
}

async function submitTerminate(): Promise<void> {
  if (!contract.value) return
  terminateError.value = null
  if (terminateForm.termination_reason.trim().length < 5) {
    terminateError.value = 'Alasan terminasi minimal 5 karakter.'
    return
  }
  terminateBusy.value = true
  try {
    contract.value = await contractService.terminate(contract.value.id, {
      terminated_at: terminateForm.terminated_at || null,
      termination_reason: terminateForm.termination_reason.trim(),
    })
    terminateOpen.value = false
    toast.add({
      severity: 'success',
      summary: 'Berhasil',
      detail: `Kontrak ${contract.value.contract_number} diterminasi`,
      life: 3500,
    })
  } catch (err) {
    terminateError.value = userMessage(toApiError(err))
  } finally {
    terminateBusy.value = false
  }
}

function openRenew(): void {
  if (!contract.value) return
  renewError.value = null
  renewForm.duration_months = contract.value.duration_months
  renewForm.start_date = ''
  renewForm.end_date = ''
  renewForm.notes = `Perpanjangan dari ${contract.value.contract_number}`
  renewOpen.value = true
}

async function submitRenew(): Promise<void> {
  if (!contract.value) return
  renewError.value = null
  renewBusy.value = true
  try {
    const created = await contractService.renew(contract.value.id, {
      duration_months: Number(renewForm.duration_months) || null,
      start_date: renewForm.start_date || null,
      end_date: renewForm.end_date || null,
      notes: renewForm.notes.trim() || null,
    })
    renewOpen.value = false
    toast.add({
      severity: 'success',
      summary: 'Berhasil',
      detail: `Draft perpanjangan ${created.contract_number} berhasil dibuat`,
      life: 3500,
    })
    await router.push(`/kontrak/${created.id}`)
  } catch (err) {
    renewError.value = userMessage(toApiError(err))
  } finally {
    renewBusy.value = false
  }
}

watch(contractId, () => {
  void load()
})

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="space-y-6">
    <div class="space-y-2">
      <BackLink to="/kontrak" label="Daftar kontrak" />
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div class="flex flex-wrap items-center gap-3">
            <h1 class="font-mono text-2xl font-semibold text-slate-900">
              {{ contract ? contract.contract_number : 'Detail Kontrak' }}
            </h1>
            <StatusBadge v-if="contract" :value="contract.status" />
          </div>
          <p v-if="contract" class="mt-1 text-sm text-slate-600">
            Pelanggan:
            <RouterLink :to="`/pelanggan/${contract.customer_id}`" :class="rowLink">
              {{ contract.customer_code }} — {{ contract.customer_name }}
            </RouterLink>
            <template v-if="contract.renewed_from_id && contract.renewed_from_number">
              · Perpanjangan dari
              <RouterLink :to="`/kontrak/${contract.renewed_from_id}`" :class="rowLink">
                {{ contract.renewed_from_number }}
              </RouterLink>
            </template>
          </p>
        </div>

        <div v-if="contract" class="flex flex-wrap items-center gap-2">
          <RouterLink
            v-if="contract.status === 'DRAFT' && can('contract.write')"
            :to="`/kontrak/${contract.id}/ubah`"
            :class="btnSecondary"
          >
            Ubah Draft
          </RouterLink>
          <button
            v-if="contract.status === 'DRAFT' && can('contract.approve')"
            type="button"
            :class="btnPrimary"
            @click="activateOpen = true"
          >
            Aktifkan Kontrak
          </button>
          <button
            v-if="
              (contract.status === 'ACTIVE' || contract.status === 'EXPIRED') &&
              can('contract.approve')
            "
            type="button"
            :class="btnPrimary"
            @click="openRenew"
          >
            Perpanjang Kontrak
          </button>
          <button
            v-if="contract.status === 'ACTIVE' && can('contract.approve')"
            type="button"
            :class="btnDanger"
            @click="openTerminate"
          >
            Terminasi
          </button>
        </div>
      </div>
    </div>

    <div
      v-if="loading"
      class="rounded-xl border border-slate-200 bg-white p-8 text-center text-sm text-slate-500"
    >
      Memuat detail kontrak…
    </div>
    <div
      v-else-if="loadError"
      class="rounded-xl border border-rose-200 bg-rose-50 p-6 text-sm text-rose-800"
      role="alert"
    >
      {{ loadError }}
    </div>

    <template v-else-if="contract">
      <!-- Banner terminasi -->
      <div
        v-if="contract.status === 'TERMINATED'"
        class="rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-900"
      >
        <div class="font-semibold">
          Kontrak diterminasi pada {{ contract.terminated_at ?? '—' }}
        </div>
        <p class="mt-1 text-rose-800">Alasan: {{ contract.termination_reason ?? '—' }}</p>
      </div>

      <!-- KPI -->
      <div class="grid gap-4 sm:grid-cols-4">
        <div class="rounded-xl border border-slate-200 bg-white p-4">
          <div class="text-xs font-medium text-slate-500">Estimasi Tagihan / Bulan</div>
          <div class="mt-1 text-xl font-semibold text-slate-900">
            {{ formatRupiah(contract.estimated_monthly) }}
          </div>
        </div>
        <div class="rounded-xl border border-slate-200 bg-white p-4">
          <div class="text-xs font-medium text-slate-500">Total Unit Kontrak</div>
          <div class="mt-1 text-xl font-semibold text-slate-900">{{ contract.total_qty }}</div>
        </div>
        <div class="rounded-xl border border-slate-200 bg-white p-4">
          <div class="text-xs font-medium text-slate-500">Unit Terpasang Aktif</div>
          <div class="mt-1 text-xl font-semibold text-slate-900">
            {{ contract.active_placement_qty }}
          </div>
        </div>
        <div class="rounded-xl border border-slate-200 bg-white p-4">
          <div class="text-xs font-medium text-slate-500">Deposit</div>
          <div class="mt-1 text-xl font-semibold text-slate-900">
            {{ formatRupiah(contract.deposit_amount) }}
          </div>
        </div>
      </div>

      <!-- Informasi periode & penagihan -->
      <div class="grid gap-6 md:grid-cols-2">
        <div class="space-y-3 rounded-xl border border-slate-200 bg-white p-5">
          <h2 class="text-sm font-semibold text-slate-900">Periode &amp; Status</h2>
          <dl class="divide-y divide-slate-100 text-sm">
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">Periode Sewa</dt>
              <dd class="font-medium text-slate-900">
                {{ contract.start_date }} s/d {{ contract.end_date }} ({{ contract.duration_months }}
                bulan)
              </dd>
            </div>
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">Dibuat oleh</dt>
              <dd class="text-slate-900">{{ contract.created_by_name }}</dd>
            </div>
            <div v-if="contract.activated_at" class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">Diaktifkan oleh</dt>
              <dd class="text-slate-900">
                {{ contract.activated_by_name ?? '—' }} ·
                {{ formatDateTime(contract.activated_at) }}
              </dd>
            </div>
            <div v-if="contract.notes" class="py-2">
              <dt class="text-slate-500">Catatan</dt>
              <dd class="mt-1 text-slate-900">{{ contract.notes }}</dd>
            </div>
          </dl>
        </div>

        <div class="space-y-3 rounded-xl border border-slate-200 bg-white p-5">
          <h2 class="text-sm font-semibold text-slate-900">Aturan Penagihan</h2>
          <dl class="divide-y divide-slate-100 text-sm">
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">Mode Tagihan</dt>
              <dd class="font-medium text-slate-900">{{ contract.billing_mode }}</dd>
            </div>
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">Tanggal Tagih</dt>
              <dd class="text-slate-900">Setiap tanggal {{ contract.billing_day_of_month }}</dd>
            </div>
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">PPN</dt>
              <dd class="text-slate-900">{{ contract.is_ppn ? 'Ya (PPN 12% DPP 11/12)' : 'Non-PPN' }}</dd>
            </div>
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">Termin Pembayaran</dt>
              <dd class="text-slate-900">{{ contract.payment_term_days }} hari</dd>
            </div>
          </dl>
        </div>
      </div>

      <!-- Tabel Item Tarif -->
      <section class="overflow-x-auto rounded-xl border border-slate-200 bg-white">
        <div class="border-b border-slate-200 px-5 py-3">
          <h2 class="text-sm font-semibold text-slate-900">
            Item Tarif Kontrak ({{ contract.items.length }})
          </h2>
        </div>
        <table class="min-w-full divide-y divide-slate-200 text-sm">
          <thead
            class="bg-slate-50 text-left text-xs font-semibold tracking-wide text-slate-600 uppercase"
          >
            <tr>
              <th class="px-4 py-3">Produk</th>
              <th class="px-4 py-3">Site Penempatan</th>
              <th class="px-4 py-3 text-right">Qty</th>
              <th class="px-4 py-3 text-right">Tarif / Bln</th>
              <th class="px-4 py-3 text-right">Subtotal / Bln</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            <tr v-for="it in contract.items" :key="it.id">
              <td class="px-4 py-3">
                <div class="font-medium text-slate-900">{{ it.product_name }}</div>
                <div class="font-mono text-xs text-slate-500">{{ it.product_sku }}</div>
              </td>
              <td class="px-4 py-3 text-slate-700">{{ it.site_name ?? 'Fleksibel / semua site' }}</td>
              <td class="px-4 py-3 text-right">{{ it.qty }}</td>
              <td class="px-4 py-3 text-right font-mono text-xs">
                {{ formatRupiah(it.monthly_rate) }}
              </td>
              <td class="px-4 py-3 text-right font-medium text-slate-900">
                {{ formatRupiah(it.line_monthly) }}
              </td>
            </tr>
          </tbody>
        </table>
      </section>
    </template>

    <!-- Konfirmasi Aktivasi -->
    <ConfirmDialog
      :open="activateOpen"
      :title="`Aktifkan kontrak '${contract?.contract_number ?? ''}'?`"
      message="Setelah aktif, detail tarif dan periode kontrak tidak dapat diubah lagi. Kontrak aktif siap digunakan untuk Sales Order dan penagihan bulanan."
      confirm-label="Aktifkan Kontrak"
      :busy="activateBusy"
      @confirm="confirmActivate"
      @cancel="activateOpen = false"
    />

    <!-- SlideOver Terminasi -->
    <SlideOver :open="terminateOpen" title="Terminasi Kontrak" @close="terminateOpen = false">
      <form class="space-y-4" @submit.prevent="submitTerminate">
        <div
          v-if="terminateError"
          class="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-800"
          role="alert"
        >
          {{ terminateError }}
        </div>
        <div>
          <label class="block text-xs font-medium text-slate-700" for="term-date">
            Tanggal Terminasi
          </label>
          <input
            id="term-date"
            v-model="terminateForm.terminated_at"
            type="date"
            class="mt-1"
            :class="inputCls"
          />
        </div>
        <div>
          <label class="block text-xs font-medium text-slate-700" for="term-reason">
            Alasan Terminasi <span class="text-rose-600">*</span>
          </label>
          <textarea
            id="term-reason"
            v-model="terminateForm.termination_reason"
            rows="3"
            required
            minlength="5"
            maxlength="500"
            placeholder="Jelaskan alasan penghentian kontrak…"
            class="mt-1"
            :class="inputCls"
          />
        </div>
        <div class="flex justify-end gap-2">
          <button type="button" :class="btnSecondary" @click="terminateOpen = false">Batal</button>
          <button type="submit" :disabled="terminateBusy" :class="btnDanger">
            {{ terminateBusy ? 'Memproses…' : 'Terminasi Kontrak' }}
          </button>
        </div>
      </form>
    </SlideOver>

    <!-- SlideOver Perpanjangan -->
    <SlideOver :open="renewOpen" title="Perpanjang Kontrak" @close="renewOpen = false">
      <form class="space-y-4" @submit.prevent="submitRenew">
        <p class="text-xs text-slate-600">
          Membuat kontrak <strong>DRAFT</strong> baru yang menyalin seluruh item tarif dari kontrak
          ini. Kosongkan tanggal mulai/berakhir untuk menyambung otomatis sehari setelah kontrak
          lama berakhir.
        </p>
        <div
          v-if="renewError"
          class="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-800"
          role="alert"
        >
          {{ renewError }}
        </div>
        <div>
          <label class="block text-xs font-medium text-slate-700" for="ren-dur">
            Durasi Baru (bulan)
          </label>
          <input
            id="ren-dur"
            v-model.number="renewForm.duration_months"
            type="number"
            min="1"
            max="120"
            required
            class="mt-1"
            :class="inputCls"
          />
        </div>
        <div class="grid gap-3 sm:grid-cols-2">
          <div>
            <label class="block text-xs font-medium text-slate-700" for="ren-start">
              Tanggal Mulai (opsional)
            </label>
            <input
              id="ren-start"
              v-model="renewForm.start_date"
              type="date"
              class="mt-1"
              :class="inputCls"
            />
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-700" for="ren-end">
              Tanggal Berakhir (opsional)
            </label>
            <input
              id="ren-end"
              v-model="renewForm.end_date"
              type="date"
              class="mt-1"
              :class="inputCls"
            />
          </div>
        </div>
        <div>
          <label class="block text-xs font-medium text-slate-700" for="ren-notes">Catatan</label>
          <textarea
            id="ren-notes"
            v-model="renewForm.notes"
            rows="2"
            maxlength="1000"
            class="mt-1"
            :class="inputCls"
          />
        </div>
        <div class="flex justify-end gap-2">
          <button type="button" :class="btnSecondary" @click="renewOpen = false">Batal</button>
          <button type="submit" :disabled="renewBusy" :class="btnPrimary">
            {{ renewBusy ? 'Membuat Draft…' : 'Buat Draft Perpanjangan' }}
          </button>
        </div>
      </form>
    </SlideOver>
  </div>
</template>
