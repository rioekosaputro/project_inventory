<script setup lang="ts">
import { computed } from 'vue'
import Textarea from 'primevue/textarea'
import { useBlurField } from '@/composables/useBlurField'

const props = withDefaults(
  defineProps<{
    name: string
    label: string
    hint?: string
    required?: boolean
    rows?: number
  }>(),
  { rows: 3 },
)

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
    <Textarea
      :id="id"
      v-model="value"
      :name="name"
      :rows="rows"
      :invalid="!!error"
      :aria-invalid="error ? 'true' : 'false'"
      :aria-describedby="describedBy"
      fluid
      auto-resize
      class="mt-1 min-h-11"
      @blur="onBlur"
      @focus="onFocus"
      @input="onInput"
    />
    <p v-if="error" :id="errorId" class="mt-1 text-sm text-red-700">{{ error }}</p>
    <p v-else-if="hint" :id="hintId" class="mt-1 text-sm text-slate-600">{{ hint }}</p>
  </div>
</template>
