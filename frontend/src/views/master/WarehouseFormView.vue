<script setup lang="ts">
// Pola halaman: Form.
import { computed } from 'vue'
import { toTypedSchema } from '@vee-validate/zod'
import { z } from 'zod'
import FormPage from '@/components/FormPage.vue'
import TextAreaField from '@/components/TextAreaField.vue'
import TextField from '@/components/TextField.vue'
import { useEntityForm } from '@/composables/useEntityForm'
import { nullIfEmpty, optionalCoordinate, optionalText, requiredText } from '@/lib/validation'
import { warehouseService } from '@/services/masterService'
import type { Warehouse } from '@/types/master'

const schema = z
  .object({
    code: requiredText('Kode', 20).regex(
      /^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$/,
      'Gunakan huruf, angka, dan tanda hubung, mis. WH-SMG-01',
    ),
    name: requiredText('Nama', 100),
    address: optionalText('Alamat', 500),
    phone: optionalText('Telepon', 30),
    latitude: optionalCoordinate('Latitude', -90, 90),
    longitude: optionalCoordinate('Longitude', -180, 180),
  })
  .superRefine((v, ctx) => {
    if ((v.latitude === '') !== (v.longitude === '')) {
      const missing = v.latitude === '' ? 'latitude' : 'longitude'
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: [missing],
        message: 'Isi latitude dan longitude bersamaan, atau kosongkan keduanya',
      })
    }
  })
type Values = z.infer<typeof schema>

const f = useEntityForm<Warehouse, Values>({
  schema: toTypedSchema(schema),
  emptyValues: { code: '', name: '', address: '', phone: '', latitude: '', longitude: '' },
  toValues: (w) => ({
    code: w.code,
    name: w.name,
    address: w.address ?? '',
    phone: w.phone ?? '',
    latitude: w.latitude === null ? '' : String(w.latitude),
    longitude: w.longitude === null ? '' : String(w.longitude),
  }),
  load: (id) => warehouseService.get(id),
  save: (v, id) => {
    const body = {
      name: v.name,
      address: nullIfEmpty(v.address),
      phone: nullIfEmpty(v.phone),
      latitude: v.latitude === '' ? null : Number(v.latitude),
      longitude: v.longitude === '' ? null : Number(v.longitude),
    }
    return id === undefined
      ? warehouseService.create({ code: v.code.trim(), ...body })
      : warehouseService.update(id, body)
  },
  fields: ['code', 'name', 'address', 'phone', 'latitude', 'longitude'],
  listRoute: '/master/gudang',
  savedMessage: (w, edit) => `Gudang ${w.code} ${edit ? 'diperbarui' : 'ditambahkan'}`,
})

const title = computed(() =>
  f.isEdit.value
    ? `Ubah Gudang${f.entity.value ? ` ${f.entity.value.code}` : ''}`
    : 'Tambah Gudang',
)
</script>

<template>
  <FormPage
    :title="title"
    description="Kode gudang dipakai pada nomor dokumen, jadi tidak dapat diubah setelah dibuat."
    :back-to="'/master/gudang'"
    back-label="Daftar gudang"
    :submit-label="f.isEdit.value ? 'Simpan Perubahan' : 'Simpan Gudang'"
    :submitting="f.submitting.value"
    :form-error="f.formError.value"
    :loading="f.loading.value"
    :load-error="f.loadError.value"
    @submit="f.submit"
  >
    <TextField
      name="code"
      label="Kode gudang"
      required
      :readonly="f.isEdit.value"
      :hint="
        f.isEdit.value
          ? 'Kode tidak dapat diubah.'
          : 'Huruf besar, angka, dan tanda hubung, mis. WH-SMG-01.'
      "
      autocomplete="off"
      enterkeyhint="next"
    />
    <TextField name="name" label="Nama gudang" required autocomplete="off" enterkeyhint="next" />
    <TextAreaField name="address" label="Alamat" />
    <TextField
      name="phone"
      label="Telepon"
      type="text"
      inputmode="tel"
      autocomplete="off"
      enterkeyhint="next"
    />
    <div class="grid gap-4 sm:grid-cols-2">
      <TextField
        name="latitude"
        label="Latitude"
        inputmode="decimal"
        hint="Opsional, mis. -6.9666667."
        autocomplete="off"
        enterkeyhint="next"
      />
      <TextField
        name="longitude"
        label="Longitude"
        inputmode="decimal"
        hint="Opsional, mis. 110.4166667."
        autocomplete="off"
        enterkeyhint="done"
      />
    </div>
  </FormPage>
</template>
