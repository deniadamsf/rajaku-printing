<script setup lang="ts">
/**
 * /showcase — Layar 2 (§18): galeri produk + preview 3D banner.
 *
 * Ini tujuan CTA sekunder dari hero mobile (`HeroStatic`) — dipisah jadi
 * halaman sendiri supaya TresJS (§16 poin 2) TIDAK pernah ikut termuat di
 * homepage. Viewer di-lazy-load lewat `ShowcaseBannerViewer` (ClientOnly +
 * IntersectionObserver di dalam komponen itu sendiri).
 *
 * Belum ada foto produk asli (lihat brief) — galeri memakai placeholder
 * monoline dua-tone (brand + ink) sesuai §26.8, bukan foto/ilustrasi berwarna.
 * Carousel mobile pakai scroll-snap CSS murni (tanpa JS tambahan); desktop
 * jadi grid — cukup lewat responsive class, tidak perlu device branching JS
 * karena tidak ada bundle berat yang perlu dihindari di sini.
 */
import {
  FlagTriangleRight,
  Gift,
  Presentation,
  Store,
  Sparkle,
  Image as ImageIcon,
  ArrowRight,
} from '@lucide/vue'

definePageMeta({ layout: 'default' })

const config = useRuntimeConfig()
const canonical = `${config.public.appBaseUrl.replace(/\/$/, '')}/showcase`

useSeoMeta({
  title: 'Showcase & Preview Desain Banner',
  description:
    'Galeri contoh kategori banner Rajaku Printing dan preview 3D interaktif — putar banner untuk lihat tampilannya sebelum order.',
  ogTitle: 'Showcase — Rajaku Printing',
  ogDescription: 'Galeri kategori banner & preview 3D interaktif sebelum Anda order.',
  ogType: 'website',
  ogUrl: canonical,
  twitterCard: 'summary',
})

useHead({
  link: [{ rel: 'canonical', href: canonical }],
})

interface GalleryItem {
  title: string
  desc: string
  icon: typeof Store
}

// Kategori generik (belum ada aset foto produk asli) — deskriptif, bukan
// data harga/produk yang direkayasa.
const galleryItems: GalleryItem[] = [
  { title: 'Banner Promosi Usaha', desc: 'Untuk toko, warung, dan bisnis lokal.', icon: Store },
  { title: 'Banner Event & Acara', desc: 'Ulang tahun, syukuran, seminar.', icon: Gift },
  { title: 'Spanduk Outdoor', desc: 'Tahan cuaca, warna presisi luar ruang.', icon: FlagTriangleRight },
  { title: 'X-Banner Pameran', desc: 'Stand pameran & booth ringkas.', icon: Presentation },
  { title: 'Backdrop Photo Booth', desc: 'Latar foto untuk acara & studio.', icon: ImageIcon },
  { title: 'Desain Custom', desc: 'Kebutuhan khusus, konsultasikan ke tim kami.', icon: Sparkle },
]
</script>

<template>
  <main>
    <!-- Header -->
    <section class="mx-auto max-w-6xl px-4 pt-16 pb-10 md:pt-24 md:pb-14">
      <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Showcase</p>
      <h1 class="mt-3 text-2xl md:text-3xl font-serif font-semibold tracking-tight text-ink-950">
        Galeri & preview banner
      </h1>
      <p class="mt-3 max-w-2xl text-sm md:text-base leading-relaxed text-ink-500">
        Lihat kategori banner yang biasa kami kerjakan, lalu coba preview 3D untuk gambaran
        tampilan banner sebelum Anda order.
      </p>
    </section>

    <!-- Gallery: swipeable di mobile (scroll-snap), grid di desktop -->
    <section class="mx-auto max-w-6xl px-4 pb-16 md:pb-24">
      <div
        class="flex snap-x snap-mandatory gap-4 overflow-x-auto pb-2 md:grid md:grid-cols-3 md:gap-6 md:overflow-visible md:pb-0"
      >
        <div
          v-for="item in galleryItems"
          :key="item.title"
          class="relative aspect-[4/3] w-[78%] shrink-0 snap-center overflow-hidden rounded-lg border border-hairline bg-canvas-alt md:w-auto md:shrink"
        >
          <div class="absolute inset-0 flex items-center justify-center">
            <component :is="item.icon" class="h-10 w-10 text-brand-500/50" :stroke-width="1.25" />
          </div>
          <div class="absolute inset-x-0 bottom-0 bg-gradient-to-t from-ink-950/80 via-ink-950/20 to-transparent p-4">
            <p class="text-sm font-semibold text-canvas">{{ item.title }}</p>
            <p class="mt-0.5 text-xs leading-relaxed text-canvas/75">{{ item.desc }}</p>
          </div>
        </div>
      </div>
      <p class="mt-3 text-xs text-ink-400 md:hidden">Geser untuk lihat kategori lainnya.</p>
    </section>

    <!-- 3D preview -->
    <section class="border-y border-hairline bg-canvas-alt">
      <div class="mx-auto max-w-4xl px-4 py-16 md:py-24">
        <div class="max-w-2xl">
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Preview 3D</p>
          <h2 class="mt-3 text-lg md:text-xl font-sans font-semibold text-ink-950">
            Coba putar contoh banner
          </h2>
          <p class="mt-2 text-sm leading-relaxed text-ink-500">
            Drag / geser untuk memutar. Ini contoh mockup — desain final mengikuti file yang Anda
            kirim atau brief yang dikerjakan tim desain kami.
          </p>
        </div>

        <div class="mt-8 mx-auto max-w-lg">
          <ShowcaseBannerViewer />
        </div>
      </div>
    </section>

    <!-- CTA -->
    <section class="mx-auto max-w-6xl px-4 py-16 md:py-24 text-center">
      <h2 class="text-xl md:text-2xl font-serif font-semibold tracking-tight text-ink-950">
        Sudah dapat gambaran? Ayo order.
      </h2>
      <div class="mt-6 flex flex-col sm:flex-row items-center justify-center gap-3">
        <NuxtLink
          to="/order"
          class="inline-flex w-full sm:w-auto items-center justify-center gap-2 rounded-md bg-brand-500 px-6 py-3 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
        >
          Order Banner
          <ArrowRight class="h-4 w-4" :stroke-width="1.5" />
        </NuxtLink>
        <NuxtLink
          to="/katalog"
          class="inline-flex w-full sm:w-auto items-center justify-center gap-2 rounded-md border border-hairline bg-canvas px-6 py-3 text-sm font-semibold text-ink-900 transition-colors hover:bg-canvas-alt hover:border-ink-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
        >
          Lihat Katalog
        </NuxtLink>
      </div>
    </section>
  </main>
</template>
