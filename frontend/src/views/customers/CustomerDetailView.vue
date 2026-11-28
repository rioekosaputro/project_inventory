<script setup lang="ts">
import { vStackTable } from '@/directives/stackTable'
// Pola halaman: Detail — Profil Pelanggan, Cabang/Site (+ Leaflet picker), dan Aset Terpasang (T2.2).
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import BackLink from '@/components/BackLink.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import CoordinatePickerMap from '@/components/CoordinatePickerMap.vue'
import SlideOver from '@/components/SlideOver.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { usePermission } from '@/composables/usePermission'
import { toApiError, userMessage } from '@/lib/apiError'
import { btnPrimary, btnRow, btnSecondary, rowLink } from '@/lib/ui'
import { customerService } from '@/services/customerService'
import type { CustomerAsset, CustomerDetail, CustomerSite } from '@/types/customer'

const inputBase =
  'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 focus:border-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-500/20'

const route = useRoute()
const { can } = usePermission()
const toast = useToast()

function showSuccess(detail: string): void {
  toast.add({ severity: 'success', summary: 'Berhasil', detail, life: 3500 })
}
function showError(detail: string): void {
  toast.add({ severity: 'error', summary: 'Gagal', detail, life: 5000 })
}

const customerId = computed(() => Number(route.params.id))

const loading = ref(true)
const loadError = ref<string | null>(null)
const customer = ref<CustomerDetail | null>(null)

type TabKey = 'profile' | 'sites' | 'assets'
const activeTab = ref<TabKey>('sites')

const assetsLoading = ref(false)
const assets = ref<CustomerAsset[]>([])
const assetSiteFilter = ref<string>('')

// Slide-over tambah / ubah site
const siteFormOpen = ref(false)
const editingSite = ref<CustomerSite | null>(null)
const siteSubmitting = ref(false)
const siteFormError = ref<string | null>(null)

const siteForm = reactive({
  site_name: '',
  pic_name: '',
  pic_phone: '',
  address_line: '',
  subdistrict: '',
  district: '',
  city: '',
  province: '',
  postal_code: '',
  latitude: '' as string,
  longitude: '' as string,
  is_primary: false,
})

const mapLat = computed<number | null>({
  get: () => {
    if (siteForm.latitude.trim() === '') return null
    const n = Number(siteForm.latitude)
    return Number.isFinite(n) ? n : null
  },
  set: (v) => {
    siteForm.latitude = v === null ? '' : String(v)
  },
})

const mapLng = computed<number | null>({
  get: () => {
    if (siteForm.longitude.trim() === '') return null
    const n = Number(siteForm.longitude)
    return Number.isFinite(n) ? n : null
  },
  set: (v) => {
    siteForm.longitude = v === null ? '' : String(v)
  },
})

// Konfirmasi nonaktifkan/aktifkan site
const pendingToggleSite = ref<CustomerSite | null>(null)
const toggleBusy = ref(false)

async function loadCustomer(): Promise<void> {
  loading.value = true
  loadError.value = null
  try {
    customer.value = await customerService.get(customerId.value)
  } catch (err) {
    loadError.value = userMessage(toApiError(err))
  } finally {
    loading.value = false
  }
}

async function loadAssets(): Promise<void> {
  assetsLoading.value = true
  try {
    const sid = assetSiteFilter.value ? Number(assetSiteFilter.value) : undefined
    assets.value = await customerService.listAssets(customerId.value, sid)
  } catch (err) {
    showError(userMessage(toApiError(err)))
  } finally {
    assetsLoading.value = false
  }
}

