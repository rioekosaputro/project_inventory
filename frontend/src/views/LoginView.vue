<script setup lang="ts">
// Pola halaman: Form (auth) — Modern Split-Screen 50:50 (Enterprise & Informative).
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useForm } from 'vee-validate'
import { toTypedSchema } from '@vee-validate/zod'
import { z } from 'zod'
import Button from 'primevue/button'
import Message from 'primevue/message'
import TextField from '@/components/TextField.vue'
import { toApiError } from '@/lib/apiError'
import { loginErrorMessage } from '@/lib/authMessages'
import { focusFirstInvalid } from '@/lib/focus'
import { homeFor, safeRedirectTarget } from '@/router/guards'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const highlights = [
  'Pelacakan Unit & Armada',
  'Manajemen Stok & Gudang',
  'Jadwal Sewa & Kalender',
] as const

const schema = toTypedSchema(
  z.object({
    email: z
      .string()
      .trim()
      .min(1, 'Masukkan email Anda, mis. nama@perusahaan.id')
      .email('Tulis email seperti nama@perusahaan.id')
      .max(120, 'Email maksimal 120 karakter'),
    password: z.string().min(1, 'Masukkan password Anda').max(72, 'Password maksimal 72 karakter'),
  }),
)

const { handleSubmit } = useForm({
  validationSchema: schema,
  initialValues: { email: '', password: '' },
})

const formEl = ref<HTMLFormElement | null>(null)
const submitting = ref(false)
const formError = ref('')

const submit = handleSubmit(
  async (values) => {
    if (submitting.value) return // cegah kirim ganda tanpa menonaktifkan tombol
    submitting.value = true
    formError.value = ''
    try {
      await auth.login(values.email, values.password)
      await router.replace(
        safeRedirectTarget(router, route.query.redirect) ??
          homeFor(router, auth.defaultHome, (p) => auth.can(p)),
      )
    } catch (e) {
      // Nilai yang sudah diisi dipertahankan agar pengguna tinggal memperbaiki.
      formError.value = loginErrorMessage(toApiError(e))
    } finally {
      submitting.value = false
    }
  },
  () => focusFirstInvalid(formEl.value),
)
</script>

