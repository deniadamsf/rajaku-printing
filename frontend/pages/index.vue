<script setup lang="ts">
/**
 * / — Landing page company profile.
 *
 * Hero adaptif §18 "scaffold 2 layar": desktop dapat hero cinematic scroll-scrub
 * (`LandingHeroScrollScrub`, §16 poin 1, GSAP ScrollTrigger + canvas frame
 * sequence), mobile dapat hero statis ringan (`LandingHeroStatic`, motion-v saja).
 * Device check pakai `useDevice()` (bukan CSS display:none) supaya bundle GSAP +
 * frame preloader benar-benar tidak terkirim ke mobile.
 *
 * Kedua varian hero dibungkus <ClientOnly> — komponen landing lain (katalog, cara
 * order, dst) SSR normal untuk SEO. Slot #fallback berisi markup hero statis murni
 * (tanpa JS) supaya crawler & first paint tetap dapat headline + CTA sebelum
 * hydration selesai memilih varian yang tepat.
 *
 * Design: patuh CLAUDE.md §26 (Fraunces display + Inter body, brand/gold/ink,
 * Lucide icon — tanpa rose/slate/emoji).
 */
import { ArrowRight } from '@lucide/vue'

const { isDesktop, isMobile } = useDevice()
const config = useRuntimeConfig()

// Gambar landing bisa diganti dari admin panel (/admin/site-media) tanpa
// deploy ulang — fetch SSR, fallback ke aset statis kalau slot kosong atau
// backend mati (lihat docblock useSiteMedia.ts). `await ready` supaya HTML
// SSR awal (dilihat crawler) langsung dapat gambar final, bukan fallback
// statis yang lalu "berkedip" ganti setelah hydration.
const { resolve: resolveMedia, ready: siteMediaReady } = useSiteMedia()
await siteMediaReady
const heroPosterDesktop = computed(() => resolveMedia('hero_poster_desktop'))
const heroPosterMobile = computed(() => resolveMedia('hero_poster_mobile'))

const canonical = config.public.appBaseUrl.replace(/\/$/, '')
// og_image: pakai URL absolut dari slot kalau diisi admin (backend sudah
// membangunnya dari APP_BASE_URL), else fallback statis relatif ke canonical.
const ogImage = computed(() => {
  const fromSlot = resolveMedia('og_image')
  return fromSlot.startsWith('http') ? fromSlot : `${canonical}${fromSlot}`
})

useSeoMeta({
  // Tanpa suffix brand — app.vue titleTemplate sudah menambahkan " — Rajaku Printing".
  title: 'Cetak Banner Cepat & Presisi',
  description:
    'Rajaku Printing — layanan cetak banner large-format untuk usaha, event, dan kebutuhan pribadi di Trenggalek. Order online, upload desain sendiri atau minta dibuatkan, lacak progres cetak real-time.',
  ogTitle: 'Rajaku Printing — Cetak Banner Cepat & Presisi',
  ogDescription:
    'Order banner online, upload desain sendiri atau minta dibuatkan tim kami, lalu lacak progres cetak sampai siap diambil.',
  ogImage,
  ogType: 'website',
  ogUrl: canonical,
  twitterCard: 'summary_large_image',
  twitterTitle: 'Rajaku Printing — Cetak Banner Cepat & Presisi',
  twitterDescription:
    'Cetak banner large-format untuk usaha, event, dan kebutuhan pribadi di Trenggalek. Order online, lacak progres cetak.',
  twitterImage: ogImage,
})

useHead({
  link: [{ rel: 'canonical', href: canonical }],
})
</script>

<template>
  <div>
    <ClientOnly>
      <LandingHeroScrollScrub v-if="isDesktop" />
      <LandingHeroStatic v-else />

      <template #fallback>
        <section data-landing-hero class="relative flex min-h-[calc(100svh-3.5rem)] flex-col justify-end overflow-hidden bg-ink-950 md:block md:min-h-0">
          <!--
            <picture> supaya mobile HANYA mengunduh poster-mobile (28KB), bukan
            poster desktop (52KB) juga. Fallback ini ter-render duluan via SSR di
            semua device, lalu hydration menggantinya dengan hero yang sesuai —
            tanpa media source, HP kebagian dua-duanya dan §18 (aset desktop
            jangan sampai ke mobile) bocor di jalur LCP.
          -->
          <picture>
            <source media="(max-width: 767px)" :srcset="heroPosterMobile" >
            <img
              :src="heroPosterDesktop"
              alt="Proses cetak banner large-format Rajaku Printing"
              width="1280"
              height="720"
              fetchpriority="high"
              class="absolute inset-0 h-full w-full object-cover object-top md:object-center"
            >
          </picture>
          <div class="absolute inset-0 bg-gradient-to-t from-ink-950/90 via-ink-950/45 to-ink-950/15" />
          <div class="relative z-10 px-5 pb-10 pt-28 text-left md:px-4 md:py-32 md:text-center">
            <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-canvas/70">
              Percetakan Banner &middot; Trenggalek
            </p>
            <h1
              class="mt-4 font-serif text-[clamp(2.5rem,11vw,3.5rem)] md:text-7xl font-semibold tracking-tight leading-[0.98] md:leading-[1.05] text-canvas"
            >
              Cetak Banner, <span class="text-gold-400">Presisi</span> Setiap Warna
            </h1>
            <p class="mt-4 max-w-[34ch] md:mx-auto md:max-w-xl text-sm md:text-base leading-relaxed text-canvas/80">
              Order online, upload desain sendiri atau minta dibuatkan tim kami,
              lalu lacak progres cetaknya sampai siap diambil.
            </p>
            <div class="mt-7 flex flex-row items-center gap-2.5 md:mt-8 md:justify-center md:gap-3">
              <NuxtLink
                to="/order"
                class="inline-flex flex-1 items-center justify-center gap-2 rounded-md bg-brand-500 px-5 py-3.5 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950 md:flex-none md:px-6 md:py-3"
              >
                Order Banner
                <ArrowRight class="h-4 w-4" :stroke-width="1.5" />
              </NuxtLink>
            </div>
          </div>
        </section>
      </template>
    </ClientOnly>

    <!--
      Urutan section sengaja berselang-seling terang/gelap dan grid/split/galeri.
      Sebelumnya semua section berpola sama (judul + grid kartu teks), yang
      membuat halaman terasa panjang tapi kosong. Kalau menambah section baru,
      jaga polanya tetap bergantian, jangan menumpuk dua grid kartu berurutan.
    -->
    <!-- Pita berjalan: jembatan dari hero gelap ke section terang di bawahnya -->
    <LandingMarqueeBand />
    <LandingProductSlider />
    <LandingServicesSection />
    <LandingProcessGallery />
    <LandingHowItWorksSection />
    <LandingMaterialsSection />
    <LandingPriceTeaser />
    <LandingWhyUsSection />
    <LandingFaqSection />
    <LandingArticlesTeaser />
    <LandingClosingCta />

    <!-- Reserve ruang footer supaya tidak ketutup sticky bottom CTA mobile (§17). -->
    <div class="h-20 md:hidden" aria-hidden="true" />

    <ClientOnly>
      <LandingMobileStickyCta v-if="isMobile" />
    </ClientOnly>
  </div>
</template>
