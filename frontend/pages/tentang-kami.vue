<script setup lang="ts">
/**
 * /tentang-kami — Profil perusahaan.
 *
 * Data NAP & jam buka SATU-SATUNYA dari `~/utils/business.ts` (masih
 * placeholder TODO(rajaku) — jangan tulis ulang alamat/telepon literal di
 * sini). JSON-LD `AboutPage` merujuk (`about.@id`) ke LocalBusiness yang
 * sudah dipasang global di `layouts/default.vue` — tidak duplikasi @id.
 *
 * Mascot/logo (§26.8) hanya boleh muncul di halaman ini/footer, bukan di
 * admin/checkout — di sini dipakai `logo-full-sm.webp` (480px) sebagai
 * elemen brand utama section cerita. LOGO.png (8.7MB) sengaja tidak dipakai.
 * favicon.svg tetap dipakai sebagai monogram kecil di header.
 *
 * Foto proses (2 dari 6 aset showcase) dipakai untuk memecah dinding teks di
 * section cerita — bukan klaim/statistik, murni gambar proses cetak asli.
 *
 * Momen orkestrasi utama halaman ini: blok cerita (emblem logo + foto
 * proses) reveal bersamaan sekali saat masuk viewport. Section lain hanya
 * fade tunggal tanpa stagger — halaman ini harus tetap terasa tenang.
 */
import { Clock, Mail, MapPin, Phone, ShieldCheck, Sparkles, Truck, Users } from '@lucide/vue'
import { motion } from 'motion-v'
import { business } from '~/utils/business'

const prefersReducedMotion = usePrefersReducedMotion()
const fadeTransition = computed(() =>
  prefersReducedMotion.value ? { duration: 0 } : { duration: 0.5, ease: [0.22, 1, 0.36, 1] as const },
)
function storyDelay(step: number) {
  return prefersReducedMotion.value ? 0 : step * 0.1
}

definePageMeta({ layout: 'default' })

const config = useRuntimeConfig()
const canonical = `${config.public.appBaseUrl.replace(/\/$/, '')}/tentang-kami`
const metaDesc =
  'Kenali Rajaku Printing — percetakan banner dan digital printing large-format di Trenggalek, Jawa Timur. Cerita, keunggulan, dan cara menghubungi kami.'

useSeoMeta({
  title: 'Tentang Kami',
  description: metaDesc,
  ogTitle: 'Tentang Kami — Rajaku Printing',
  ogDescription: metaDesc,
  ogType: 'website',
  ogUrl: canonical,
  twitterCard: 'summary',
})

useHead({
  link: [{ rel: 'canonical', href: canonical }],
  script: [
    {
      type: 'application/ld+json',
      innerHTML: JSON.stringify({
        '@context': 'https://schema.org',
        '@type': 'AboutPage',
        name: 'Tentang Rajaku Printing',
        url: canonical,
        description: metaDesc,
        about: { '@id': `${config.public.appBaseUrl.replace(/\/$/, '')}/#business` },
      }),
    },
  ],
})

const values = [
  {
    icon: ShieldCheck,
    title: 'Transparan tiap tahap',
    desc: 'Status order, ongkir, dan verifikasi pembayaran bisa dipantau lewat nomor resi — tidak ada informasi yang disembunyikan.',
  },
  {
    icon: Sparkles,
    title: 'Kualitas cetak konsisten',
    desc: 'Kalibrasi mesin large-format rutin supaya warna hasil cetak sesuai desain, dari pesanan pertama sampai berikutnya.',
  },
  {
    icon: Users,
    title: 'Fleksibel sesuai kebutuhan',
    desc: 'Bawa desain sendiri atau serahkan ke tim kami — cocok untuk usaha kecil maupun kebutuhan event mendadak.',
  },
  {
    icon: Truck,
    title: 'Pickup atau dikirim',
    desc: 'Ambil langsung di tempat untuk hemat waktu, atau biar kami kirimkan ke alamat Anda.',
  },
]

function waLink(): string {
  return `https://wa.me/${business.whatsapp}`
}
</script>

