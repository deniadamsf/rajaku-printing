<script setup lang="ts">
/**
 * /artikel/[slug] — detail artikel SSR.
 *
 * SEO §15:
 *   - useSeoMeta dinamis (title, description, og:*, twitter:*, canonical)
 *   - useHead JSON-LD Article + LocalBusiness (schema.org)
 * Rendering:
 *   - Markdown → HTML server-side via `marked` di useAsyncData (bundle
 *     ke server chunk saja saat SSR; client hydration hanya butuh HTML string).
 *   - Hasil `marked` WAJIB lewat DOMPurify sebelum masuk `v-html`. Markdown
 *     mengizinkan HTML mentah, dan `marked` tidak menyanitasi apa pun sejak
 *     opsi `sanitize`-nya dihapus. Tanpa ini, pemegang role "Admin Artikel"
 *     (§10 — bukan super admin) bisa menanam <script> di badan artikel yang
 *     lalu dieksekusi di browser SETIAP pengunjung halaman publik, termasuk
 *     super admin — jalur naik hak akses, bukan sekadar defacement.
 *     DOMPurify juga menutup vektor yang tidak tertutup oleh sekadar
 *     melarang HTML mentah, mis. tautan `javascript:` dari sintaks Markdown
 *     biasa `[teks](javascript:...)`.
 * Design:
 *   - Fraunces headline, Inter body dengan leading-relaxed, max-w-2xl untuk
 *     text-heavy, generous spacing.
 */
import { marked } from 'marked'
import DOMPurify from 'isomorphic-dompurify'
import type { Article } from '~/types/cms'
import { ApiError } from '~/composables/useApi'

definePageMeta({ layout: 'default' })

const route = useRoute()
const cms = useCms()
const config = useRuntimeConfig()

const slug = computed(() => String(route.params.slug))

interface DetailPayload {
  article: Article
  contentHTML: string
}

const { data, error } = await useAsyncData<DetailPayload>(
  () => `article-${slug.value}`,
  async () => {
    const article = await cms.getBySlug(slug.value)
    // marked.parse selalu string kalau input sync → cast safe.
    const contentHTML = DOMPurify.sanitize(
      marked.parse(article.content_md, { async: false }) as string,
    )
    return { article, contentHTML }
  },
  { watch: [slug] },
)

// 404 kalau backend return not-found.
if (error.value) {
  const status = error.value instanceof ApiError && error.value.status === 404 ? 404 : 500
  throw createError({
    statusCode: status,
    statusMessage: status === 404 ? 'Artikel tidak ditemukan' : 'Gagal memuat artikel',
    fatal: true,
  })
}

const article = computed(() => data.value?.article ?? null)
const contentHTML = computed(() => data.value?.contentHTML ?? '')

/**
 * Estimasi waktu baca dari jumlah kata sumber Markdown (~200 kata/menit).
 * Ini hitungan nyata dari isi artikel, bukan angka hiasan — dibulatkan ke atas
 * dan minimal 1 menit supaya artikel pendek tidak tertulis "0 menit".
 */
const readingMinutes = computed(() => {
  const words = (article.value?.content_md || '').trim().split(/\s+/).filter(Boolean).length
  return Math.max(1, Math.ceil(words / 200))
})

const canonical = computed(() =>
  article.value ? `${config.public.appBaseUrl}/artikel/${article.value.slug}` : '',
)

const coverURL = computed(() =>
  article.value?.cover_image_id ? cms.imageUrl(article.value.cover_image_id) : null,
)

// Tanpa suffix brand — `app.vue` titleTemplate sudah menambahkan
// " — Rajaku Printing"; menambahkannya lagi di sini membuat judul tab berbunyi
// "Judul — Rajaku Printing — Rajaku Printing".
const metaTitle = computed(() => article.value?.meta_title || article.value?.title || '')
const metaDesc = computed(
  () => article.value?.meta_description || article.value?.excerpt || '',
)

