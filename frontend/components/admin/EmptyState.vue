<script setup lang="ts">
/**
 * EmptyState — placeholder saat data kosong. Ikon default Inbox (monoline).
 * Caller boleh override lewat slot #icon kalau butuh ikon lain.
 */
import type { Component } from 'vue'
import { Inbox } from '@lucide/vue'

defineProps<{
  title: string
  message?: string
  /** Override ikon default. */
  icon?: Component
}>()
</script>

<template>
  <div class="rounded-lg border border-dashed border-hairline bg-canvas px-6 py-14 text-center">
    <div class="mx-auto mb-3 flex h-10 w-10 items-center justify-center rounded-md bg-canvas-alt text-ink-400">
      <slot name="icon">
        <component :is="icon || Inbox" class="h-5 w-5" :stroke-width="1.5" />
      </slot>
    </div>
    <h3 class="text-sm font-semibold text-ink-900">{{ title }}</h3>
    <p v-if="message" class="mt-1 text-sm text-ink-500">{{ message }}</p>
    <div class="mt-4">
      <slot />
    </div>
  </div>
</template>
