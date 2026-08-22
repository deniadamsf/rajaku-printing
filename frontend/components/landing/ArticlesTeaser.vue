<script setup lang="ts">
/**
 * ArticlesTeaser — 3 artikel terbaru berstatus published dari modul CMS (§14).
 *
 * Anti-kosong DIBALIK dari section lain: kalau tidak ada artikel published
 * atau API gagal, section ini TIDAK dirender sama sekali (bukan grid statis) —
 * brief eksplisit meminta ini karena tidak ada konten faktual pengganti yang
 * bisa ditampilkan untuk "artikel" selain artikel itu sendiri.
 *
 * Cover: `article.cover_image_id` → `useCms().imageUrl(id)` kalau ada, else
 * `<ArtOrnament variant="halftone">` sebagai latar dekoratif di belakang judul.
 */
import { ArrowRight } from '@lucide/vue'
import { motion } from 'motion-v'
import type { Article } from '~/types/cms'

const cms = useCms()
const { container, item } = useRevealVariants()

const { data } = await useAsyncData('landing-articles-teaser', async () => {
  try {
    const res = await cms.listPublic({ page: 1, limit: 3 })
    return res.items
  } catch {
    // Backend mati/gagal → section ini no-render (lihat docblock), bukan
    // menampilkan kotak error.
    return []
  }
})

const articles = computed<Article[]>(() => data.value ?? [])

function formatDate(iso: string | null | undefined): string {
  if (!iso) return ''
  return new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'long', year: 'numeric' }).format(
    new Date(iso),
  )
}
</script>

<template>
  <section v-if="articles.length > 0" id="artikel" class="mx-auto max-w-6xl px-4 py-12 md:py-20">
    <div class="flex flex-wrap items-end justify-between gap-4">
      <div class="max-w-2xl">
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Artikel</p>
        <h2 class="mt-3 text-2xl md:text-3xl font-serif font-semibold tracking-tight text-ink-950">
          Tips & bacaan seputar cetak banner
        </h2>
      </div>
      <NuxtLink
        to="/artikel"
        class="inline-flex items-center gap-1.5 text-sm font-medium text-brand-500 transition-colors hover:text-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas rounded-sm"
      >
        Lihat semua artikel
        <ArrowRight class="h-3.5 w-3.5" :stroke-width="1.75" />
      </NuxtLink>
    </div>

    <motion.ul
      class="mt-8 flex snap-x snap-mandatory gap-6 overflow-x-auto pb-2 md:grid md:gap-6 md:overflow-visible md:pb-0 md:grid-cols-2 lg:grid-cols-3"
      :variants="container"
      initial="hidden"
      while-in-view="show"
      :in-view-options="{ once: true, margin: '-40px' }"
    >
      <motion.li v-for="a in articles" :key="a.id" :variants="item" class="w-[80%] shrink-0 snap-center md:w-auto md:shrink">
        <NuxtLink
          :to="`/artikel/${a.slug}`"
          class="group flex h-full flex-col overflow-hidden rounded-lg border border-hairline bg-canvas transition-[border-color,box-shadow,transform] duration-200 ease-out hover:-translate-y-0.5 hover:border-ink-300 hover:shadow-sm"
        >
          <div class="relative aspect-[4/3] w-full overflow-hidden bg-canvas-alt">
            <img
              v-if="a.cover_image_id"
              :src="cms.imageUrl(a.cover_image_id)"
              :alt="a.title"
              width="400"
              height="300"
              loading="lazy"
              class="h-full w-full object-cover transition-transform duration-300 ease-out group-hover:scale-[1.03]"
            >
            <template v-else>
              <div class="absolute inset-0">
                <ArtOrnament variant="halftone" :opacity="0.6" />
              </div>
              <div class="relative flex h-full items-center justify-center p-6">
                <p class="text-center font-serif text-base font-semibold leading-snug text-ink-800 line-clamp-3">
                  {{ a.title }}
                </p>
              </div>
            </template>
          </div>
          <div class="flex flex-1 flex-col p-6 md:p-8">
            <p v-if="a.published_at" class="text-[10px] font-medium uppercase tracking-[0.1em] text-ink-500">
              {{ formatDate(a.published_at) }}
            </p>
            <h3 class="mt-2 text-sm font-sans font-semibold text-ink-950 line-clamp-2">{{ a.title }}</h3>
            <p v-if="a.excerpt" class="mt-2 text-sm leading-relaxed text-ink-500 line-clamp-2">
              {{ a.excerpt }}
            </p>
            <span
              class="mt-4 inline-flex items-center gap-1.5 text-sm font-medium text-brand-500 transition-colors group-hover:text-brand-600"
            >
              Baca artikel
              <ArrowRight class="h-3.5 w-3.5" :stroke-width="1.75" />
            </span>
          </div>
        </NuxtLink>
      </motion.li>
    </motion.ul>
    <p class="mt-3 text-xs text-ink-500 md:hidden">Geser ke samping untuk melihat artikel lainnya.</p>
  </section>
</template>