function openCreateSite(): void {
  editingSite.value = null
  siteFormError.value = null
  const c = customer.value
  siteForm.site_name = ''
  siteForm.pic_name = c?.pic_name ?? ''
  siteForm.pic_phone = c?.phone ?? ''
  siteForm.address_line = ''
  siteForm.subdistrict = ''
  siteForm.district = ''
  siteForm.city = ''
  siteForm.province = ''
  siteForm.postal_code = ''
  siteForm.latitude = ''
  siteForm.longitude = ''
  // Bila belum ada site aktif sama sekali, sarankan sebagai site utama.
  siteForm.is_primary = (c?.sites.filter((s) => s.is_active).length ?? 0) === 0
  siteFormOpen.value = true
}

function openEditSite(site: CustomerSite): void {
  editingSite.value = site
  siteFormError.value = null
  siteForm.site_name = site.site_name
  siteForm.pic_name = site.pic_name
  siteForm.pic_phone = site.pic_phone
  siteForm.address_line = site.address_line
  siteForm.subdistrict = site.subdistrict ?? ''
  siteForm.district = site.district ?? ''
  siteForm.city = site.city
  siteForm.province = site.province
  siteForm.postal_code = site.postal_code
  siteForm.latitude = site.latitude === null ? '' : String(site.latitude)
  siteForm.longitude = site.longitude === null ? '' : String(site.longitude)
  siteForm.is_primary = site.is_primary
  siteFormOpen.value = true
}

async function submitSite(): Promise<void> {
  siteFormError.value = null
  const hasLat = siteForm.latitude.trim() !== ''
  const hasLng = siteForm.longitude.trim() !== ''
  if (hasLat !== hasLng) {
    siteFormError.value = 'Isi latitude dan longitude bersamaan, atau kosongkan keduanya.'
    return
  }
  const lat = hasLat ? Number(siteForm.latitude) : null
  const lng = hasLng ? Number(siteForm.longitude) : null
  if (lat !== null && (!Number.isFinite(lat) || lat < -90 || lat > 90)) {
    siteFormError.value = 'Latitude harus angka di antara -90 dan 90.'
    return
  }
  if (lng !== null && (!Number.isFinite(lng) || lng < -180 || lng > 180)) {
    siteFormError.value = 'Longitude harus angka di antara -180 dan 180.'
    return
  }

  siteSubmitting.value = true
  try {
    const payload = {
      site_name: siteForm.site_name.trim(),
      pic_name: siteForm.pic_name.trim(),
      pic_phone: siteForm.pic_phone.trim(),
      address_line: siteForm.address_line.trim(),
      subdistrict: siteForm.subdistrict.trim() || null,
      district: siteForm.district.trim() || null,
      city: siteForm.city.trim(),
      province: siteForm.province.trim(),
      postal_code: siteForm.postal_code.trim(),
      latitude: lat,
      longitude: lng,
      is_primary: siteForm.is_primary,
    }
    if (editingSite.value) {
      await customerService.updateSite(editingSite.value.id, payload)
      showSuccess(`Site '${payload.site_name}' diperbarui`)
    } else {
      await customerService.createSite(customerId.value, payload)
      showSuccess(`Site '${payload.site_name}' ditambahkan`)
    }
    siteFormOpen.value = false
    await loadCustomer()
  } catch (err) {
    siteFormError.value = userMessage(toApiError(err))
  } finally {
    siteSubmitting.value = false
  }
}

async function promotePrimary(site: CustomerSite): Promise<void> {
  try {
    await customerService.updateSite(site.id, { is_primary: true })
    showSuccess(`'${site.site_name}' ditetapkan sebagai site utama`)
    await loadCustomer()
  } catch (err) {
    showError(userMessage(toApiError(err)))
  }
}

async function confirmToggleSite(): Promise<void> {
  const target = pendingToggleSite.value
  if (!target) return
  toggleBusy.value = true
  try {
    await customerService.updateSite(target.id, { is_active: !target.is_active })
    showSuccess(`Site '${target.site_name}' ${target.is_active ? 'dinonaktifkan' : 'diaktifkan'}`)
    pendingToggleSite.value = null
    await loadCustomer()
  } catch (err) {
    showError(userMessage(toApiError(err)))
  } finally {
    toggleBusy.value = false
  }
}

