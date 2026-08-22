<script setup lang="ts">
/**
 * ProcessGallery — galeri proses cetak (deliverable §1 brief "kurang gambar").
 *
 * 8 foto: 6 lama (proses-01..06, 1000x562) + 2 baru (proses-07 detail panel
 * kontrol & tabung tinta CMYK 800x800, proses-08 detail carriage/print-head
 * di atas banner yang sedang tercetak 1000x750).
 *
 * Layout bento asimetris di desktop lewat `grid-template-areas` scoped CSS
 * (lebih terbaca & pasti benar dibanding merangkai banyak nilai arbitrary
 * Tailwind untuk grid-area). Baris ketiga sekarang 4 sel tunggal (e,f,g,h)
 * untuk menampung 2 foto baru tanpa menambah tinggi total galeri. Di
 * mobile/tablet turun jadi grid rapi 1-2 kolom (Tailwind, di atas CSS bento),
 * `object-cover` menjaga aspect ratio seragam meski sumber foto beda rasio.
 *
 * Reveal stagger via `useRevealVariants()` — `once: true`, cuma opacity/translateY.
 * Hover: HANYA image di-scale sedikit di dalam container `overflow-hidden`
 * (bukan kartunya) — brief §1 eksplisit larang scale kartu yang menggeser layout.
 * Klik kartu membuka `ProcessLightbox`.
 */
import { ZoomIn } from '@lucide/vue'
import { motion } from 'motion-v'

interface ProcessItem {
  area: 'a' | 'b' | 'c' | 'd' | 'e' | 'f' | 'g' | 'h'
  /** Slot sitemedia (§ modul sitemedia) — dipetakan ke path statis di useSiteMedia.ts. */
  slot: string
  alt: string
  caption: string
  label: string
  /** Dimensi intrinsik foto asli (bukan semua 1000x562 — proses-07/08 beda rasio). */
  width: number
  height: number
}

const rawItems: ProcessItem[] = [
  {
    area: 'a',
    slot: 'proses_4',
    alt: 'Mesin cetak large-format Rajaku Printing tampak penuh, siap memproses pesanan',
    caption: 'Mesin cetak large-format Rajaku Printing, siap memproses pesanan banner.',
    label: 'Workshop',
    width: 1000,
    height: 562,
  },
  {
    area: 'b',
    slot: 'proses_1',
    alt: 'Panel kontrol mesin cetak dan tabung tinta',
    caption: 'Panel kontrol & tabung tinta dicek sebelum proses cetak dimulai.',
    label: 'Kalibrasi',
    width: 1000,
    height: 562,
  },
  {
    area: 'c',
    slot: 'proses_3',
    alt: 'Detail print-head bergerak di atas bahan banner',
    caption: 'Print-head bergerak presisi mengaplikasikan tinta ke bahan banner.',
    label: 'Print-head',
    width: 1000,
    height: 562,
  },
  {
    area: 'd',
    slot: 'proses_2',
    alt: 'Banner mulai tercetak keluar dari mesin',
    caption: 'Banner mulai tercetak, keluar dari mesin lembar demi lembar.',
    label: 'Mencetak',
    width: 1000,
    height: 562,
  },
  {
    area: 'e',
    slot: 'proses_5',
    alt: 'Banner hasil cetak digulung rapi di atas roll',
    caption: 'Banner hasil cetak digulung rapi setelah proses selesai.',
    label: 'Selesai cetak',
    width: 1000,
    height: 562,
  },
  {
    area: 'f',
    slot: 'proses_6',
    alt: 'Hasil akhir cetak banner tampak lebar',
    caption: 'Hasil akhir cetak — warna presisi, siap masuk tahap finishing.',
    label: 'Hasil akhir',
    width: 1000,
    height: 562,
  },
  {
    area: 'g',
    slot: 'proses_7',
    alt: 'Detail panel kontrol mesin dan tabung tinta CMYK',
    caption: 'Tabung tinta CMYK dan panel kontrol — dicek rutin untuk warna yang konsisten.',
    label: 'Tinta CMYK',
    width: 800,
    height: 800,
  },
  {
    area: 'h',
    slot: 'proses_8',
    alt: 'Carriage print-head melintas di atas banner yang sedang tercetak',
    caption: 'Carriage print-head melintas di atas banner yang sedang tercetak, lapis demi lapis.',
    label: 'Carriage',
    width: 1000,
    height: 750,
  },
]

