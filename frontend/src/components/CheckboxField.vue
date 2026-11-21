<script setup lang="ts">
import { computed } from 'vue'
import { useField } from 'vee-validate'

const props = defineProps<{
  name: string
  label: string
  hint?: string
  disabled?: boolean
}>()

const { value, errorMessage } = useField<boolean>(props.name)

const id = computed(() => `field-${props.name}`)
const hintId = computed(() => `${id.value}-hint`)
const errorId = computed(() => `${id.value}-error`)
const describedBy = computed(() =>
  errorMessage.value ? errorId.value : props.hint ? hintId.value : undefined,
)
</script>

<template>
  <div>
    <!-- Seluruh baris adalah area sentuh (min. 44px). -->
    <label
      :for="id"
      class="flex min-h-11 cursor-pointer items-center gap-3 text-sm font-medium text-slate-800"
    >
      <input
        :id="id"
        v-model="value"
        type="checkbox"
        :name="name"
        :disabled="disabled"
        :aria-invalid="errorMessage ? 'true' : 'false'"
        :aria-describedby="describedBy"
        class="size-5 rounded border-slate-400 accent-indigo-700"
      />
      <span>{{ label }}</span>
    </label>
    <p v-if="errorMessage" :id="errorId" class="ms-8 text-sm text-red-700">{{ errorMessage }}</p>
    <p v-else-if="hint" :id="hintId" class="ms-8 text-sm text-slate-600">{{ hint }}</p>
  </div>
</template>