<template>
  <main>
    <!-- Header -->
    <section class="mx-auto max-w-6xl px-4 pt-16 pb-10 md:pt-24 md:pb-14">
      <div class="flex items-start gap-4">
        <img
          src="/favicon.svg"
          alt="Monogram Rajaku Printing"
          width="40"
          height="40"
          loading="lazy"
          class="mt-1 h-10 w-10 shrink-0 rounded-md ring-1 ring-inset ring-gold-500/30"
        >
        <div>
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Tentang Kami</p>
          <h1 class="mt-2 text-2xl md:text-3xl font-serif font-semibold tracking-tight text-ink-950">
            Rajaku Printing
          </h1>
        </div>
      </div>
      <p class="mt-5 max-w-2xl text-sm md:text-base leading-relaxed text-ink-700">
        {{ business.description }} Kami melayani pelanggan perorangan, usaha kecil, hingga
        penyelenggara acara yang butuh hasil cetak rapi dengan proses yang jelas dari order
        sampai banner siap diambil atau dikirim.
      </p>
    </section>

    <!-- Cerita singkat — logo emblem + foto proses memecah dinding teks -->
    <section class="mx-auto max-w-6xl px-4 pb-16 md:pb-24">
      <div class="rounded-lg border border-hairline bg-canvas p-6 md:p-10">
        <div class="grid gap-8 md:grid-cols-[220px_1fr] md:items-center">
          <motion.div
            class="mx-auto flex h-40 w-40 items-center justify-center rounded-lg border border-hairline bg-ink-950 p-6 md:mx-0 md:h-full md:w-full"
            :initial="{ opacity: 0, scale: 0.96 }"
            :while-in-view="{ opacity: 1, scale: 1 }"
            :in-view-options="{ once: true, margin: '-80px' }"
            :transition="{ ...fadeTransition, delay: storyDelay(0) }"
          >
            <img
              src="/brand/logo-full-sm.webp"
              alt="Logo Rajaku Printing"
              width="480"
              height="461"
              loading="lazy"
              class="h-full w-full object-contain"
            >
          </motion.div>

          <motion.div
            :initial="{ opacity: 0, y: 12 }"
            :while-in-view="{ opacity: 1, y: 0 }"
            :in-view-options="{ once: true, margin: '-80px' }"
            :transition="{ ...fadeTransition, delay: storyDelay(1) }"
          >
            <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Cerita kami</p>
            <h2 class="mt-3 max-w-2xl text-lg md:text-xl font-serif font-medium leading-relaxed text-ink-950">
              Dibangun dari kebutuhan sederhana: banner yang tepat waktu, warnanya presisi, dan
              prosesnya tidak bikin bingung.
            </h2>
            <p class="mt-4 max-w-2xl text-sm leading-relaxed text-ink-600">
              Rajaku Printing berangkat dari pengalaman melayani kebutuhan cetak sehari-hari di
              {{ business.addressLocality }} dan sekitarnya — mulai dari banner toko, spanduk acara,
              sampai kebutuhan cetak mendadak yang tidak bisa menunggu lama. Setiap order kami
              perlakukan dengan proses yang sama: jelas di awal soal harga dan bahan, transparan soal
              status pengerjaan, dan konsisten soal kualitas hasil cetak.
            </p>
          </motion.div>
        </div>

        <motion.div
          class="mt-8 overflow-hidden rounded-lg border border-hairline"
          :initial="{ opacity: 0, y: 16 }"
          :while-in-view="{ opacity: 1, y: 0 }"
          :in-view-options="{ once: true, margin: '-80px' }"
          :transition="{ ...fadeTransition, delay: storyDelay(2) }"
        >
          <img
            src="/proses/proses-04.webp"
            alt="Mesin cetak large-format Rajaku Printing"
            width="1000"
            height="562"
            loading="lazy"
            class="h-full w-full object-cover"
          >
        </motion.div>
      </div>
    </section>

    <!-- Jeda visual: dua foto proses memecah transisi cerita → nilai -->
    <section class="mx-auto max-w-6xl px-4 pb-16 md:pb-24">
      <motion.div
        class="grid gap-4 sm:grid-cols-2"
        :initial="{ opacity: 0, y: 16 }"
        :while-in-view="{ opacity: 1, y: 0 }"
        :in-view-options="{ once: true, margin: '-100px' }"
        :transition="fadeTransition"
      >
        <div class="overflow-hidden rounded-lg border border-hairline">
          <img
            src="/proses/proses-01.webp"
            alt="Panel kontrol mesin cetak dan tabung tinta"
            width="1000"
            height="562"
            loading="lazy"
            class="h-full w-full object-cover"
          >
        </div>
        <div class="overflow-hidden rounded-lg border border-hairline">
          <img
            src="/proses/proses-06.webp"
            alt="Hasil cetak banner yang sudah selesai"
            width="1000"
            height="562"
            loading="lazy"
            class="h-full w-full object-cover"
          >
        </div>
      </motion.div>
    </section>

    <!-- Keunggulan / values -->
    <section class="bg-canvas-alt border-y border-hairline">
      <div class="mx-auto max-w-6xl px-4 py-16 md:py-24">
        <div class="max-w-2xl">
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Kenapa memilih kami</p>
          <h2 class="mt-3 text-lg md:text-xl font-sans font-semibold text-ink-950">
            Nilai yang kami pegang di setiap order
          </h2>
        </div>

        <ul class="mt-10 grid gap-6 sm:grid-cols-2">
          <li v-for="v in values" :key="v.title" class="flex gap-4">
            <span
              class="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-md border border-hairline bg-canvas text-brand-500"
            >
              <component :is="v.icon" class="h-5 w-5" :stroke-width="1.5" />
            </span>
            <div>
              <h3 class="text-sm font-sans font-semibold text-ink-950">{{ v.title }}</h3>
              <p class="mt-1 text-sm leading-relaxed text-ink-500">{{ v.desc }}</p>
            </div>
          </li>
        </ul>
      </div>
    </section>

    <!-- Kontak & jam buka -->
    <section class="mx-auto max-w-6xl px-4 py-16 md:py-24">
      <div class="grid gap-10 md:grid-cols-2">
        <div>
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Kontak</p>
          <h2 class="mt-3 text-lg md:text-xl font-sans font-semibold text-ink-950">
            Hubungi atau kunjungi kami
          </h2>

          <dl class="mt-6 space-y-4">
            <div class="flex items-start gap-3">
              <MapPin class="mt-0.5 h-4 w-4 shrink-0 text-brand-500" :stroke-width="1.5" />
              <dd class="text-sm leading-relaxed text-ink-700">
                {{ business.streetAddress }}, {{ business.addressLocality }},
                {{ business.addressRegion }} {{ business.postalCode }}
              </dd>
            </div>
            <div class="flex items-start gap-3">
              <Phone class="mt-0.5 h-4 w-4 shrink-0 text-brand-500" :stroke-width="1.5" />
              <dd>
                <a
                  :href="`tel:${business.telephone}`"
                  class="font-mono text-xs text-ink-700 hover:text-brand-500 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas rounded-sm"
                >
                  {{ business.telephone }}
                </a>
              </dd>
            </div>
            <div class="flex items-start gap-3">
              <Mail class="mt-0.5 h-4 w-4 shrink-0 text-brand-500" :stroke-width="1.5" />
              <dd>
                <a
                  :href="`mailto:${business.email}`"
                  class="text-sm text-ink-700 hover:text-brand-500 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas rounded-sm"
                >
                  {{ business.email }}
                </a>
              </dd>
            </div>
            <div class="flex items-start gap-3">
              <Clock class="mt-0.5 h-4 w-4 shrink-0 text-brand-500" :stroke-width="1.5" />
              <dd class="space-y-1">
                <p v-for="h in business.openingHours" :key="h.label" class="text-sm leading-relaxed text-ink-700">
                  {{ h.label }}
                </p>
              </dd>
            </div>
          </dl>

          <div class="mt-6 flex flex-wrap gap-2">
            <span
              v-for="area in business.serviceArea"
              :key="area"
              class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium text-ink-600 bg-canvas-alt ring-1 ring-inset ring-hairline"
            >
              {{ area }}
            </span>
          </div>

          <a
            :href="waLink()"
            target="_blank"
            rel="noopener noreferrer"
            class="mt-8 inline-flex items-center justify-center gap-2 rounded-md bg-brand-500 px-6 py-3 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
          >
            Chat via WhatsApp
          </a>
        </div>

        <div class="rounded-lg border border-hairline bg-ink-950 p-8 md:p-10 flex flex-col justify-center">
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-gold-400">Siap dibantu</p>
          <h2 class="mt-3 font-serif text-xl md:text-2xl font-semibold text-canvas">
            Punya kebutuhan cetak? Mulai order kapan saja.
          </h2>
          <p class="mt-3 text-sm leading-relaxed text-canvas/75">
            Order online lewat website, atau datang langsung ke lokasi kami untuk konsultasi
            bahan dan ukuran.
          </p>
          <NuxtLink
            to="/order"
            class="mt-6 inline-flex w-fit items-center justify-center gap-2 rounded-md border border-canvas/25 px-6 py-3 text-sm font-semibold text-canvas transition-colors hover:bg-canvas/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
          >
            Order Banner
          </NuxtLink>
        </div>
      </div>
    </section>
  </main>
</template>
