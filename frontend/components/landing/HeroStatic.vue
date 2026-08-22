<script setup lang="ts">
/**
 * HeroStatic — hero mobile (CLAUDE.md §18 "Layar 1"): satu poster besar,
 * headline, CTA. TIDAK memuat frame sequence maupun GSAP — komponen ini
 * dipilih lewat `useDevice()` di halaman induk (bukan CSS display:none),
 * supaya bundle hero desktop benar-benar tidak terkirim ke HP.
 *
 * Yang diperbaiki dari versi sebelumnya:
 *  1. Tingginya ditentukan oleh padding isi (`pt-24 pb-16`), jadi poster cuma
 *     mengisi ~60% layar dan gambar terpotong di posisi acak — kesan pertama
 *     jadi "gambar nanggung + ruang kosong". Sekarang hero mengunci satu layar
 *     penuh: `100svh` dikurangi tinggi navbar.
 *  2. Tiga tombol bertumpuk vertikal (Order, Showcase, Lacak) memakan ruang
 *     dan bikin hero terasa seperti daftar menu. Sekarang satu baris: CTA
 *     utama + satu tombol sekunder, `Lacak Resi` turun jadi tautan teks.
 *
 * `100svh` (small viewport height), bukan `100vh`: di browser HP `100vh`
 * dihitung dari layar TANPA bar URL, sehingga bagian bawah hero — tempat
 * tombolnya — tersembunyi di balik bar browser saat halaman pertama dibuka.
 *
 * Motion: satu momen orkestrasi (stagger masuk saat load) + Ken Burns sangat
 * lambat pada poster. Keduanya mati total saat `prefers-reduced-motion`.
 */
import { ArrowRight, ChevronDown, PackageSearch } from '@lucide/vue'
import { motion } from 'motion-v'

const { container, item, prefersReduced } = useRevealVariants({ stagger: 0.07, y: 14 })

// Bisa diganti admin (/admin/site-media, slot hero_poster_mobile) tanpa deploy
// ulang — fallback ke aset statis kalau slot kosong/backend mati.
const { resolve: resolveMedia } = useSiteMedia()
const heroPosterMobile = computed(() => resolveMedia('hero_poster_mobile'))

/**
 * Tiga hal yang membedakan layanan ini, diambil dari kemampuan yang memang
 * ada di sistem (§6 upload/request desain, §5 lacak resi publik, §7 bayar
 * manual transfer/QRIS) — bukan klaim angka yang tidak bisa dibuktikan.
 */
/**
 * Headline dipecah per kata supaya bisa masuk bergiliran, bukan satu blok
 * yang muncul sekaligus. Ini "momen orkestrasi" utama hero — sisanya
 * (subjudul, tombol, fakta) sengaja tetap tenang, satu gelombang saja.
 */
const headlineWords = [
  { text: 'Cetak', gold: false },
  { text: 'Banner,', gold: false },
  { text: 'Presisi', gold: true },
  { text: 'Setiap', gold: false },
  { text: 'Warna', gold: false },
]

// Headline masuk kata demi kata. Gilirannya diatur delay eksplisit per kata,
// BUKAN `staggerChildren` bertingkat: headline ini berada di dalam container
// hero yang juga punya stagger sendiri, dan variants bersarang di motion-v
// tidak meneruskan giliran ke cucu — hasilnya kata-katanya muncul serempak
// (terlihat saat diukur: tidak ada satu frame pun yang opacity-nya berbeda
// antar kata). Delay per indeks tidak bergantung pada perilaku pewarisan itu.
const WORD_STAGGER_S = 0.07
const WORD_LEAD_S = 0.18 // menyusul eyebrow, mendahului subjudul

function wordTransition(i: number) {
  if (prefersReduced.value) return { duration: 0 }
  return {
    duration: 0.55,
    delay: WORD_LEAD_S + i * WORD_STAGGER_S,
    ease: [0.22, 1, 0.36, 1] as const,
  }
}

const wordInitial = computed(() =>
  prefersReduced.value ? { opacity: 1, y: 0 } : { opacity: 0, y: 18 },
)

const facts = [
  'Desain sendiri / dibuatkan',
  'Progres dilacak lewat resi',
  'Transfer & QRIS',
]

const kenBurns = computed(() =>
  prefersReduced.value
    ? {}
    : {
        scale: [1, 1.08],
        transition: { duration: 24, ease: 'linear' as const, repeat: Infinity, repeatType: 'reverse' as const },
      },
)
</script>

