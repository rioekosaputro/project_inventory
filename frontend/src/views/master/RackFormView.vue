<script setup lang="ts">
// Pola halaman: Form.
import { computed } from 'vue'
import { toTypedSchema } from '@vee-validate/zod'
import { z } from 'zod'
import FormPage from '@/components/FormPage.vue'
import SelectField from '@/components/SelectField.vue'
import TextAreaField from '@/components/TextAreaField.vue'
import TextField from '@/components/TextField.vue'
import { useEntityForm } from '@/composables/useEntityForm'
import { useOptions } from '@/composables/useOptions'
import { nullIfEmpty, optionalText, requiredText } from '@/lib/validation'
import { rackService, warehouseService } from '@/services/masterService'
import type { Rack } from '@/types/master'

const schema = z.object({
  warehouse_id: z.string().min(1, 'Pilih gudang'),
  code: requiredText('Kode rak', 30).regex(
    /^[A-Za-z0-9][A-Za-z0-9._-]*$/,
    'Gunakan huruf, angka, titik, garis bawah, atau tanda hubung, mis. A-02-01',
  ),
  zone: optionalText('Zona', 30),
  is_sellable: z.string(), // '' = ikuti zona (hanya saat buat)
  description: optionalText('Keterangan', 1000),
})
type Values = z.infer<typeof schema>

const warehouses = useOptions(
  () => warehouseService.list({ limit: 100, sort: 'code', is_active: true }),
  (w) => ({ value: String(w.id), label: `${w.code} — ${w.name}` }),
)

const f = useEntityForm<Rack, Values>({
  schema: toTypedSchema(schema),
  emptyValues: { warehouse_id: '', code: '', zone: '', is_sellable: '', description: '' },
  toValues: (r) => ({
    warehouse_id: String(r.warehouse_id),
    code: r.code,
    zone: r.zone ?? '',
    is_sellable: String(r.is_sellable),
    description: r.description ?? '',
  }),
  load: (id) => rackService.get(id),
  save: (v, id) => {
    const shared = { zone: nullIfEmpty(v.zone), description: nullIfEmpty(v.description) }
    if (id !== undefined)
      return rackService.update(id, { ...shared, is_sellable: v.is_sellable === 'true' })
    return rackService.create({
      warehouse_id: Number(v.warehouse_id),
      code: v.code.trim(),
      ...shared,
      ...(v.is_sellable === '' ? {} : { is_sellable: v.is_sellable === 'true' }),
    })
  },
  fields: ['warehouse_id', 'code', 'zone', 'is_sellable', 'description'],
  listRoute: '/master/rak',
  savedMessage: (r, edit) => `Rak ${r.code} ${edit ? 'diperbarui' : 'ditambahkan'}`,
})

// Saat ubah, gudang rak ini harus tetap tampil walau sudah tidak aktif.
const warehouseOptions = computed(() => {
  const r = f.entity.value
  if (r && !warehouses.options.value.some((o) => o.value === String(r.warehouse_id))) {
    return [{ value: String(r.warehouse_id), label: r.warehouse_code }, ...warehouses.options.value]
  }
  return warehouses.options.value
})

const sellableOptions = computed(() => [
  { value: 'true', label: 'Ya, dapat dijual' },
  { value: 'false', label: 'Tidak (stok tidak dihitung tersedia)' },
])

const title = computed(() =>
  f.isEdit.value ? `Ubah Rak${f.entity.value ? ` ${f.entity.value.code}` : ''}` : 'Tambah Rak',
)
</script>

<template>
  <FormPage
    :title="title"
    description="Gudang dan kode rak tidak dapat diubah setelah dibuat (kode tercetak pada label rak)."
    :back-to="'/master/rak'"
    back-label="Daftar rak"
    :submit-label="f.isEdit.value ? 'Simpan Perubahan' : 'Simpan Rak'"
    :submitting="f.submitting.value"
    :form-error="f.formError.value"
    :loading="f.loading.value"
    :load-error="f.loadError.value"
    @submit="f.submit"
  >
    <SelectField
      name="warehouse_id"
      label="Gudang"
      :options="warehouseOptions"
      placeholder="Pilih gudang"
      :disabled="f.isEdit.value"
      required
    />
    <TextField
      name="code"
      label="Kode rak"
      required
      :readonly="f.isEdit.value"
      :hint="f.isEdit.value ? 'Kode tidak dapat diubah.' : 'Zona-Rak-Level, mis. A-02-01.'"
      autocomplete="off"
      enterkeyhint="next"
    />
    <TextField
      name="zone"
      label="Zona"
      hint="Mis. PRINTER, SPAREPART, KARANTINA, WORKSHOP. KARANTINA dan WORKSHOP otomatis tidak dapat dijual."
      autocomplete="off"
      enterkeyhint="next"
    />
    <SelectField
      name="is_sellable"
      label="Dapat dijual"
      :options="sellableOptions"
      :placeholder="f.isEdit.value ? undefined : 'Ikuti zona'"
      hint="Stok di rak yang tidak dapat dijual tetap tercatat, tetapi tidak dihitung sebagai tersedia."
    />
    <TextAreaField name="description" label="Keterangan" />
  </FormPage>
</template>
