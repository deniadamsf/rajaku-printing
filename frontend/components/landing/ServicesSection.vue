<script setup lang="ts">
/**
 * ServicesSection — daftar produk dari modul catalog (§9). Fetch via
 * useAsyncData supaya ter-SSR (penting untuk SEO — konten tersedia tanpa JS).
 *
 * Visual: tiap kartu punya area gambar 4:3 di atas — foto asli produk
 * (`product.image_url`, kalau admin sudah unggah) atau, selama belum ada foto,
 * artwork vektor per jenis produk (`artworkVariantFor()` → `<ArtProduct>`,
 * lihat `utils/artwork.ts`). Ini menggantikan ikon generik yang sama di semua
 * kartu (penyebab utama landing terasa "template kosong").
 *
 * Anti-kosong (backend Postgres di lingkungan pemilik sering mati): kalau
 * fetch GAGAL atau hasilnya kosong, section ini TIDAK menampilkan kotak error
 * sebagai isi utama — ia merender grid statis 6 jenis layanan yang memang kami
 * layani, memakai artwork yang sama, tanpa harga (harga hanya boleh dari API).
 * Halaman tetap terasa penuh & disengaja walau backend down.
 *
 * `CatalogProduct` (list endpoint) belum tentu menyertakan rentang harga — hanya
 * `CatalogProductDetail` (per-slug) punya `pricings`. Supaya tidak menampilkan
 * data yang direkayasa, section ini menampilkan tipe kalkulasi harga (per m² /
 * paket) sebagai badge, bukan angka harga.
 *
 * Motion: reveal stagger disiplin via `useRevealVariants()` (`once: true`), kartu
 * hover lift halus (translateY -2px + border/shadow naik tipis, ditaruh di elemen
 * anak — bukan di `<motion.li>` itu sendiri, supaya inline transform motion-v
 * tidak bentrok dengan class hover Tailwind).
 */
import { ArrowRight, RotateCcw } from '@lucide/vue'
import { motion } from 'motion-v'
import { CORE_SERVICES } from '~/utils/catalog-fallback'
import type { CatalogProduct } from '~/types/catalog'

const catalog = useCatalog()
const { container, item } = useRevealVariants()

const { data, pending, error, refresh } = await useAsyncData('landing-catalog-products', () =>
  catalog.listProducts(),
)

const products = computed<CatalogProduct[]>(() => data.value?.products ?? [])

// Grid statis anti-kosong — daftar layanan inti hidup di `utils/artwork.ts`
// supaya landing dan /katalog tidak pernah menampilkan daftar yang berbeda.
const FALLBACK_SERVICES = CORE_SERVICES

function pricingLabel(p: CatalogProduct): string {
  return p.pricing_type === 'paket' ? 'Harga paket' : 'Harga per m²'
}
</script>

