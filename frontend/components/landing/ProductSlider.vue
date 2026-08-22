<script setup lang="ts">
/**
 * ProductSlider — auto-slider crossfade untuk sorotan produk (brief "kesan mahal").
 *
 * Data produk NYATA dari catalog (SSR via `useAsyncData`, sumber sama dengan
 * `LandingServicesSection`) — bukan konten dummy. Sumber gambar per slide:
 * `product.image_url` (foto asli, kalau admin sudah unggah) — kalau kosong,
 * artwork vektor per jenis produk (`artworkVariantFor()` → `<ArtProduct>`,
 * lihat `utils/artwork.ts`), BUKAN lagi satu foto proses cetak yang sama
 * diulang untuk semua slide (kesan template kosong).
 *
 * Kontras teks — dua treatment berbeda tergantung sumber gambar:
 *  - Foto asli: full-bleed `object-cover` + gradient scrim gelap di bawah
 *    (pola lama, sudah aman kontras — teks canvas selalu di zona scrim
 *    paling pekat/90%).
 *  - Artwork (fallback): TIDAK dipasang di bawah scrim gelap yang sama,
 *    karena scrim akan menutupi sebagian besar detail ilustrasi — padahal
 *    tujuan artwork justru menonjolkan bentuk khas tiap produk. Sebagai
 *    gantinya artwork dibingkai sebagai panel terpisah (rounded, latar
 *    terangnya sendiri) yang diletakkan di atas latar section yang SUDAH
 *    gelap (`bg-ink-950` di container slider) — teks selalu duduk langsung
 *    di atas ink-950 solid, kontras terjamin tanpa perlu overlay tambahan.
 *
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
 * - Geser jari (HP) / seret kursor (desktop), plus panah kiri-kanan keyboard.
 * - `prefers-reduced-motion`: auto-advance mati total, transisi jadi instan
 *   (durasi 0) — kontrol manual tetap berfungsi, cuma tanpa animasi.
 * - Tanpa `aria-live` cerewet — cukup `aria-label` wajar di tiap kontrol.
 */
import { ArrowRight, ChevronLeft, ChevronRight } from '@lucide/vue'
import { motion } from 'motion-v'
import type { CatalogProduct } from '~/types/catalog'
import type { ArtworkVariant } from '~/utils/artwork'

const catalog = useCatalog()
const prefersReduced = usePrefersReducedMotion()

// Loading/error/empty untuk katalog sudah ditampilkan lengkap oleh
// `LandingServicesSection` di bawah slider ini — di sini cukup no-render diam
// kalau data belum ada, supaya tidak dobel pesan error di satu halaman.
const { data } = await useAsyncData('landing-product-slider', () => catalog.listProducts())

const products = computed<CatalogProduct[]>(() => data.value?.products ?? [])

interface Slide {
  id: string
  name: string
  description: string
  pricingLabel: string
  image: string | null
  variant: ArtworkVariant
}

const slides = computed<Slide[]>(() =>
  products.value.slice(0, 6).map((p) => ({
    id: p.id,
    name: p.name,
    description:
      p.description || 'Cetak presisi, warna konsisten — siap pakai untuk kebutuhan Anda.',
    pricingLabel: p.pricing_type === 'paket' ? 'Harga paket' : 'Harga per m²',
    image: p.image_url || null,
    variant: artworkVariantFor(p),
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

// --- Geser jari / seret kursor -------------------------------------------
// Panah & titik saja tidak cukup: di HP, refleks pertama orang terhadap
// gambar besar adalah menggesernya, dan slider yang tidak merespons gesekan
// terasa mati. Ambang 40px supaya sentuhan/klik biasa pada tautan di dalam
// slide tidak ikut terbaca sebagai geseran.
const SWIPE_THRESHOLD_PX = 40
let pointerStartX: number | null = null

function onPointerDown(e: PointerEvent) {
  // Abaikan klik kanan/tengah — hanya gerakan utama yang menggeser slide.
  if (e.button !== 0) return
  pointerStartX = e.clientX
}

function onPointerUp(e: PointerEvent) {
  if (pointerStartX === null) return
  const dx = e.clientX - pointerStartX
  pointerStartX = null
  if (Math.abs(dx) < SWIPE_THRESHOLD_PX) return
  if (dx < 0) next()
  else prev()
}

function onPointerCancel() {
  pointerStartX = null
}

// Panah kiri/kanan saat slider difokuskan keyboard — pasangan wajar dari
// gesekan di HP, dan membuat slider bisa dijalankan tanpa tetikus.
function onKeydown(e: KeyboardEvent) {
  if (e.key === 'ArrowLeft') {
    e.preventDefault()
    prev()
  } else if (e.key === 'ArrowRight') {
    e.preventDefault()
    next()
  }
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
  <section v-if="slides.length > 0" id="produk-unggulan" class="mx-auto max-w-6xl px-4 py-12 md:py-20">
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
      :in-view-options="{ once: true, margin: '-40px' }"
      :transition="revealTransition"
      @mouseenter="onEnter"
      @mouseleave="onLeave"
      @focusin="onEnter"
      @focusout="onLeave"
      @pointerdown="onPointerDown"
      @pointerup="onPointerUp"
      @pointercancel="onPointerCancel"
      @keydown="onKeydown"
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
        <!-- Foto asli produk: full-bleed + scrim gelap (kontras teks terjamin). -->
        <template v-if="s.image">
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
        </template>

        <!--
          Artwork fallback: TIDAK ditumpuk scrim gelap (akan menutupi detail
          ilustrasi). Sebagai gantinya, artwork dibingkai jadi panel terpisah
          di atas latar ink-950 milik container — teks selalu di atas solid
          dark, kontras terjamin tanpa mengorbankan visibilitas artwork.
        -->
        <div
          v-else
          class="flex h-full flex-col-reverse items-center justify-center gap-6 px-6 py-8 md:flex-row md:justify-between md:gap-10 md:px-12 lg:px-16"
        >
          <div class="w-full max-w-md text-center md:text-left">
            <span
              class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium text-gold-400 ring-1 ring-inset ring-gold-500/40"
            >
              {{ s.pricingLabel }}
            </span>
            <h3 class="mt-3 text-xl md:text-3xl font-serif font-semibold tracking-tight text-canvas">
              {{ s.name }}
            </h3>
            <p class="mt-2 text-sm leading-relaxed text-canvas/75 line-clamp-2">
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
          <div
            class="relative aspect-[4/3] w-full max-w-[240px] shrink-0 overflow-hidden rounded-lg ring-1 ring-canvas/10 sm:max-w-[280px] md:max-w-sm"
          >
            <ArtProduct :variant="s.variant" :label="s.name" />
          </div>
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
