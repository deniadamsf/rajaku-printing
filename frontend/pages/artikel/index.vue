<script setup lang="ts">
/**
 * /artikel — list artikel published (SSR untuk SEO §15).
 * Compliant CLAUDE.md §26 — Fraunces display, brand tokens, generous spacing.
 */
import { Newspaper, RefreshCw, SearchX } from '@lucide/vue'
import type { Article, ArticleListResponse } from '~/types/cms'

definePageMeta({ layout: 'default' })

const route = useRoute()
const cms = useCms()
const config = useRuntimeConfig()

const currentPage = computed(() => {
  const p = Number(route.query.page)
  return Number.isFinite(p) && p > 0 ? p : 1
})
const searchQuery = computed(() => (route.query.q as string) || '')

const perPage = 9

// SSR fetch — useAsyncData supaya hydration konsisten & tidak flicker.
const { data, pending, error, refresh } = await useAsyncData<ArticleListResponse>(
  () => `articles-list-${currentPage.value}-${searchQuery.value}`,
  () => cms.listPublic({
    page: currentPage.value,
    limit: perPage,
    q: searchQuery.value || undefined,
  }),
  { watch: [currentPage, searchQuery] },
)

const items = computed<Article[]>(() => data.value?.items ?? [])
const total = computed(() => data.value?.total ?? 0)
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / perPage)))

// --- SEO meta ---
const canonical = computed(() => {
  const base = config.public.appBaseUrl
  const qs = new URLSearchParams()
  if (currentPage.value > 1) qs.set('page', String(currentPage.value))
  if (searchQuery.value) qs.set('q', searchQuery.value)
  const query = qs.toString()
  return `${base}/artikel${query ? `?${query}` : ''}`
})

useSeoMeta({
  // Tanpa suffix brand — `app.vue` titleTemplate sudah menambahkan
  // " — Rajaku Printing". Menuliskannya lagi di sini membuat judul tab & hasil
  // pencarian berbunyi "… — Rajaku Printing — Rajaku Printing".
  title: () =>
    currentPage.value > 1
      ? `Artikel — halaman ${currentPage.value}`
      : 'Artikel & Tips Cetak Banner',
  description:
    'Panduan, tips, dan referensi seputar cetak banner outdoor & indoor dari Rajaku Printing — bahan, ukuran, harga, hingga trik desain.',
  ogTitle: () => 'Artikel Rajaku Printing',
  ogType: 'website',
  ogUrl: () => canonical.value,
  twitterCard: 'summary',
})

useHead({
  link: [{ rel: 'canonical', href: canonical.value }],
})

// --- Search input handling ---
const searchInput = ref(searchQuery.value)
let searchTimer: ReturnType<typeof setTimeout> | null = null

function onSearchInput() {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    const q = searchInput.value.trim()
    navigateTo({
      path: '/artikel',
      query: { ...(q ? { q } : {}) },
    })
  }, 350)
}

function pageURL(p: number) {
  const q: Record<string, string> = {}
  if (p > 1) q.page = String(p)
  if (searchQuery.value) q.q = searchQuery.value
  return { path: '/artikel', query: q }
}

function coverURL(a: Article): string | null {
  return a.cover_image_id ? cms.imageUrl(a.cover_image_id) : null
}

/**
 * Huruf awal judul, dipakai sebagai penanda visual kartu artikel yang belum
 * punya cover. Tujuannya membedakan kartu satu sama lain — kalau semua kartu
 * tanpa cover memakai ikon yang sama, grid-nya kembali terlihat seperti
 * placeholder kosong. Murni dekoratif (`aria-hidden` di template).
 */
function coverInitial(title: string): string {
  return title.trim().charAt(0).toUpperCase() || 'R'
}

function fmtDate(s?: string | null): string {
  if (!s) return ''
  try {
    return new Date(s).toLocaleDateString('id-ID', {
      day: '2-digit', month: 'long', year: 'numeric',
    })
  } catch {
    return s
  }
}
</script>