// Bisa diganti admin (/admin/site-media, slot proses_1..8) tanpa deploy ulang.
// `await ready` — komponen ini SSR normal (tidak di dalam <ClientOnly>), jadi
// HTML awal wajib sudah dapat gambar final untuk SEO/first paint.
const { resolve: resolveMedia, ready: siteMediaReady } = useSiteMedia()
await siteMediaReady
const items = computed(() =>
  rawItems.map((p) => ({ ...p, src: resolveMedia(p.slot) })),
)

const { container, item } = useRevealVariants({ y: 20 })

const lightboxOpen = ref(false)
const lightboxIndex = ref(0)

function openAt(i: number, e: MouseEvent) {
  // Safari tidak selalu memberi focus otomatis ke <button> saat diklik — focus
  // eksplisit di sini supaya ProcessLightbox bisa diandalkan mengembalikan focus
  // ke tombol ini saat ditutup (aksesibilitas, bukan sekadar visual).
  ;(e.currentTarget as HTMLElement)?.focus()
  lightboxIndex.value = i
  lightboxOpen.value = true
}
</script>

<template>
  <!--
    Section ini sengaja GELAP. Dua alasan, bukan sekadar selera:
    1. Ritme halaman — dari hero gelap ke bawah semuanya terang, sehingga
       halaman terbaca sebagai satu blok putih panjang. Pita gelap di tengah
       memberi jeda dan membuat section berikutnya terasa "dibuka" lagi.
    2. Foto: latar gelap membuat foto workshop menonjol, sedangkan di atas
       putih foto-foto ini melebur dengan halaman.
    Konsekuensinya SELURUH teks & garis di dalam sini harus ikut dibalik ke
    varian on-dark (canvas/gold), bukan cuma latarnya yang diganti.
  -->
  <section id="proses-cetak" class="process-section relative overflow-hidden bg-ink-950">
    <div class="pointer-events-none absolute inset-0" aria-hidden="true">
      <ArtOrnament variant="arc" :opacity="0.35" />
    </div>

    <div class="relative mx-auto max-w-6xl px-4 py-12 md:py-20">
    <div class="max-w-2xl">
      <p class="flex items-center gap-2 text-[10px] font-medium uppercase tracking-[0.14em] text-canvas/60">
        <span class="h-px w-6 bg-gold-500" aria-hidden="true" />
        Proses Cetak
      </p>
      <h2 class="mt-3 text-2xl md:text-3xl font-serif font-semibold tracking-tight text-canvas">
        Dari file desain sampai banner jadi
      </h2>
      <p class="mt-3 text-sm leading-relaxed text-canvas/70">
        Sekilas proses cetak large-format di workshop kami — klik gambar untuk melihat lebih
        besar.
      </p>
    </div>

    <motion.div
      class="process-bento mt-10 flex snap-x snap-mandatory gap-4 overflow-x-auto pb-2 md:grid md:grid-cols-2 md:gap-4 md:overflow-visible md:pb-0"
      :variants="container"
      initial="hidden"
      while-in-view="show"
      :in-view-options="{ once: true, margin: '-40px' }"
    >
      <motion.button
        v-for="(p, i) in items"
        :key="p.area"
        type="button"
        :variants="item"
        :class="['process-area-' + p.area, 'w-[88%] shrink-0 snap-center sm:w-[70%] md:w-auto md:shrink']"
        class="group relative aspect-[4/3] overflow-hidden rounded-lg border border-white/10 bg-white/5 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold-400/60 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950 md:aspect-[16/10] lg:aspect-auto"
        @click="openAt(i, $event)"
      >
        <!--
          `scale-105` sebagai posisi diam, bukan hanya saat hover: foto dibuat
          sedikit lebih besar dari bingkainya supaya punya ruang bergeser
          (kelas `.process-drift` di bawah) tanpa memperlihatkan tepi kosong.
        -->
        <img
          :src="p.src"
          :alt="p.alt"
          :width="p.width"
          :height="p.height"
          loading="lazy"
          class="process-drift h-full w-full scale-105 object-cover transition-transform duration-500 ease-out group-hover:scale-[1.12]"
        >
        <div
          class="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-ink-950/85 via-ink-950/20 to-transparent p-4 pt-10"
        >
          <p class="text-xs font-medium uppercase tracking-[0.1em] text-gold-400">
            {{ String(i + 1).padStart(2, '0') }} &middot; {{ p.label }}
          </p>
          <p class="mt-1 text-sm leading-snug text-canvas line-clamp-2">{{ p.caption }}</p>
        </div>
        <span
          class="pointer-events-none absolute right-3 top-3 inline-flex h-8 w-8 items-center justify-center rounded-md bg-ink-950/60 text-canvas opacity-0 backdrop-blur transition-opacity duration-200 ease-out group-hover:opacity-100"
          aria-hidden="true"
        >
          <ZoomIn class="h-4 w-4" :stroke-width="1.5" />
        </span>
      </motion.button>
    </motion.div>
    <p class="mt-3 text-xs text-canvas/50 md:hidden">Geser ke samping untuk melihat foto lainnya.</p>
    </div>

    <LandingProcessLightbox
      :open="lightboxOpen"
      :items="items"
      :index="lightboxIndex"
      @update:open="lightboxOpen = $event"
      @update:index="lightboxIndex = $event"
    />
  </section>
