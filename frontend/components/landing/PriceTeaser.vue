<script setup lang="ts">
/**
 * PriceTeaser — widget estimasi harga ringkas di landing (deliverable §3 brief).
 *
 * Versi TEASER dari kalkulator lengkap di `/katalog` — sengaja tidak menduplikasi
 * seluruh UI-nya (tanpa daftar bahan terpisah, tanpa catatan rentang ukuran).
 * Backend (`useCatalog().quote()`) tetap satu-satunya sumber harga — tidak ada
 * kalkulasi harga di frontend sama sekali.
 *
 * State: loading (fetch produk/bahan awal + saat quote), error (fetch gagal /
 * quote gagal), empty (katalog belum di-seed) — semua degrade rapi, tidak pernah
 * pecah kalau backend kosong.
 */
import { ArrowRight, Calculator, Loader2, PackageSearch } from '@lucide/vue'
import type { CatalogProduct, CatalogProductDetail, CatalogQuote } from '~/types/catalog'
import { ApiError } from '~/composables/useApi'

const catalog = useCatalog()

const { data: productsData, pending: productsPending, error: productsError } = await useAsyncData(
  'landing-price-teaser-products',
  () => catalog.listProducts(),
)

const products = computed<CatalogProduct[]>(() => productsData.value?.products ?? [])

const productId = ref('')
const materialId = ref('')
const width = ref<number>(0)
const height = ref<number>(0)

const detail = ref<CatalogProductDetail | null>(null)
const detailLoading = ref(false)
const detailError = ref<string | null>(null)

watch(productId, async (pid) => {
  detail.value = null
  materialId.value = ''
  width.value = 0
  height.value = 0
  quote.value = null
  quoteError.value = null
  detailError.value = null
  if (!pid) return
  const p = products.value.find((x) => x.id === pid)
  if (!p) return
  detailLoading.value = true
  try {
    detail.value = await catalog.getProduct(p.slug)
    if (detail.value.pricings.length === 1) {
      materialId.value = detail.value.pricings[0].material_id
    }
  } catch (e) {
    detailError.value = e instanceof ApiError ? e.message : 'Gagal memuat detail produk'
  } finally {
    detailLoading.value = false
  }
})

// Tipe `paket` — ukuran mengikuti preset material, bukan input bebas (sama pola /katalog).
watch(materialId, (matId) => {
  const d = detail.value
  if (!d || !matId) return
  if (d.pricing_type === 'paket') {
    const row = d.pricings.find((r) => r.material_id === matId)
    if (row?.width_cm) width.value = row.width_cm
    if (row?.height_cm) height.value = row.height_cm
  }
})

const isPaket = computed(() => detail.value?.pricing_type === 'paket')

