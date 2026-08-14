<script setup lang="ts">
/**
 * ServicesSection — daftar produk dari modul catalog (§9). Fetch via
 * useAsyncData supaya ter-SSR (penting untuk SEO — konten tersedia tanpa JS).
 *
 * `CatalogProduct` (list endpoint) belum menyertakan rentang harga — hanya
 * `CatalogProductDetail` (per-slug) punya `pricings`. Supaya tidak menampilkan
 * data yang direkayasa, section ini menampilkan tipe kalkulasi harga (per m² /
 * paket) sebagai badge, bukan angka harga yang tidak tersedia di tipe list.
 *
 * Motion: reveal stagger disiplin via `useRevealVariants()` (`once: true`), kartu
 * hover lift halus (translateY -2px + border/shadow naik tipis) — bukan scale.
 */
import { ArrowRight, Layers, PackageSearch } from '@lucide/vue'
import { motion } from 'motion-v'
import type { CatalogProduct } from '~/types/catalog'

const catalog = useCatalog()
const { container, item } = useRevealVariants()

const { data, pending, error, refresh } = await useAsyncData('landing-catalog-products', () =>
  catalog.listProducts(),
)

const products = computed<CatalogProduct[]>(() => data.value?.products ?? [])

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

    <!-- Error -->
    <div
      v-if="error"
      class="mt-10 rounded-lg border border-hairline bg-canvas-alt p-6 text-sm text-ink-500"
    >
      Katalog belum bisa dimuat saat ini.
      <button
        type="button"
        class="ml-1 font-medium text-brand-500 underline underline-offset-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas rounded-sm"
        @click="refresh()"
      >
        Coba lagi
      </button>
    </div>

    <!-- Loading skeleton -->
    <div v-else-if="pending" class="mt-10 grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
      <div
        v-for="i in 3"
        :key="i"
        class="h-44 animate-pulse rounded-lg border border-hairline bg-canvas-alt"
      />
    </div>

    <!-- Empty state (backend belum di-seed) -->
    <div
      v-else-if="products.length === 0"
      class="mt-10 rounded-lg border border-hairline bg-canvas p-8 md:p-10 text-center"
    >
      <PackageSearch class="mx-auto h-6 w-6 text-ink-400" :stroke-width="1.5" />
      <p class="mt-3 text-sm font-semibold text-ink-900">Katalog sedang disiapkan</p>
      <p class="mt-1 text-sm text-ink-500">
        Produk akan tampil di sini setelah tim kami melengkapi katalog. Hubungi kami
        langsung untuk kebutuhan cetak Anda.
      </p>
      <NuxtLink
        to="/order"
        class="mt-5 inline-flex items-center justify-center gap-2 rounded-md bg-brand-500 px-5 py-2.5 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
      >
        Order Banner
      </NuxtLink>
    </div>

    <!-- Products -->
    <motion.ul
      v-else
      class="mt-10 grid gap-6 sm:grid-cols-2 lg:grid-cols-3"
      :variants="container"
      initial="hidden"
      while-in-view="show"
      :in-view-options="{ once: true, margin: '-100px' }"
    >
      <!--
        Motion-v (`variants`) mengontrol entrance di <motion.li> (opacity/y) —
        efek hover (transform translateY + shadow) sengaja ditaruh di <div> anak,
        BUKAN elemen yang sama, supaya inline style transform dari motion-v tidak
        bentrok/menang atas class Tailwind hover:-translate-y (brief motion §4).
      -->
      <motion.li v-for="p in products" :key="p.id" :variants="item">
        <div
          class="h-full rounded-lg border border-hairline bg-canvas p-6 transition-[border-color,box-shadow,transform] duration-200 ease-out hover:-translate-y-0.5 hover:border-ink-300 hover:shadow-sm md:p-8"
        >
          <Layers class="h-6 w-6 text-brand-500" :stroke-width="1.5" />
          <h3 class="mt-4 text-sm font-sans font-semibold text-ink-950">{{ p.name }}</h3>
          <p v-if="p.description" class="mt-2 text-sm leading-relaxed text-ink-500 line-clamp-2">
            {{ p.description }}
          </p>
          <span
            class="mt-4 inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium text-gold-700 bg-gold-50 ring-1 ring-inset ring-gold-500/30"
          >
            {{ pricingLabel(p) }}
          </span>
        </div>
      </motion.li>
    </motion.ul>
  </section>
</template>
