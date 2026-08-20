<script setup lang="ts">
/**
 * Layout default — sticky navbar (§17): CTA "Order Banner" & "Login" wajib selalu
 * visible di semua halaman publik. Compliant dengan CLAUDE.md §26 (brand tokens
 * + Fraunces wordmark, tanpa rose/slate).
 *
 * JSON-LD LocalBusiness (§15) dipasang sekali di sini (bukan per-halaman) —
 * data NAP diambil dari satu sumber `~/utils/business.ts`.
 */
import { business } from '~/utils/business'

const auth = useAuthStore()
const route = useRoute()
const config = useRuntimeConfig()

// Focus ring §26.6 — wajib di semua elemen interaktif. Dijadikan konstanta karena
// dipakai di 8 link/button navbar & footer; menulis ulang inline bikin drift.
const focusRing =
  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas rounded-sm'

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
    geo: {
      '@type': 'GeoCoordinates',
      latitude: business.geo.latitude,
      longitude: business.geo.longitude,
    },
    areaServed: business.serviceArea,
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

async function onLogout() {
  await auth.logout()
  await navigateTo('/')
}

function isActive(prefix: string) {
  return route.path === prefix || route.path.startsWith(prefix + '/')
}
</script>

<template>
  <div class="min-h-screen flex flex-col bg-canvas text-ink-900 font-sans">
    <header class="sticky top-0 z-40 bg-canvas/85 backdrop-blur border-b border-hairline">
      <div class="mx-auto max-w-6xl px-4 h-14 flex items-center justify-between">
        <NuxtLink to="/" :class="['font-serif text-lg tracking-tight text-ink-950', focusRing]">
          Rajaku
          <span class="text-gold-500 font-normal">Printing</span>
        </NuxtLink>

        <nav class="flex items-center gap-4 sm:gap-6">
          <NuxtLink
            to="/katalog"
            :class="[
              'hidden sm:inline text-sm font-medium transition-colors',
              isActive('/katalog') ? 'text-brand-500' : 'text-ink-700 hover:text-ink-950',
              focusRing,
            ]"
          >
            Katalog
          </NuxtLink>

          <NuxtLink
            to="/showcase"
            :class="[
              'hidden sm:inline text-sm font-medium transition-colors',
              isActive('/showcase') ? 'text-brand-500' : 'text-ink-700 hover:text-ink-950',
              focusRing,
            ]"
          >
            Showcase
          </NuxtLink>

          <NuxtLink
            to="/artikel"
            :class="[
              'hidden sm:inline text-sm font-medium transition-colors',
              isActive('/artikel') ? 'text-brand-500' : 'text-ink-700 hover:text-ink-950',
              focusRing,
            ]"
          >
            Artikel
          </NuxtLink>

          <NuxtLink
            to="/order"
            class="inline-flex items-center rounded-md bg-brand-500 text-canvas px-4 py-2 text-sm font-semibold hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors"
          >
            Order Banner
          </NuxtLink>

          <template v-if="auth.isAuthenticated">
            <NuxtLink
              :to="auth.homePath"
              :class="['text-sm font-medium text-ink-700 hover:text-ink-950 transition-colors', focusRing]"
            >
              {{ auth.user?.name?.split(' ')[0] || 'Akun' }}
            </NuxtLink>
            <button
              type="button"
              :class="['text-sm font-medium text-ink-500 hover:text-brand-500 transition-colors', focusRing]"
              @click="onLogout"
            >
              Keluar
            </button>
          </template>
          <template v-else>
            <NuxtLink
              to="/login"
              :class="['text-sm font-medium text-ink-700 hover:text-ink-950 transition-colors', focusRing]"
            >
              Login
            </NuxtLink>
          </template>
        </nav>
      </div>
    </header>

    <main class="flex-1">
      <slot />
    </main>

    <footer class="border-t border-hairline py-8 mt-16">
      <div class="mx-auto max-w-6xl px-4 flex flex-col sm:flex-row items-center justify-between gap-4 text-xs text-ink-500">
        <div class="flex items-center gap-2">
          <img
            :src="footerLogo"
            alt="Rajaku Printing"
            width="480"
            height="461"
            loading="lazy"
            class="h-8 w-auto"
          >
        </div>
        <nav class="flex flex-wrap items-center justify-center gap-5">
          <NuxtLink to="/katalog" :class="['hover:text-ink-900 transition-colors', focusRing]">Katalog</NuxtLink>
          <NuxtLink to="/showcase" :class="['hover:text-ink-900 transition-colors', focusRing]">Showcase</NuxtLink>
          <NuxtLink to="/tentang-kami" :class="['hover:text-ink-900 transition-colors', focusRing]">Tentang Kami</NuxtLink>
          <NuxtLink to="/artikel" :class="['hover:text-ink-900 transition-colors', focusRing]">Artikel</NuxtLink>
          <NuxtLink to="/order" :class="['hover:text-ink-900 transition-colors', focusRing]">Order Banner</NuxtLink>
          <NuxtLink to="/lacak" :class="['hover:text-ink-900 transition-colors', focusRing]">Lacak Resi</NuxtLink>
        </nav>
        <p>&copy; {{ new Date().getFullYear() }} Rajaku Printing</p>
      </div>
    </footer>
  </div>
</template>
