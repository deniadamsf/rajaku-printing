<script setup lang="ts">
import { AnimatePresence, motion } from 'motion-v'

interface Props {
  message: string
  variant?: 'error' | 'success' | 'info'
}

withDefaults(defineProps<Props>(), {
  variant: 'error',
})

// error → brand crimson wash (bukan rose bawaan Tailwind — CLAUDE.md §26.1).
// success/info tetap semantic (emerald/sky) sesuai §26.10.
const variantClasses = {
  error: 'bg-brand-50 border-brand-200 text-brand-800',
  success: 'bg-emerald-50 border-emerald-200 text-emerald-800',
  info: 'bg-sky-50 border-sky-200 text-sky-800',
} as const
</script>

<template>
  <AnimatePresence>
    <motion.div
      v-if="message"
      :initial="{ opacity: 0, y: -6 }"
      :animate="{ opacity: 1, y: 0 }"
      :exit="{ opacity: 0, y: -6 }"
      :transition="{ duration: 0.2 }"
      class="rounded-md border px-3 py-2 text-sm"
      :class="variantClasses[variant]"
      role="alert"
    >
      {{ message }}
    </motion.div>
  </AnimatePresence>
</template>