<template>
  <div class="relative mx-auto max-w-6xl px-4 py-16 md:py-24">
    <!--
      Ornamen kisi tipis hanya di area header — memberi tekstur pada ruang
      kosong di sekitar judul tanpa mengganggu keterbacaan kartu di bawahnya.
    -->
    <div class="pointer-events-none absolute inset-x-0 top-0 h-72 overflow-hidden" aria-hidden="true">
      <ArtOrnament variant="grid" :opacity="0.6" />
    </div>

    <!-- Header -->
    <header class="relative max-w-2xl">
      <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
        Panduan & Tips
      </p>
      <h1 class="mt-3 font-serif text-4xl md:text-5xl font-semibold tracking-tight text-ink-950 leading-[1.1]">
        Artikel <span class="text-brand-500">Rajaku Printing</span>
      </h1>
      <p class="mt-4 text-base text-ink-500 leading-relaxed">
        Referensi seputar cetak banner outdoor &amp; indoor — bahan, ukuran, harga,
        dan tips desain dari tim Rajaku Printing.
      </p>
    </header>

    <!-- Search -->
    <div class="relative mt-8 max-w-md">
      <label for="artikel-search" class="sr-only">Cari artikel</label>
      <input
        id="artikel-search"
        v-model="searchInput"
        type="search"
        placeholder="Cari artikel…"
        class="block w-full rounded-md border border-hairline bg-canvas px-4 py-2.5 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
        @input="onSearchInput"
      >
    </div>

    <!--
      Error / loading / empty semuanya memakai bahasa visual yang sama dengan
      kartu artikel (kotak `border-hairline`, ikon monoline, satu CTA) supaya
      halaman tidak pernah jatuh jadi baris teks telanjang saat backend mati.
    -->
    <div
      v-if="error"
      class="relative mt-12 rounded-lg border border-hairline bg-canvas px-6 py-14 text-center md:py-16"
    >
      <RefreshCw class="mx-auto h-6 w-6 text-ink-400" :stroke-width="1.5" />
      <p class="mt-4 font-serif text-xl font-semibold text-ink-950">Artikel belum bisa dimuat</p>
      <p class="mx-auto mt-2 max-w-sm text-sm leading-relaxed text-ink-500">
        Koneksi ke server sedang bermasalah. Anda tetap bisa langsung memesan atau
        melihat daftar produk kami.
      </p>
      <div class="mt-6 flex flex-wrap items-center justify-center gap-3">
        <button
          type="button"
          class="rounded-md border border-hairline bg-canvas px-4 py-2 text-sm font-medium text-ink-900 transition-colors hover:bg-canvas-alt focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
          @click="refresh()"
        >
          Coba lagi
        </button>
        <NuxtLink
          to="/katalog"
          class="rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
        >
          Lihat Katalog
        </NuxtLink>
      </div>
    </div>

    <div v-else-if="pending" class="relative mt-12 grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
      <div
        v-for="i in 3"
        :key="i"
        class="h-72 animate-pulse rounded-lg border border-hairline bg-canvas-alt"
      />
    </div>

    <div
      v-else-if="items.length === 0"
      class="relative mt-12 rounded-lg border border-hairline bg-canvas px-6 py-14 text-center md:py-16"
    >
      <component
        :is="searchQuery ? SearchX : Newspaper"
        class="mx-auto h-6 w-6 text-ink-400"
        :stroke-width="1.5"
      />
      <p class="mt-4 font-serif text-xl font-semibold text-ink-950">
        {{ searchQuery ? 'Tidak ada artikel yang cocok' : 'Artikel sedang disiapkan' }}
      </p>
      <p class="mx-auto mt-2 max-w-sm text-sm leading-relaxed text-ink-500">
        {{
          searchQuery
            ? `Tidak ada hasil untuk "${searchQuery}". Coba kata kunci lain.`
            : 'Panduan dan tips seputar cetak banner akan terbit di halaman ini. Sementara itu, silakan lihat produk yang kami layani.'
        }}
      </p>
      <NuxtLink
        v-if="!searchQuery"
        to="/katalog"
        class="mt-6 inline-flex items-center justify-center rounded-md bg-brand-500 px-5 py-2.5 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
      >
        Lihat Katalog
      </NuxtLink>
    </div>

    <section v-else class="relative mt-12">
      <ul class="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
        <li v-for="a in items" :key="a.id" class="group">
          <NuxtLink :to="`/artikel/${a.slug}`" class="block rounded-lg border border-hairline bg-canvas overflow-hidden transition-colors hover:border-ink-300">
            <div class="aspect-[16/10] bg-canvas-alt overflow-hidden">
              <img
                v-if="coverURL(a)"
                :src="coverURL(a) as string"
                :alt="a.title"
                width="640"
                height="400"
                loading="lazy"
                class="w-full h-full object-cover transition-transform duration-500 group-hover:scale-[1.02]"
              >
              <!--
                Cadangan saat artikel belum punya cover: raster halftone + huruf
                awal judul. Bukan ikon gambar generik yang sama di semua kartu —
                itu justru menegaskan kesan "belum jadi".
              -->
              <div v-else class="relative h-full w-full bg-canvas-alt" aria-hidden="true">
                <div class="absolute inset-0">
                  <ArtOrnament variant="halftone" :opacity="0.8" />
                </div>
                <span
                  class="absolute inset-0 flex select-none items-center justify-center font-serif text-6xl font-semibold text-ink-200"
                >
                  {{ coverInitial(a.title) }}
                </span>
                <span
                  class="absolute bottom-3 left-4 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-400"
                >
                  Artikel
                </span>
              </div>
            </div>
            <div class="p-5">
              <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
                {{ fmtDate(a.published_at) }}
              </p>
              <h2 class="mt-2 font-serif text-xl font-semibold tracking-tight text-ink-950 group-hover:text-brand-500 transition-colors leading-tight">
                {{ a.title }}
              </h2>
              <p v-if="a.excerpt" class="mt-2 text-sm text-ink-500 leading-relaxed line-clamp-3">
                {{ a.excerpt }}
              </p>
            </div>
          </NuxtLink>
        </li>
      </ul>

      <!-- Pagination -->
      <nav
        v-if="totalPages > 1"
        class="mt-12 flex items-center justify-between border-t border-hairline pt-6 text-sm text-ink-500"
        aria-label="Pagination"
      >
        <p>Halaman {{ currentPage }} / {{ totalPages }}</p>
        <div class="flex items-center gap-2">
          <NuxtLink
            v-if="currentPage > 1"
            :to="pageURL(currentPage - 1)"
            class="rounded-md border border-hairline px-3 py-1.5 text-ink-700 hover:border-ink-300 hover:text-ink-950 transition-colors"
          >
            ‹ Sebelumnya
          </NuxtLink>
          <NuxtLink
            v-if="currentPage < totalPages"
            :to="pageURL(currentPage + 1)"
            class="rounded-md bg-ink-950 px-3 py-1.5 font-medium text-canvas hover:bg-ink-900 transition-colors"
          >
            Selanjutnya ›
          </NuxtLink>
        </div>
      </nav>
    </section>
  </div>
</template>