useSeoMeta({
  title: () => metaTitle.value,
  description: () => metaDesc.value || undefined,
  ogTitle: () => article.value?.meta_title || article.value?.title || '',
  ogDescription: () => metaDesc.value || undefined,
  ogType: 'article',
  ogUrl: () => canonical.value,
  ogImage: () => coverURL.value || undefined,
  twitterCard: () => (coverURL.value ? 'summary_large_image' : 'summary'),
  articlePublishedTime: () => article.value?.published_at || undefined,
  articleModifiedTime: () => article.value?.updated_at || undefined,
})

// JSON-LD — Article + LocalBusiness (§15). Ditaruh di <head> via useHead
// script[type=application/ld+json]. Google/Bing/DuckDuckGo semua baca ini.
const jsonLd = computed(() => {
  if (!article.value) return []
  const base = config.public.appBaseUrl
  const articleSchema = {
    '@context': 'https://schema.org',
    '@type': 'Article',
    headline: article.value.title,
    description: metaDesc.value || undefined,
    image: coverURL.value ? [coverURL.value] : undefined,
    datePublished: article.value.published_at || undefined,
    dateModified: article.value.updated_at,
    mainEntityOfPage: { '@type': 'WebPage', '@id': canonical.value },
    publisher: {
      '@type': 'Organization',
      name: 'Rajaku Printing',
      url: base,
    },
  }
  const localBusinessSchema = {
    '@context': 'https://schema.org',
    '@type': 'LocalBusiness',
    name: 'Rajaku Printing',
    url: base,
    address: {
      '@type': 'PostalAddress',
      addressLocality: 'Trenggalek',
      addressRegion: 'Jawa Timur',
      addressCountry: 'ID',
    },
    areaServed: 'Trenggalek dan sekitarnya',
  }
  return [articleSchema, localBusinessSchema]
})

useHead({
  link: () => (canonical.value ? [{ rel: 'canonical', href: canonical.value }] : []),
  script: () =>
    jsonLd.value.map((data) => ({
      type: 'application/ld+json',
      innerHTML: JSON.stringify(data),
    })),
})

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
  <article v-if="article" class="mx-auto max-w-3xl px-4 py-16 md:py-24">
    <!-- Breadcrumb -->
    <nav class="text-xs text-ink-500 flex items-center gap-2 mb-8" aria-label="Breadcrumb">
      <NuxtLink to="/" class="hover:text-ink-900 transition-colors">Beranda</NuxtLink>
      <span class="text-ink-300">/</span>
      <NuxtLink to="/artikel" class="hover:text-ink-900 transition-colors">Artikel</NuxtLink>
      <span class="text-ink-300">/</span>
      <span class="text-ink-700 truncate">{{ article.title }}</span>
    </nav>

    <!-- Meta line -->
    <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
      {{ fmtDate(article.published_at) }}
      <span class="mx-1.5 text-ink-300">&middot;</span>
      {{ readingMinutes }} menit baca
    </p>

    <!-- Headline -->
    <h1 class="mt-3 font-serif text-4xl md:text-5xl font-semibold tracking-tight text-ink-950 leading-[1.1]">
      {{ article.title }}
    </h1>

    <p v-if="article.excerpt" class="mt-5 text-lg text-ink-700 leading-relaxed">
      {{ article.excerpt }}
    </p>

    <!-- Cover -->
    <figure v-if="coverURL" class="mt-10 rounded-lg overflow-hidden border border-hairline">
      <img
        :src="coverURL"
        :alt="article.title"
        width="1280"
        height="720"
        class="w-full aspect-[16/9] object-cover"
        loading="eager"
      >
    </figure>

    <!--
      Artikel tanpa cover tetap dapat pemisah visual antara kepala dan badan
      teks — pita ornamen tipis, bukan foto palsu. Bahasa visualnya sama dengan
      kartu tanpa cover di /artikel supaya terbaca sebagai keputusan desain.
    -->
    <div
      v-else
      class="relative mt-10 aspect-[21/9] overflow-hidden rounded-lg border border-hairline bg-canvas-alt"
      aria-hidden="true"
    >
      <div class="absolute inset-0">
        <ArtOrnament variant="halftone" :opacity="0.8" />
      </div>
      <span
        class="absolute inset-0 flex select-none items-center justify-center font-serif text-7xl font-semibold text-ink-200"
      >
        {{ article.title.trim().charAt(0).toUpperCase() }}
      </span>
    </div>

    <!-- Body -->
    <!-- contentHTML sudah disanitasi DOMPurify di script setup di atas.
         Lihat catatan XSS di header file sebelum mengubah baris ini. -->
    <!-- eslint-disable-next-line vue/no-v-html -->
    <div class="mt-10 article-body" v-html="contentHTML" />

    <!-- Footer nav -->
    <hr class="mt-16 border-hairline">
    <div class="mt-6 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 text-sm">
      <NuxtLink
        to="/artikel"
        class="inline-flex items-center gap-1 text-ink-700 hover:text-brand-500 transition-colors"
      >
        ‹ Semua artikel
      </NuxtLink>
      <NuxtLink
        to="/order"
        class="inline-flex items-center rounded-md bg-brand-500 text-canvas px-4 py-2 font-semibold hover:bg-brand-600 transition-colors"
      >
        Order Banner Sekarang
      </NuxtLink>
    </div>
  </article>
