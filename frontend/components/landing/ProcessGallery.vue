<script setup lang="ts">
/**
 * ProcessGallery — galeri proses cetak (deliverable §1 brief "kurang gambar").
 *
 * Layout bento asimetris (2x2 / 2x1 / 1x1) di desktop lewat `grid-template-areas`
 * scoped CSS (lebih terbaca & pasti benar dibanding merangkai banyak nilai
 * arbitrary Tailwind untuk grid-area). Di mobile/tablet turun jadi grid rapi
 * 1-2 kolom, aspect ratio seragam (semua foto sumber 1000x562).
 *
 * Reveal stagger via `useRevealVariants()` — `once: true`, cuma opacity/translateY.
 * Hover: HANYA image di-scale sedikit di dalam container `overflow-hidden`
 * (bukan kartunya) — brief §1 eksplisit larang scale kartu yang menggeser layout.
 * Klik kartu membuka `ProcessLightbox`.
 */
import { ZoomIn } from '@lucide/vue'
import { motion } from 'motion-v'

interface ProcessItem {
  area: 'a' | 'b' | 'c' | 'd' | 'e' | 'f'
  src: string
  alt: string
  caption: string
  label: string
}

const items: ProcessItem[] = [
  {
    area: 'a',
    src: '/proses/proses-04.webp',
    alt: 'Mesin cetak large-format Rajaku Printing tampak penuh, siap memproses pesanan',
    caption: 'Mesin cetak large-format Rajaku Printing, siap memproses pesanan banner.',
    label: 'Workshop',
  },
  {
    area: 'b',
    src: '/proses/proses-01.webp',
    alt: 'Panel kontrol mesin cetak dan tabung tinta',
    caption: 'Panel kontrol & tabung tinta dicek sebelum proses cetak dimulai.',
    label: 'Kalibrasi',
  },
  {
    area: 'c',
    src: '/proses/proses-03.webp',
    alt: 'Detail print-head bergerak di atas bahan banner',
    caption: 'Print-head bergerak presisi mengaplikasikan tinta ke bahan banner.',
    label: 'Print-head',
  },
  {
    area: 'd',
    src: '/proses/proses-02.webp',
    alt: 'Banner mulai tercetak keluar dari mesin',
    caption: 'Banner mulai tercetak, keluar dari mesin lembar demi lembar.',
    label: 'Mencetak',
  },
  {
    area: 'e',
    src: '/proses/proses-05.webp',
    alt: 'Banner hasil cetak digulung rapi di atas roll',
    caption: 'Banner hasil cetak digulung rapi setelah proses selesai.',
    label: 'Selesai cetak',
  },
  {
    area: 'f',
    src: '/proses/proses-06.webp',
    alt: 'Hasil akhir cetak banner tampak lebar',
    caption: 'Hasil akhir cetak — warna presisi, siap masuk tahap finishing.',
    label: 'Hasil akhir',
  },
]

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
  <section id="proses-cetak" class="mx-auto max-w-6xl px-4 py-16 md:py-24">
    <div class="max-w-2xl">
      <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Proses Cetak</p>
      <h2 class="mt-3 text-2xl md:text-3xl font-serif font-semibold tracking-tight text-ink-950">
        Dari file desain sampai banner jadi
      </h2>
      <p class="mt-3 text-sm leading-relaxed text-ink-500">
        Sekilas proses cetak large-format di workshop kami — klik gambar untuk melihat lebih
        besar.
      </p>
    </div>

    <motion.div
      class="process-bento mt-10 grid grid-cols-1 gap-4 sm:grid-cols-2"
      :variants="container"
      initial="hidden"
      while-in-view="show"
      :in-view-options="{ once: true, margin: '-100px' }"
    >
      <motion.button
        v-for="(p, i) in items"
        :key="p.src"
        type="button"
        :variants="item"
        :class="['process-area-' + p.area]"
        class="group relative aspect-[16/10] overflow-hidden rounded-lg border border-hairline bg-canvas-alt text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas lg:aspect-auto"
        @click="openAt(i, $event)"
      >
        <img
          :src="p.src"
          :alt="p.alt"
          width="1000"
          height="562"
          loading="lazy"
          class="h-full w-full object-cover transition-transform duration-300 ease-out group-hover:scale-[1.03]"
        />
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
/* Bento asimetris — cuma diaktifkan di desktop (lg). Di bawah itu grid biasa
   1-2 kolom (Tailwind, di atas) yang menang. Ditulis di sini (bukan arbitrary
   Tailwind class) supaya `grid-template-areas` tetap terbaca sebagai satu blok
   dan tidak riskan kena bug "kelas ada di DOM tapi CSS kosong". */
@media (min-width: 1024px) {
  .process-bento {
    grid-template-columns: repeat(4, 1fr);
    grid-template-rows: repeat(3, 200px);
    grid-template-areas:
      'a a b c'
      'a a d d'
      'e f f f';
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
}
</style>
