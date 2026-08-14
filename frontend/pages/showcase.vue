<script setup lang="ts">
/**
 * /showcase — Layar 2 (§18): galeri produk + preview 3D banner.
 *
 * Ini tujuan CTA sekunder dari hero mobile (`HeroStatic`) — dipisah jadi
 * halaman sendiri supaya TresJS (§16 poin 2) TIDAK pernah ikut termuat di
 * homepage. Viewer di-lazy-load lewat `ShowcaseBannerViewer` (ClientOnly +
 * IntersectionObserver di dalam komponen itu sendiri).
 *
 * Galeri sekarang memakai 6 foto proses cetak asli (bukan lagi placeholder
 * ikon monoline) — layout bento asimetris di desktop, carousel scroll-snap
 * di mobile (satu array, dua kelas responsif, sama seperti pola sebelumnya).
 * Klik foto membuka `ShowcaseGalleryLightbox`.
 *
 * Momen orkestrasi utama halaman ini (§ prinsip motion "satu momen per
 * halaman"): stagger reveal bento gallery saat masuk viewport. Elemen lain
 * (header, judul section, CTA) hanya fade/slide tunggal tanpa stagger.
 */
import { ArrowRight, FlagTriangleRight, Gift, Presentation, Store, Sparkle, Image as ImageIcon } from '@lucide/vue'
import { motion } from 'motion-v'

definePageMeta({ layout: 'default' })

const config = useRuntimeConfig()
const canonical = `${config.public.appBaseUrl.replace(/\/$/, '')}/showcase`

useSeoMeta({
  title: 'Showcase & Preview Desain Banner',
  description:
    'Galeri proses cetak Rajaku Printing dan preview 3D interaktif — putar banner untuk lihat tampilannya sebelum order.',
  ogTitle: 'Showcase — Rajaku Printing',
  ogDescription: 'Galeri proses cetak & preview 3D interaktif sebelum Anda order.',
  ogType: 'website',
  ogUrl: canonical,
  twitterCard: 'summary',
})

useHead({
  link: [{ rel: 'canonical', href: canonical }],
})

const prefersReducedMotion = usePrefersReducedMotion()
const fadeTransition = computed(() =>
  prefersReducedMotion.value ? { duration: 0 } : { duration: 0.5, ease: [0.22, 1, 0.36, 1] as const },
)

interface CategoryChip {
  label: string
  icon: typeof Store
}

// Kategori generik (belum ada foto per-kategori) — chip kecil, bukan lagi
// kartu besar dengan placeholder ikon.
const categories: CategoryChip[] = [
  { label: 'Banner Promosi Usaha', icon: Store },
  { label: 'Banner Event & Acara', icon: Gift },
  { label: 'Spanduk Outdoor', icon: FlagTriangleRight },
  { label: 'X-Banner Pameran', icon: Presentation },
  { label: 'Backdrop Photo Booth', icon: ImageIcon },
  { label: 'Desain Custom', icon: Sparkle },
]

interface ProcessPhoto {
  src: string
  alt: string
  title: string
  desc: string
  /** Kelas span grid bento, hanya berlaku di md+ (mobile pakai carousel flex). */
  bento: string
}

// 6 foto proses cetak asli, 1000x562, desaturasi ~85% (§26.8) — dari mesin,
// print-head, sampai hasil akhir. Urutan dipilih untuk alur cerita: gambaran
// besar mesin dulu, lalu detail proses, ditutup hasil akhir.
const processPhotos: ProcessPhoto[] = [
  {
    src: '/proses/proses-04.webp',
    alt: 'Mesin cetak large-format Rajaku Printing tampak penuh',
    title: 'Mesin cetak large-format',
    desc: 'Kalibrasi rutin untuk hasil warna yang konsisten.',
    bento: 'md:col-span-2 md:row-span-2',
  },
  {
    src: '/proses/proses-01.webp',
    alt: 'Panel kontrol mesin cetak dan tabung tinta',
    title: 'Panel kontrol & tinta',
    desc: 'Presisi warna dipantau dari panel kontrol mesin.',
    bento: 'md:col-span-1 md:row-span-1',
  },
  {
    src: '/proses/proses-03.webp',
    alt: 'Print-head mesin bekerja di atas material banner',
    title: 'Print-head detail',
    desc: 'Resolusi cetak tinggi untuk hasil yang tajam.',
    bento: 'md:col-span-1 md:row-span-1',
  },
  {
    src: '/proses/proses-02.webp',
    alt: 'Mesin sedang mencetak banner yang keluar dari printer',
    title: 'Proses cetak berjalan',
    desc: 'Banner dicetak langsung dari file desain Anda.',
    bento: 'md:col-span-2 md:row-span-1',
  },
  {
    src: '/proses/proses-05.webp',
    alt: 'Gulungan bahan banner siap cetak',
    title: 'Bahan & roll banner',
    desc: 'Bahan dipilih sesuai kebutuhan indoor atau outdoor.',
    bento: 'md:col-span-2 md:row-span-1',
  },
  {
    src: '/proses/proses-06.webp',
    alt: 'Hasil cetak banner yang sudah selesai',
    title: 'Hasil cetak akhir',
    desc: 'Siap diambil di tempat atau dikirim ke lokasi Anda.',
    bento: 'md:col-span-2 md:row-span-1',
  },
]