<template>
  <div class="flex min-h-dvh w-full flex-col overflow-x-hidden lg:grid lg:grid-cols-2">
    <!-- Banner Mobile (< lg): identitas gradien indigo ringkas agar konsisten & ringan di HP -->
    <header
      class="relative overflow-hidden bg-gradient-to-br from-[#1b1947] via-[#272368] to-[#3b31a8] px-5 pt-7 pb-12 text-white lg:hidden"
    >
      <div
        aria-hidden="true"
        class="pointer-events-none absolute -top-12 -right-12 h-40 w-40 rounded-full bg-violet-400/20 blur-2xl"
      />
      <div class="relative z-10 mx-auto max-w-md">
        <div class="flex items-center gap-2 text-base font-semibold tracking-tight">
          <span
            class="flex h-8 w-8 items-center justify-center rounded-lg bg-white/10 ring-1 ring-white/20"
          >
            <i class="pi pi-box text-sm text-indigo-200" aria-hidden="true" />
          </span>
          <span>Rental ERP</span>
        </div>
        <p class="mt-3 text-lg leading-snug font-bold tracking-tight">
          Sistem Inventaris &amp; Manajemen Rental Terintegrasi
        </p>
        <p class="mt-1 text-xs leading-relaxed text-indigo-100/85">
          Kelola armada, stok, jadwal sewa, dan laporan operasional dalam satu platform.
        </p>
      </div>
    </header>

    <!-- Panel Kiri (Desktop >= lg): Identitas Enterprise, Value Proposition, & Ilustrasi Isometrik -->
    <section
      aria-label="Tentang Rental ERP"
      class="relative hidden flex-col justify-between overflow-hidden bg-gradient-to-br from-[#1b1947] via-[#272368] to-[#3b31a8] px-10 py-10 text-white lg:flex xl:px-14 xl:py-12"
    >
      <!-- Aksen cahaya gradien lembut -->
      <div
        aria-hidden="true"
        class="pointer-events-none absolute -top-24 -left-24 h-80 w-80 rounded-full bg-indigo-400/15 blur-3xl"
      />
      <div
        aria-hidden="true"
        class="pointer-events-none absolute -right-24 -bottom-24 h-96 w-96 rounded-full bg-violet-400/20 blur-3xl"
      />

      <div class="relative z-10">
        <div class="flex items-center gap-2.5 text-lg font-semibold tracking-tight">
          <span
            class="flex h-9 w-9 items-center justify-center rounded-lg bg-white/10 ring-1 ring-white/20"
          >
            <i class="pi pi-box text-indigo-200" aria-hidden="true" />
          </span>
          <span>Rental ERP</span>
        </div>

        <h2 class="mt-10 max-w-lg text-3xl leading-tight font-bold tracking-tight xl:text-4xl">
          Sistem Inventaris &amp; Manajemen Rental Terintegrasi
        </h2>
        <p class="mt-4 max-w-md text-sm leading-relaxed text-indigo-100/85 xl:text-base">
          Kelola armada, stok, jadwal sewa, dan laporan operasional dalam satu platform.
        </p>

        <ul class="mt-7 flex max-w-lg flex-wrap gap-2.5">
          <li
            v-for="item in highlights"
            :key="item"
            class="inline-flex items-center gap-2 rounded-full border border-white/15 bg-white/10 px-3.5 py-1.5 text-xs font-medium text-indigo-50 backdrop-blur-sm"
          >
            <i class="pi pi-check-circle text-[11px] text-indigo-200" aria-hidden="true" />
            <span>{{ item }}</span>
          </li>
        </ul>
      </div>

      <!-- Ilustrasi Isometrik Gudang, Armada, & Analitik -->
      <div class="relative z-10 mt-8 flex justify-center" aria-hidden="true">
        <svg
          viewBox="0 0 520 300"
          fill="none"
          xmlns="http://www.w3.org/2000/svg"
          class="h-auto w-full max-w-[460px] drop-shadow-2xl"
        >
          <defs>
            <linearGradient id="isoFloor" x1="0" y1="0" x2="1" y2="1">
              <stop offset="0%" stop-color="#818cf8" stop-opacity="0.22" />
              <stop offset="100%" stop-color="#c4b5fd" stop-opacity="0.04" />
            </linearGradient>
            <linearGradient id="cardGlass" x1="0" y1="0" x2="1" y2="1">
              <stop offset="0%" stop-color="#ffffff" stop-opacity="0.24" />
              <stop offset="100%" stop-color="#a5b4fc" stop-opacity="0.08" />
            </linearGradient>
            <linearGradient id="truckBody" x1="0" y1="0" x2="1" y2="1">
              <stop offset="0%" stop-color="#f8fafc" />
              <stop offset="100%" stop-color="#cbd5e1" />
            </linearGradient>
            <linearGradient id="rackFill" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stop-color="#e0e7ff" stop-opacity="0.92" />
              <stop offset="100%" stop-color="#818cf8" stop-opacity="0.45" />
            </linearGradient>
          </defs>

          <!-- Lantai isometrik & garis grid -->
          <polygon
            points="260,72 490,185 260,290 30,185"
            fill="url(#isoFloor)"
            stroke="#a5b4fc"
            stroke-opacity="0.25"
            stroke-width="1"
          />
          <path
            d="M115 145 L345 250 M175 115 L405 220 M375 128 L145 235 M430 156 L200 262"
            stroke="#c7d2fe"
            stroke-opacity="0.16"
            stroke-dasharray="4 4"
          />

          <!-- Panel Dashboard Analitik Melayang (Kiri Atas) -->
          <g transform="translate(48, 22)">
            <polygon
              points="0,42 130,0 130,86 0,128"
              fill="url(#cardGlass)"
              stroke="#e0e7ff"
              stroke-opacity="0.45"
              stroke-width="1.2"
            />
            <!-- Header titik -->
            <circle cx="14" cy="48" r="2.5" fill="#c7d2fe" />
            <circle cx="24" cy="45" r="2.5" fill="#a5b4fc" />
            <circle cx="34" cy="42" r="2.5" fill="#818cf8" />
            <!-- Chart batang isometrik -->
            <polygon points="18,104 28,101 28,74 18,77" fill="#c7d2fe" fill-opacity="0.85" />
            <polygon points="36,98 46,95 46,58 36,61" fill="#e0e7ff" fill-opacity="0.95" />
            <polygon points="54,92 64,89 64,66 54,69" fill="#a5b4fc" fill-opacity="0.85" />
            <polygon points="72,86 82,83 82,46 72,49" fill="#ffffff" fill-opacity="0.95" />
            <polygon points="90,80 100,77 100,54 90,57" fill="#c4b5fd" fill-opacity="0.9" />
            <!-- Garis tren -->
            <polyline
              points="23,72 41,56 59,62 77,42 95,48 114,34"
              stroke="#ffffff"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
            <circle cx="114" cy="34" r="3" fill="#ffffff" />
          </g>

          <!-- Rak Gudang Bertingkat (Kanan) -->
          <g transform="translate(306, 48)">
            <!-- Tiang vertikal rak -->
            <line x1="12" y1="22" x2="12" y2="128" stroke="#e0e7ff" stroke-width="2.5" />
            <line x1="78" y1="54" x2="78" y2="160" stroke="#e0e7ff" stroke-width="2.5" />
            <line x1="136" y1="26" x2="136" y2="132" stroke="#c7d2fe" stroke-width="2" />
            <line
              x1="70"
              y1="0"
              x2="70"
              y2="100"
              stroke="#c7d2fe"
              stroke-opacity="0.7"
              stroke-width="2"
            />

            <!-- Lapisan rak 3 (atas) -->
            <polygon
              points="12,32 70,6 136,38 78,64"
              fill="#c7d2fe"
              fill-opacity="0.35"
              stroke="#e0e7ff"
              stroke-width="1"
            />
            <!-- Boks rak atas -->
            <polygon points="28,28 48,19 64,27 44,36" fill="#ffffff" />
            <polygon points="28,28 44,36 44,50 28,42" fill="#c7d2fe" />
            <polygon points="44,36 64,27 64,41 44,50" fill="#a5b4fc" />

            <polygon points="74,26 96,16 114,25 92,35" fill="#f1f5f9" />
            <polygon points="74,26 92,35 92,50 74,41" fill="#cbd5e1" />
            <polygon points="92,35 114,25 114,40 92,50" fill="#94a3b8" />

            <!-- Lapisan rak 2 (tengah) -->
            <polygon
              points="12,72 70,46 136,78 78,104"
              fill="#c7d2fe"
              fill-opacity="0.35"
              stroke="#e0e7ff"
              stroke-width="1"
            />
            <polygon points="24,68 48,57 68,67 44,78" fill="#e0e7ff" />
            <polygon points="24,68 44,78 44,94 24,84" fill="#a5b4fc" />
            <polygon points="44,78 68,67 68,83 44,94" fill="#818cf8" />

            <polygon points="78,66 102,55 120,64 96,75" fill="#ffffff" />
            <polygon points="78,66 96,75 96,91 78,82" fill="#cbd5e1" />
            <polygon points="96,75 120,64 120,80 96,91" fill="#94a3b8" />

            <!-- Lapisan rak 1 (bawah) -->
            <polygon
              points="12,114 70,88 136,120 78,146"
              fill="url(#rackFill)"
              stroke="#e0e7ff"
              stroke-width="1"
            />
            <polygon points="42,108 68,96 90,107 64,119" fill="#ffffff" />
            <polygon points="42,108 64,119 64,135 42,124" fill="#c7d2fe" />
            <polygon points="64,119 90,107 90,123 64,135" fill="#818cf8" />
          </g>

          <!-- Truk Pengiriman Isometrik (Tengah) -->
          <g transform="translate(154, 124)">
            <!-- Bayangan truk -->
            <polygon points="10,74 106,28 156,52 60,98" fill="#0f172a" fill-opacity="0.32" />
            <!-- Box Kargo -->
            <polygon points="42,26 106,-4 146,16 82,46" fill="#ffffff" />
            <polygon points="42,26 82,46 82,82 42,62" fill="url(#truckBody)" />
            <polygon points="82,46 146,16 146,52 82,82" fill="#94a3b8" />
            <!-- Garis aksen kargo -->
            <line
              x1="90"
              y1="54"
              x2="138"
              y2="31"
              stroke="#6366f1"
              stroke-width="3"
              stroke-linecap="round"
            />
            <!-- Kabin depan -->
            <polygon points="14,50 42,36 74,52 46,66" fill="#e0e7ff" />
            <polygon points="14,50 46,66 46,88 14,72" fill="#cbd5e1" />
            <polygon points="46,66 74,52 74,74 46,88" fill="#818cf8" />
            <!-- Kaca depan -->
            <polygon points="20,52 41,42 54,48 33,58" fill="#312e81" />
            <!-- Roda -->
            <ellipse
              cx="32"
              cy="84"
              rx="7"
              ry="10"
              fill="#1e1b4b"
              stroke="#c7d2fe"
              stroke-width="2"
            />
            <ellipse
              cx="96"
              cy="76"
              rx="7"
              ry="10"
              fill="#1e1b4b"
              stroke="#c7d2fe"
              stroke-width="2"
            />
            <ellipse
              cx="128"
              cy="61"
              rx="7"
              ry="10"
              fill="#1e1b4b"
              stroke="#c7d2fe"
              stroke-width="2"
            />
          </g>

          <!-- Forklift & Palet Boks (Kiri Bawah) -->
          <g transform="translate(68, 178)">
            <ellipse cx="44" cy="64" rx="40" ry="14" fill="#0f172a" fill-opacity="0.28" />
            <!-- Badan forklift -->
            <polygon points="12,32 34,22 52,31 30,41" fill="#e0e7ff" />
            <polygon points="12,32 30,41 30,58 12,49" fill="#a5b4fc" />
            <polygon points="30,41 52,31 52,48 30,58" fill="#6366f1" />
            <!-- Tiang mast -->
            <line x1="50" y1="8" x2="50" y2="54" stroke="#f8fafc" stroke-width="3" />
            <line x1="56" y1="11" x2="56" y2="57" stroke="#c7d2fe" stroke-width="2.5" />
            <!-- Palet & Boks terangkat -->
            <polygon points="56,34 80,23 98,32 74,43" fill="#ffffff" />
            <polygon points="56,34 74,43 74,58 56,49" fill="#c7d2fe" />
            <polygon points="74,43 98,32 98,47 74,58" fill="#818cf8" />
            <!-- Roda -->
            <circle cx="22" cy="56" r="6" fill="#1e1b4b" stroke="#e0e7ff" stroke-width="2" />
            <circle cx="42" cy="56" r="5.5" fill="#1e1b4b" stroke="#e0e7ff" stroke-width="2" />
          </g>

          <!-- Unit Kendaraan Armada (Kanan Bawah) -->
          <g transform="translate(274, 202)">
            <ellipse cx="42" cy="44" rx="38" ry="13" fill="#0f172a" fill-opacity="0.28" />
            <polygon points="14,22 48,6 76,20 42,36" fill="#ffffff" />
            <polygon points="14,22 42,36 42,48 14,34" fill="#cbd5e1" />
            <polygon points="42,36 76,20 76,32 42,48" fill="#94a3b8" />
            <polygon points="24,18 46,8 62,16 40,26" fill="#312e81" />
            <circle cx="26" cy="40" r="5" fill="#1e1b4b" stroke="#e0e7ff" stroke-width="1.8" />
            <circle cx="62" cy="36" r="5" fill="#1e1b4b" stroke="#e0e7ff" stroke-width="1.8" />
          </g>
        </svg>
      </div>
    </section>

    <!-- Panel Kanan: Brand (desktop) & Kartu Form Login -->
    <div
      class="relative z-10 -mt-6 flex flex-1 flex-col items-center justify-start rounded-t-3xl bg-slate-50/70 px-4 pt-6 pb-10 sm:px-8 lg:mt-0 lg:justify-center lg:rounded-none lg:bg-white lg:py-10"
    >
      <div class="w-full max-w-md">
        <p
          class="mb-6 hidden items-center justify-center gap-2 text-lg font-semibold text-slate-900 lg:flex"
        >
          <i class="pi pi-box text-indigo-600" aria-hidden="true" />
          <span>Rental ERP</span>
        </p>

        <section class="rounded-2xl border border-slate-200/80 bg-white p-5 shadow-sm sm:p-8">
          <h1 class="text-xl font-bold text-slate-900">Masuk</h1>
          <p class="mt-1 text-sm text-slate-500">Gunakan akun yang diberikan administrator.</p>

          <form ref="formEl" class="mt-6 space-y-4" novalidate @submit.prevent="submit">
            <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>

            <TextField
              name="email"
              label="Email"
              type="email"
              autocomplete="username"
              inputmode="email"
              enterkeyhint="next"
              required
            />
            <TextField
              name="password"
              label="Password"
              type="password"
              autocomplete="current-password"
              enterkeyhint="go"
              required
              revealable
            />

            <Button
              type="submit"
              :label="submitting ? 'Memproses…' : 'Masuk'"
              :icon="submitting ? 'pi pi-spinner pi-spin' : undefined"
              :aria-busy="submitting"
              fluid
              class="min-h-11 bg-indigo-600! border-indigo-600! hover:bg-indigo-700!"
            />
          </form>

          <p class="mt-5 text-xs leading-relaxed text-slate-500">
            Lupa password? Hubungi administrator untuk mereset akun Anda.
          </p>
        </section>
      </div>
    </div>
  </div>
</template>
