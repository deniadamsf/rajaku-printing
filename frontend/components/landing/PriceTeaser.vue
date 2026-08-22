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
import { ArrowRight, Calculator, Loader2 } from '@lucide/vue'
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

const prefersReduced = usePrefersReducedMotion()

/**
 * Angka estimasi dijalankan naik/turun ke nilai baru, tidak melompat.
 *
 * Bukan hiasan: quote dihitung ulang di server tiap kali ukuran diubah, dan
 * angka yang berganti diam-diam sering tidak disadari — orang mengira
 * kalkulatornya tidak bereaksi. Gerakan singkat inilah tanda "ini sudah
 * dihitung ulang". Mati total saat prefers-reduced-motion.
 */
const displayedPrice = ref(0)
let countRaf: number | null = null

function stopCount() {
  if (countRaf !== null) {
    cancelAnimationFrame(countRaf)
    countRaf = null
  }
}

watch(
  () => quote.value?.total_price ?? null,
  (to) => {
    stopCount()
    if (to == null) {
      displayedPrice.value = 0
      return
    }
    if (prefersReduced.value) {
      displayedPrice.value = to
      return
    }
    const from = displayedPrice.value
    const t0 = performance.now()
    const DUR = 450
    const step = (now: number) => {
      const p = Math.min(1, (now - t0) / DUR)
      const eased = 1 - Math.pow(1 - p, 3)
      displayedPrice.value = Math.round(from + (to - from) * eased)
      countRaf = p < 1 ? requestAnimationFrame(step) : null
    }
    countRaf = requestAnimationFrame(step)
  },
)

onBeforeUnmount(() => {
  stopCount()
  if (quoteTimer) clearTimeout(quoteTimer)
})

/**
 * Tautan "Lanjut Order" membawa pilihan yang sudah dibuat di sini.
 * Tanpa ini, pengunjung yang baru saja memilih produk, bahan, dan ukuran
 * harus memilih ulang semuanya dari nol di halaman order — kerja yang sama
 * dua kali, tepat di langkah paling menentukan.
 */
const orderHref = computed(() => {
  const d = detail.value
  if (!d || !materialId.value) return '/order'
  const q = new URLSearchParams({ produk: d.id, bahan: materialId.value })
  if (width.value > 0) q.set('lebar', String(width.value))
  if (height.value > 0) q.set('tinggi', String(height.value))
  return `/order?${q.toString()}`
})

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
  <!--
    Section ini HILANG total kalau katalog tidak bisa dimuat, bukan berubah jadi
    kotak "kalkulator belum bisa dimuat". Alasannya: backend sering mati di
    lingkungan pemilik, dan kotak error sepanjang satu layar adalah yang paling
    membuat landing terasa rusak. Daftar produk & tautan katalog sudah dijamin
    tampil oleh `ServicesSection` (punya fallback statis), jadi menyembunyikan
    kalkulator tidak menghilangkan informasi apa pun dari halaman.
    `productsPending` tetap dipertahankan supaya skeleton muncul saat navigasi
    sisi klien, bukan section yang berkedip hilang lalu muncul.
  -->
  <section
    v-if="!productsError && (productsPending || products.length > 0)"
    id="estimasi-harga"
    class="bg-canvas-alt"
  >
    <div class="mx-auto max-w-6xl px-4 py-12 md:py-20">
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

      <!-- Loading skeleton -->
      <div
        v-if="productsPending"
        class="mt-8 h-56 animate-pulse rounded-lg border border-hairline bg-canvas md:p-8"
      />

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

          <!--
            Panel hasil sengaja gelap di dalam section terang: ini satu-satunya
            angka di seluruh halaman depan, dan sebagai kartu abu-abu di antara
            kotak input ia terbaca seperti keterangan tambahan, bukan jawaban.
            Token on-dark mengikuti §26.12.
          -->
          <div class="flex flex-col rounded-md border border-white/10 bg-ink-950 p-5">
            <p class="flex items-center gap-1.5 text-[10px] font-medium uppercase tracking-[0.14em] text-canvas/60">
              <Calculator class="h-3 w-3 text-gold-400" :stroke-width="1.75" />
              Estimasi
            </p>

            <div v-if="quoteLoading" class="mt-3 flex items-center gap-2 text-sm text-canvas/70">
              <Loader2 class="h-4 w-4 animate-spin" :stroke-width="1.75" />
              Menghitung…
            </div>
            <div v-else-if="quoteError" class="mt-3 text-xs text-gold-400">{{ quoteError }}</div>

            <div v-else-if="quote" class="mt-3">
              <p class="font-serif text-3xl font-semibold leading-none tracking-tight text-canvas">
                {{ fmtIDR(displayedPrice) }}
              </p>

              <!--
                Rincian singkat: angka tanpa penjelasan gampang dicurigai.
                Semua nilai datang dari balasan server, tidak ada yang dihitung
                ulang di sini.
              -->
              <dl class="mt-4 space-y-1.5 border-t border-white/10 pt-3 text-xs">
                <div class="flex items-baseline justify-between gap-3">
                  <dt class="text-canvas/50">Ukuran</dt>
                  <dd class="font-mono text-canvas/80">{{ quote.width_cm }} × {{ quote.height_cm }} cm</dd>
                </div>
                <div v-if="quote.chargeable_m2 != null || quote.area_m2 != null" class="flex items-baseline justify-between gap-3">
                  <dt class="text-canvas/50">Luas dihitung</dt>
                  <dd class="font-mono text-canvas/80">
                    {{ (quote.chargeable_m2 ?? quote.area_m2)?.toFixed(2) }} m²
                  </dd>
                </div>
                <div v-if="quote.price_per_m2 != null" class="flex items-baseline justify-between gap-3">
                  <dt class="text-canvas/50">Harga bahan</dt>
                  <dd class="font-mono text-canvas/80">{{ fmtIDR(quote.price_per_m2) }}/m²</dd>
                </div>
                <div v-else-if="quote.package_label" class="flex items-baseline justify-between gap-3">
                  <dt class="text-canvas/50">Paket</dt>
                  <dd class="text-canvas/80">{{ quote.package_label }}</dd>
                </div>
              </dl>

              <p class="mt-3 text-[11px] leading-relaxed text-canvas/45">
                Belum termasuk ongkir dan biaya jasa desain (kalau ada).
              </p>
            </div>

            <p v-else class="mt-3 text-sm text-canvas/60">
              Pilih produk, bahan{{ isPaket ? '' : ', dan ukuran' }} untuk melihat estimasi.
            </p>

            <NuxtLink
              :to="orderHref"
              class="mt-5 inline-flex w-full items-center justify-center gap-2 rounded-md bg-brand-500 px-4 py-2.5 text-sm font-semibold text-canvas transition-colors duration-200 ease-out hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold-400/60 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950"
            >
              Lanjut Order
              <ArrowRight class="h-4 w-4" :stroke-width="1.5" />
            </NuxtLink>
            <p v-if="quote" class="mt-2 text-center text-[11px] text-canvas/45">
              Pilihan Anda ikut terbawa ke formulir order.
            </p>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
