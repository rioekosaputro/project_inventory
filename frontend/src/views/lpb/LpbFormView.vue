<script setup lang="ts">
// Pola halaman: Form (buat draft LPB / ubah draft). Scan serial number dilakukan di halaman rincian
// setelah draft tersimpan — pada saat itu item (dan id-nya) sudah ada.
import { computed, nextTick, ref, watch } from 'vue'
import { useFieldArray } from 'vee-validate'
import { toTypedSchema } from '@vee-validate/zod'
import { z } from 'zod'
import FormPage from '@/components/FormPage.vue'
import SelectField from '@/components/SelectField.vue'
import TextAreaField from '@/components/TextAreaField.vue'
import TextField from '@/components/TextField.vue'
import { useEntityForm } from '@/composables/useEntityForm'
import { useOptions, type Option } from '@/composables/useOptions'
import { ApiError } from '@/lib/apiError'
import { formatDate, todayWIB } from '@/lib/format'
import { listAll } from '@/lib/listAll'
import { CREATABLE_SUB_TYPES, STATUS_LABELS, SUB_TYPE_LABELS } from '@/lib/lpb'
import { btnSecondary } from '@/lib/ui'
import { nullIfEmpty, optionalMoney, optionalText } from '@/lib/validation'
import { lpbService } from '@/services/lpbService'
import {
  productService,
  rackService,
  supplierService,
  warehouseService,
} from '@/services/masterService'
import { useAuthStore } from '@/stores/auth'
import type { LPBDetail } from '@/types/lpb'

const auth = useAuthStore()

const itemSchema = z.object({
  product_id: z.string().min(1, 'Pilih produk'),
  rack_id: z.string().min(1, 'Pilih rak'),
  qty: z
    .string()
    .trim()
    .regex(/^[1-9]\d{0,6}$/, 'Isi jumlah dengan bilangan bulat, minimal 1'),
  unit_cost: optionalMoney,
  notes: z.string(),
})

const schema = z
  .object({
    sub_type: z.string().min(1, 'Pilih jenis LPB'),
    warehouse_id: z.string().min(1, 'Pilih gudang'),
    doc_date: z
      .string()
      .min(1, 'Tanggal dokumen wajib diisi')
      .refine((v) => v <= todayWIB(), 'Tanggal dokumen tidak boleh di masa depan'),
    // Tidak dirender untuk jenis selain pembelian; vee-validate membuang nilai field yang tidak tampil.
    supplier_id: z.string().optional(),
    reference_number: optionalText('Nomor referensi', 60).optional(),
    notes: optionalText('Catatan', 1000),
    items: z.array(itemSchema).min(1, 'Tambahkan minimal satu item'),
  })
  .superRefine((v, ctx) => {
    if (v.sub_type === 'PURCHASE' && !v.supplier_id) {
      ctx.addIssue({ code: 'custom', path: ['supplier_id'], message: 'Pilih supplier' })
    }
    // Satu pasangan (produk, rak) hanya boleh sekali; gabungkan jumlahnya di satu baris.
    const seen = new Set<string>()
    v.items.forEach((it, i) => {
      if (!it.product_id || !it.rack_id) return
      const key = `${it.product_id}:${it.rack_id}`
      if (seen.has(key)) {
        ctx.addIssue({
          code: 'custom',
          path: ['items', i, 'rack_id'],
          message: 'Produk dan rak ini sudah ada di baris lain — gabungkan jumlahnya',
        })
      }
      seen.add(key)
    })
  })
type Values = z.infer<typeof schema>

const newItem = () => ({ product_id: '', rack_id: '', qty: '1', unit_cost: '', notes: '' })

const userWarehouse = auth.user?.warehouse_id ? String(auth.user.warehouse_id) : ''