const materialOptions = computed(() => {
  const d = detail.value
  if (!d) return []
  return d.pricings.map((r) => ({
    id: r.material_id,
    label:
      d.pricing_type === 'paket'
        ? `${r.material_name}${r.package_label ? ' · ' + r.package_label : ''}`
        : r.material_name,
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
  const d = detail.value
  if (!d || !materialId.value || width.value <= 0 || height.value <= 0) {
    quote.value = null
    quoteError.value = null
    return
  }
  quoteLoading.value = true
  quoteError.value = null
  try {
    quote.value = await catalog.quote({
      product_id: d.id,
      material_id: materialId.value,
      width_cm: width.value,
      height_cm: height.value,
    })
  } catch (e) {
    quote.value = null
    quoteError.value = e instanceof ApiError ? e.message : 'Gagal menghitung estimasi harga'
  } finally {
    quoteLoading.value = false
  }
}

watch([materialId, width, height], scheduleQuote)

function fmtIDR(v?: number | null): string {
  if (v == null) return '—'
  return new Intl.NumberFormat('id-ID', {
    style: 'currency',
    currency: 'IDR',
    maximumFractionDigits: 0,
  }).format(v)
}
</script>

<template>
  <section id="estimasi-harga" class="bg-canvas-alt">
    <div class="mx-auto max-w-6xl px-4 py-16 md:py-24">
      <div class="max-w-2xl">
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Estimasi Harga</p>
        <h2 class="mt-3 text-2xl md:text-3xl font-serif font-semibold tracking-tight text-ink-950">
          Cek kira-kira harganya dulu
        </h2>
        <p class="mt-3 text-sm leading-relaxed text-ink-500">
          Pilih produk, bahan, dan ukuran — estimasi langsung dihitung dari katalog kami. Total
          final (termasuk ongkir bila dikirim) tetap dikonfirmasi tim sebelum pembayaran.
        </p>
      </div>

      <!-- Error fetch produk -->
      <div
        v-if="productsError"
        class="mt-8 rounded-lg border border-hairline bg-canvas p-6 text-sm text-ink-500 md:p-8"
      >
        Kalkulator belum bisa dimuat saat ini. Silakan cek langsung di
        <NuxtLink to="/katalog" class="font-medium text-brand-500 underline underline-offset-2">
          halaman katalog
        </NuxtLink>.
      </div>

      <!-- Loading skeleton -->
      <div
        v-else-if="productsPending"
        class="mt-8 h-56 animate-pulse rounded-lg border border-hairline bg-canvas md:p-8"
      />

      <!-- Empty state -->
      <div
        v-else-if="products.length === 0"
        class="mt-8 rounded-lg border border-hairline bg-canvas p-8 text-center md:p-10"
      >
        <PackageSearch class="mx-auto h-6 w-6 text-ink-400" :stroke-width="1.5" />
        <p class="mt-3 text-sm font-semibold text-ink-900">Katalog sedang disiapkan</p>
        <p class="mt-1 text-sm text-ink-500">
          Estimasi harga akan tersedia setelah katalog produk lengkap. Hubungi kami langsung untuk
          kebutuhan cetak Anda.
        </p>
        <NuxtLink
          to="/order"
          class="mt-5 inline-flex items-center justify-center gap-2 rounded-md bg-brand-500 px-5 py-2.5 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
        >
          Order Banner
        </NuxtLink>
      </div>

      <!-- Widget -->
      <div v-else class="mt-8 rounded-lg border border-hairline bg-canvas p-6 shadow-sm md:p-8">
        <div class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_260px]">
          <div class="space-y-4">
            <div class="grid gap-4 sm:grid-cols-2">
              <div>
                <label for="teaser-product" class="block text-sm font-medium text-ink-900">
                  Produk
                </label>
                <select
                  id="teaser-product"
                  v-model="productId"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 transition-colors focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-500/20"
                >
                  <option value="">Pilih produk</option>
                  <option v-for="p in products" :key="p.id" :value="p.id">{{ p.name }}</option>
                </select>
              </div>
              <div>
                <label for="teaser-material" class="block text-sm font-medium text-ink-900">
                  Bahan
                </label>
                <select
                  id="teaser-material"
                  v-model="materialId"
                  :disabled="!detail || detailLoading"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 transition-colors focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-500/20 disabled:bg-canvas-alt disabled:text-ink-500"
                >
                  <option value="">{{ detailLoading ? 'Memuat…' : 'Pilih bahan' }}</option>
                  <option v-for="m in materialOptions" :key="m.id" :value="m.id">{{ m.label }}</option>
                </select>
              </div>
            </div>

            <div class="grid gap-4 sm:grid-cols-2">
              <div>
                <label for="teaser-width" class="block text-sm font-medium text-ink-900">
                  Lebar (cm)
                </label>
                <input
                  id="teaser-width"
                  v-model.number="width"
                  type="number"
                  min="1"
                  :disabled="isPaket || !detail"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 transition-colors focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-500/20 disabled:bg-canvas-alt disabled:text-ink-500"
                >
              </div>
              <div>
                <label for="teaser-height" class="block text-sm font-medium text-ink-900">
                  Tinggi (cm)
                </label>
                <input
                  id="teaser-height"
                  v-model.number="height"
                  type="number"
                  min="1"
                  :disabled="isPaket || !detail"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 transition-colors focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-500/20 disabled:bg-canvas-alt disabled:text-ink-500"
                >
              </div>
            </div>
            <p v-if="isPaket" class="text-xs text-ink-500">Ukuran mengikuti paket yang dipilih.</p>
            <p v-if="detailError" class="text-xs text-brand-700">{{ detailError }}</p>
          </div>

          <div class="rounded-md border border-hairline bg-canvas-alt p-5">
            <p class="flex items-center gap-1.5 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
              <Calculator class="h-3 w-3" :stroke-width="1.75" />
              Estimasi
            </p>
            <div v-if="quoteLoading" class="mt-3 flex items-center gap-2 text-sm text-ink-500">
              <Loader2 class="h-4 w-4 animate-spin" :stroke-width="1.75" />
              Menghitung…
            </div>
            <div v-else-if="quoteError" class="mt-3 text-xs text-brand-700">{{ quoteError }}</div>
            <div v-else-if="quote" class="mt-3">
              <p class="font-serif text-2xl font-semibold text-ink-950">{{ fmtIDR(quote.total_price) }}</p>
              <p v-if="quote.area_m2 != null" class="mt-1 font-mono text-xs text-ink-500">
                {{ quote.area_m2.toFixed(2) }} m²
              </p>
            </div>
            <p v-else class="mt-3 text-sm text-ink-500">
              Pilih produk, bahan{{ isPaket ? '' : ', dan ukuran' }} untuk melihat estimasi.
            </p>

            <NuxtLink
              to="/order"
              class="mt-5 inline-flex w-full items-center justify-center gap-2 rounded-md bg-brand-500 px-4 py-2.5 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
            >
              Lanjut Order
              <ArrowRight class="h-4 w-4" :stroke-width="1.5" />
            </NuxtLink>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
