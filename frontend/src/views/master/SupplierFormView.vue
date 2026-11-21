<script setup lang="ts">
// Pola halaman: Form.
import { computed } from 'vue'
import { toTypedSchema } from '@vee-validate/zod'
import { z } from 'zod'
import FormPage from '@/components/FormPage.vue'
import TextAreaField from '@/components/TextAreaField.vue'
import TextField from '@/components/TextField.vue'
import { useEntityForm } from '@/composables/useEntityForm'
import { nullIfEmpty, optionalText, requiredText } from '@/lib/validation'
import { supplierService } from '@/services/masterService'
import type { Supplier } from '@/types/master'

const schema = z.object({
  code: requiredText('Kode', 30).regex(
    /^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$/,
    'Gunakan huruf, angka, dan tanda hubung, mis. SUP-001',
  ),
  name: requiredText('Nama supplier', 150),
  pic_name: optionalText('Nama PIC', 100),
  phone: optionalText('Telepon', 30),
  email: z
    .string()
    .trim()
    .max(120, 'Email maksimal 120 karakter')
    .refine((v) => v === '' || /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v), {
      message: 'Isi email seperti nama@perusahaan.co.id',
    }),
  npwp: optionalText('NPWP', 30),
  address: optionalText('Alamat', 500),
})
type Values = z.infer<typeof schema>

const f = useEntityForm<Supplier, Values>({
  schema: toTypedSchema(schema),
  emptyValues: { code: '', name: '', pic_name: '', phone: '', email: '', npwp: '', address: '' },
  toValues: (s) => ({
    code: s.code,
    name: s.name,
    pic_name: s.pic_name ?? '',
    phone: s.phone ?? '',
    email: s.email ?? '',
    npwp: s.npwp ?? '',
    address: s.address ?? '',
  }),
  load: (id) => supplierService.get(id),
  save: (v, id) => {
    const body = {
      name: v.name,
      pic_name: nullIfEmpty(v.pic_name),
      phone: nullIfEmpty(v.phone),
      email: nullIfEmpty(v.email),
      npwp: nullIfEmpty(v.npwp),
      address: nullIfEmpty(v.address),
    }
    return id === undefined
      ? supplierService.create({ code: v.code.trim(), ...body })
      : supplierService.update(id, body)
  },
  fields: Object.keys(schema.shape),
  listRoute: '/master/supplier',
  savedMessage: (s, edit) => `Supplier ${s.code} ${edit ? 'diperbarui' : 'ditambahkan'}`,
})

const title = computed(() =>
  f.isEdit.value
    ? `Ubah Supplier${f.entity.value ? ` ${f.entity.value.code}` : ''}`
    : 'Tambah Supplier',
)
</script>

<template>
  <FormPage
    :title="title"
    description="Kode supplier tidak dapat diubah setelah dibuat."
    :back-to="'/master/supplier'"
    back-label="Daftar supplier"
    :submit-label="f.isEdit.value ? 'Simpan Perubahan' : 'Simpan Supplier'"
    :submitting="f.submitting.value"
    :form-error="f.formError.value"
    :loading="f.loading.value"
    :load-error="f.loadError.value"
    @submit="f.submit"
  >
    <div class="grid gap-4 sm:grid-cols-2">
      <TextField
        name="code"
        label="Kode supplier"
        required
        :readonly="f.isEdit.value"
        :hint="
          f.isEdit.value
            ? 'Kode tidak dapat diubah.'
            : 'Huruf besar, angka, dan tanda hubung, mis. SUP-001.'
        "
        autocomplete="off"
        enterkeyhint="next"
      />
      <TextField
        name="name"
        label="Nama supplier"
        required
        autocomplete="organization"
        enterkeyhint="next"
      />
    </div>
    <div class="grid gap-4 sm:grid-cols-2">
      <TextField name="pic_name" label="Nama PIC" autocomplete="off" enterkeyhint="next" />
      <TextField
        name="phone"
        label="Telepon"
        inputmode="tel"
        autocomplete="off"
        enterkeyhint="next"
      />
    </div>
    <div class="grid gap-4 sm:grid-cols-2">
      <TextField name="email" label="Email" type="email" autocomplete="off" enterkeyhint="next" />
      <TextField
        name="npwp"
        label="NPWP"
        inputmode="numeric"
        autocomplete="off"
        enterkeyhint="next"
      />
    </div>
    <TextAreaField name="address" label="Alamat" />
  </FormPage>
</template>
