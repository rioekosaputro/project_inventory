<script setup lang="ts">
// Pola halaman: Form (auth) — kartu di tengah, tanpa sidebar.
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
  <section class="rounded-xl border border-slate-200 bg-white p-6 shadow-sm md:p-8">
    <h1 class="text-xl font-semibold text-slate-900">Masuk</h1>
    <p class="mt-1 text-sm text-slate-600">Gunakan akun yang diberikan administrator.</p>

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
        class="min-h-11"
      />
    </form>

    <p class="mt-4 text-sm text-slate-600">
      Lupa password? Hubungi administrator untuk mereset akun Anda.
    </p>
  </section>
</template>
