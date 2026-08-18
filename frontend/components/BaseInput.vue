<script setup lang="ts">
interface Props {
  id: string
  label: string
  type?: string
  modelValue: string
  placeholder?: string
  autocomplete?: string
  required?: boolean
  disabled?: boolean
  error?: string
  helper?: string
}

const props = withDefaults(defineProps<Props>(), {
  type: 'text',
  placeholder: '',
  autocomplete: 'off',
  required: false,
  disabled: false,
  error: '',
  helper: '',
})

const emit = defineEmits<{
  (e: 'update:modelValue', v: string): void
}>()

// Expose `focus()` supaya parent bisa `templateRef.value?.focus()` pada
// komponen ini (mis. auto-focus ulang input setelah error/ganti sub-state) —
// template ref ke komponen custom bukan langsung ke elemen DOM.
const inputEl = ref<HTMLInputElement | null>(null)
defineExpose({
  focus: () => inputEl.value?.focus(),
})
</script>

<template>
  <div>
    <label :for="id" class="block text-sm font-medium text-ink-900">
      {{ label }}
      <span v-if="required" class="text-brand-500">*</span>
    </label>
    <input
      :id="id"
      ref="inputEl"
      :type="type"
      :value="modelValue"
      :placeholder="placeholder"
      :autocomplete="autocomplete"
      :required="required"
      :disabled="disabled"
      :aria-invalid="error ? 'true' : 'false'"
      :aria-describedby="error ? `${id}-error` : helper ? `${id}-helper` : undefined"
      class="mt-1 block w-full rounded-md border bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 transition-colors focus:outline-none focus:ring-2 disabled:cursor-not-allowed disabled:bg-canvas-alt disabled:text-ink-500"
      :class="[
        error
          ? 'border-brand-500 focus:border-brand-500 focus:ring-brand-500/30'
          : 'border-hairline focus:border-brand-500 focus:ring-brand-500/20',
      ]"
      @input="emit('update:modelValue', ($event.target as HTMLInputElement).value)"
    >
    <p v-if="error" :id="`${id}-error`" class="mt-1 text-xs text-brand-700">
      {{ error }}
    </p>
    <p v-else-if="helper" :id="`${id}-helper`" class="mt-1 text-xs text-ink-500">
      {{ helper }}
    </p>
  </div>
</template>
