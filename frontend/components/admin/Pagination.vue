<script setup lang="ts">
const props = defineProps<{
  page: number
  limit: number
  total: number
}>()

const emit = defineEmits<{
  (e: 'update:page', value: number): void
}>()

const totalPages = computed(() => Math.max(1, Math.ceil(props.total / props.limit)))
const startItem = computed(() => (props.total === 0 ? 0 : (props.page - 1) * props.limit + 1))
const endItem = computed(() => Math.min(props.total, props.page * props.limit))

function go(p: number) {
  const clamped = Math.min(Math.max(1, p), totalPages.value)
  if (clamped !== props.page) emit('update:page', clamped)
}
</script>

<template>
  <div class="flex items-center justify-between text-sm text-ink-500 mt-4">
    <p>
      Menampilkan
      <span class="font-medium text-ink-900">{{ startItem }}–{{ endItem }}</span>
      dari
      <span class="font-medium text-ink-900">{{ total }}</span>
    </p>
    <div class="flex items-center gap-1">
      <button
        type="button"
        class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
        :disabled="page <= 1"
        @click="go(page - 1)"
      >
        ‹ Prev
      </button>
      <span class="px-2 text-xs text-ink-500">Hal. {{ page }} / {{ totalPages }}</span>
      <button
        type="button"
        class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
        :disabled="page >= totalPages"
        @click="go(page + 1)"
      >
        Next ›
      </button>
    </div>
  </div>
</template>
