<script setup lang="ts">
/**
 * /lacak — Landing untuk input resi. Redirect ke /lacak/:resi setelah user
 * kirim.
 */
import { Search, Package } from '@lucide/vue'

definePageMeta({ layout: 'default' })

useSeoMeta({
  title: 'Lacak Pesanan — Rajaku Printing',
  description: 'Cek status pesanan Anda dengan memasukkan nomor resi RJK-XXXXXXXX.',
  robots: 'noindex,nofollow',
})

const resi = ref('')

// Normalisasi format resi — kapital + trim + auto-add prefix RJK- kalau user
// hanya input suffix (8 alfanumerik).
const normalizedResi = computed(() => {
  const raw = resi.value.trim().toUpperCase().replace(/\s+/g, '')
  if (!raw) return ''
  if (raw.startsWith('RJK-')) return raw
  if (/^[A-Z0-9]+$/.test(raw)) return `RJK-${raw}`
  return raw
})

const canSubmit = computed(() => /^RJK-[A-Z0-9]+$/.test(normalizedResi.value))

async function onSubmit() {
  if (!canSubmit.value) return
  await navigateTo(`/lacak/${normalizedResi.value}`)
}
</script>

<template>
  <main class="mx-auto max-w-md px-4 py-16 md:py-24">
    <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2 flex items-center gap-2">
      <Search class="h-3.5 w-3.5" :stroke-width="1.75" />
      Lacak Pesanan
    </p>
    <h1 class="font-serif text-2xl md:text-3xl font-semibold tracking-tight text-ink-950">
      Cek status pesanan Anda
    </h1>
    <p class="mt-2 text-sm text-ink-500 leading-relaxed">
      Masukkan nomor resi yang Anda dapat setelah order. Format:
      <span class="font-mono text-ink-700">RJK-XXXXXXXX</span>.
    </p>

    <form class="mt-8 space-y-4" @submit.prevent="onSubmit">
      <div>
        <label for="lacak-resi" class="block text-sm font-medium text-ink-900">Nomor resi</label>
        <input
          id="lacak-resi"
          v-model="resi"
          type="text"
          required
          autofocus
          autocomplete="off"
          spellcheck="false"
          placeholder="RJK-8F3K2A9X"
          class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm font-mono uppercase tracking-wider placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
        >
        <p v-if="resi && !canSubmit" class="mt-1 text-xs text-brand-700">
          Format resi tidak valid. Contoh: <span class="font-mono">RJK-8F3K2A9X</span>.
        </p>
      </div>

      <button
        type="submit"
        :disabled="!canSubmit"
        class="inline-flex w-full items-center justify-center gap-2 rounded-md bg-brand-500 px-4 py-2.5 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
      >
        <Search class="h-4 w-4" :stroke-width="1.75" />
        Cek status
      </button>
    </form>

    <div class="mt-10 rounded-lg border border-hairline bg-canvas p-5 text-xs text-ink-500 leading-relaxed">
      <div class="flex items-start gap-2">
        <Package class="h-3.5 w-3.5 flex-none mt-0.5" :stroke-width="1.75" />
        <p>
          Belum pernah order?
          <NuxtLink to="/order" class="font-medium text-brand-700 hover:underline">Order banner</NuxtLink>
          langsung — Anda akan dapat resi setelah pesanan tercatat.
        </p>
      </div>
    </div>
  </main>
</template>