const lightboxIndex = ref<number | null>(null)
</script>

<template>
  <main>
    <!-- Header -->
    <section class="mx-auto max-w-6xl px-4 pt-16 pb-10 md:pt-24 md:pb-14">
      <motion.div
        :initial="{ opacity: 0, y: 12 }"
        :while-in-view="{ opacity: 1, y: 0 }"
        :in-view-options="{ once: true, margin: '-80px' }"
        :transition="fadeTransition"
      >
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Showcase</p>
        <h1 class="mt-3 text-2xl md:text-3xl font-serif font-semibold tracking-tight text-ink-950">
          Galeri & preview banner
        </h1>
        <p class="mt-3 max-w-2xl text-sm md:text-base leading-relaxed text-ink-500">
          Lihat proses cetak kami dari dekat, lalu coba preview 3D untuk gambaran tampilan
          banner sebelum Anda order.
        </p>
      </motion.div>

      <!-- Kategori: chip kecil, bukan kartu placeholder -->
      <div class="mt-6 flex flex-wrap gap-2">
        <span
          v-for="cat in categories"
          :key="cat.label"
          class="inline-flex items-center gap-1.5 rounded-full border border-hairline bg-canvas px-3 py-1.5 text-xs font-medium text-ink-600"
        >
          <component :is="cat.icon" class="h-3.5 w-3.5 text-brand-500" :stroke-width="1.5" />
          {{ cat.label }}
        </span>
      </div>
    </section>

    <!-- Gallery: bento asimetris di desktop, carousel scroll-snap di mobile -->
    <section class="mx-auto max-w-6xl px-4 pb-16 md:pb-24">
      <div
        class="flex snap-x snap-mandatory gap-4 overflow-x-auto pb-2 md:grid md:grid-cols-4 md:auto-rows-[170px] md:gap-4 md:overflow-visible md:pb-0 lg:auto-rows-[210px]"
      >
        <motion.div
          v-for="(photo, idx) in processPhotos"
          :key="photo.src"
          class="w-[78%] shrink-0 snap-center md:w-auto md:shrink"
          :class="photo.bento"
          :initial="{ opacity: 0, y: 16 }"
          :while-in-view="{ opacity: 1, y: 0 }"
          :in-view-options="{ once: true, margin: '-100px' }"
          :transition="
            prefersReducedMotion
              ? { duration: 0 }
              : { duration: 0.5, ease: [0.22, 1, 0.36, 1], delay: idx * 0.06 }
          "
        >
          <button
            type="button"
            class="group relative block aspect-[4/3] w-full overflow-hidden rounded-lg border border-hairline bg-canvas-alt focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas md:aspect-auto md:h-full"
            :aria-label="`Lihat besar: ${photo.title}`"
            @click="lightboxIndex = idx"
          >
            <img
              :src="photo.src"
              :alt="photo.alt"
              width="1000"
              height="562"
              loading="lazy"
              class="h-full w-full object-cover transition-transform duration-300 ease-out group-hover:scale-[1.03]"
            >
            <div
              class="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-ink-950/80 via-ink-950/20 to-transparent p-4"
            >
              <p class="text-sm font-semibold text-canvas">{{ photo.title }}</p>
              <p class="mt-0.5 text-xs leading-relaxed text-canvas/75">{{ photo.desc }}</p>
            </div>
          </button>
        </motion.div>
      </div>
      <p class="mt-3 text-xs text-ink-400 md:hidden">Geser untuk lihat foto lainnya.</p>
    </section>

    <ShowcaseGalleryLightbox
      :items="processPhotos.map((p) => ({ src: p.src, alt: p.alt, title: p.title, desc: p.desc }))"
      :index="lightboxIndex"
      @update:index="lightboxIndex = $event"
    />

    <!-- 3D preview -->
    <section class="border-y border-hairline bg-canvas-alt">
      <div class="mx-auto max-w-4xl px-4 py-16 md:py-24">
        <motion.div
          class="max-w-2xl"
          :initial="{ opacity: 0, y: 12 }"
          :while-in-view="{ opacity: 1, y: 0 }"
          :in-view-options="{ once: true, margin: '-80px' }"
          :transition="fadeTransition"
        >
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Preview 3D</p>
          <h2 class="mt-3 text-lg md:text-xl font-sans font-semibold text-ink-950">
            Coba putar contoh banner
          </h2>
          <p class="mt-2 text-sm leading-relaxed text-ink-500">
            Pilih rasio dan contoh desain, lalu drag / geser untuk memutar. Ini contoh mockup —
            desain final mengikuti file yang Anda kirim atau brief yang dikerjakan tim desain
            kami.
          </p>
        </motion.div>

        <div class="mt-8 mx-auto max-w-lg">
          <ShowcaseBannerViewer />
        </div>
      </div>
    </section>

    <!-- CTA -->
    <section class="mx-auto max-w-6xl px-4 py-16 md:py-24 text-center">
      <motion.div
        :initial="{ opacity: 0, y: 12 }"
        :while-in-view="{ opacity: 1, y: 0 }"
        :in-view-options="{ once: true, margin: '-80px' }"
        :transition="fadeTransition"
      >
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
      </motion.div>
    </section>
  </main>
</template>
