<script setup lang="ts">
/**
 * Layout default — kerangka halaman publik: navbar (`SiteHeader`, §17 — CTA
 * "Order Banner" & akses login selalu terlihat), slot konten, dan footer.
 *
 * Navbar sengaja tinggal di komponennya sendiri: dia punya state (drawer
 * mobile, status scroll) yang tidak ada urusannya dengan layout.
 *
 * JSON-LD LocalBusiness (§15) dipasang sekali di sini (bukan per-halaman) —
 * data NAP diambil dari satu sumber `~/utils/business.ts`.
 */
import { ArrowRight, Clock, Mail, MapPin, MessageCircle, PackageSearch } from '@lucide/vue'
import { business } from '~/utils/business'

const config = useRuntimeConfig()

// Focus ring §26.6 — wajib di semua elemen interaktif. Dijadikan konstanta karena
// dipakai di 8 link/button navbar & footer; menulis ulang inline bikin drift.
/** Tautan kolom footer — satu definisi, dipakai belasan kali di bawah. */
const footerLink =
  'text-canvas/70 transition-colors duration-200 ease-out hover:text-canvas'

const footerExplore = [
  { label: 'Katalog', to: '/katalog' },
  { label: 'Showcase', to: '/showcase' },
  { label: 'Artikel', to: '/artikel' },
  { label: 'Tentang Kami', to: '/tentang-kami' },
] as const

// Offset ring memakai ink-950 karena seluruh pemakaiannya kini ada di footer
// yang berlatar gelap — offset canvas akan menggambar cincin putih di atas
// hitam, terlihat seperti cacat render.
const focusRing =
  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold-400/50 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950 rounded-sm'

const baseUrl = config.public.appBaseUrl.replace(/\/$/, '')

// Bisa diganti admin (/admin/site-media) tanpa deploy ulang — fallback ke aset
// statis kalau slot kosong/backend mati (lihat docblock useSiteMedia.ts).
const { resolve: resolveMedia } = useSiteMedia()
const footerLogo = computed(() => resolveMedia('brand_logo_full_sm'))
const schemaImage = computed(() => {
  const fromSlot = resolveMedia('og_image')
  return fromSlot.startsWith('http') ? fromSlot : `${baseUrl}${fromSlot}`
})

// Dibungkus computed (bukan JSON.stringify eager) supaya reaktif terhadap
// `schemaImage` — data site-media di-fetch non-blocking (tanpa top-level
// await) karena layout ini bukan dalam boundary <Suspense> milik NuxtPage;
// unhead resolve ref/computed ini lazy saat serialisasi, jadi kalau
// `pages/index.vue` (child, punya Suspense) sudah men-`await` fetch dengan
// key sama, nilainya sudah benar tersedia sebelum head diserialisasi.
const jsonLd = computed(() =>
  JSON.stringify({
    '@context': 'https://schema.org',
    '@type': 'LocalBusiness',
    '@id': `${baseUrl}/#business`,
    name: business.name,
    legalName: business.legalName,
    description: business.description,
    url: baseUrl,
    image: schemaImage.value,
    telephone: business.telephone,
    email: business.email,
    priceRange: business.priceRange,
    address: {
      '@type': 'PostalAddress',
      streetAddress: business.streetAddress,
      addressLocality: business.addressLocality,
      addressRegion: business.addressRegion,
      postalCode: business.postalCode,
      addressCountry: business.addressCountry,
    },
    // `undefined` otomatis dihilangkan JSON.stringify — jadi kalau koordinat
    // belum diverifikasi, blok `geo` tidak ikut terkirim sama sekali (lihat
    // alasannya di utils/business.ts). Jangan diganti jadi objek berisi 0/null:
    // koordinat 0,0 menaruh toko di Samudra Atlantik.
    geo: business.geo
      ? {
          '@type': 'GeoCoordinates',
          latitude: business.geo.latitude,
          longitude: business.geo.longitude,
        }
      : undefined,
    areaServed: business.serviceArea,
    paymentAccepted: business.paymentAccepted.join(', '),
    currenciesAccepted: 'IDR',
    openingHoursSpecification: business.openingHours.map((h) => ({
      '@type': 'OpeningHoursSpecification',
      dayOfWeek: h.days,
      opens: h.opens,
      closes: h.closes,
    })),
  }),
)

useHead({
  script: [
    {
      type: 'application/ld+json',
      innerHTML: jsonLd,
    },
  ],
})
</script>