<template>
  <!--
    `data-landing-hero`: penanda untuk LandingMobileStickyCta — bar CTA
    melayang di bawah baru muncul setelah elemen ini keluar dari layar.
  -->
  <section
    data-landing-hero
    class="hero-shell relative isolate flex min-h-[calc(100svh-3.5rem)] flex-col justify-end overflow-hidden bg-ink-950"
  >
    <!--
      Poster potret 900x1600 (`object-top`): fotonya mengisi paruh atas lalu
      meleleh ke ink-950 di paruh bawah — zona itu memang alas headline &
      tombol. Aset dibuat khusus potret; sebelumnya foto lanskap 828x466
      dipaksa mengisi layar potret, sehingga ter-crop besar dan yang paling
      terlihat justru penutup mesin yang polos.

      `object-top`, bukan center: dengan rasio HP yang bermacam-macam,
      penjangkaran ke atas menjaga susunan foto-lalu-teks tetap utuh.
    -->
    <!--
      Dua lapis gerak yang sengaja dipisah supaya tidak berebut `transform`
      pada satu elemen: pembungkus mengurus hanyutan mengikuti scroll (CSS,
      digerakkan posisi elemen di layar), fotonya sendiri mengurus Ken Burns
      (motion-v, digerakkan waktu). Kalau ditumpuk di elemen yang sama, salah
      satunya pasti menang dan yang lain hilang tanpa jejak.

      Pembungkus dibuat lebih tinggi dari sectionnya (`-top/-bottom-[6%]`)
      supaya saat dihanyutkan tidak ada tepi kosong yang muncul.
    -->
    <div class="hero-parallax absolute -top-[6%] -bottom-[6%] left-0 right-0 -z-10">
      <motion.img
        :src="heroPosterMobile"
        alt="Proses cetak banner large-format Rajaku Printing"
        width="900"
        height="1600"
        fetchpriority="high"
        class="h-full w-full object-cover object-top"
        :animate="kenBurns"
      />
    </div>

    <!--
      Scrim tiga zona, bukan satu gradient rata:
        bawah  — pekat, ini alas teks & tombol (kontras wajib terjamin);
        tengah — paling bening, di sinilah subjek fotonya berada;
        atas   — dipekatkan lagi supaya bidang mesin yang polos meleleh jadi
                 gelap, sekaligus jadi alas navbar.
      Versi sebelumnya justru kebalikannya (atas paling bening), sehingga yang
      paling terlihat adalah bagian foto yang paling tidak menarik.
    -->
    <div
      class="absolute inset-0 -z-10 bg-gradient-to-t from-ink-950 via-ink-950/45 to-transparent"
      aria-hidden="true"
    />
    <div
      class="absolute inset-x-0 top-0 -z-10 h-24 bg-gradient-to-b from-ink-950/75 to-transparent"
      aria-hidden="true"
    />

    <motion.div
      class="relative px-5 pb-10 pt-28"
      :variants="container"
      initial="hidden"
      animate="show"
    >
      <motion.p
        :variants="item"
        class="flex items-center gap-2 text-[10px] font-medium uppercase tracking-[0.14em] text-canvas/70"
      >
        <span class="h-px w-6 bg-gold-500" aria-hidden="true" />
        Percetakan Banner &middot; Trenggalek
      </motion.p>

      <h1
        class="mt-4 font-serif text-[clamp(2.25rem,10vw,3.25rem)] font-semibold leading-[0.98] tracking-tight text-canvas"
      >
        <!--
          `inline-block` per kata wajib: transform tidak berlaku pada elemen
          inline biasa, jadi tanpa ini kata-katanya cuma memudar tanpa naik.
          Spasi ditulis sebagai teks di antara span (bukan margin) supaya
          pemenggalan baris tetap normal dan teks tetap bisa diseleksi utuh.
        -->
        <template v-for="(w, i) in headlineWords" :key="w.text">
          <motion.span
            :initial="wordInitial"
            :animate="{ opacity: 1, y: 0 }"
            :transition="wordTransition(i)"
            :class="['inline-block', w.gold ? 'text-gold-400' : '']"
          >{{ w.text }}</motion.span>{{ i < headlineWords.length - 1 ? ' ' : '' }}
        </template>
      </h1>

      <motion.p :variants="item" class="mt-4 max-w-[34ch] text-sm leading-relaxed text-canvas/75">
        Order online, upload desain sendiri atau minta dibuatkan tim kami, lalu
        lacak progres cetaknya sampai siap diambil.
      </motion.p>

      <motion.div :variants="item" class="mt-7 flex items-center gap-2.5">
        <NuxtLink
          to="/order"
          class="inline-flex flex-1 items-center justify-center gap-2 rounded-md bg-brand-500 px-5 py-3.5 text-sm font-semibold text-canvas transition-colors duration-200 ease-out hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/50 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
        >
          Order Banner
          <ArrowRight class="h-4 w-4" :stroke-width="1.75" />
        </NuxtLink>
        <NuxtLink
          to="/showcase"
          class="inline-flex shrink-0 items-center justify-center rounded-md border border-canvas/25 px-4 py-3.5 text-sm font-semibold text-canvas transition-colors duration-200 ease-out hover:bg-canvas/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/50 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
        >
          Showcase
        </NuxtLink>
      </motion.div>

      <motion.div :variants="item" class="mt-4">
        <NuxtLink
          to="/lacak"
          class="inline-flex items-center gap-1.5 rounded-sm text-sm font-medium text-canvas/70 transition-colors duration-200 ease-out hover:text-canvas focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/50 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
        >
          <PackageSearch class="h-4 w-4" :stroke-width="1.5" />
          Lacak resi pesanan
        </NuxtLink>
      </motion.div>

      <!-- Tiga fakta layanan: mengisi kaki hero dengan isi, bukan ruang kosong -->
      <motion.ul
        :variants="item"
        class="mt-7 flex items-center border-t border-canvas/15 pt-4 text-[11px] leading-tight text-canvas/60"
      >
        <li
          v-for="(f, i) in facts"
          :key="f"
          :class="['flex-1', i > 0 ? 'border-l border-canvas/15 pl-3' : 'pr-3']"
        >
          {{ f }}
        </li>
      </motion.ul>
    </motion.div>

    <!-- Petunjuk gulir: hero setinggi layar butuh tanda bahwa masih ada isi -->
    <motion.div
      v-if="!prefersReduced"
      class="pointer-events-none absolute inset-x-0 bottom-2 flex justify-center"
      aria-hidden="true"
      :initial="{ opacity: 0 }"
      :animate="{ opacity: [0, 0.55, 0], y: [0, 6, 0] }"
      :transition="{ duration: 2.4, ease: 'easeInOut', repeat: Infinity, delay: 1.2 }"
    >
      <ChevronDown class="h-5 w-5 text-canvas" :stroke-width="1.5" />
    </motion.div>
  </section>