watch(activeTab, (tab) => {
  if (tab === 'assets') {
    void loadAssets()
  }
})

watch(assetSiteFilter, () => {
  if (activeTab.value === 'assets') {
    void loadAssets()
  }
})

onMounted(() => {
  void loadCustomer()
})
</script>

<template>
  <div class="space-y-6">
    <div class="space-y-2">
      <BackLink to="/pelanggan" label="Daftar pelanggan" />
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 class="text-2xl font-semibold text-slate-900">
            {{ customer ? customer.company_name : 'Detail Pelanggan' }}
          </h1>
          <p v-if="customer" class="mt-1 text-sm text-slate-600">
            Kode: <span class="font-mono">{{ customer.customer_code }}</span> · PIC:
            {{ customer.pic_name }}
          </p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <RouterLink
            v-if="customer && can('customer.write')"
            :to="`/pelanggan/${customer.id}/ubah`"
            :class="btnSecondary"
          >
            Ubah Pelanggan
          </RouterLink>
          <button
            v-if="customer && can('customer.write')"
            type="button"
            :class="btnPrimary"
            @click="openCreateSite"
          >
            Tambah Site
          </button>
        </div>
      </div>
    </div>

    <div
      v-if="loading"
      class="rounded-xl border border-slate-200 bg-white p-8 text-center text-sm text-slate-500"
    >
      Memuat detail pelanggan…
    </div>
    <div
      v-else-if="loadError"
      class="rounded-xl border border-rose-200 bg-rose-50 p-6 text-sm text-rose-800"
      role="alert"
    >
      {{ loadError }}
    </div>

    <template v-else-if="customer">
      <!-- Ringkasan status & KPI kecil -->
      <div class="grid gap-4 sm:grid-cols-4">
        <div class="rounded-xl border border-slate-200 bg-white p-4">
          <div class="text-xs font-medium text-slate-500">Status Pelanggan</div>
          <div class="mt-2 flex items-center gap-2">
            <StatusBadge :value="customer.is_active ? 'ACTIVE' : 'INACTIVE'" />
            <span
              class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
              :class="customer.is_pkp ? 'bg-blue-50 text-blue-700' : 'bg-slate-100 text-slate-600'"
            >
              {{ customer.is_pkp ? 'PKP' : 'Non-PKP' }}
            </span>
          </div>
        </div>
        <div class="rounded-xl border border-slate-200 bg-white p-4">
          <div class="text-xs font-medium text-slate-500">Cabang / Site Aktif</div>
          <div class="mt-1 text-2xl font-semibold text-slate-900">{{ customer.site_count }}</div>
        </div>
        <div class="rounded-xl border border-slate-200 bg-white p-4">
          <div class="text-xs font-medium text-slate-500">Unit Sewa Terpasang</div>
          <div class="mt-1 text-2xl font-semibold text-slate-900">
            {{ customer.active_asset_count }}
          </div>
        </div>
        <div class="rounded-xl border border-slate-200 bg-white p-4">
          <div class="text-xs font-medium text-slate-500">Termin Pembayaran</div>
          <div class="mt-1 text-2xl font-semibold text-slate-900">
            {{ customer.payment_term_days }}
            <span class="text-sm font-normal text-slate-500">hari</span>
          </div>
        </div>
      </div>

      <!-- Navigasi tab -->
      <div class="border-b border-slate-200">
        <nav class="-mb-px flex gap-6" aria-label="Bagian detail pelanggan">
          <button
            type="button"
            class="border-b-2 px-1 pb-3 text-sm font-medium transition-colors"
            :class="
              activeTab === 'sites'
                ? 'border-blue-600 text-blue-700'
                : 'border-transparent text-slate-600 hover:border-slate-300 hover:text-slate-900'
            "
            @click="activeTab = 'sites'"
          >
            Cabang / Site ({{ customer.sites.length }})
          </button>
          <button
            type="button"
            class="border-b-2 px-1 pb-3 text-sm font-medium transition-colors"
            :class="
              activeTab === 'assets'
                ? 'border-blue-600 text-blue-700'
                : 'border-transparent text-slate-600 hover:border-slate-300 hover:text-slate-900'
            "
            @click="activeTab = 'assets'"
          >
            Aset Terpasang ({{ customer.active_asset_count }})
          </button>
          <button
            type="button"
            class="border-b-2 px-1 pb-3 text-sm font-medium transition-colors"
            :class="
              activeTab === 'profile'
                ? 'border-blue-600 text-blue-700'
                : 'border-transparent text-slate-600 hover:border-slate-300 hover:text-slate-900'
            "
            @click="activeTab = 'profile'"
          >
            Profil &amp; Perpajakan
          </button>
        </nav>
      </div>

      <!-- TAB: Cabang / Site -->
      <section v-if="activeTab === 'sites'" class="space-y-4">
        <div
          v-if="customer.sites.length === 0"
          class="rounded-xl border border-dashed border-slate-300 bg-white p-8 text-center"
        >
          <p class="text-sm text-slate-600">Belum ada cabang / site untuk pelanggan ini.</p>
          <button
            v-if="can('customer.write')"
            type="button"
            class="mt-3"
            :class="btnPrimary"
            @click="openCreateSite"
          >
            Tambah Site Pertama
          </button>
        </div>

        <div v-else class="grid gap-4 md:grid-cols-2">
          <article
            v-for="site in customer.sites"
            :key="site.id"
            class="space-y-3 rounded-xl border bg-white p-5 shadow-xs"
            :class="site.is_primary ? 'border-blue-300 ring-1 ring-blue-100' : 'border-slate-200'"
          >
            <div class="flex items-start justify-between gap-3">
              <div>
                <div class="flex flex-wrap items-center gap-2">
                  <h3 class="text-base font-semibold text-slate-900">{{ site.site_name }}</h3>
                  <span
                    v-if="site.is_primary"
                    class="inline-flex items-center rounded-full bg-blue-100 px-2.5 py-0.5 text-xs font-medium text-blue-800"
                  >
                    Site Utama
                  </span>
                  <StatusBadge :value="site.is_active ? 'ACTIVE' : 'INACTIVE'" />
                </div>
                <p class="mt-1 text-xs text-slate-500">
                  PIC: {{ site.pic_name }} · {{ site.pic_phone }}
                </p>
              </div>
            </div>

            <p class="text-sm text-slate-700">
              {{ site.address_line }}
              <span v-if="site.subdistrict || site.district" class="block text-xs text-slate-500">
                {{ [site.subdistrict, site.district].filter(Boolean).join(', ') }}
              </span>
              <span class="block text-xs text-slate-500">
                {{ site.city }}, {{ site.province }} {{ site.postal_code }}
              </span>
            </p>

            <div
              class="flex flex-wrap items-center justify-between gap-2 border-t border-slate-100 pt-3 text-xs"
            >
              <div class="text-slate-600">
                <template v-if="site.latitude !== null && site.longitude !== null">
                  <a
                    :href="`https://www.openstreetmap.org/?mlat=${site.latitude}&mlon=${site.longitude}#map=16/${site.latitude}/${site.longitude}`"
                    target="_blank"
                    rel="noopener noreferrer"
                    class="font-mono text-blue-700 hover:underline"
                  >
                    {{ site.latitude.toFixed(6) }}, {{ site.longitude.toFixed(6) }}
                  </a>
                </template>
                <span v-else class="text-slate-400">Koordinat belum diatur</span>
              </div>

              <div v-if="can('customer.write')" class="flex flex-wrap items-center gap-2">
                <button
                  v-if="site.is_active && !site.is_primary"
                  type="button"
                  :class="btnRow"
                  @click="promotePrimary(site)"
                >
                  Jadikan Utama
                </button>
                <button type="button" :class="btnRow" @click="openEditSite(site)">Ubah</button>
                <button type="button" :class="btnRow" @click="pendingToggleSite = site">
                  {{ site.is_active ? 'Nonaktifkan' : 'Aktifkan' }}
                </button>
              </div>
            </div>
          </article>
        </div>
      </section>

      <!-- TAB: Aset Terpasang -->
      <section v-else-if="activeTab === 'assets'" class="space-y-4">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <p class="text-sm text-slate-600">
            Unit berserial berstatus <strong>RENTED</strong> yang sedang terpasang di lokasi
            pelanggan.
          </p>
          <select
            v-model="assetSiteFilter"
            aria-label="Filter aset berdasarkan site"
            class="rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-sm text-slate-800"
          >
            <option value="">Semua site</option>
            <option v-for="s in customer.sites" :key="s.id" :value="String(s.id)">
              {{ s.site_name }}
            </option>
          </select>
        </div>

        <div class="overflow-x-auto rounded-xl border border-slate-200 bg-white">
          <table v-stack-table class="min-w-full divide-y divide-slate-200 text-sm">
            <thead
              class="bg-slate-50 text-left text-xs font-semibold tracking-wide text-slate-600 uppercase"
            >
              <tr>
                <th class="px-4 py-3">Serial Number</th>
                <th class="px-4 py-3">Asset Tag</th>
                <th class="px-4 py-3">Produk</th>
                <th class="px-4 py-3">Site Penempatan</th>
                <th class="px-4 py-3">Kontrak</th>
                <th class="px-4 py-3">Kondisi</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-100">
              <tr v-if="assetsLoading">
                <td colspan="6" class="px-4 py-6 text-center text-slate-500">
                  Memuat aset terpasang…
                </td>
              </tr>
              <tr v-else-if="assets.length === 0">
                <td colspan="6" class="px-4 py-6 text-center text-slate-500">
                  Belum ada unit sewa aktif yang terpasang.
                </td>
              </tr>
              <tr v-for="a in assets" :key="a.serial_id" class="hover:bg-slate-50/70">
                <td class="px-4 py-3 font-mono text-xs">
                  <RouterLink :to="`/stok/serial/${a.serial_id}`" :class="rowLink">
                    {{ a.serial_number }}
                  </RouterLink>
                </td>
                <td class="px-4 py-3 font-mono text-xs text-slate-700">{{ a.asset_tag }}</td>
                <td class="px-4 py-3">
                  <div class="font-medium text-slate-900">{{ a.product_name }}</div>
                  <div class="text-xs text-slate-500">{{ a.product_sku }}</div>
                </td>
                <td class="px-4 py-3 text-slate-700">
                  {{ a.site_name ?? '—' }}
                  <span v-if="a.site_city" class="block text-xs text-slate-500">{{
                    a.site_city
                  }}</span>
                </td>
                <td class="px-4 py-3 font-mono text-xs text-slate-700">
                  {{ a.contract_number ?? '—' }}
                </td>
                <td class="px-4 py-3">
                  <StatusBadge :value="a.condition" />
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <!-- TAB: Profil & Perpajakan -->
      <section v-else class="grid gap-6 md:grid-cols-2">
        <div class="space-y-3 rounded-xl border border-slate-200 bg-white p-5">
          <h3 class="text-sm font-semibold text-slate-900">Identitas &amp; Kontak</h3>
          <dl class="divide-y divide-slate-100 text-sm">
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">Kode Pelanggan</dt>
              <dd class="font-mono font-medium text-slate-900">{{ customer.customer_code }}</dd>
            </div>
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">Perusahaan</dt>
              <dd class="text-right font-medium text-slate-900">{{ customer.company_name }}</dd>
            </div>
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">PIC</dt>
              <dd class="text-slate-900">{{ customer.pic_name }}</dd>
            </div>
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">Telepon</dt>
              <dd class="text-slate-900">{{ customer.phone }}</dd>
            </div>
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">Email</dt>
              <dd class="text-slate-900">{{ customer.email ?? '—' }}</dd>
            </div>
          </dl>
        </div>

        <div class="space-y-3 rounded-xl border border-slate-200 bg-white p-5">
          <h3 class="text-sm font-semibold text-slate-900">Perpajakan &amp; Penagihan</h3>
          <dl class="divide-y divide-slate-100 text-sm">
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">Status PKP</dt>
              <dd class="font-medium text-slate-900">{{ customer.is_pkp ? 'PKP' : 'Non-PKP' }}</dd>
            </div>
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">NPWP</dt>
              <dd class="font-mono text-slate-900">{{ customer.npwp ?? '—' }}</dd>
            </div>
            <div class="py-2">
              <dt class="text-slate-500">Alamat Pajak</dt>
              <dd class="mt-1 text-slate-900">{{ customer.tax_address ?? '—' }}</dd>
            </div>
            <div class="py-2">
              <dt class="text-slate-500">Alamat Penagihan</dt>
              <dd class="mt-1 text-slate-900">
                {{ customer.billing_address }} ({{ customer.billing_postal_code }})
              </dd>
            </div>
            <div class="flex justify-between gap-4 py-2">
              <dt class="text-slate-500">Email Invoice</dt>
              <dd class="text-slate-900">{{ customer.billing_email ?? '—' }}</dd>
            </div>
          </dl>
        </div>
      </section>
    </template>

    <!-- SlideOver Form Tambah / Ubah Site + Leaflet Map Picker -->
    <SlideOver
      :open="siteFormOpen"
      :title="editingSite ? `Ubah Site: ${editingSite.site_name}` : 'Tambah Cabang / Site'"
      @close="siteFormOpen = false"
    >
      <form class="space-y-4" @submit.prevent="submitSite">
        <div
          v-if="siteFormError"
          class="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-800"
          role="alert"
        >
          {{ siteFormError }}
        </div>

        <div>
          <label class="block text-xs font-medium text-slate-700" for="site-name">
            Nama Site <span class="text-rose-600">*</span>
          </label>
          <input
            id="site-name"
            v-model="siteForm.site_name"
            type="text"
            required
            maxlength="100"
            placeholder="Mis. Kantor Pusat / Cabang Pemuda"
            class="mt-1"
            :class="inputBase"
          />
        </div>

        <div class="grid gap-3 sm:grid-cols-2">
          <div>
            <label class="block text-xs font-medium text-slate-700" for="site-pic">
              PIC Site <span class="text-rose-600">*</span>
            </label>
            <input
              id="site-pic"
              v-model="siteForm.pic_name"
              type="text"
              required
              maxlength="100"
              class="mt-1"
              :class="inputBase"
            />
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-700" for="site-phone">
              Telepon PIC <span class="text-rose-600">*</span>
            </label>
            <input
              id="site-phone"
              v-model="siteForm.pic_phone"
              type="text"
              required
              maxlength="30"
              inputmode="tel"
              class="mt-1"
              :class="inputBase"
            />
          </div>
        </div>

        <div>
          <label class="block text-xs font-medium text-slate-700" for="site-addr">
            Alamat Lengkap <span class="text-rose-600">*</span>
          </label>
          <textarea
            id="site-addr"
            v-model="siteForm.address_line"
            rows="2"
            required
            maxlength="500"
            class="mt-1"
            :class="inputBase"
          />
        </div>

        <div class="grid gap-3 sm:grid-cols-2">
          <div>
            <label class="block text-xs font-medium text-slate-700" for="site-subdistrict">
              Kelurahan / Desa
            </label>
            <input
              id="site-subdistrict"
              v-model="siteForm.subdistrict"
              type="text"
              maxlength="80"
              class="mt-1"
              :class="inputBase"
            />
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-700" for="site-district">
              Kecamatan
            </label>
            <input
              id="site-district"
              v-model="siteForm.district"
              type="text"
              maxlength="80"
              class="mt-1"
              :class="inputBase"
            />
          </div>
        </div>

        <div class="grid gap-3 sm:grid-cols-3">
          <div>
            <label class="block text-xs font-medium text-slate-700" for="site-city">
              Kota / Kab <span class="text-rose-600">*</span>
            </label>
            <input
              id="site-city"
              v-model="siteForm.city"
              type="text"
              required
              maxlength="80"
              class="mt-1"
              :class="inputBase"
            />
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-700" for="site-province">
              Provinsi <span class="text-rose-600">*</span>
            </label>
            <input
              id="site-province"
              v-model="siteForm.province"
              type="text"
              required
              maxlength="80"
              class="mt-1"
              :class="inputBase"
            />
          </div>
          <div>
            <label class="block text-xs font-medium text-slate-700" for="site-postal">
              Kode Pos <span class="text-rose-600">*</span>
            </label>
            <input
              id="site-postal"
              v-model="siteForm.postal_code"
              type="text"
              required
              maxlength="10"
              class="mt-1"
              :class="inputBase"
            />
          </div>
        </div>

        <!-- Picker Koordinat Peta Leaflet -->
        <div class="space-y-3 rounded-lg border border-slate-200 bg-slate-50/70 p-3">
          <CoordinatePickerMap
            v-if="siteFormOpen"
            v-model:latitude="mapLat"
            v-model:longitude="mapLng"
            height="220px"
          />
          <div class="grid gap-3 sm:grid-cols-2">
            <div>
              <label class="block text-xs font-medium text-slate-700" for="site-lat"
                >Latitude</label
              >
              <input
                id="site-lat"
                v-model="siteForm.latitude"
                type="text"
                inputmode="decimal"
                placeholder="-6.9825000"
                class="mt-1 font-mono text-xs"
                :class="inputBase"
              />
            </div>
            <div>
              <label class="block text-xs font-medium text-slate-700" for="site-lng"
                >Longitude</label
              >
              <input
                id="site-lng"
                v-model="siteForm.longitude"
                type="text"
                inputmode="decimal"
                placeholder="110.4091000"
                class="mt-1 font-mono text-xs"
                :class="inputBase"
              />
            </div>
          </div>
        </div>

        <label class="flex items-center gap-2 text-sm text-slate-800">
          <input
            v-model="siteForm.is_primary"
            type="checkbox"
            class="h-4 w-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500"
          />
          Jadikan sebagai Site Utama (menggantikan site utama sebelumnya)
        </label>

        <div class="flex justify-end gap-2 pt-2">
          <button type="button" :class="btnSecondary" @click="siteFormOpen = false">Batal</button>
          <button type="submit" :disabled="siteSubmitting" :class="btnPrimary">
            {{ siteSubmitting ? 'Menyimpan…' : editingSite ? 'Simpan Perubahan' : 'Tambah Site' }}
          </button>
        </div>
      </form>
    </SlideOver>

    <ConfirmDialog
      :open="pendingToggleSite !== null"
      :title="`${pendingToggleSite?.is_active ? 'Nonaktifkan' : 'Aktifkan'} site '${pendingToggleSite?.site_name ?? ''}'?`"
      :message="
        pendingToggleSite?.is_active
          ? 'Site yang masih memiliki unit sewa aktif tidak dapat dinonaktifkan.'
          : 'Site akan diaktifkan kembali agar dapat dipilih untuk pengiriman dan penempatan unit.'
      "
      :confirm-label="pendingToggleSite?.is_active ? 'Nonaktifkan Site' : 'Aktifkan Site'"
      :busy="toggleBusy"
      @confirm="confirmToggleSite"
      @cancel="pendingToggleSite = null"
    />
  </div>
</template>