<template>
  <div class="min-h-screen flex flex-col bg-canvas text-ink-900 font-sans">
    <!--
      Garis progres baca (gaya di assets/css/tailwind.css). Murni penanda
      visual, karenanya `aria-hidden` — pembaca layar sudah punya cara
      sendiri mengetahui posisi di dalam dokumen.
    -->
    <div class="scroll-progress" aria-hidden="true" />

    <SiteHeader />

    <main class="flex-1">
      <slot />
    </main>

    <!--
      Footer sengaja GELAP dan menempel langsung ke section penutup yang juga
      gelap — sebelumnya ada jarak putih `mt-16` di antara keduanya, yang
      terbaca sebagai sobekan terang di kaki halaman, bukan sebagai penutup.

      Isinya NAP (nama-alamat-telepon) dari `~/utils/business.ts`, sumber yang
      sama dengan JSON-LD LocalBusiness di atas. Menampilkannya sebagai teks
      biasa penting untuk SEO lokal (§15): data yang cuma hidup di JSON-LD
      tidak pernah terbaca pengunjung, dan mesin pencari lebih percaya NAP
      yang konsisten antara markup dan tampilan.
    -->
    <footer class="bg-ink-950 text-canvas/70 print:hidden">
      <div class="mx-auto max-w-6xl px-4 py-10 md:py-16">
        <div class="grid gap-8 md:grid-cols-[1.5fr_1fr_1fr] md:gap-12">
          <div>
            <img
              :src="footerLogo"
              alt="Rajaku Printing"
              width="480"
              height="461"
              loading="lazy"
              class="h-16 w-auto"
            >
            <p class="mt-4 max-w-xs text-sm leading-relaxed text-canvas/60">
              {{ business.description }}
            </p>

            <ul class="mt-6 space-y-3 text-sm">
              <li class="flex gap-3">
                <MapPin class="mt-0.5 h-4 w-4 shrink-0 text-gold-500" :stroke-width="1.5" />
                <span class="text-canvas/70">
                  {{ business.streetAddress }},
                  {{ business.addressLocality }}, {{ business.addressRegion }}
                </span>
              </li>
              <li class="flex gap-3">
                <Clock class="mt-0.5 h-4 w-4 shrink-0 text-gold-500" :stroke-width="1.5" />
                <span class="text-canvas/70">{{ business.openingHours[0].label }}</span>
              </li>
            </ul>
          </div>

          <nav aria-label="Jelajahi">
            <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-canvas/45">Jelajahi</p>
            <ul class="mt-4 space-y-2.5 text-sm">
              <li v-for="l in footerExplore" :key="l.to">
                <NuxtLink :to="l.to" :class="[footerLink, focusRing]">{{ l.label }}</NuxtLink>
              </li>
            </ul>
          </nav>

          <div>
            <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-canvas/45">Hubungi</p>
            <ul class="mt-4 space-y-2.5 text-sm">
              <li>
                <a
                  :href="`https://wa.me/${business.whatsapp}`"
                  target="_blank"
                  rel="noopener"
                  :class="['inline-flex items-center gap-2', footerLink, focusRing]"
                >
                  <MessageCircle class="h-4 w-4 shrink-0" :stroke-width="1.5" />
                  WhatsApp
                </a>
              </li>
              <li>
                <a :href="`mailto:${business.email}`" :class="['inline-flex items-center gap-2', footerLink, focusRing]">
                  <Mail class="h-4 w-4 shrink-0" :stroke-width="1.5" />
                  {{ business.email }}
                </a>
              </li>
              <li>
                <NuxtLink to="/lacak" :class="['inline-flex items-center gap-2', footerLink, focusRing]">
                  <PackageSearch class="h-4 w-4 shrink-0" :stroke-width="1.5" />
                  Lacak Resi
                </NuxtLink>
              </li>
            </ul>

            <NuxtLink
              to="/order"
              :class="[
                'mt-6 inline-flex items-center gap-2 rounded-md bg-brand-500 px-4 py-2.5 text-sm font-semibold text-canvas transition-colors duration-200 ease-out hover:bg-brand-600',
                focusRing,
              ]"
            >
              Order Banner
              <ArrowRight class="h-4 w-4" :stroke-width="1.5" />
            </NuxtLink>
          </div>
        </div>

        <div
          class="mt-8 flex flex-col gap-2 border-t border-white/10 pt-6 text-xs text-canvas/45 sm:flex-row sm:items-center sm:justify-between"
        >
          <p>&copy; {{ new Date().getFullYear() }} {{ business.name }}</p>
          <p>Melayani {{ business.serviceArea.join(' · ') }}</p>
        </div>
      </div>
    </footer>
  </div>
</template>
