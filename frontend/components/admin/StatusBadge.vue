<script setup lang="ts">
/**
 * StatusBadge — colored pill untuk status artikel/order/payment/dll.
 * Preset warna berdasarkan kata kunci status; fallback ke neutral.
 *
 * Tone `rose` di-map ke brand crimson wash sesuai §26.5 (destructive/error
 * pakai brand-*, bukan rose bawaan Tailwind). Tone netral/nonaktif bernama
 * `ink` (warm gray) — bukan `slate`, yang dilarang §26.9.
 */
const props = defineProps<{
  status: string
  /** Override warna manual kalau perlu. */
  tone?: 'green' | 'amber' | 'rose' | 'ink' | 'sky'
}>()

// Map keyword → tone. Match paling spesifik dulu.
const presetTone = computed<'green' | 'amber' | 'rose' | 'ink' | 'sky'>(() => {
  if (props.tone) return props.tone
  const s = props.status.toLowerCase()
  if (['published', 'dibayar', 'approved', 'selesai', 'siap_kirim', 'siap_ambil', 'verified'].some((k) => s.includes(k))) return 'green'
  if (['draft', 'pending', 'menunggu'].some((k) => s.includes(k))) return 'amber'
  if (['rejected', 'ditolak', 'dibatalkan', 'failed', 'error'].some((k) => s.includes(k))) return 'rose'
  if (['archived', 'purged'].some((k) => s.includes(k))) return 'ink'
  if (['dikirim', 'proses', 'shipping'].some((k) => s.includes(k))) return 'sky'
  return 'ink'
})

const toneClass = computed(() => {
  switch (presetTone.value) {
    case 'green': return 'bg-emerald-50 text-emerald-800 ring-emerald-200'
    case 'amber': return 'bg-amber-50 text-amber-800 ring-amber-200'
    case 'rose':  return 'bg-brand-50 text-brand-700 ring-brand-200'
    case 'sky':   return 'bg-sky-50 text-sky-800 ring-sky-200'
    default:      return 'bg-ink-100 text-ink-700 ring-ink-200'
  }
})
</script>

<template>
  <span
    :class="['inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset', toneClass]"
  >
    {{ status }}
  </span>
</template>
