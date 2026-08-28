<script setup lang="ts">
/**
 * AdminToggleSwitch — switch on/off aksesibel untuk setting boolean (mis.
 * `membership_enabled`, §30.1). Dipakai lewat `v-model` (boolean).
 *
 * Kenapa komponen baru, bukan `<input type="checkbox">` polos: pola switch
 * (track + knob bergeser) lebih jelas menyampaikan "saklar on/off yang
 * langsung berefek" dibanding checkbox kotak, yang secara konvensi dibaca
 * sebagai "pilih dari daftar". Dibangun native `<button role="switch">`
 * (bukan library terpisah) — cukup ringan untuk satu pola, dan
 * `aria-checked` + fokus keyboard sudah didapat gratis dari role ini.
 */
const props = withDefaults(
  defineProps<{
    modelValue: boolean
    disabled?: boolean
    /** Label aksesibel untuk screen reader — WAJIB kalau tidak ada `<label>` terpisah yang mem-`for` ke id ini. */
    ariaLabel?: string
  }>(),
  { disabled: false, ariaLabel: undefined },
)

const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()

function toggle() {
  if (props.disabled) return
  emit('update:modelValue', !props.modelValue)
}
</script>

<template>
  <button
    type="button"
    role="switch"
    :aria-checked="modelValue"
    :aria-label="ariaLabel"
    :disabled="disabled"
    :class="[
      'relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors duration-200 ease-out',
      'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas',
      modelValue ? 'bg-brand-500' : 'bg-ink-200',
      disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer',
    ]"
    @click="toggle"
  >
    <span
      :class="[
        'inline-block h-[18px] w-[18px] transform rounded-full bg-canvas shadow-sm transition-transform duration-200 ease-out',
        modelValue ? 'translate-x-[22px]' : 'translate-x-[3px]',
      ]"
    />
  </button>
</template>
