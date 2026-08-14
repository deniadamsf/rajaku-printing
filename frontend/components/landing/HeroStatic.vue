<script setup lang="ts">
/**
 * HeroStatic — hero mobile statis (CLAUDE.md §18 Layar 1).
 * Poster tunggal + headline + CTA "Order Banner", animasi ringan fade/slide via
 * motion-v saja. TIDAK memuat frame sequence maupun GSAP — komponen ini dipilih
 * lewat conditional rendering `useDevice()` di halaman induk, bukan CSS display:none,
 * supaya bundle desktop (HeroScrollScrub + gsap) benar-benar tidak terkirim ke mobile.
 */
import { ArrowRight } from '@lucide/vue'
import { motion } from 'motion-v'
</script>

<template>
  <section class="relative overflow-hidden bg-ink-950">
    <img
      src="/hero/poster-mobile.webp"
      alt="Proses cetak banner large-format Rajaku Printing"
      width="828"
      height="466"
      fetchpriority="high"
      class="absolute inset-0 h-full w-full object-cover"
    />
    <div class="absolute inset-0 bg-gradient-to-t from-ink-950/90 via-ink-950/45 to-ink-950/15" />

    <motion.div
      class="relative z-10 px-4 pt-24 pb-16 text-center"
      :initial="{ opacity: 0, y: 16 }"
      :animate="{ opacity: 1, y: 0 }"
      :transition="{ duration: 0.5, ease: 'easeOut' }"
    >
      <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-canvas/70">
        Percetakan Banner &middot; Trenggalek
      </p>
      <h1 class="mt-4 text-4xl font-serif font-semibold tracking-tight leading-[1.05] text-canvas">
        Cetak Banner, <span class="text-gold-400">Presisi</span> Setiap Warna
      </h1>
      <p class="mx-auto mt-4 max-w-sm text-sm leading-relaxed text-canvas/80">
        Order online, upload desain sendiri atau minta dibuatkan tim kami, lalu
        lacak progres cetaknya sampai siap diambil.
      </p>

      <div class="mt-8 flex flex-col gap-3">
        <NuxtLink
          to="/order"
          class="inline-flex items-center justify-center gap-2 rounded-md bg-brand-500 px-6 py-3 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
        >
          Order Banner
          <ArrowRight class="h-4 w-4" :stroke-width="1.5" />
        </NuxtLink>
        <NuxtLink
          to="/showcase"
          class="inline-flex items-center justify-center gap-2 rounded-md border border-canvas/25 px-6 py-3 text-sm font-semibold text-canvas transition-colors hover:bg-canvas/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
        >
          Lihat Showcase
        </NuxtLink>
        <NuxtLink
          to="/lacak"
          class="inline-flex items-center justify-center gap-2 text-sm font-medium text-canvas/70 transition-colors hover:text-canvas focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950 rounded-sm"
        >
          Lacak Resi
        </NuxtLink>
      </div>
    </motion.div>
  </section>
</template>
