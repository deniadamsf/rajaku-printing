<script setup lang="ts">
/**
 * /showcase — Layar 2 (§18): galeri produk + preview 3D banner.
 *
 * Ini tujuan CTA sekunder dari hero mobile (`HeroStatic`) — dipisah jadi
 * halaman sendiri supaya TresJS (§16 poin 2) TIDAK pernah ikut termuat di
 * homepage. Viewer di-lazy-load lewat `ShowcaseBannerViewer` (ClientOnly +
 * IntersectionObserver di dalam komponen itu sendiri).
 *
 * Rombak dari versi chip-kategori-polos: sekarang halaman punya dua sumber
 * "isi" utama —
 *  1. Grid 8 kartu jenis produk (`<ArtProduct>`) dengan deskripsi jujur +
 *     tautan order. Ini artwork ILUSTRASI (bukan foto portfolio), lihat
 *     catatan di `utils/artwork.ts` — jangan pernah diberi caption seolah
 *     "hasil cetak pelanggan kami".
 *  2. Bento galeri 8 foto proses cetak asli + lightbox.
 *
 * Momen orkestrasi utama halaman ini (§ prinsip motion "satu momen per
 * halaman"): stagger reveal grid produk saat masuk viewport (`useRevealVariants`).
 * Bento galeri & elemen lain hanya fade/slide tunggal tanpa stagger, supaya
 * tidak menumpuk dua stagger sekaligus di satu halaman.
 */
import { ArrowRight } from '@lucide/vue'
import { motion } from 'motion-v'
import type { ArtworkVariant } from '~/utils/artwork'

definePageMeta({ layout: 'default' })

const config = useRuntimeConfig()
const canonical = `${config.public.appBaseUrl.replace(/\/$/, '')}/showcase`

