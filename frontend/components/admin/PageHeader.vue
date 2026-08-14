<script setup lang="ts">
/**
 * PageHeader — konsisten header untuk halaman admin. Title (Fraunces) +
 * subtitle + slot untuk action button (kanan atas).
 */
defineProps<{
  title: string
  subtitle?: string
  /** Breadcrumb items sederhana (label + to). */
  breadcrumb?: Array<{ label: string; to?: string }>
}>()
</script>

<template>
  <div class="mb-6">
    <nav
      v-if="breadcrumb && breadcrumb.length"
      class="mb-2 text-xs text-ink-500 flex items-center gap-1"
      aria-label="Breadcrumb"
    >
      <template v-for="(b, i) in breadcrumb" :key="i">
        <NuxtLink v-if="b.to" :to="b.to" class="hover:text-ink-900 transition-colors">{{ b.label }}</NuxtLink>
        <span v-else>{{ b.label }}</span>
        <span v-if="i < breadcrumb.length - 1" class="text-ink-400">/</span>
      </template>
    </nav>

    <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
      <div>
        <h1 class="font-serif text-2xl md:text-3xl font-semibold tracking-tight text-ink-950">{{ title }}</h1>
        <p v-if="subtitle" class="mt-1 text-sm text-ink-500 leading-relaxed max-w-2xl">{{ subtitle }}</p>
      </div>
      <div class="flex items-center gap-2">
        <slot name="actions" />
      </div>
    </div>
  </div>
</template>
