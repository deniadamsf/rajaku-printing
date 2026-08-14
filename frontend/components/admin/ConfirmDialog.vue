<script setup lang="ts">
/**
 * ConfirmDialog — modal konfirmasi untuk aksi destructive/irreversible.
 * Controlled via v-model:open. Emit confirm & cancel.
 *
 * Variant:
 *   - default: primary CTA warna ink-950 (netral serious)
 *   - danger:  primary CTA brand crimson (§26.5 destructive pakai brand, bukan rose)
 */
const props = withDefaults(
  defineProps<{
    open: boolean
    title: string
    message?: string
    confirmLabel?: string
    cancelLabel?: string
    variant?: 'default' | 'danger'
    /** Disable confirm sementara aksi async berjalan. */
    loading?: boolean
  }>(),
  {
    confirmLabel: 'Konfirmasi',
    cancelLabel: 'Batal',
    variant: 'default',
    loading: false,
  },
)

const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'confirm'): void
  (e: 'cancel'): void
}>()

function close() {
  emit('update:open', false)
  emit('cancel')
}
function confirm() {
  emit('confirm')
}

const confirmClass = computed(() =>
  props.variant === 'danger'
    ? 'bg-brand-500 hover:bg-brand-600 focus-visible:ring-brand-500/40'
    : 'bg-ink-950 hover:bg-ink-900 focus-visible:ring-brand-500/40',
)
</script>

<template>
  <Teleport to="body">
    <Transition name="fade">
      <div v-if="open" class="fixed inset-0 z-50 flex items-center justify-center p-4">
        <div class="absolute inset-0 bg-ink-950/60" aria-hidden="true" @click="!loading && close()" />
        <div
          role="dialog"
          aria-modal="true"
          class="relative w-full max-w-md rounded-lg bg-canvas shadow-xl ring-1 ring-black/5 p-6"
        >
          <h3 class="font-serif text-lg font-semibold text-ink-950">{{ title }}</h3>
          <p v-if="message" class="mt-2 text-sm text-ink-500 leading-relaxed">{{ message }}</p>
          <div class="mt-5 flex justify-end gap-2">
            <button
              type="button"
              class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-50"
              :disabled="loading"
              @click="close"
            >
              {{ cancelLabel }}
            </button>
            <button
              type="button"
              :class="['inline-flex items-center gap-2 rounded-md px-3 py-1.5 text-sm font-semibold text-canvas focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-60', confirmClass]"
              :disabled="loading"
              @click="confirm"
            >
              <span
                v-if="loading"
                class="inline-block h-3 w-3 rounded-full border-2 border-canvas/70 border-t-transparent animate-spin"
                aria-hidden="true"
              />
              {{ confirmLabel }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.15s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