</template>

<style scoped>
/*
 * Hanyutan hero mengikuti scroll. Timeline-nya `view()` — dijalankan compositor
 * dari posisi elemen di layar, tanpa listener scroll di JavaScript.
 *
 * Rentangnya `exit`: hero berada di puncak halaman, jadi fase "entry" tidak
 * pernah terjadi — yang ada hanya saat ia bergeser keluar layar, dan di situlah
 * hanyutannya terasa.
 *
 * Browser tanpa dukungan `animation-timeline` (Safari saat ini) melewati blok
 * ini: foto diam, tidak ada yang rusak.
 */
@supports (animation-timeline: view()) {
  @media (prefers-reduced-motion: no-preference) {
    /*
      Timeline diberi NAMA di section, lalu dirujuk dari pembungkus foto —
      bukan `animation-timeline: view()` langsung di pembungkusnya.
      Alasannya: `view()` mengukur elemen terhadap wadah scroll terdekat, dan
      section ini `overflow-hidden`, yang menurut CSS sudah dihitung sebagai
      wadah scroll tersendiri. Dipasang langsung, hanyutannya diam total
      karena wadah itu memang tidak pernah tergulir. Section-nya sendiri
      diukur terhadap dokumen, jadi namanya membawa progres yang benar.
    */
    .hero-shell {
      view-timeline-name: --hero-view;
    }

    .hero-parallax {
      animation: hero-parallax linear both;
      animation-timeline: --hero-view;
      animation-range: exit 0% exit 100%;
    }
  }
}

@keyframes hero-parallax {
  from {
    transform: translateY(0);
  }
  to {
    transform: translateY(4%);
  }
}
</style>
