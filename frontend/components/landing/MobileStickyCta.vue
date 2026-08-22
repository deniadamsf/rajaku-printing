<script setup lang="ts">
/**
 * MobileStickyCta — sticky bottom bar mobile, CTA "Order Banner" selalu
 * terjangkau jempol (CLAUDE.md §17 & deliverable §5 brief).
 *
 * Muncul HANYA setelah user scroll melewati hero mobile (supaya tidak menutupi
 * hero & CTA yang sudah ada di sana), slide-up + fade saat muncul/hilang, exit
 * lebih cepat dari enter. Dipasang oleh `pages/index.vue` (bukan layout) — hanya
 * relevan di halaman yang punya hero long-scroll seperti landing.
 *
 * Reserve ruang di footer: `pages/index.vue` menaruh spacer `md:hidden` di akhir
 * konten supaya footer tidak ketutup bar fixed ini saat discroll ke paling bawah.
 */
import { ArrowRight } from '@lucide/vue'
import { AnimatePresence, motion } from 'motion-v'
import { useIntersectionObserver, useWindowScroll, useWindowSize } from '@vueuse/core'

const prefersReduced = usePrefersReducedMotion()
const { y } = useWindowScroll()
const { height } = useWindowSize()

/**
 * Bar ini baru boleh muncul SETELAH hero benar-benar lewat — kalau tidak, dia
 * menutupi CTA yang sudah ada di dalam hero (dua tombol "Order Banner"
 * bertumpuk di layar yang sama).
 *
 * Patokannya elemen hero itu sendiri, bukan angka piksel: tinggi hero
 * mengikuti tinggi layar (lihat HeroStatic), jadi ambang tetap seperti "560px"
 * akan salah di setiap HP yang layarnya lebih panjang atau lebih pendek.
 * Ambang piksel hanya dipakai sebagai cadangan kalau penanda hero tidak
 * ditemukan (mis. komponen ini dipasang di halaman tanpa hero).
 */
const heroEl = ref<HTMLElement | null>(null)
const heroVisible = ref(true)

onMounted(() => {
  heroEl.value = document.querySelector<HTMLElement>('[data-landing-hero]')
  if (!heroEl.value) heroVisible.value = false
})

useIntersectionObserver(heroEl, ([entry]) => {
  heroVisible.value = entry.isIntersecting
})

const visible = computed(() =>
  heroEl.value ? !heroVisible.value : y.value > height.value * 0.9,
)
</script>

<template>
  <AnimatePresence>
    <motion.div
      v-if="visible"
      key="mobile-sticky-cta"
      class="fixed inset-x-0 bottom-0 z-30 border-t border-hairline bg-canvas/95 px-4 pb-[calc(0.75rem+env(safe-area-inset-bottom))] pt-3 backdrop-blur"
      :initial="{ y: 72, opacity: 0 }"
      :animate="{
        y: 0,
        opacity: 1,
        transition: { duration: prefersReduced ? 0 : 0.25, ease: [0.22, 1, 0.36, 1] },
      }"
      :exit="{
        y: 72,
        opacity: 0,
        transition: { duration: prefersReduced ? 0 : 0.18, ease: [0.22, 1, 0.36, 1] },
      }"
    >
      <NuxtLink
        to="/order"
        class="flex items-center justify-center gap-2 rounded-md bg-brand-500 px-4 py-3 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
      >
        Order Banner
        <ArrowRight class="h-4 w-4" :stroke-width="1.5" />
      </NuxtLink>
    </motion.div>
  </AnimatePresence>
</template>
