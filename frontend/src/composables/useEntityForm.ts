import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter, type RouteLocationRaw } from 'vue-router'
import { useForm } from 'vee-validate'
import type { TypedSchema } from 'vee-validate'
import { useToast } from 'primevue/usetoast'
import { toApiError } from '@/lib/apiError'
import { focusFirstInvalid } from '@/lib/focus'

type Values = Record<string, unknown>

export interface EntityFormOptions<T, V extends Values> {
  schema: TypedSchema<V, V>
  /** Nilai awal untuk mode buat. */
  emptyValues: V
  /** Mengubah data dari API menjadi nilai form (mode ubah). */
  toValues: (entity: T) => V
  load: (id: number) => Promise<T>
  /** Simpan: id ada = ubah (PATCH), selain itu buat (POST). */
  save: (values: V, id: number | undefined) => Promise<T>
  /** Nama field form; galat server untuk field lain menjadi galat tingkat form. */
  fields: readonly string[]
  listRoute: RouteLocationRaw
  /** Tujuan setelah simpan bila bukan daftar (mis. halaman detail dokumen yang baru dibuat). */
  afterSave?: (saved: T, isEdit: boolean) => RouteLocationRaw
  savedMessage: (entity: T, isEdit: boolean) => string
  /** Terjemahan pesan galat server per field (opsional). */
  fieldMessage?: (field: string, message: string) => string
}

/**
 * Logika bersama form buat/ubah data master: memuat data (mode ubah), validasi blur-first,
 * pemetaan galat server (duplikat, aturan bisnis) ke field, lalu kembali ke daftar dengan toast.
 */
export function useEntityForm<T, V extends Values>(opts: EntityFormOptions<T, V>) {
  const route = useRoute()
  const router = useRouter()
  const toast = useToast()

  const id = computed(() => {
    const raw = route.params.id
    const n = typeof raw === 'string' ? Number(raw) : NaN
    return Number.isInteger(n) && n > 0 ? n : undefined
  })
  const isEdit = computed(() => id.value !== undefined)

  const form = useForm<V, V>({
    validationSchema: opts.schema,
    initialValues: opts.emptyValues as never,
  })

  const entity = ref<T | null>(null) as { value: T | null }
  const loading = ref(false)
  const loadError = ref('')
  const submitting = ref(false)
  const formError = ref('')

  async function loadEntity(): Promise<void> {
    if (id.value === undefined) return
    loading.value = true
    loadError.value = ''
    try {
      const data = await opts.load(id.value)
      entity.value = data
      form.resetForm({ values: opts.toValues(data) as never })
    } catch (e) {
      loadError.value = toApiError(e).message
    } finally {
      loading.value = false
    }
  }
  onMounted(loadEntity)

  const submit = form.handleSubmit(
    async (values) => {
      if (submitting.value) return
      submitting.value = true
      formError.value = ''
      try {
        const saved = await opts.save(values as V, id.value)
        toast.add({
          severity: 'success',
          summary: opts.savedMessage(saved, isEdit.value),
          life: 4000,
        })
        await router.push(opts.afterSave?.(saved, isEdit.value) ?? opts.listRoute)
      } catch (e) {
        const err = toApiError(e)
        const known = err.fieldErrors.filter((fe) => opts.fields.includes(fe.field))
        for (const fe of known) {
          const msg = opts.fieldMessage?.(fe.field, fe.message) ?? fe.message
          form.setFieldError(fe.field as never, msg.charAt(0).toUpperCase() + msg.slice(1))
        }
        if (known.length > 0) {
          await focusFirstInvalid(document.getElementById('entity-form'))
        } else {
          formError.value = err.message
        }
      } finally {
        submitting.value = false
      }
    },
    () => focusFirstInvalid(document.getElementById('entity-form')),
  )

  return { id, isEdit, entity, loading, loadError, submitting, formError, submit, form }
}
