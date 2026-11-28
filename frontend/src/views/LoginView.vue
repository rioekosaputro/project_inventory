<script setup lang="ts">
// Pola halaman: Form (auth) — Modern Split-Screen 50:50 (Enterprise & Informative).
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useForm } from 'vee-validate'
import { toTypedSchema } from '@vee-validate/zod'
import { z } from 'zod'
import Button from 'primevue/button'
import Message from 'primevue/message'
import illustration from '@/assets/login-illustration.svg'
import TextField from '@/components/TextField.vue'
import { toApiError } from '@/lib/apiError'
import { loginErrorMessage, sessionEndMessage } from '@/lib/authMessages'
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
  'Laporan & Analitik Real-Time',
] as const

const APP_VERSION = '1.0.0'
const year = new Date().getFullYear()
// Diisi saat pengguna dikembalikan ke sini karena batas sesi.
const sessionNotice = sessionEndMessage(route.query.reason)

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
  <div class="flex min-h-dvh w-full flex-col lg:grid lg:grid-cols-2">
    <!-- Identitas produk: header ringkas di HP, panel penuh 50% di layar lebar. -->
    <section
      aria-label="Tentang Rental ERP"
      class="flex flex-col bg-linear-to-br from-[#131a2e] via-[#221f56] to-[#352c85] px-5 py-6 text-white sm:px-8 lg:px-12 lg:py-10 xl:px-20 xl:py-14"
    >
      <div class="mx-auto flex w-full max-w-md flex-1 flex-col lg:max-w-xl">
        <div class="flex items-center gap-2.5 text-lg font-bold tracking-tight lg:text-xl">
          <i class="pi pi-box text-xl text-violet-400 lg:text-2xl" aria-hidden="true" />
          <span>Rental ERP</span>
        </div>

        <h2
          class="mt-4 text-xl leading-snug font-bold tracking-tight text-balance sm:text-2xl lg:mt-8 lg:tall:mt-12 lg:text-3xl lg:leading-tight xl:text-4xl"
        >
          Sistem Inventaris &amp; Manajemen Rental Terintegrasi
        </h2>
        <p class="mt-2 max-w-md text-sm leading-relaxed text-indigo-100/80 lg:mt-4 lg:text-base">
          Kelola armada, stok barang, jadwal sewa, dan laporan operasional dalam satu platform.
        </p>

        <ul
          class="mt-4 flex flex-wrap gap-2 lg:mt-7 lg:gap-2.5 lg:tall:flex-col lg:tall:items-start"
        >
          <li
            v-for="item in highlights"
            :key="item"
            class="inline-flex items-center gap-1.5 rounded-full border border-white/15 bg-white/10 px-3 py-1 text-xs font-medium text-indigo-50 lg:gap-2 lg:px-3.5 lg:py-1.5 lg:text-sm"
          >
            <i class="pi pi-check text-[10px] lg:text-xs" aria-hidden="true" />
            <span>{{ item }}</span>
          </li>
        </ul>

        <!-- Ilustrasi mengisi sisa tinggi; absolute agar tidak memaksa panel memanjang. -->
        <div class="relative mt-6 hidden min-h-56 flex-1 lg:block">
          <img
            :src="illustration"
            alt=""
            width="505"
            height="482"
            loading="lazy"
            class="absolute inset-0 h-full w-full object-contain"
          />
        </div>
      </div>
    </section>

    <!-- Form -->
    <div class="flex flex-1 flex-col items-center bg-slate-50 px-4 py-6 sm:px-8">
      <div class="flex w-full max-w-md flex-1 flex-col justify-center">
        <p
          class="mb-6 hidden items-center justify-center gap-2 text-lg font-bold text-slate-900 lg:flex"
        >
          <i class="pi pi-box text-indigo-600" aria-hidden="true" />
          <span>Rental ERP</span>
        </p>

        <section class="rounded-xl border border-slate-200 bg-white p-5 shadow-sm sm:p-8">
          <h1 class="text-xl font-bold text-slate-900">Masuk</h1>
          <p class="mt-1 text-sm text-slate-500">Gunakan akun yang diberikan administrator.</p>

          <form ref="formEl" class="mt-6 space-y-4" novalidate @submit.prevent="submit">
            <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
            <Message v-else-if="sessionNotice" severity="info" :closable="false">
              {{ sessionNotice }}
            </Message>

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

          <p class="mt-5 text-sm leading-relaxed text-slate-500">
            Lupa password? Hubungi administrator untuk mereset akun Anda.
          </p>
        </section>
      </div>

      <p class="mt-6 text-xs text-slate-500">
        &copy; {{ year }} Rental ERP &bull; Versi {{ APP_VERSION }}
      </p>
    </div>
  </div>
</template>