useSeoMeta({
  title: 'Showcase & Preview Desain Banner',
  description:
    'Jenis produk cetak Rajaku Printing — spanduk, X-banner, roll-up, baliho, backdrop, stiker, backlite — lengkap galeri proses cetak dan preview 3D interaktif.',
  ogTitle: 'Showcase — Rajaku Printing',
  ogDescription: 'Jenis produk cetak, galeri proses, dan preview 3D interaktif sebelum Anda order.',
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

const { container: gridContainer, item: gridItem } = useRevealVariants({ stagger: 0.06 })

interface ProductTile {
  variant: ArtworkVariant
  title: string
  desc: string
}

// 8 jenis produk — deskripsi peruntukan jujur (bukan klaim kualitas/statistik
// yang tidak ada datanya, sesuai batasan brief). Artwork murni ilustrasi
// jenis produk lewat `<ArtProduct>`, bukan foto hasil kerja pelanggan.
const productTiles: ProductTile[] = [
  {
    variant: 'spanduk',
    title: 'Spanduk',
    desc: 'Banner horizontal dengan mata ayam, cocok dipasang membentang di depan toko atau lokasi acara.',
  },
  {
    variant: 'x-banner',
    title: 'X-Banner',
    desc: 'Banner berdiri dengan rangka X, praktis dipindah — biasa dipakai untuk pameran atau depan meja resepsionis.',
  },
  {
    variant: 'roll-up',
    title: 'Roll-Up Banner',
    desc: 'Banner dengan kaset penggulung, ringkas dibawa bepergian dan cepat dipasang untuk booth atau seminar.',
  },
  {
    variant: 'baliho',
    title: 'Baliho',
    desc: 'Ukuran besar di atas tiang, untuk promosi yang perlu terbaca dari jarak jauh di pinggir jalan.',
  },
  {
    variant: 'backdrop',
    title: 'Backdrop',
    desc: 'Latar panggung atau area foto, dipasang dengan rangka untuk acara, seminar, atau photo booth.',
  },
  {
    variant: 'stiker',
    title: 'Stiker',
    desc: 'Cutting/print vinyl untuk label produk, kaca toko, atau kendaraan — dipotong sesuai bentuk desain.',
  },
  {
    variant: 'backlite',
    title: 'Backlite',
    desc: 'Bahan tembus cahaya untuk panel lightbox atau neon box, menyala terang saat disinari dari belakang.',
  },
  {
    variant: 'umum',
    title: 'Cetak Custom Lainnya',
    desc: 'Punya kebutuhan cetak large-format di luar daftar ini? Konsultasikan ukuran dan bahannya lewat form order.',
  },
]

interface ProcessPhoto {
  src: string
  alt: string
  title: string
  desc: string
  width: number
  height: number
  /** Kelas span grid bento, hanya berlaku di md+ (mobile pakai carousel flex). */
  bento: string
}

// 8 foto proses cetak asli — desaturasi ~85% (§26.8) — dari mesin, print-head,
// panel kontrol, sampai hasil akhir. Urutan dipilih untuk alur cerita: gambaran
// besar mesin dulu, lalu detail proses, ditutup hasil akhir.
const processPhotos: ProcessPhoto[] = [
  {
    src: '/proses/proses-04.webp',
    alt: 'Mesin cetak large-format Rajaku Printing tampak penuh',
    title: 'Mesin cetak large-format',
    desc: 'Kalibrasi rutin untuk hasil warna yang konsisten.',
    width: 1000,
    height: 562,
    bento: 'md:col-span-2 md:row-span-2',
  },
  {
    src: '/proses/proses-01.webp',
    alt: 'Panel kontrol mesin cetak dan tabung tinta',
    title: 'Panel kontrol & tinta',
    desc: 'Presisi warna dipantau dari panel kontrol mesin.',
    width: 1000,
    height: 562,
    bento: 'md:col-span-1 md:row-span-1',
  },
  {
    src: '/proses/proses-07.webp',
    alt: 'Detail panel kontrol dan tabung tinta CMYK mesin cetak',
    title: 'Tabung tinta CMYK',
    desc: 'Empat warna dasar dicampur presisi untuk hasil akurat.',
    width: 800,
    height: 800,
    bento: 'md:col-span-1 md:row-span-1',
  },
  {
    src: '/proses/proses-03.webp',
    alt: 'Print-head mesin bekerja di atas material banner',
    title: 'Print-head detail',
    desc: 'Resolusi cetak tinggi untuk hasil yang tajam.',
    width: 1000,
    height: 562,
    bento: 'md:col-span-1 md:row-span-1',
  },
  {
    src: '/proses/proses-08.webp',
    alt: 'Carriage dan print-head bergerak di atas banner yang sedang tercetak',
    title: 'Carriage saat mencetak',
    desc: 'Carriage bergerak bolak-balik menyapukan tinta ke bahan.',
    width: 1000,
    height: 750,
    bento: 'md:col-span-1 md:row-span-1',
  },
  {
    src: '/proses/proses-02.webp',
    alt: 'Mesin sedang mencetak banner yang keluar dari printer',
    title: 'Proses cetak berjalan',
    desc: 'Banner dicetak langsung dari file desain Anda.',
    width: 1000,
    height: 562,
    bento: 'md:col-span-2 md:row-span-1',
  },
  {
    src: '/proses/proses-05.webp',
    alt: 'Gulungan bahan banner siap cetak',
    title: 'Bahan & roll banner',
    desc: 'Bahan dipilih sesuai kebutuhan indoor atau outdoor.',
    width: 1000,
    height: 562,
    bento: 'md:col-span-2 md:row-span-1',
  },
  {
    src: '/proses/proses-06.webp',
    alt: 'Hasil cetak banner yang sudah selesai',
    title: 'Hasil cetak akhir',
    desc: 'Siap diambil di tempat atau dikirim ke lokasi Anda.',
    width: 1000,
    height: 562,
    bento: 'md:col-span-2 md:row-span-1',
  },
]

const lightboxIndex = ref<number | null>(null)
</script>

<template>
  <main>
    <!-- Header -->
    <section class="relative overflow-hidden">
      <div class="pointer-events-none absolute inset-0">
        <ArtOrnament variant="grid" :opacity="0.35" />
      </div>
      <div class="relative mx-auto max-w-6xl px-4 pt-16 pb-10 md:pt-24 md:pb-14">
        <motion.div
          :initial="{ opacity: 0, y: 12 }"
          :while-in-view="{ opacity: 1, y: 0 }"
          :in-view-options="{ once: true, margin: '-80px' }"
          :transition="fadeTransition"
        >
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Showcase</p>
          <h1 class="mt-3 text-2xl md:text-3xl font-serif font-semibold tracking-tight text-ink-950">
            Jenis produk, proses cetak & preview 3D
          </h1>
          <p class="mt-3 max-w-2xl text-sm md:text-base leading-relaxed text-ink-500">
            Pilih jenis produk yang sesuai kebutuhan Anda, lihat proses cetak kami dari dekat, lalu
            coba preview 3D untuk gambaran tampilan banner sebelum order.
          </p>
        </motion.div>
      </div>
    </section>

    <!-- Grid jenis produk — ini sumber "isi" utama halaman -->
    <section class="mx-auto max-w-6xl px-4 pb-16 md:pb-24">
      <motion.ul
        class="grid gap-6 sm:grid-cols-2 lg:grid-cols-4"
        :variants="gridContainer"
        initial="hidden"
        while-in-view="show"
        :in-view-options="{ once: true, margin: '-100px' }"
      >
        <motion.li v-for="tile in productTiles" :key="tile.variant" :variants="gridItem">
          <NuxtLink
            to="/order"
            class="group flex h-full flex-col overflow-hidden rounded-lg border border-hairline bg-canvas transition-colors hover:border-ink-300 hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
          >
            <div class="aspect-[4/3] w-full bg-canvas-alt">
              <ArtProduct :variant="tile.variant" decorative />
            </div>
            <div class="flex flex-1 flex-col p-5">
              <h2 class="text-sm font-sans font-semibold text-ink-950">{{ tile.title }}</h2>
              <p class="mt-1.5 flex-1 text-xs leading-relaxed text-ink-500">{{ tile.desc }}</p>
              <span class="mt-3 inline-flex items-center gap-1 text-xs font-semibold text-brand-500">
                Order jenis ini
                <ArrowRight class="h-3.5 w-3.5 transition-transform duration-200 ease-out group-hover:translate-x-0.5" :stroke-width="1.75" />
              </span>
            </div>
          </NuxtLink>
        </motion.li>
      </motion.ul>
    </section>

    <!-- Gallery: bento asimetris di desktop, carousel scroll-snap di mobile -->
    <section class="border-t border-hairline bg-canvas-alt">
      <div class="mx-auto max-w-6xl px-4 py-16 md:py-24">
        <motion.div
          :initial="{ opacity: 0, y: 12 }"
          :while-in-view="{ opacity: 1, y: 0 }"
          :in-view-options="{ once: true, margin: '-80px' }"
          :transition="fadeTransition"
        >
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Di balik layar</p>
          <h2 class="mt-3 text-lg md:text-xl font-sans font-semibold text-ink-950">
            Proses cetak dari dekat
          </h2>
        </motion.div>

        <div
          class="mt-8 flex snap-x snap-mandatory gap-4 overflow-x-auto pb-2 md:grid md:grid-cols-4 md:auto-rows-[170px] md:gap-4 md:overflow-visible md:pb-0 lg:auto-rows-[210px]"
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
                : { duration: 0.5, ease: [0.22, 1, 0.36, 1], delay: idx * 0.05 }
            "
          >
            <button
              type="button"
              class="group relative block aspect-[4/3] w-full overflow-hidden rounded-lg border border-hairline bg-canvas focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas md:aspect-auto md:h-full"
              :aria-label="`Lihat besar: ${photo.title}`"
              @click="lightboxIndex = idx"
            >
              <img
                :src="photo.src"
                :alt="photo.alt"
                :width="photo.width"
                :height="photo.height"
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
      </div>
    </section>

    <ShowcaseGalleryLightbox
      :items="processPhotos.map((p) => ({ src: p.src, alt: p.alt, title: p.title, desc: p.desc }))"
      :index="lightboxIndex"
      @update:index="lightboxIndex = $event"
    />

    <!-- 3D preview -->
    <section>
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
    <section class="border-t border-hairline bg-canvas-alt">
      <div class="mx-auto max-w-6xl px-4 py-16 md:py-24 text-center">
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
      </div>
    </section>
  </main>
</template>
