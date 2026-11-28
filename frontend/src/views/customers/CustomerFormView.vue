<script setup lang="ts">
// Pola halaman: Form — Tambah / Ubah Pelanggan (T2.2).
import { computed } from 'vue'
import { toTypedSchema } from '@vee-validate/zod'
import { useField } from 'vee-validate'
import { z } from 'zod'
import FormPage from '@/components/FormPage.vue'
import TextAreaField from '@/components/TextAreaField.vue'
import TextField from '@/components/TextField.vue'
import { useEntityForm } from '@/composables/useEntityForm'
import { nullIfEmpty, optionalText, requiredText } from '@/lib/validation'
import { customerService } from '@/services/customerService'
import type { Customer } from '@/types/customer'

const optionalEmail = z
  .string()
  .trim()
  .max(120, 'Maksimal 120 karakter')
  .refine((v) => v === '' || /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v), 'Format email tidak valid')

const schema = z.object({
  customer_code: requiredText('Kode pelanggan', 30).regex(
    /^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$/,
    'Gunakan huruf, angka, dan tanda hubung, mis. CUST-001',
  ),
  company_name: requiredText('Nama perusahaan', 150),
  pic_name: requiredText('Nama PIC', 100),
  phone: requiredText('Telepon', 30),
  email: optionalEmail,
  is_pkp: z.boolean(),
  npwp: optionalText('NPWP', 30),
  tax_address: optionalText('Alamat pajak', 500),
  billing_address: requiredText('Alamat penagihan', 500),
  billing_postal_code: requiredText('Kode pos penagihan', 10),
  billing_email: optionalEmail,
  payment_term_days: z
    .string()
    .trim()
    .refine(
      (v) => /^\d+$/.test(v) && Number(v) >= 0 && Number(v) <= 365,
      'Termin pembayaran harus bilangan bulat 0–365 hari',
    ),
})
type Values = z.infer<typeof schema>

const f = useEntityForm<Customer, Values>({
  schema: toTypedSchema(schema),
  emptyValues: {
    customer_code: '',
    company_name: '',
    pic_name: '',
    phone: '',
    email: '',
    is_pkp: true,
    npwp: '',
    tax_address: '',
    billing_address: '',
    billing_postal_code: '',
    billing_email: '',
    payment_term_days: '30',
  },
  toValues: (c) => ({
    customer_code: c.customer_code,
    company_name: c.company_name,
    pic_name: c.pic_name,
    phone: c.phone,
    email: c.email ?? '',
    is_pkp: c.is_pkp,
    npwp: c.npwp ?? '',
    tax_address: c.tax_address ?? '',
    billing_address: c.billing_address,
    billing_postal_code: c.billing_postal_code,
    billing_email: c.billing_email ?? '',
    payment_term_days: String(c.payment_term_days),
  }),
  load: (id) => customerService.get(id),
  save: (v, id) => {
    const body = {
      company_name: v.company_name,
      pic_name: v.pic_name,
      phone: v.phone,
      email: nullIfEmpty(v.email),
      is_pkp: v.is_pkp,
      npwp: nullIfEmpty(v.npwp),
      tax_address: nullIfEmpty(v.tax_address),
      billing_address: v.billing_address,
      billing_postal_code: v.billing_postal_code,
      billing_email: nullIfEmpty(v.billing_email),
      payment_term_days: Number(v.payment_term_days),
    }
    return id === undefined
      ? customerService.create({ customer_code: v.customer_code.trim(), ...body })
      : customerService.update(id, body)
  },
  fields: [
    'customer_code',
    'company_name',
    'pic_name',
    'phone',
    'email',
    'is_pkp',
    'npwp',
    'tax_address',
    'billing_address',
    'billing_postal_code',
    'billing_email',
    'payment_term_days',
  ],
  listRoute: '/pelanggan',
  savedMessage: (c, edit) => `Pelanggan ${c.customer_code} ${edit ? 'diperbarui' : 'ditambahkan'}`,
})

const { value: isPkp } = useField<boolean>('is_pkp')

const title = computed(() =>
  f.isEdit.value
    ? `Ubah Pelanggan${f.entity.value ? ` ${f.entity.value.customer_code}` : ''}`
    : 'Tambah Pelanggan',
)
const backTo = computed(() =>
  f.isEdit.value && f.entity.value ? `/pelanggan/${f.entity.value.id}` : '/pelanggan',
)
</script>

<template>
  <FormPage
    :title="title"
    description="Data identitas perusahaan, perpajakan (PKP/NPWP), dan alamat penagihan pelanggan."
    :back-to="backTo"
    :back-label="f.isEdit.value ? 'Detail pelanggan' : 'Daftar pelanggan'"
    :submit-label="f.isEdit.value ? 'Simpan Perubahan' : 'Simpan Pelanggan'"
    :submitting="f.submitting.value"
    :form-error="f.formError.value"
    :loading="f.loading.value"
    :load-error="f.loadError.value"
    @submit="f.submit"
  >
    <div class="grid gap-4 sm:grid-cols-2">
      <TextField
        name="customer_code"
        label="Kode pelanggan"
        required
        :readonly="f.isEdit.value"
        :hint="
          f.isEdit.value
            ? 'Kode pelanggan tidak dapat diubah.'
            : 'Huruf besar, angka, dan tanda hubung, mis. CUST-001.'
        "
        autocomplete="off"
      />
      <TextField name="company_name" label="Nama perusahaan" required autocomplete="off" />
    </div>

    <div class="grid gap-4 sm:grid-cols-3">
      <TextField name="pic_name" label="Nama PIC" required autocomplete="off" />
      <TextField name="phone" label="Telepon" required inputmode="tel" autocomplete="off" />
      <TextField name="email" label="Email PIC" type="email" autocomplete="off" />
    </div>

    <div class="rounded-lg border border-slate-200 bg-slate-50/70 p-4 space-y-4">
      <label class="flex items-center gap-2 text-sm font-medium text-slate-800">
        <input
          v-model="isPkp"
          type="checkbox"
          class="h-4 w-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500"
        />
        Pengusaha Kena Pajak (PKP)
      </label>
      <div class="grid gap-4 sm:grid-cols-2">
        <TextField
          name="npwp"
          label="NPWP (16 digit / format Coretax)"
          hint="Opsional untuk non-PKP."
          autocomplete="off"
        />
        <TextField
          name="payment_term_days"
          label="Termin pembayaran (hari)"
          required
          inputmode="numeric"
          hint="Default 30 hari (TOP 30)."
          autocomplete="off"
        />
      </div>
      <TextAreaField name="tax_address" label="Alamat pajak (sesuai NPWP)" />
    </div>

    <div class="space-y-4">
      <TextAreaField name="billing_address" label="Alamat penagihan" required />
      <div class="grid gap-4 sm:grid-cols-2">
        <TextField
          name="billing_postal_code"
          label="Kode pos penagihan"
          required
          autocomplete="off"
        />
        <TextField
          name="billing_email"
          label="Email tujuan invoice"
          type="email"
          hint="Opsional, untuk pengiriman invoice."
          autocomplete="off"
        />
      </div>
    </div>
  </FormPage>
</template>
