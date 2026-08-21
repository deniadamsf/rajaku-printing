<script setup lang="ts">
/**
 * /katalog — Katalog produk publik (SSR untuk SEO, §15).
 *
 * - Daftar produk & bahan di-fetch via useAsyncData supaya ter-render di HTML
 *   awal (crawler dapat konten tanpa perlu eksekusi JS).
 * - Kalkulator estimasi harga memakai `catalog.quote()` — pola form sama
 *   dengan `/order` (pilih produk → detail pricing → pilih bahan → hitung),
 *   tapi berdiri sendiri di sini sebagai alat bantu sebelum order.
 * - `CatalogProduct` (list endpoint) TIDAK menyertakan harga — hanya badge
 *   tipe kalkulasi yang ditampilkan di grid produk, sesuai catatan di brief.
 *
 * Design: patuh CLAUDE.md §26 (Fraunces + Inter + Lucide, brand/gold/ink).
 *
 * Motion: reveal stagger disiplin di grid produk (`useRevealVariants()`,
 * `once: true`, sama pola dengan `LandingServicesSection`) — momen utama
 * halaman ini. Grid bahan & kartu kalkulator cuma fade tunggal tanpa stagger,
 * supaya halaman tetap tenang (bukan tiap section beranimasi).
 */
import {
  Calculator,
  Layers,
  Loader2,
  RotateCcw,
  Ruler,
  Package as PackageIcon,
  ArrowRight,
} from '@lucide/vue'
import { motion } from 'motion-v'
import { CORE_SERVICES, FALLBACK_MATERIALS } from '~/utils/catalog-fallback'
import type {
  CatalogMaterial,
  CatalogProduct,
  CatalogProductDetail,
  CatalogQuote,
} from '~/types/catalog'
import { ApiError } from '~/composables/useApi'

definePageMeta({ layout: 'default' })

const catalog = useCatalog()
const config = useRuntimeConfig()
const canonical = `${config.public.appBaseUrl.replace(/\/$/, '')}/katalog`

const { container, item } = useRevealVariants()
const prefersReducedMotion = usePrefersReducedMotion()
const fadeTransition = computed(() =>
  prefersReducedMotion.value ? { duration: 0 } : { duration: 0.5, ease: [0.22, 1, 0.36, 1] as const },
)

// -------------------- SSR fetch: produk + bahan --------------------
const {
  data: productsData,
  pending: productsPending,
  error: productsError,
  refresh: refreshProducts,
} = await useAsyncData('katalog-products', () => catalog.listProducts())

// `error` sengaja tidak diambil: kegagalan fetch bahan ditangani dengan
// menampilkan daftar bahan bawaan sistem (lihat `materials` di bawah), bukan
// dengan menampilkan pesan gagal.
const { data: materialsData, pending: materialsPending } = await useAsyncData(
  'katalog-materials',
  () => catalog.listMaterials(),
)

const products = computed<CatalogProduct[]>(() => productsData.value?.products ?? [])

// Bahan: kalau endpoint gagal atau kosong, pakai daftar bahan bawaan sistem
// (`utils/catalog-fallback.ts`) — bukan baris "daftar bahan belum bisa dimuat".
// Daftar itu memang bahan yang kami sediakan, jadi menampilkannya tetap jujur.
const materials = computed<ReadonlyArray<CatalogMaterial>>(() => {
  const fromApi = materialsData.value?.materials ?? []
  return fromApi.length > 0 ? fromApi : FALLBACK_MATERIALS
})

function pricingLabel(p: CatalogProduct): string {
  return p.pricing_type === 'paket' ? 'Harga paket' : 'Harga per m²'
}
function pricingIcon(p: CatalogProduct) {
  return p.pricing_type === 'paket' ? PackageIcon : Ruler
}

// -------------------- Kalkulator estimasi harga --------------------
const calcProductId = ref('')
const calcMaterialId = ref('')
const calcWidth = ref<number>(0)
const calcHeight = ref<number>(0)

const productDetail = ref<CatalogProductDetail | null>(null)
const detailLoading = ref(false)
const detailError = ref<string | null>(null)

watch(calcProductId, async (pid) => {
  productDetail.value = null
  calcMaterialId.value = ''
  calcWidth.value = 0
  calcHeight.value = 0
  quote.value = null
  quoteError.value = null
  detailError.value = null
  if (!pid) return
  const p = products.value.find((x) => x.id === pid)
  if (!p) return
  detailLoading.value = true
  try {
    productDetail.value = await catalog.getProduct(p.slug)
    if (productDetail.value.pricings.length === 1) {
      calcMaterialId.value = productDetail.value.pricings[0].material_id
    }
  } catch (e) {
    detailError.value = e instanceof ApiError ? e.message : 'Gagal memuat detail produk'
  } finally {
    detailLoading.value = false
  }
})

