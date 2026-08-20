<script setup lang="ts">
/**
 * /artikel — list artikel published (SSR untuk SEO §15).
 * Compliant CLAUDE.md §26 — Fraunces display, brand tokens, generous spacing.
 */
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
  title: () =>
    currentPage.value > 1
      ? `Artikel — halaman ${currentPage.value} · Rajaku Printing`
      : 'Artikel & Tips Cetak Banner — Rajaku Printing',
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
  <div class="mx-auto max-w-6xl px-4 py-16 md:py-24">
    <!-- Header -->
    <header class="max-w-2xl">
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
    <div class="mt-8 max-w-md">
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

    <!-- Error / loading / empty / grid -->
    <div v-if="error" class="mt-12 rounded-md border border-brand-200 bg-brand-50 p-4 text-sm text-brand-800">
      Gagal memuat daftar artikel. <button type="button" class="underline font-medium ml-1" @click="refresh()">Coba lagi</button>
    </div>

    <div v-else-if="pending" class="mt-12 text-sm text-ink-500">Memuat…</div>

    <div v-else-if="items.length === 0" class="mt-16 text-center">
      <p class="font-serif text-2xl text-ink-900">Belum ada artikel</p>
      <p class="mt-2 text-sm text-ink-500">
        {{ searchQuery ? `Tidak ada hasil untuk "${searchQuery}".` : 'Konten sedang disiapkan tim kami.' }}
      </p>
    </div>

    <section v-else class="mt-12">
      <ul class="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
        <li v-for="a in items" :key="a.id" class="group">
          <NuxtLink :to="`/artikel/${a.slug}`" class="block rounded-lg border border-hairline bg-canvas overflow-hidden transition-colors hover:border-ink-300">
            <div class="aspect-[16/10] bg-canvas-alt overflow-hidden">
              <img
                v-if="coverURL(a)"
                :src="coverURL(a) as string"
                :alt="a.title"
                loading="lazy"
                class="w-full h-full object-cover transition-transform duration-500 group-hover:scale-[1.02]"
              >
              <div v-else class="w-full h-full flex items-center justify-center text-ink-300">
                <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor" class="w-8 h-8">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M2.25 15.75l5.159-5.159a2.25 2.25 0 013.182 0l5.159 5.159m-1.5-1.5l1.409-1.409a2.25 2.25 0 013.182 0l2.909 2.909m-18 3.75h16.5a1.5 1.5 0 001.5-1.5V6a1.5 1.5 0 00-1.5-1.5H3.75A1.5 1.5 0 002.25 6v12a1.5 1.5 0 001.5 1.5zm10.5-11.25h.008v.008h-.008V8.25zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0z" />
                </svg>
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