<template>
  <section id="layanan" class="mx-auto max-w-6xl px-4 py-16 md:py-24">
    <div class="max-w-2xl">
      <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Layanan</p>
      <h2 class="mt-3 text-2xl md:text-3xl font-serif font-semibold tracking-tight text-ink-950">
        Produk cetak yang kami layani
      </h2>
      <p class="mt-3 text-sm leading-relaxed text-ink-500">
        Dari banner outdoor tahan cuaca sampai kebutuhan indoor event — ukuran dan
        bahan disesuaikan kebutuhan Anda.
      </p>
      <NuxtLink
        to="/katalog"
        class="mt-4 inline-flex items-center gap-1.5 text-sm font-medium text-brand-500 transition-colors hover:text-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas rounded-sm"
      >
        Lihat katalog lengkap
        <ArrowRight class="h-3.5 w-3.5" :stroke-width="1.75" />
      </NuxtLink>
    </div>

    <!-- Loading skeleton -->
    <div v-if="pending" class="mt-10 flex snap-x snap-mandatory gap-6 overflow-x-auto pb-2 md:grid md:gap-6 md:overflow-visible md:pb-0 md:grid-cols-2 lg:grid-cols-3">
      <div
        v-for="i in 3"
        :key="i"
        class="h-72 animate-pulse rounded-lg border border-hairline bg-canvas-alt"
      />
    </div>

    <!-- Data katalog nyata -->
    <template v-else-if="products.length > 0">
    <motion.ul
      class="mt-10 flex snap-x snap-mandatory gap-6 overflow-x-auto pb-2 md:grid md:gap-6 md:overflow-visible md:pb-0 md:grid-cols-2 lg:grid-cols-3"
      :variants="container"
      initial="hidden"
      while-in-view="show"
      :in-view-options="{ once: true, margin: '-100px' }"
    >
      <motion.li v-for="p in products" :key="p.id" :variants="item" class="w-[80%] shrink-0 snap-center md:w-auto md:shrink">
        <NuxtLink
          :to="`/katalog#${p.slug}`"
          class="group flex h-full flex-col overflow-hidden rounded-lg border border-hairline bg-canvas transition-[border-color,box-shadow,transform] duration-200 ease-out hover:-translate-y-0.5 hover:border-ink-300 hover:shadow-sm"
        >
          <div class="aspect-[4/3] w-full overflow-hidden bg-canvas-alt">
            <img
              v-if="p.image_url"
              :src="p.image_url"
              :alt="p.name"
              width="400"
              height="300"
              loading="lazy"
              class="h-full w-full object-cover transition-transform duration-300 ease-out group-hover:scale-[1.03]"
            >
            <ArtProduct
              v-else
              :variant="artworkVariantFor(p)"
              decorative
              class="transition-transform duration-300 ease-out group-hover:scale-[1.03]"
            />
          </div>
          <div class="flex flex-1 flex-col p-6 md:p-8">
            <h3 class="text-sm font-sans font-semibold text-ink-950">{{ p.name }}</h3>
            <p v-if="p.description" class="mt-2 text-sm leading-relaxed text-ink-500 line-clamp-2">
              {{ p.description }}
            </p>
            <span
              class="mt-4 inline-flex w-fit items-center rounded-full px-2 py-0.5 text-xs font-medium text-gold-700 bg-gold-50 ring-1 ring-inset ring-gold-500/30"
            >
              {{ pricingLabel(p) }}
            </span>
          </div>
        </NuxtLink>
      </motion.li>
    </motion.ul>
    <p class="mt-3 text-xs text-ink-500 md:hidden">Geser ke samping untuk melihat produk lainnya.</p>
    </template>

    <!-- Anti-kosong: katalog gagal dimuat / belum di-seed — grid statis tetap penuh -->
    <template v-else>
      <motion.ul
        class="mt-10 grid gap-6 sm:grid-cols-2 lg:grid-cols-3"
        :variants="container"
        initial="hidden"
        while-in-view="show"
        :in-view-options="{ once: true, margin: '-100px' }"
      >
        <motion.li v-for="s in FALLBACK_SERVICES" :key="s.name" :variants="item" class="w-[80%] shrink-0 snap-center md:w-auto md:shrink">
          <NuxtLink
            to="/katalog"
            class="group flex h-full flex-col overflow-hidden rounded-lg border border-hairline bg-canvas transition-[border-color,box-shadow,transform] duration-200 ease-out hover:-translate-y-0.5 hover:border-ink-300 hover:shadow-sm"
          >
            <div class="aspect-[4/3] w-full overflow-hidden bg-canvas-alt">
              <ArtProduct
                :variant="s.variant"
                decorative
                class="transition-transform duration-300 ease-out group-hover:scale-[1.03]"
              />
            </div>
            <div class="flex flex-1 flex-col p-6 md:p-8">
              <h3 class="text-sm font-sans font-semibold text-ink-950">{{ s.name }}</h3>
              <p class="mt-2 text-sm leading-relaxed text-ink-500">{{ s.desc }}</p>
            </div>
          </NuxtLink>
        </motion.li>
      </motion.ul>
      <p class="mt-3 text-xs text-ink-500 md:hidden">Geser ke samping untuk melihat produk lainnya.</p>

      <div class="mt-6 flex flex-wrap items-center gap-x-4 gap-y-2 text-sm text-ink-500">
        <p>
          Daftar harga lengkap ada di halaman
          <NuxtLink
            to="/katalog"
            class="font-medium text-brand-500 underline underline-offset-2 hover:text-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas rounded-sm"
          >
            katalog
          </NuxtLink>.
        </p>
        <button
          v-if="error"
          type="button"
          class="inline-flex items-center gap-1.5 text-xs font-medium text-ink-500 transition-colors hover:text-ink-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas rounded-sm"
          @click="refresh()"
        >
          <RotateCcw class="h-3 w-3" :stroke-width="1.75" />
          Coba muat ulang
        </button>
      </div>
    </template>
  </section>
</template>
