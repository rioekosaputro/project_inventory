<script setup lang="ts">
import { computed } from 'vue'
import { useBlurField } from '@/composables/useBlurField'

const props = defineProps<{
  name: string
  label: string
  options: { value: string; label: string }[]
  /** Teks pilihan kosong ("Pilih gudang"); tanpa ini tidak ada pilihan kosong. */
  placeholder?: string
  hint?: string
  required?: boolean
  disabled?: boolean
}>()

const { value, error, onBlur, onFocus, onInput } = useBlurField(
  () => props.name,
  () => props.label,
)

const id = computed(() => `field-${props.name}`)
const hintId = computed(() => `${id.value}-hint`)
const errorId = computed(() => `${id.value}-error`)
const describedBy = computed(() =>
  error.value ? errorId.value : props.hint ? hintId.value : undefined,
)
</script>

<template>
  <div>
    <label :for="id" class="block text-sm font-medium text-slate-800">
      {{ label }}
      <span v-if="required" class="text-red-700" aria-hidden="true">*</span>
    </label>
    <select
      :id="id"
      v-model="value"
      :name="name"
      :required="required"
      :disabled="disabled"
      :aria-invalid="error ? 'true' : 'false'"
      :aria-describedby="describedBy"
      class="mt-1 min-h-11 w-full rounded-md border bg-white px-3 text-base disabled:bg-slate-100 disabled:text-slate-600"
      :class="error ? 'border-red-600' : 'border-slate-300'"
      @blur="onBlur"
      @focus="onFocus"
      @change="onInput"
    >
      <option v-if="placeholder" value="">{{ placeholder }}</option>
      <option v-for="o in options" :key="o.value" :value="o.value">{{ o.label }}</option>
    </select>
    <p v-if="error" :id="errorId" class="mt-1 text-sm text-red-700">{{ error }}</p>
    <p v-else-if="hint" :id="hintId" class="mt-1 text-sm text-slate-600">{{ hint }}</p>
  </div>
</template>