</template>

<style scoped>
/*
 * Hanyutan halus foto di dalam bingkainya, dijalankan oleh scroll halaman.
 * Dipakai `animation-timeline: view()` — animasi digerakkan posisi elemen di
 * layar, bukan oleh listener scroll di JavaScript, jadi tidak ada satu pun
 * frame yang dihitung di main thread.
 *
 * Browser yang belum mendukung `animation-timeline` (Safari saat ini)
 * mengabaikan seluruh blok ini: fotonya diam, dan tidak ada yang rusak.
 * Ditutup juga untuk prefers-reduced-motion.
 */
@supports (animation-timeline: view()) {
  @media (prefers-reduced-motion: no-preference) {
    /*
      Timeline dinamai di SECTION, lalu dirujuk semua fotonya.
      Dua wadah scroll menghalangi kalau namanya ditaruh lebih dalam:
      bingkai kartu `overflow-hidden`, dan di layar HP daftar kartunya sendiri
      `overflow-x-auto`. Keduanya dihitung sebagai wadah scroll tersendiri,
      sehingga `view()` di dalamnya mengukur terhadap wadah yang tidak pernah
      tergulir vertikal — progresnya membeku di satu angka.

      Efek sampingnya justru diinginkan: semua foto berbagi satu progres, jadi
      hanyutannya seirama saat section ini lewat, bukan tiap foto bergerak
      sendiri-sendiri (yang akan terbaca ramai).
    */
    .process-section {
      view-timeline-name: --process-view;
    }

    .process-drift {
      animation: process-drift linear both;
      animation-timeline: --process-view;
      animation-range: entry 0% exit 100%;
    }
  }
}

@keyframes process-drift {
  from {
    transform: scale(1.05) translateY(-2.5%);
  }
  to {
    transform: scale(1.05) translateY(2.5%);
  }
}

/* Bento asimetris — cuma diaktifkan di desktop (lg). Di bawah itu grid biasa
   1-2 kolom (Tailwind, di atas) yang menang. Ditulis di sini (bukan arbitrary
   Tailwind class) supaya `grid-template-areas` tetap terbaca sebagai satu blok
   dan tidak riskan kena bug "kelas ada di DOM tapi CSS kosong". Baris ketiga
   sekarang 4 sel tunggal (e,f,g,h) untuk menampung 2 foto baru (07, 08). */
@media (min-width: 1024px) {
  .process-bento {
    grid-template-columns: repeat(4, 1fr);
    /* 260px, bukan 200px — dan tidak ada lagi baris berisi empat sel sempit.
       Susunan lama menaruh empat foto berdampingan di baris terakhir, masing-
       masing cuma ~276x200: detail print-head & tabung tinta jadi tidak
       terbaca, yang justru bagian paling menarik dari foto-foto ini. */
    grid-template-rows: repeat(4, 260px);
    grid-template-areas:
      'a a b c'
      'a a d d'
      'e e f f'
      'g g h h';
  }
  .process-area-a {
    grid-area: a;
  }
  .process-area-b {
    grid-area: b;
  }
  .process-area-c {
    grid-area: c;
  }
  .process-area-d {
    grid-area: d;
  }
  .process-area-e {
    grid-area: e;
  }
  .process-area-f {
    grid-area: f;
  }
  .process-area-g {
    grid-area: g;
  }
  .process-area-h {
    grid-area: h;
  }
}
</style>
