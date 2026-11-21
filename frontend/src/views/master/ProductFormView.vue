<script setup lang="ts">
// Pola halaman: Form.
import { computed } from 'vue'
import { toTypedSchema } from '@vee-validate/zod'
import { z } from 'zod'
import CheckboxField from '@/components/CheckboxField.vue'
import FormPage from '@/components/FormPage.vue'
import SelectField from '@/components/SelectField.vue'
import TextAreaField from '@/components/TextAreaField.vue'
import TextField from '@/components/TextField.vue'
import { useEntityForm } from '@/composables/useEntityForm'
import { useOptions } from '@/composables/useOptions'
import {
  nullIfEmpty,
  optionalMoney,
  optionalText,
  requiredText,
  wholeNumber,
} from '@/lib/validation'
import { categoryService, productService } from '@/services/masterService'
import type { Product } from '@/types/master'

const schema = z.object({
  sku: requiredText('SKU', 50).regex(
    /^[A-Za-z0-9][A-Za-z0-9._/-]*$/,
    'Gunakan huruf, angka, titik, garis miring, garis bawah, atau tanda hubung tanpa spasi',
  ),
  name: requiredText('Nama produk', 150),
  category_id: z.string().min(1, 'Pilih kategori'),
  brand: optionalText('Merek', 80),
  model: optionalText('Model', 80),
  unit: requiredText('Satuan', 20),
  has_serial_number: z.boolean(),
  min_stock_alert: wholeNumber('Batas stok minimum'),
  default_monthly_rate: optionalMoney,
  default_sell_price: optionalMoney,
  description: optionalText('Deskripsi', 1000),
})
type Values = z.infer<typeof schema>

const categories = useOptions(
  () => categoryService.list({ limit: 100, sort: 'name', is_active: true }),
  (c) => ({
    value: String(c.id),
    label: `${c.name} (${c.item_type === 'ASSET' ? 'aset' : 'habis pakai'})`,
  }),
)

const f = useEntityForm<Product, Values>({
  schema: toTypedSchema(schema),
  emptyValues: {
    sku: '',
    name: '',
    category_id: '',
    brand: '',
    model: '',
    unit: 'unit',
    has_serial_number: false,
    min_stock_alert: '0',
    default_monthly_rate: '',
    default_sell_price: '',
    description: '',
  },
  toValues: (p) => ({
    sku: p.sku,
    name: p.name,
    category_id: String(p.category_id),
    brand: p.brand ?? '',
    model: p.model ?? '',
    unit: p.unit,
    has_serial_number: p.has_serial_number,
    min_stock_alert: String(p.min_stock_alert),
    default_monthly_rate: p.default_monthly_rate ?? '',
    default_sell_price: p.default_sell_price ?? '',
    description: p.description ?? '',
  }),
  load: (id) => productService.get(id),
  save: (v, id) => {
    const body = {
      sku: v.sku.trim(),
      name: v.name,
      category_id: Number(v.category_id),
      brand: nullIfEmpty(v.brand),
      model: nullIfEmpty(v.model),
      unit: v.unit,
      has_serial_number: v.has_serial_number,
      min_stock_alert: Number(v.min_stock_alert),
      default_monthly_rate: nullIfEmpty(v.default_monthly_rate),
      default_sell_price: nullIfEmpty(v.default_sell_price),
      description: nullIfEmpty(v.description),
    }
    return id === undefined ? productService.create(body) : productService.update(id, body)
  },
  fields: Object.keys(schema.shape),
  listRoute: '/master/produk',
  savedMessage: (p, edit) => `Produk ${p.sku} ${edit ? 'diperbarui' : 'ditambahkan'}`,
})

// Saat ubah, kategori produk ini harus tetap tampil walau sudah tidak aktif.
const categoryOptions = computed(() => {
  const p = f.entity.value
  if (p && !categories.options.value.some((o) => o.value === String(p.category_id))) {
    return [{ value: String(p.category_id), label: p.category_name }, ...categories.options.value]
  }
  return categories.options.value
})

const title = computed(() =>
  f.isEdit.value ? `Ubah Produk${f.entity.value ? ` ${f.entity.value.sku}` : ''}` : 'Tambah Produk',
)
</script>

<template>
  <FormPage
    :title="title"
    description="SKU dan pengaturan serial number terkunci setelah produk dipakai stok atau transaksi."
    :back-to="'/master/produk'"
    back-label="Daftar produk"
    :submit-label="f.isEdit.value ? 'Simpan Perubahan' : 'Simpan Produk'"
    :submitting="f.submitting.value"
    :form-error="f.formError.value"
    :loading="f.loading.value"
    :load-error="f.loadError.value"
    @submit="f.submit"
  >
    <div class="grid gap-4 sm:grid-cols-2">
      <TextField
        name="sku"
        label="SKU"
        required
        hint="Juga dipakai sebagai barcode barang tanpa serial number."
        autocomplete="off"
        enterkeyhint="next"
      />
      <SelectField
        name="category_id"
        label="Kategori"
        :options="categoryOptions"
        placeholder="Pilih kategori"
        required
      />
    </div>
    <TextField name="name" label="Nama produk" required autocomplete="off" enterkeyhint="next" />
    <div class="grid gap-4 sm:grid-cols-2">
      <TextField name="brand" label="Merek" autocomplete="off" enterkeyhint="next" />
      <TextField name="model" label="Model" autocomplete="off" enterkeyhint="next" />
    </div>
    <div class="grid gap-4 sm:grid-cols-2">
      <TextField name="unit" label="Satuan" required autocomplete="off" enterkeyhint="next" />
      <TextField
        name="min_stock_alert"
        label="Batas stok minimum"
        required
        inputmode="numeric"
        hint="Stok di bawah angka ini ditandai menipis."
        autocomplete="off"
        enterkeyhint="next"
      />
    </div>
    <CheckboxField
      name="has_serial_number"
      label="Lacak per unit dengan serial number"
      hint="Tidak untuk barang habis pakai. Tidak dapat diubah setelah produk dipakai."
    />
    <div class="grid gap-4 sm:grid-cols-2">
      <TextField
        name="default_monthly_rate"
        label="Tarif sewa per bulan (Rp)"
        inputmode="decimal"
        hint="Opsional. Contoh: 550000 atau 550000.50. Dapat diubah per kontrak."
        autocomplete="off"
        enterkeyhint="next"
      />
      <TextField
        name="default_sell_price"
        label="Harga jual (Rp)"
        inputmode="decimal"
        hint="Opsional. Untuk barang habis pakai."
        autocomplete="off"
        enterkeyhint="next"
      />
    </div>
    <TextAreaField name="description" label="Deskripsi" />
  </FormPage>
</template>
