<script setup lang="ts">
import { computed, ref } from 'vue'
import InputText from 'primevue/inputtext'
import { useBlurField } from '@/composables/useBlurField'

const props = withDefaults(
  defineProps<{
    /** Nama field pada form vee-validate. */
    name: string
    label: string
    type?: 'text' | 'email' | 'password' | 'date'
    autocomplete?: string
    inputmode?: 'text' | 'email' | 'numeric' | 'decimal' | 'tel'
    enterkeyhint?: 'next' | 'done' | 'go' | 'send'
    /** Teks bantuan; digantikan pesan error saat field invalid. */
    hint?: string
    required?: boolean
    /** Tampil tetapi tidak dapat diubah (mis. kode yang terkunci setelah dibuat). */
    readonly?: boolean
    /** Tombol tampilkan/sembunyikan (untuk password). */
    revealable?: boolean
  }>(),
  { type: 'text', required: false, revealable: false, readonly: false },
)

const { value, error, onBlur, onFocus, onInput } = useBlurField(
  () => props.name,
  () => props.label,
)

const id = computed(() => `field-${props.name}`)
const hintId = computed(() => `${id.value}-hint`)
const errorId = computed(() => `${id.value}-error`)

const revealed = ref(false)
const inputType = computed(() => (props.revealable && revealed.value ? 'text' : props.type))

// Error ditautkan ke input HANYA saat tampil (teks yang direferensikan aria-describedby
// tetap dibacakan walau tersembunyi). Bantuan dilepas saat error menggantikannya.
const describedBy = computed(() => {
  if (error.value) return errorId.value
  return props.hint ? hintId.value : undefined
})
</script>

<template>
  <div>
    <label :for="id" class="block text-sm font-medium text-slate-800">
      {{ label }}
      <span v-if="required" class="text-red-700" aria-hidden="true">*</span>
    </label>

    <div class="relative mt-1">
      <InputText
        :id="id"
        v-model="value"
        :name="name"
        :type="inputType"
        :autocomplete="autocomplete"
        :inputmode="inputmode"
        :enterkeyhint="enterkeyhint"
        :required="required"
        :readonly="readonly"
        :invalid="!!error"
        :aria-invalid="error ? 'true' : 'false'"
        :aria-describedby="describedBy"
        spellcheck="false"
        fluid
        :class="[
          'min-h-11',
          revealable ? 'pe-12' : '',
          readonly ? 'bg-slate-100 text-slate-700' : '',
        ]"
        @blur="onBlur"
        @focus="onFocus"
        @input="onInput"
      />
      <button
        v-if="revealable"
        type="button"
        class="absolute inset-y-0 end-0 inline-flex min-h-11 min-w-11 items-center justify-center rounded-e-md text-slate-600 hover:text-slate-900"
        :aria-label="revealed ? 'Sembunyikan password' : 'Tampilkan password'"
        :aria-pressed="revealed"
        @click="revealed = !revealed"
      >
        <i :class="revealed ? 'pi pi-eye-slash' : 'pi pi-eye'" aria-hidden="true" />
      </button>
    </div>

    <p v-if="error" :id="errorId" class="mt-1 text-sm text-red-700">{{ error }}</p>
    <p v-else-if="hint" :id="hintId" class="mt-1 text-sm text-slate-600">{{ hint }}</p>
  </div>
</template>
