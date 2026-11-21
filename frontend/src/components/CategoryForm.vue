<script setup lang="ts">
// Form singkat (2 field) di dalam SlideOver: buat atau ubah kategori.
import { ref } from 'vue'
import { useForm } from 'vee-validate'
import { toTypedSchema } from '@vee-validate/zod'
import { z } from 'zod'
import Message from 'primevue/message'
import { useToast } from 'primevue/usetoast'
import SelectField from '@/components/SelectField.vue'
import TextField from '@/components/TextField.vue'
import { toApiError } from '@/lib/apiError'
import { focusFirstInvalid } from '@/lib/focus'
import { btnPrimary, btnSecondary } from '@/lib/ui'
import { requiredText } from '@/lib/validation'
import { categoryService } from '@/services/masterService'
import type { Category, ItemType } from '@/types/master'

const props = defineProps<{ category: Category | null }>()
const emit = defineEmits<{ saved: []; cancel: [] }>()

const schema = z.object({
  name: requiredText('Nama kategori', 100),
  item_type: z.enum(['ASSET', 'CONSUMABLE'], {
    errorMap: () => ({ message: 'Pilih jenis barang' }),
  }),
})

const { handleSubmit, setFieldError } = useForm({
  validationSchema: toTypedSchema(schema),
  initialValues: {
    name: props.category?.name ?? '',
    item_type: (props.category?.item_type ?? '') as ItemType,
  },
})

const toast = useToast()
const root = ref<HTMLFormElement | null>(null)
const submitting = ref(false)
const formError = ref('')

const itemTypeOptions = [
  { value: 'ASSET', label: 'Aset (disewakan / dilacak per unit)' },
  { value: 'CONSUMABLE', label: 'Habis pakai (dikelola per jumlah)' },
]

const submit = handleSubmit(
  async (values) => {
    if (submitting.value) return
    submitting.value = true
    formError.value = ''
    try {
      const saved = props.category
        ? await categoryService.update(props.category.id, values)
        : await categoryService.create(values)
      toast.add({
        severity: 'success',
        summary: `Kategori ${saved.name} ${props.category ? 'diperbarui' : 'ditambahkan'}`,
        life: 4000,
      })
      emit('saved')
    } catch (e) {
      const err = toApiError(e)
      const known = err.fieldErrors.filter((fe) => fe.field === 'name' || fe.field === 'item_type')
      for (const fe of known) {
        setFieldError(
          fe.field as 'name' | 'item_type',
          fe.message.charAt(0).toUpperCase() + fe.message.slice(1),
        )
      }
      if (known.length > 0) await focusFirstInvalid(root.value)
      else formError.value = err.message
    } finally {
      submitting.value = false
    }
  },
  () => focusFirstInvalid(root.value),
)
</script>

<template>
  <form ref="root" class="space-y-4" novalidate @submit.prevent="submit">
    <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
    <TextField name="name" label="Nama kategori" required autocomplete="off" enterkeyhint="next" />
    <SelectField
      name="item_type"
      label="Jenis barang"
      :options="itemTypeOptions"
      placeholder="Pilih jenis barang"
      required
      hint="Jenis tidak dapat diubah setelah kategori dipakai produk."
    />
    <div class="flex flex-wrap items-center gap-3 pt-2">
      <button type="submit" :class="btnPrimary" :aria-busy="submitting">
        <i v-if="submitting" class="pi pi-spinner pi-spin" aria-hidden="true" />
        {{ submitting ? 'Menyimpan…' : category ? 'Simpan Perubahan' : 'Simpan Kategori' }}
      </button>
      <button type="button" :class="btnSecondary" @click="emit('cancel')">Batal</button>
    </div>
  </form>
</template>