const f = useEntityForm<LPBDetail, Values>({
  schema: toTypedSchema(schema),
  emptyValues: {
    sub_type: 'PURCHASE',
    warehouse_id: userWarehouse,
    doc_date: todayWIB(),
    supplier_id: '',
    reference_number: '',
    notes: '',
    items: [newItem()],
  },
  toValues: (d) => ({
    sub_type: d.sub_type,
    warehouse_id: String(d.warehouse_id),
    doc_date: d.doc_date,
    supplier_id: d.supplier_id ? String(d.supplier_id) : '',
    reference_number: d.reference_number ?? '',
    notes: d.notes ?? '',
    items: d.items.map((it) => ({
      product_id: String(it.product_id),
      rack_id: String(it.rack_id),
      qty: String(it.qty),
      unit_cost: it.unit_cost ?? '',
      notes: it.notes ?? '',
    })),
  }),
  load: async (id) => {
    const d = await lpbService.get(id)
    if (d.status !== 'DRAFT') {
      throw new ApiError(
        409,
        'LPB_NOT_DRAFT',
        `LPB ${d.lpb_number} berstatus ${STATUS_LABELS[d.status]} sehingga tidak dapat diubah lagi.`,
      )
    }
    return d
  },
  save: (v, id) => {
    const purchase = v.sub_type === 'PURCHASE'
    const costApplies = purchase || v.sub_type === 'INITIAL_STOCK'
    const items = v.items.map((it) => ({
      product_id: Number(it.product_id),
      rack_id: Number(it.rack_id),
      qty: Number(it.qty),
      unit_cost: costApplies ? nullIfEmpty(it.unit_cost) : null,
      notes: nullIfEmpty(it.notes),
    }))
    const common = {
      doc_date: v.doc_date,
      supplier_id: purchase && v.supplier_id ? Number(v.supplier_id) : null,
      reference_number: purchase ? nullIfEmpty(v.reference_number ?? '') : null,
      notes: nullIfEmpty(v.notes),
      items,
    }
    return id === undefined
      ? lpbService.create({
          ...common,
          sub_type: v.sub_type as never,
          warehouse_id: Number(v.warehouse_id),
        })
      : lpbService.update(id, common)
  },
  fields: ['sub_type', 'warehouse_id', 'doc_date', 'supplier_id', 'reference_number', 'notes'],
  listRoute: '/lpb',
  afterSave: (saved) => `/lpb/${saved.id}`,
  savedMessage: (d, edit) =>
    edit
      ? `LPB ${d.lpb_number} diperbarui`
      : `LPB ${d.lpb_number} dibuat sebagai draft. Lanjutkan dengan scan serial number.`,
})

const { fields: itemRows, push, remove } = useFieldArray<Values['items'][number]>('items')

const values = f.form.values
const isPurchase = computed(() => values.sub_type === 'PURCHASE')
const showCost = computed(
  () => values.sub_type === 'PURCHASE' || values.sub_type === 'INITIAL_STOCK',
)

// ---- pilihan
const warehouses = useOptions(
  () => warehouseService.list({ limit: 100, sort: 'code', is_active: true }),
  (w) => ({ value: String(w.id), label: `${w.code} — ${w.name}` }),
)
const warehouseOptions = computed(() =>
  userWarehouse
    ? warehouses.options.value.filter((o) => o.value === userWarehouse)
    : warehouses.options.value,
)

const suppliers = useOptions(
  () => listAll(supplierService.list, { sort: 'name', is_active: true }),
  (s) => ({ value: String(s.id), label: `${s.code} — ${s.name}` }),
)
const products = useOptions(
  () => listAll(productService.list, { sort: 'sku', is_active: true }),
  (p) => ({
    value: String(p.id),
    label: `${p.sku} · ${p.name}${p.has_serial_number ? ' (serial)' : ''}`,
  }),
)

