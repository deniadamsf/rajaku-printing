<script setup lang="ts">
/**
 * ProductSlider — auto-slider crossfade untuk sorotan produk (brief "kesan mahal").
 *
 * Data produk NYATA dari catalog (SSR via `useAsyncData`, sumber sama dengan
 * `LandingServicesSection`) — bukan konten dummy. Katalog belum punya field
 * gambar per-produk (lihat `types/catalog.ts`), jadi tiap slide dipasangkan
 * foto proses cetak asli (`/proses/*.webp`) secara berulang sebagai backdrop.
 * Daftar produk LENGKAP & sepenuhnya crawlable tetap ada di
 * `LandingServicesSection` (grid statis, semua produk, semua di HTML awal) —
 * slider ini murni sorotan visual di atasnya, jadi aman kalau slide non-aktif
 * "kurang menonjol" secara SEO (bukan satu-satunya sumber info produk).
 *
 * Motion: crossfade via prop `:animate` reaktif motion-v (BUKAN AnimatePresence)
 * — semua slide tetap satu kali di-mount (SSR-friendly, tanpa unmount/remount
 * bolak-balik), cuma `opacity` yang berubah. Auto-advance tiap ~6.5 detik,
 * transisi 700ms ease-out, hanya opacity (§26.6 — no layout reflow).
 *
 * Interaksi wajib (brief slider):
 * - Jeda otomatis saat hover ATAU fokus keyboard di dalam slider.
 * - Kontrol manual (panah + dot), keyboard-reachable, focus ring §26.6.
 * - `prefers-reduced-motion`: auto-advance mati total, transisi jadi instan
 *   (durasi 0) — kontrol manual tetap berfungsi, cuma tanpa animasi.
 * - Tanpa `aria-live` cerewet — cukup `aria-label` wajar di tiap kontrol.
 */
import { ArrowRight, ChevronLeft, ChevronRight } from '@lucide/vue'
import { motion } from 'motion-v'
import type { CatalogProduct } from '~/types/catalog'

const catalog = useCatalog()
const prefersReduced = usePrefersReducedMotion()

// Loading/error/empty untuk katalog sudah ditampilkan lengkap oleh
// `LandingServicesSection` di bawah slider ini — di sini cukup no-render diam
// kalau data belum ada, supaya tidak dobel pesan error di satu halaman.
const { data } = await useAsyncData('landing-product-slider', () => catalog.listProducts())

const products = computed<CatalogProduct[]>(() => data.value?.products ?? [])

// Foto proses dipakai berulang sebagai backdrop visual (bukan foto per-produk
// asli — katalog belum menyimpan gambar produk).
const backdrops = [
  '/proses/proses-04.webp',
  '/proses/proses-02.webp',
  '/proses/proses-06.webp',
  '/proses/proses-01.webp',
  '/proses/proses-05.webp',
  '/proses/proses-03.webp',
]

interface Slide {
  id: string
  name: string
  description: string
  pricingLabel: string
  image: string
}

const slides = computed<Slide[]>(() =>
  products.value.slice(0, 6).map((p, i) => ({
    id: p.id,
    name: p.name,
    description:
      p.description || 'Cetak presisi, warna konsisten — siap pakai untuk kebutuhan Anda.',
    pricingLabel: p.pricing_type === 'paket' ? 'Harga paket' : 'Harga per m²',
    image: backdrops[i % backdrops.length],
  })),
)

const active = ref(0)
const paused = ref(false)
const DWELL_MS = 6500

let timer: ReturnType<typeof setTimeout> | null = null

function clearTimer() {
  if (timer) {
    clearTimeout(timer)
    timer = null
  }
}

function scheduleNext() {
  clearTimer()
  if (prefersReduced.value || paused.value || slides.value.length <= 1) return
  timer = setTimeout(() => {
    active.value = (active.value + 1) % slides.value.length
    scheduleNext()
  }, DWELL_MS)
}

function goTo(i: number) {
  active.value = i
  scheduleNext()
}

function prev() {
  goTo((active.value - 1 + slides.value.length) % slides.value.length)
}

function next() {
  goTo((active.value + 1) % slides.value.length)
}

function onEnter() {
  paused.value = true
  clearTimer()
}

function onLeave() {
  paused.value = false
  scheduleNext()
}

watch(prefersReduced, (reduced) => {
  if (reduced) clearTimer()
  else scheduleNext()
})

onMounted(scheduleNext)
onUnmounted(clearTimer)

const slideTransition = computed(() =>
  prefersReduced.value ? { duration: 0 } : { duration: 0.7, ease: [0.22, 1, 0.36, 1] as const },
)

