<script setup lang="ts">
/**
 * / — Landing page.
 *
 * Masih scaffold: hero cinematic scroll-scrub (desktop, §16 poin 1) & hero statis
 * mobile (§18 Layar 1) menyusul saat modul frontend hero dibangun — halaman ini
 * cuma placeholder yang sudah patuh design language.
 *
 * Design: patuh CLAUDE.md §26 (Fraunces display + Inter body, brand/gold/ink,
 * Lucide icon — tanpa rose/slate/emoji).
 */
import { ArrowRight, Search } from '@lucide/vue'

useSeoMeta({
  title: 'Cetak Banner Cepat & Rapi',
  description:
    'Rajaku Printing — layanan cetak banner untuk usaha, event, dan kebutuhan pribadi di Trenggalek. Order online, lacak progres cetak real-time.',
})
</script>

<template>
  <section class="mx-auto max-w-6xl px-4 py-24 md:py-32 text-center">
    <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
      Percetakan Banner &middot; Trenggalek
    </p>

    <h1
      class="mt-4 text-5xl md:text-7xl font-serif font-semibold tracking-tight leading-[1.05] text-ink-950"
    >
      Rajaku <span class="text-gold-500">Printing</span>
    </h1>

    <p class="mx-auto mt-6 max-w-2xl text-base font-normal leading-relaxed text-ink-500">
      Cetak banner untuk usaha, event, dan kebutuhan pribadi. Order online, upload
      desain sendiri atau minta dibuatkan, lalu lacak progres cetaknya sampai siap
      diambil.
    </p>

    <div class="mt-10 flex flex-col sm:flex-row items-center justify-center gap-3">
      <NuxtLink
        to="/order"
        class="inline-flex w-full sm:w-auto items-center justify-center gap-2 rounded-md bg-brand-500 px-6 py-3 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
      >
        Mulai Order Banner
        <ArrowRight class="h-4 w-4" :stroke-width="1.5" />
      </NuxtLink>

      <NuxtLink
        to="/lacak"
        class="inline-flex w-full sm:w-auto items-center justify-center gap-2 rounded-md border border-hairline bg-canvas px-6 py-3 text-sm font-semibold text-ink-900 transition-colors hover:bg-canvas-alt focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
      >
        <Search class="h-4 w-4" :stroke-width="1.5" />
        Lacak Resi
      </NuxtLink>
    </div>
  </section>
</template>