// Untuk tipe `paket`, ukuran ikut preset material terpilih (bukan input bebas)
// — sama seperti pola di /order.
watch(calcMaterialId, (matId) => {
  const p = productDetail.value
  if (!p || !matId) return
  if (p.pricing_type === 'paket') {
    const row = p.pricings.find((r) => r.material_id === matId)
    if (row?.width_cm) calcWidth.value = row.width_cm
    if (row?.height_cm) calcHeight.value = row.height_cm
  }
})

const isPaket = computed(() => productDetail.value?.pricing_type === 'paket')
const isPerM2 = computed(() => productDetail.value?.pricing_type === 'per_m2')

const materialOptions = computed(() => {
  const p = productDetail.value
  if (!p) return []
  return p.pricings.map((r) => ({
    id: r.material_id,
    label:
      p.pricing_type === 'paket'
        ? `${r.material_name}${r.package_label ? ' · ' + r.package_label : ''}${r.price_total != null ? ' · ' + fmtIDR(r.price_total) : ''}`
        : `${r.material_name}${r.price_per_m2 != null ? ' · ' + fmtIDR(r.price_per_m2) + '/m²' : ''}`,
  }))
})

const quote = ref<CatalogQuote | null>(null)
const quoteLoading = ref(false)
const quoteError = ref<string | null>(null)

let quoteTimer: ReturnType<typeof setTimeout> | null = null
function scheduleQuote() {
  if (quoteTimer) clearTimeout(quoteTimer)
  quoteTimer = setTimeout(runQuote, 400)
}

async function runQuote() {
  const p = productDetail.value
  if (!p || !calcMaterialId.value || calcWidth.value <= 0 || calcHeight.value <= 0) {
    quote.value = null
    quoteError.value = null
    return
  }
  quoteLoading.value = true
  quoteError.value = null
  try {
    quote.value = await catalog.quote({
      product_id: p.id,
      material_id: calcMaterialId.value,
      width_cm: calcWidth.value,
      height_cm: calcHeight.value,
    })
  } catch (e) {
    quote.value = null
    quoteError.value = e instanceof ApiError ? e.message : 'Gagal menghitung estimasi harga'
  } finally {
    quoteLoading.value = false
  }
}

watch([calcMaterialId, calcWidth, calcHeight], scheduleQuote)

function fmtIDR(v?: number | null): string {
  if (v == null) return '—'
  return new Intl.NumberFormat('id-ID', {
    style: 'currency',
    currency: 'IDR',
    maximumFractionDigits: 0,
  }).format(v)
}

function selectProductForCalc(p: CatalogProduct) {
  calcProductId.value = p.id
  if (import.meta.client) {
    document.getElementById('kalkulator')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }
}

// -------------------- SEO --------------------
useSeoMeta({
  title: 'Katalog Produk Cetak Banner',
  description:
    'Katalog lengkap produk cetak banner Rajaku Printing — bahan tersedia, tipe kalkulasi harga per m² atau paket, dan kalkulator estimasi harga langsung.',
  ogTitle: 'Katalog Produk — Rajaku Printing',
  ogDescription:
    'Lihat produk cetak banner yang kami layani, bahan yang tersedia, dan hitung estimasi harga sebelum order.',
  ogType: 'website',
  ogUrl: canonical,
  twitterCard: 'summary',
})

useHead({
  link: [{ rel: 'canonical', href: canonical }],
  script: () => {
    if (products.value.length === 0) return []
    return [
      {
        type: 'application/ld+json',
        innerHTML: JSON.stringify({
          '@context': 'https://schema.org',
          '@type': 'ItemList',
          name: 'Katalog Produk Rajaku Printing',
          itemListElement: products.value.map((p, i) => ({
            '@type': 'ListItem',
            position: i + 1,
            item: {
              '@type': 'Product',
              name: p.name,
              description: p.description || undefined,
              category: p.category || undefined,
              url: `${canonical}#produk-${p.slug}`,
            },
          })),
        }),
      },
    ]
  },
})
</script>