// Rak mengikuti gudang yang dipilih.
const racks = ref<Option[]>([])
async function loadRacks(warehouseId: string): Promise<void> {
  if (warehouseId === '') {
    racks.value = []
    return
  }
  try {
    const page = await listAll(rackService.list, {
      warehouse_id: warehouseId,
      sort: 'code',
      is_active: true,
    })
    racks.value = page.items.map((r) => ({
      value: String(r.id),
      label: `${r.code}${r.is_sellable ? '' : ' (non-jual)'}`,
    }))
  } catch {
    racks.value = []
  }
}
watch(
  () => values.warehouse_id,
  (next, prev) => {
    // Pengguna mengganti gudang (dari satu gudang ke gudang lain): pilihan rak lama tidak berlaku.
    // Perubahan dari kosong (pemuatan data mode ubah, atau pilihan pertama) tidak mereset apa pun.
    if (prev && next !== prev) {
      values.items?.forEach((_, i) =>
        f.form.setFieldValue(`items[${i}].rack_id` as never, '' as never),
      )
    }
    void loadRacks(next ?? '')
  },
  { immediate: true },
)

/** Mode ubah: produk/rak/supplier yang sudah nonaktif tetap tampil agar nilai tersimpan tidak hilang. */
function withStored(base: Option[], extra: Option[]): Option[] {
  const have = new Set(base.map((o) => o.value))
  return [...extra.filter((o) => !have.has(o.value)), ...base]
}
const productOptions = computed(() =>
  withStored(
    products.options.value,
    (f.entity.value?.items ?? []).map((it) => ({
      value: String(it.product_id),
      label: `${it.sku} · ${it.product_name}`,
    })),
  ),
)
const rackOptions = computed(() =>
  withStored(
    racks.value,
    (f.entity.value?.items ?? []).map((it) => ({ value: String(it.rack_id), label: it.rack_code })),
  ),
)
const supplierOptions = computed(() => {
  const d = f.entity.value
  return d?.supplier_id && d.supplier_name
    ? withStored(suppliers.options.value, [
        { value: String(d.supplier_id), label: d.supplier_name },
      ])
    : suppliers.options.value
})
const subTypeOptions = computed(() => {
  const list = CREATABLE_SUB_TYPES.map((v) => ({ value: v, label: SUB_TYPE_LABELS[v] }))
  const current = f.entity.value?.sub_type
  return current && !CREATABLE_SUB_TYPES.includes(current)
    ? [{ value: current, label: SUB_TYPE_LABELS[current] }, ...list]
    : list
})

// ---- baris item
const rowsEl = ref<HTMLElement | null>(null)
async function addRow(): Promise<void> {
  push(newItem())
  await nextTick()
  const selects = rowsEl.value?.querySelectorAll<HTMLElement>('select[name$=".product_id"]')
  selects?.[selects.length - 1]?.focus()
}
async function removeRow(index: number): Promise<void> {
  remove(index)
  await nextTick()
  // Fokus tidak boleh hilang bersama baris yang dihapus: pindah ke tombol tambah.
  document.getElementById('add-item')?.focus()
}

const itemsError = computed(() => {
  const e = f.form.errors.value as Record<string, string | undefined>
  return e.items
})

const title = computed(() =>
  f.isEdit.value
    ? `Ubah Draft LPB${f.entity.value ? ` ${f.entity.value.lpb_number}` : ''}`
    : 'Buat LPB',
)
const backTo = computed(() => (f.isEdit.value && f.id.value ? `/lpb/${f.id.value}` : '/lpb'))
</script>

