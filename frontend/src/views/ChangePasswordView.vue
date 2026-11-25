<script setup lang="ts">
// Pola halaman: Form.
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useForm } from 'vee-validate'
import { toTypedSchema } from '@vee-validate/zod'
import { z } from 'zod'
import Button from 'primevue/button'
import Message from 'primevue/message'
import { useToast } from 'primevue/usetoast'
import TextField from '@/components/TextField.vue'
import { toApiError } from '@/lib/apiError'
import { rateLimitMessage } from '@/lib/authMessages'
import { focusFirstInvalid } from '@/lib/focus'
import { homeFor } from '@/router/guards'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()
const toast = useToast()

const schema = toTypedSchema(
  z
    .object({
      current_password: z
        .string()
        .min(1, 'Masukkan password Anda saat ini')
        .max(72, 'Maksimal 72 karakter'),
      new_password: z.string().min(8, 'Gunakan minimal 8 karakter').max(72, 'Maksimal 72 karakter'),
    })
    .superRefine((v, ctx) => {
      if (v.new_password && v.new_password === v.current_password) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['new_password'],
          message: 'Gunakan password yang berbeda dari password saat ini',
        })
      }
    }),
)

type FieldName = 'current_password' | 'new_password'
const FIELDS: readonly string[] = ['current_password', 'new_password']

const { handleSubmit, setFieldError, resetForm } = useForm({
  validationSchema: schema,
  initialValues: { current_password: '', new_password: '' },
})

const formEl = ref<HTMLFormElement | null>(null)
const submitting = ref(false)
const formError = ref('')

function serverFieldMessage(field: string, message: string): string {
  if (field === 'current_password')
    return 'Password saat ini salah. Periksa kembali lalu coba lagi.'
  return message.charAt(0).toUpperCase() + message.slice(1)
}

const submit = handleSubmit(
  async (values) => {
    if (submitting.value) return
    submitting.value = true
    formError.value = ''
    const wasForced = auth.mustChangePassword
    try {
      await auth.changePassword(values.current_password, values.new_password)
      toast.add({ severity: 'success', summary: 'Password berhasil diganti', life: 4000 })
      if (wasForced) {
        await router.replace(homeFor(router, auth.defaultHome, (p) => auth.can(p)))
      } else {
        resetForm()
      }
    } catch (e) {
      const err = toApiError(e)
      if (err.code === 'TOO_MANY_REQUESTS') {
        formError.value = rateLimitMessage(err)
      } else if (err.fieldErrors.length > 0) {
        for (const fe of err.fieldErrors) {
          if (FIELDS.includes(fe.field))
            setFieldError(fe.field as FieldName, serverFieldMessage(fe.field, fe.message))
        }
        await focusFirstInvalid(formEl.value)
      } else {
        formError.value = err.message
      }
    } finally {
      submitting.value = false
    }
  },
  () => focusFirstInvalid(formEl.value),
)
</script>

<template>
  <section class="max-w-lg">
    <h1 class="text-xl font-semibold text-slate-900">Ganti Password</h1>
    <p class="mt-1 text-sm text-slate-600">
      Setelah diganti, Anda tetap masuk di perangkat ini dan keluar dari perangkat lain.
    </p>

    <Message v-if="auth.mustChangePassword" severity="warn" :closable="false" class="mt-4">
      Anda wajib mengganti password sementara sebelum dapat memakai aplikasi.
    </Message>

    <form
      ref="formEl"
      class="mt-6 space-y-4 rounded-xl border border-slate-200 bg-white p-6 shadow-sm"
      novalidate
      @submit.prevent="submit"
    >
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>

      <TextField
        name="current_password"
        label="Password saat ini"
        type="password"
        autocomplete="current-password"
        enterkeyhint="next"
        required
        revealable
      />
      <TextField
        name="new_password"
        label="Password baru"
        type="password"
        autocomplete="new-password"
        enterkeyhint="done"
        hint="Minimal 8 karakter. Password manager di browser dapat membuatkan yang kuat."
        required
        revealable
      />

      <Button
        type="submit"
        :label="submitting ? 'Menyimpan…' : 'Simpan Password'"
        :icon="submitting ? 'pi pi-spinner pi-spin' : undefined"
        :aria-busy="submitting"
        class="min-h-11"
      />
    </form>
  </section>
</template>