<template>
  <main>
    <!-- Header -->
    <section class="mx-auto max-w-6xl px-4 pt-16 pb-10 md:pt-24 md:pb-14">
      <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Katalog</p>
      <h1 class="mt-3 text-2xl md:text-3xl font-serif font-semibold tracking-tight text-ink-950">
        Produk cetak yang kami layani
      </h1>
      <p class="mt-3 max-w-2xl text-sm md:text-base leading-relaxed text-ink-500">
        Pilih produk, cek bahan yang tersedia, lalu hitung estimasi harga langsung sebelum order.
        Ongkir dan biaya jasa desain (kalau ada) dihitung terpisah oleh tim kami.
      </p>
    </section>

    <!-- Products grid -->
    <section class="mx-auto max-w-6xl px-4 pb-16 md:pb-24">
      <div v-if="productsPending" class="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
        <div v-for="i in 6" :key="i" class="h-72 animate-pulse rounded-lg border border-hairline bg-canvas-alt" />
      </div>

      <!--
        Katalog gagal dimuat ATAU belum diisi → tetap tampilkan jenis layanan
        yang memang kami kerjakan (artwork, tanpa harga), bukan kotak error
        setinggi satu layar. Harga & kalkulator memang butuh backend hidup, dan
        itu dijelaskan apa adanya di bawah grid.
      -->
      <div v-else-if="products.length === 0">
        <ul class="flex snap-x snap-mandatory gap-6 overflow-x-auto pb-2 md:grid md:gap-6 md:overflow-visible md:pb-0 md:grid-cols-2 lg:grid-cols-3">
          <li
            v-for="service in CORE_SERVICES"
            :key="service.name"
            class="w-[80%] shrink-0 snap-center md:w-auto md:shrink overflow-hidden rounded-lg border border-hairline bg-canvas"
          >
            <div class="aspect-[4/3] w-full overflow-hidden bg-canvas-alt">
              <ArtProduct :variant="service.variant" decorative />
            </div>
            <div class="p-6 md:p-8">
              <h2 class="text-sm font-sans font-semibold text-ink-950">{{ service.name }}</h2>
              <p class="mt-2 text-sm leading-relaxed text-ink-500">{{ service.desc }}</p>
            </div>
          </li>
        </ul>
        <p class="mt-3 text-xs text-ink-500 md:hidden">Geser ke samping untuk melihat produk lainnya.</p>

        <div class="mt-8 flex flex-wrap items-center gap-x-4 gap-y-2 text-sm text-ink-500">
          <p>
            {{
              productsError
                ? 'Harga dan kalkulator estimasi belum bisa dimuat saat ini.'
                : 'Daftar harga sedang dilengkapi tim kami.'
            }}
            Hubungi kami lewat form order untuk penawaran.
          </p>
          <button
            v-if="productsError"
            type="button"
            class="inline-flex items-center gap-1.5 text-xs font-medium text-ink-500 transition-colors hover:text-ink-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas rounded-sm"
            @click="refreshProducts()"
          >
            <RotateCcw class="h-3.5 w-3.5" :stroke-width="1.75" />
            Coba muat ulang
          </button>
          <NuxtLink
            to="/order"
            class="inline-flex items-center justify-center gap-2 rounded-md bg-brand-500 px-5 py-2.5 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
          >
            Order Banner
          </NuxtLink>
        </div>
      </div>

      <motion.ul
        v-else
        class="flex snap-x snap-mandatory gap-6 overflow-x-auto pb-2 md:grid md:gap-6 md:overflow-visible md:pb-0 md:grid-cols-2 lg:grid-cols-3"
        :variants="container"
        initial="hidden"
        while-in-view="show"
        :in-view-options="{ once: true, margin: '-100px' }"
      >
        <motion.li
          v-for="p in products"
          :id="`produk-${p.slug}`"
          :key="p.id"
          :variants="item"
          class="w-[80%] shrink-0 snap-center md:w-auto md:shrink scroll-mt-24 overflow-hidden rounded-lg border border-hairline bg-canvas transition-colors hover:border-ink-300"
        >
          <!--
            Area visual kartu: foto asli produk kalau admin sudah mengunggahnya
            (`image_url`), selama belum ada pakai artwork vektor per jenis produk.
            Pola & rasio disamakan dengan `LandingServicesSection` supaya kartu
            produk terlihat sama di landing dan di katalog.
          -->
          <div class="aspect-[4/3] w-full overflow-hidden bg-canvas-alt">
            <img
              v-if="p.image_url"
              :src="p.image_url"
              :alt="p.name"
              width="800"
              height="600"
              loading="lazy"
              class="h-full w-full object-cover"
            >
            <ArtProduct v-else :variant="artworkVariantFor(p)" decorative />
          </div>

          <div class="p-6 md:p-8">
            <div class="flex items-center gap-2 text-ink-400">
              <component :is="pricingIcon(p)" class="h-4 w-4" :stroke-width="1.5" />
              <span class="text-[10px] font-medium uppercase tracking-[0.14em]">
                {{ p.category }}
              </span>
            </div>
            <h2 class="mt-3 text-sm font-sans font-semibold text-ink-950">{{ p.name }}</h2>
            <p v-if="p.description" class="mt-2 text-sm leading-relaxed text-ink-500 line-clamp-3">
              {{ p.description }}
            </p>
            <div class="mt-4 flex flex-wrap items-center gap-2">
              <span
                class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium text-gold-700 bg-gold-50 ring-1 ring-inset ring-gold-500/30"
              >
                {{ pricingLabel(p) }}
              </span>
            </div>
            <div class="mt-5 flex items-center gap-4">
              <button
                type="button"
                class="inline-flex items-center gap-1.5 text-sm font-medium text-ink-700 transition-colors hover:text-brand-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas rounded-sm"
                @click="selectProductForCalc(p)"
              >
                <Calculator class="h-4 w-4" :stroke-width="1.5" />
                Hitung estimasi
              </button>
              <NuxtLink
                to="/order"
                class="inline-flex items-center gap-1.5 text-sm font-semibold text-brand-500 transition-colors hover:text-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas rounded-sm"
              >
                Order
                <ArrowRight class="h-3.5 w-3.5" :stroke-width="1.75" />
              </NuxtLink>
            </div>
          </div>
        </motion.li>
      </motion.ul>
    </section>

    <!-- Materials -->
    <section class="bg-canvas-alt border-y border-hairline">
      <div class="mx-auto max-w-6xl px-4 py-16 md:py-24">
        <div class="max-w-2xl">
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Bahan</p>
          <h2 class="mt-3 text-lg md:text-xl font-sans font-semibold text-ink-950">
            Bahan cetak yang tersedia
          </h2>
          <p class="mt-2 text-sm leading-relaxed text-ink-500">
            Ketersediaan bahan bisa berbeda per produk — pilih produk di kalkulator untuk lihat
            harga per bahan.
          </p>
        </div>

        <div v-if="materialsPending" class="mt-8 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <div v-for="i in 4" :key="i" class="h-16 animate-pulse rounded-md border border-hairline bg-canvas" />
        </div>
        <motion.ul
          v-else
          class="mt-8 grid gap-3 sm:grid-cols-2 lg:grid-cols-3"
          :initial="{ opacity: 0, y: 16 }"
          :while-in-view="{ opacity: 1, y: 0 }"
          :in-view-options="{ once: true, margin: '-100px' }"
          :transition="fadeTransition"
        >
          <li
            v-for="m in materials"
            :key="m.id"
            class="flex items-start gap-3 rounded-md border border-hairline bg-canvas p-4"
          >
            <Layers class="mt-0.5 h-4 w-4 shrink-0 text-brand-500" :stroke-width="1.5" />
            <div>
              <p class="text-sm font-semibold text-ink-900">{{ m.name }}</p>
              <p v-if="m.description" class="mt-0.5 text-xs leading-relaxed text-ink-500">
                {{ m.description }}
              </p>
            </div>
          </li>
        </motion.ul>
        <p class="mt-3 text-xs text-ink-500 md:hidden">Geser ke samping untuk melihat produk lainnya.</p>
      </div>
    </section>

    <!--
      Kalkulator hanya berarti kalau katalog benar-benar termuat. Saat kosong,
      seluruh section disembunyikan (bukan diisi kalimat "kalkulator tersedia
      setelah katalog siap") — grid layanan di atas sudah menjelaskan situasinya
      sekali, tidak perlu diulang sebagai bidang kosong kedua.
    -->
    <section
      v-if="products.length > 0"
      id="kalkulator"
      class="mx-auto max-w-4xl px-4 py-16 md:py-24 scroll-mt-16"
    >
      <div class="max-w-2xl">
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Kalkulator</p>
        <h2 class="mt-3 text-lg md:text-xl font-sans font-semibold text-ink-950">
          Estimasi harga sebelum order
        </h2>
        <p class="mt-2 text-sm leading-relaxed text-ink-500">
          Hasil ini estimasi — total final sekaligus ongkir (kalau dikirim) tetap dikonfirmasi tim
          kami sebelum pembayaran.
        </p>
      </div>

      <motion.div
        class="mt-8 rounded-lg border border-hairline bg-canvas p-6 md:p-8"
        :initial="{ opacity: 0, y: 16 }"
        :while-in-view="{ opacity: 1, y: 0 }"
        :in-view-options="{ once: true, margin: '-100px' }"
        :transition="fadeTransition"
      >
        <div class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_260px]">
          <div class="space-y-4">
            <div class="grid gap-4 sm:grid-cols-2">
              <div>
                <label for="calc-product" class="block text-sm font-medium text-ink-900">
                  Produk
                </label>
                <select
                  id="calc-product"
                  v-model="calcProductId"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                >
                  <option value="">Pilih produk</option>
                  <option v-for="p in products" :key="p.id" :value="p.id">{{ p.name }}</option>
                </select>
              </div>
              <div>
                <label for="calc-material" class="block text-sm font-medium text-ink-900">
                  Bahan
                </label>
                <select
                  id="calc-material"
                  v-model="calcMaterialId"
                  :disabled="!productDetail || detailLoading"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:bg-canvas-alt disabled:text-ink-500"
                >
                  <option value="">{{ detailLoading ? 'Memuat…' : 'Pilih bahan' }}</option>
                  <option v-for="m in materialOptions" :key="m.id" :value="m.id">{{ m.label }}</option>
                </select>
                <p v-if="isPaket" class="mt-1 text-xs text-ink-500">
                  Ukuran mengikuti paket yang dipilih.
                </p>
              </div>
            </div>

            <div class="grid gap-4 sm:grid-cols-2">
              <div>
                <label for="calc-width" class="block text-sm font-medium text-ink-900">
                  Lebar (cm)
                </label>
                <input
                  id="calc-width"
                  v-model.number="calcWidth"
                  type="number"
                  min="1"
                  :disabled="isPaket || !productDetail"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:bg-canvas-alt disabled:text-ink-500"
                >
              </div>
              <div>
                <label for="calc-height" class="block text-sm font-medium text-ink-900">
                  Tinggi (cm)
                </label>
                <input
                  id="calc-height"
                  v-model.number="calcHeight"
                  type="number"
                  min="1"
                  :disabled="isPaket || !productDetail"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:bg-canvas-alt disabled:text-ink-500"
                >
              </div>
            </div>

            <p v-if="isPerM2 && productDetail && (productDetail.min_width_cm || productDetail.max_width_cm)" class="text-xs text-ink-500">
              Ukuran custom · {{ productDetail.min_width_cm }}–{{ productDetail.max_width_cm }} cm (lebar)
              × {{ productDetail.min_height_cm }}–{{ productDetail.max_height_cm }} cm (tinggi)
            </p>
            <p v-if="detailError" class="text-xs text-brand-700">{{ detailError }}</p>
          </div>

          <!-- Result -->
          <div class="rounded-md border border-hairline bg-canvas-alt p-5">
            <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Estimasi</p>
            <div v-if="quoteLoading" class="mt-3 flex items-center gap-2 text-sm text-ink-500">
              <Loader2 class="h-4 w-4 animate-spin" :stroke-width="1.75" />
              Menghitung…
            </div>
            <div v-else-if="quoteError" class="mt-3 text-xs text-brand-700">{{ quoteError }}</div>
            <div v-else-if="quote" class="mt-3 space-y-2">
              <p class="font-serif text-2xl font-semibold text-ink-950">{{ fmtIDR(quote.total_price) }}</p>
              <dl class="space-y-1 text-xs text-ink-500">
                <div v-if="quote.area_m2 != null" class="flex justify-between">
                  <dt>Luas</dt>
                  <dd class="font-mono">{{ quote.area_m2.toFixed(2) }} m²</dd>
                </div>
                <div v-if="quote.price_per_m2 != null" class="flex justify-between">
                  <dt>Harga / m²</dt>
                  <dd>{{ fmtIDR(quote.price_per_m2) }}</dd>
                </div>
                <div v-if="quote.package_label" class="flex justify-between">
                  <dt>Paket</dt>
                  <dd>{{ quote.package_label }}</dd>
                </div>
              </dl>
            </div>
            <p v-else class="mt-3 text-sm text-ink-500">
              Pilih produk, bahan, {{ isPaket ? '' : 'dan ukuran ' }}untuk melihat estimasi.
            </p>

            <NuxtLink
              to="/order"
              class="mt-5 inline-flex w-full items-center justify-center gap-2 rounded-md bg-brand-500 px-4 py-2.5 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
            >
              Lanjut Order
            </NuxtLink>
          </div>
        </div>
      </motion.div>
    </section>
  </main>
</template>