<template>
  <FormPage
    wide
    :title="title"
    :description="
      f.isEdit.value
        ? 'Jenis dan gudang tidak dapat diubah; batalkan draft lalu buat baru bila keliru.'
        : 'Draft belum mengubah stok. Setelah disimpan, scan serial number lalu ajukan untuk persetujuan.'
    "
    :back-to="backTo"
    :back-label="f.isEdit.value ? 'Kembali ke LPB' : 'Daftar LPB'"
    :submit-label="f.isEdit.value ? 'Simpan Perubahan' : 'Simpan Draft'"
    :submitting="f.submitting.value"
    :form-error="f.formError.value"
    :loading="f.loading.value"
    :load-error="f.loadError.value"
    @submit="f.submit"
  >
    <div class="grid gap-4 sm:grid-cols-2">
      <SelectField
        name="sub_type"
        label="Jenis LPB"
        :options="subTypeOptions"
        :disabled="f.isEdit.value"
        required
      />
      <SelectField
        name="warehouse_id"
        label="Gudang"
        :options="warehouseOptions"
        placeholder="Pilih gudang"
        :disabled="f.isEdit.value"
        required
      />
    </div>

    <div class="grid gap-4 sm:grid-cols-2">
      <TextField name="doc_date" label="Tanggal dokumen" type="date" required />
      <SelectField
        v-if="isPurchase"
        name="supplier_id"
        label="Supplier"
        :options="supplierOptions"
        placeholder="Pilih supplier"
        required
      />
    </div>
    <TextField
      v-if="isPurchase"
      name="reference_number"
      label="Nomor faktur / surat jalan supplier"
      hint="Opsional. Membantu mencocokkan dokumen ini dengan tagihan pemasok."
      autocomplete="off"
      enterkeyhint="next"
    />

    <!-- Daftar item: satu kartu per baris agar tetap terbaca di layar sempit. -->
    <div>
      <h2 class="text-base font-semibold text-slate-900">Item</h2>
      <p class="mt-1 text-sm text-slate-600">
        Pilih produk dan rak tujuan. Produk bertanda “serial” di-scan satu per satu setelah draft
        disimpan.
      </p>
      <p v-if="itemsError" class="mt-2 text-sm text-red-700" role="alert">{{ itemsError }}</p>

      <div ref="rowsEl" class="mt-3 space-y-3">
        <fieldset
          v-for="(row, index) in itemRows"
          :key="row.key"
          class="rounded-lg border border-slate-200 bg-slate-50/60 p-4"
        >
          <legend class="px-1 text-sm font-semibold text-slate-800">Item {{ index + 1 }}</legend>
          <div class="grid gap-3 sm:grid-cols-6">
            <div class="sm:col-span-3">
              <SelectField
                :name="`items[${index}].product_id`"
                label="Produk"
                :options="productOptions"
                placeholder="Pilih produk"
                required
              />
            </div>
            <div class="sm:col-span-3">
              <SelectField
                :name="`items[${index}].rack_id`"
                label="Rak tujuan"
                :options="rackOptions"
                :placeholder="values.warehouse_id ? 'Pilih rak' : 'Pilih gudang dulu'"
                required
              />
            </div>
            <div :class="showCost ? 'sm:col-span-2' : 'sm:col-span-3'">
              <TextField
                :name="`items[${index}].qty`"
                label="Jumlah"
                inputmode="numeric"
                autocomplete="off"
                enterkeyhint="next"
                required
              />
            </div>
            <div v-if="showCost" class="sm:col-span-3">
              <TextField
                :name="`items[${index}].unit_cost`"
                label="Biaya satuan (Rp)"
                inputmode="decimal"
                hint="Opsional. Contoh: 1500000 atau 1500000.50"
                autocomplete="off"
                enterkeyhint="next"
              />
            </div>
            <div
              class="flex items-end justify-end"
              :class="showCost ? 'sm:col-span-1' : 'sm:col-span-3'"
            >
              <button
                v-if="itemRows.length > 1"
                type="button"
                :class="btnSecondary"
                @click="removeRow(index)"
              >
                <i class="pi pi-trash text-xs" aria-hidden="true" />Hapus<span class="sr-only">
                  item {{ index + 1 }}</span
                >
              </button>
            </div>
          </div>
        </fieldset>
      </div>

      <button id="add-item" type="button" :class="[btnSecondary, 'mt-3']" @click="addRow">
        <i class="pi pi-plus text-xs" aria-hidden="true" />Tambah Item
      </button>
    </div>

    <TextAreaField name="notes" label="Catatan" hint="Opsional." />

    <p v-if="f.isEdit.value && f.entity.value" class="text-sm text-slate-600">
      Dibuat {{ formatDate(f.entity.value.created_at) }} oleh {{ f.entity.value.created_by_name }}.
    </p>
  </FormPage>
</template>