const revealTransition = computed(() =>
  prefersReduced.value ? { duration: 0 } : { duration: 0.5, ease: [0.22, 1, 0.36, 1] as const },
)
</script>

<template>
  <section v-if="slides.length > 0" id="produk-unggulan" class="mx-auto max-w-6xl px-4 py-16 md:py-24">
    <div class="max-w-2xl">
      <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Sorotan Produk</p>
      <h2 class="mt-3 text-2xl md:text-3xl font-serif font-semibold tracking-tight text-ink-950">
        Produk cetak pilihan
      </h2>
    </div>

    <motion.div
      class="relative mt-10 aspect-[4/5] w-full overflow-hidden rounded-lg border border-hairline bg-ink-950 sm:aspect-[16/9] md:aspect-[21/9]"
      role="group"
      aria-roledescription="carousel"
      aria-label="Sorotan produk"
      :initial="{ opacity: 0, y: prefersReduced ? 0 : 16 }"
      :while-in-view="{ opacity: 1, y: 0 }"
      :in-view-options="{ once: true, margin: '-100px' }"
      :transition="revealTransition"
      @mouseenter="onEnter"
      @mouseleave="onLeave"
      @focusin="onEnter"
      @focusout="onLeave"
    >
      <motion.div
        v-for="(s, i) in slides"
        :key="s.id"
        class="absolute inset-0"
        :class="i === active ? 'pointer-events-auto' : 'pointer-events-none'"
        :data-carousel-slide="i"
        :animate="{ opacity: i === active ? 1 : 0 }"
        :transition="slideTransition"
        :aria-hidden="i !== active"
      >
        <img
          :src="s.image"
          alt=""
          width="1600"
          height="900"
          :loading="i === 0 ? 'eager' : 'lazy'"
          class="h-full w-full object-cover"
        >
        <div class="absolute inset-0 bg-gradient-to-t from-ink-950/90 via-ink-950/35 to-ink-950/10" />
        <div class="absolute inset-x-0 bottom-0 p-6 md:p-10">
          <span
            class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium text-gold-400 ring-1 ring-inset ring-gold-500/40"
          >
            {{ s.pricingLabel }}
          </span>
          <h3 class="mt-3 text-xl md:text-3xl font-serif font-semibold tracking-tight text-canvas">
            {{ s.name }}
          </h3>
          <p class="mt-2 max-w-lg text-sm leading-relaxed text-canvas/75 line-clamp-2">
            {{ s.description }}
          </p>
          <NuxtLink
            to="/order"
            :tabindex="i === active ? 0 : -1"
            class="mt-5 inline-flex items-center gap-2 text-sm font-semibold text-canvas transition-colors hover:text-gold-400 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold-400/60 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950 rounded-sm"
          >
            Order sekarang
            <ArrowRight class="h-4 w-4" :stroke-width="1.5" />
          </NuxtLink>
        </div>
      </motion.div>

      <!-- Panah manual -->
      <button
        v-if="slides.length > 1"
        type="button"
        class="absolute left-3 top-1/2 z-10 inline-flex h-9 w-9 -translate-y-1/2 items-center justify-center rounded-full bg-ink-950/50 text-canvas backdrop-blur transition-colors duration-200 ease-out hover:bg-ink-950/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold-400/60 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
        aria-label="Produk sebelumnya"
        @click="prev"
      >
        <ChevronLeft class="h-5 w-5" :stroke-width="1.75" />
      </button>
      <button
        v-if="slides.length > 1"
        type="button"
        class="absolute right-3 top-1/2 z-10 inline-flex h-9 w-9 -translate-y-1/2 items-center justify-center rounded-full bg-ink-950/50 text-canvas backdrop-blur transition-colors duration-200 ease-out hover:bg-ink-950/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold-400/60 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
        aria-label="Produk berikutnya"
        @click="next"
      >
        <ChevronRight class="h-5 w-5" :stroke-width="1.75" />
      </button>

      <!-- Dot indicator -->
      <div v-if="slides.length > 1" class="absolute inset-x-0 bottom-3 z-10 flex items-center justify-center gap-1">
        <button
          v-for="(s, i) in slides"
          :key="s.id"
          type="button"
          class="group inline-flex h-6 w-6 items-center justify-center rounded-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold-400/60 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
          :aria-label="`Ke slide ${i + 1}: ${s.name}`"
          :aria-current="i === active ? 'true' : undefined"
          @click="goTo(i)"
        >
          <span
            class="block h-1.5 rounded-full transition-[width,background-color] duration-200 ease-out"
            :class="i === active ? 'w-6 bg-gold-400' : 'w-1.5 bg-canvas/40 group-hover:bg-canvas/70'"
          />
        </button>
      </div>
    </motion.div>
  </section>
</template>