</template>

<style>
/* Article body typography — di-scope ke .article-body supaya tidak polute
   komponen lain. Bukan Tailwind class karena innerHTML tidak di-scan JIT. */
.article-body {
  color: theme('colors.ink.900');
  font-size: 1rem;
  line-height: 1.75;
}
.article-body > * + * {
  margin-top: 1.25em;
}
.article-body h2 {
  font-family: theme('fontFamily.serif');
  font-size: 1.75rem;
  font-weight: 600;
  letter-spacing: -0.015em;
  color: theme('colors.ink.950');
  margin-top: 2em;
  line-height: 1.25;
}
.article-body h3 {
  font-size: 1.25rem;
  font-weight: 600;
  color: theme('colors.ink.950');
  margin-top: 1.75em;
  line-height: 1.3;
}
.article-body h4 {
  font-size: 1rem;
  font-weight: 600;
  color: theme('colors.ink.950');
  margin-top: 1.5em;
}
.article-body p {
  color: theme('colors.ink.800');
}
.article-body ul,
.article-body ol {
  padding-left: 1.4em;
}
.article-body ul { list-style: disc; }
.article-body ol { list-style: decimal; }
.article-body li + li { margin-top: 0.4em; }
.article-body a {
  color: theme('colors.brand.500');
  text-decoration: underline;
  text-decoration-color: theme('colors.brand.200');
  text-underline-offset: 3px;
  transition: color 150ms ease;
}
.article-body a:hover {
  color: theme('colors.brand.700');
}
.article-body strong { color: theme('colors.ink.950'); font-weight: 600; }
.article-body em { font-style: italic; }
.article-body code {
  font-family: theme('fontFamily.mono');
  font-size: 0.875em;
  background: theme('colors.canvas.alt');
  color: theme('colors.ink.700');
  padding: 0.1em 0.35em;
  border-radius: 4px;
}
.article-body pre {
  font-family: theme('fontFamily.mono');
  background: theme('colors.ink.950');
  color: theme('colors.canvas.DEFAULT');
  padding: 1em 1.25em;
  border-radius: 8px;
  overflow-x: auto;
  font-size: 0.85em;
  line-height: 1.6;
}
.article-body pre code {
  background: transparent;
  color: inherit;
  padding: 0;
}
.article-body blockquote {
  border-left: 3px solid theme('colors.gold.500');
  padding-left: 1em;
  color: theme('colors.ink.700');
  font-style: italic;
}
.article-body img {
  border-radius: 8px;
  border: 1px solid theme('colors.hairline');
  max-width: 100%;
  height: auto;
}
.article-body hr {
  border: 0;
  border-top: 1px solid theme('colors.hairline');
  margin: 2.5em 0;
}
.article-body table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.9em;
}
.article-body th,
.article-body td {
  border-bottom: 1px solid theme('colors.hairline');
  padding: 0.5em 0.75em;
  text-align: left;
}
.article-body th {
  color: theme('colors.ink.500');
  font-weight: 500;
  text-transform: uppercase;
  font-size: 0.75em;
  letter-spacing: 0.05em;
}
</style>
