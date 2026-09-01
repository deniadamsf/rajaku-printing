<script setup lang="ts">
/**
 * /order — Halaman order publik (§17 CTA utama, §32 multi-item).
 *
 * Flow:
 *   1. Customer pilih 1..20 baris banner (produk + bahan + ukuran + sumber
 *      desain per baris) → live quote per baris via /catalog/quote (§32.4).
 *   2. Pilih ambil (pickup / kirim + alamat) — level order, bukan per baris.
 *   3. Isi identitas (auto-fill kalau login; guest → nama + WA).
 *   4. Submit → order dibuat status `order_masuk`, dapat SATU resi
 *      RJK-xxxxxxxx untuk seluruh baris.
 *   5. Success panel: resi + ringkasan tiap baris + link `/lacak/:resi`.
 *
 * Auth opsional — Bearer token auto-attach kalau ada; kalau tidak, guest path
 * pakai nama + phone. Redirect POST-nya sama.
 *
 * Design: patuh CLAUDE.md §26 (Fraunces + Inter + Lucide, brand/ink/hairline).
 * Mobile (§18): tiap baris banner jadi kartu bertumpuk — bukan tabel yang
 * menggulir ke samping, sejak awal (bukan cuma resize dari desktop).
 */
import {
  User,
  Package,
  Truck,
  Receipt,
  StickyNote,
  ExternalLink,
  RotateCcw,
  CheckCircle2,
  Loader2,
  Plus,
  Trash2,
} from '@lucide/vue'
import type { CatalogProduct, CatalogProductDetail, CatalogQuote } from '~/types/catalog'
import type { Order, DesignSource, MetodeAmbil } from '~/types/order'
import { ApiError } from '~/composables/useApi'

definePageMeta({ layout: 'default' })

useSeoMeta({
  title: 'Order Banner',
  description:
    'Order banner cetak custom online. Upload desain sendiri atau minta jasa desain. Ambil di tempat atau dikirim ke alamat Anda.',
  ogTitle: 'Order Banner — Rajaku Printing',
})

const catalog = useCatalog()
const orderApi = useOrder()
const auth = useAuthStore()

// -------------------- katalog --------------------
const products = ref<CatalogProduct[]>([])
const loadingProducts = ref(false)

// -------------------- baris item (§32) --------------------
const MAX_ITEMS = 20

interface OrderItemRow {
  /** ID lokal stabil untuk :key & timer debounce — TIDAK dikirim ke backend. */
  key: number
  productId: string
  materialId: string
  widthCm: number
  heightCm: number
  quantity: number
  designSource: DesignSource
  designBrief: string
  productDetail: CatalogProductDetail | null
  loadingDetail: boolean
  quote: CatalogQuote | null
  quoteLoading: boolean
  quoteError: string | null
}

let rowKeySeq = 0
function makeRow(): OrderItemRow {
  rowKeySeq += 1
  return {
    key: rowKeySeq,
    productId: '',
    materialId: '',
    widthCm: 0,
    heightCm: 0,
    quantity: 1,
    designSource: 'upload',
    designBrief: '',
    productDetail: null,
    loadingDetail: false,
    quote: null,
    quoteLoading: false,
    quoteError: null,
  }
}

const items = ref<OrderItemRow[]>([makeRow()])
const atMaxItems = computed(() => items.value.length >= MAX_ITEMS)

function addItem() {
  if (atMaxItems.value) return
  items.value.push(makeRow())
}

function removeItem(key: number) {
  // Baris pertama tidak bisa dihapus — order selalu butuh minimal 1 item.
  if (items.value.length <= 1) return
  const timer = quoteTimers.get(key)
  if (timer) {
    clearTimeout(timer)
    quoteTimers.delete(key)
  }
  items.value = items.value.filter((r) => r.key !== key)
}

// -------------------- fulfillment & kontak (level order) --------------------
const form = reactive({
  metodeAmbil: 'pickup' as MetodeAmbil,
  shippingAddress: '',
  shippingRecipientName: '',
  shippingRecipientPhone: '',
  guestName: '',
  guestPhone: '',
  notes: '',
})

// Auto-fill guest fields dari user profile kalau login sebagai customer.
onMounted(() => {
  if (auth.isCustomer && auth.user) {
    form.guestName = auth.user.name
    form.guestPhone = auth.user.phone || ''
  }
})

const submitting = ref(false)
const submitError = ref<string | null>(null)
const successResult = ref<Order | null>(null)

// -------------------- data load --------------------
onMounted(async () => {
  loadingProducts.value = true
  try {
    const res = await catalog.listProducts()
    products.value = res.products
    applyPrefillFromQuery()
  } catch (e) {
    submitError.value = e instanceof ApiError ? e.message : 'Gagal memuat katalog'
  } finally {
    loadingProducts.value = false
  }
})

/**
 * Isi awal baris pertama dari query string, dikirim widget estimasi di
 * landing (`LandingPriceTeaser` → `/order?produk=…&bahan=…&lebar=…&tinggi=…`).
 *
 * Tanpa ini, orang yang baru saja menyusun estimasi harus memilih produk,
 * bahan, dan mengetik ukuran yang sama sekali lagi dari nol — persis di
 * langkah yang paling menentukan.
 *
 * Nilainya TIDAK dipercaya begitu saja: id produk & bahan dicocokkan dulu ke
 * katalog yang benar-benar dimuat, ukuran wajib angka positif. Query string
 * bisa diketik siapa saja, dan formulir yang terisi data ngawur lebih buruk
 * daripada formulir kosong. Harga tetap dihitung ulang server seperti biasa.
 */
function applyPrefillFromQuery() {
  const q = useRoute().query
  const pid = typeof q.produk === 'string' ? q.produk : ''
  if (!pid || !products.value.some((p) => p.id === pid)) return

  const row = items.value[0]
  row.productId = pid

  void onProductChange(row).then(() => {
    const mid = typeof q.bahan === 'string' ? q.bahan : ''
    if (mid && row.productDetail?.pricings.some((r) => r.material_id === mid)) {
      row.materialId = mid
    }
    const w = Number(q.lebar)
    const h = Number(q.tinggi)
    if (Number.isFinite(w) && w > 0) row.widthCm = w
    if (Number.isFinite(h) && h > 0) row.heightCm = h
    scheduleQuote(row)
  })
}

// -------------------- per-baris: produk / bahan / quote --------------------
async function onProductChange(row: OrderItemRow) {
  row.productDetail = null
  row.materialId = ''
  row.widthCm = 0
  row.heightCm = 0
  row.quote = null
  row.quoteError = null
  if (!row.productId) return
  const p = products.value.find((x) => x.id === row.productId)
  if (!p) return
  row.loadingDetail = true
  try {
    row.productDetail = await catalog.getProduct(p.slug)
    if (row.productDetail.pricings.length === 1) {
      row.materialId = row.productDetail.pricings[0].material_id
      onMaterialChange(row)
    }
  } catch (e) {
    submitError.value = e instanceof ApiError ? e.message : 'Gagal memuat detail produk'
  } finally {
    row.loadingDetail = false
  }
}

function onMaterialChange(row: OrderItemRow) {
  const p = row.productDetail
  if (p && row.materialId && p.pricing_type === 'paket') {
    const found = p.pricings.find((r) => r.material_id === row.materialId)
    if (found?.width_cm) row.widthCm = found.width_cm
    if (found?.height_cm) row.heightCm = found.height_cm
  }
  scheduleQuote(row)
}

const quoteTimers = new Map<number, ReturnType<typeof setTimeout>>()
function scheduleQuote(row: OrderItemRow) {
  const existing = quoteTimers.get(row.key)
  if (existing) clearTimeout(existing)
  quoteTimers.set(
    row.key,
    setTimeout(() => runQuote(row), 400),
  )
}

async function runQuote(row: OrderItemRow) {
  const p = row.productDetail
  if (!p || !row.materialId || row.widthCm <= 0 || row.heightCm <= 0) {
    row.quote = null
    row.quoteError = null
    return
  }
  row.quoteLoading = true
  row.quoteError = null
  try {
    row.quote = await catalog.quote({
      product_id: p.id,
      material_id: row.materialId,
      width_cm: row.widthCm,
      height_cm: row.heightCm,
    })
  } catch (e) {
    row.quote = null
    row.quoteError = e instanceof ApiError ? e.message : 'Gagal menghitung harga'
  } finally {
    row.quoteLoading = false
  }
}

// -------------------- derived --------------------
function isPerM2(row: OrderItemRow): boolean {
  return row.productDetail?.pricing_type === 'per_m2'
}
function isPaket(row: OrderItemRow): boolean {
  return row.productDetail?.pricing_type === 'paket'
}
const isKirim = computed(() => form.metodeAmbil === 'kirim')

function availableMaterials(row: OrderItemRow) {
  const p = row.productDetail
  if (!p) return []
  return p.pricings.map((r) => ({
    id: r.material_id,
    label:
      p.pricing_type === 'paket'
        ? `${r.material_name}${r.package_label ? ' · ' + r.package_label : ''}${r.price_total != null ? ' · ' + fmtIDR(r.price_total) : ''}`
        : `${r.material_name}${r.price_per_m2 != null ? ' · ' + fmtIDR(r.price_per_m2) + '/m²' : ''}`,
  }))
}

function rowSubtotal(row: OrderItemRow): number {
  return row.quote ? row.quote.total_price * row.quantity : 0
}
const grandSubtotal = computed(() => items.value.reduce((sum, r) => sum + rowSubtotal(r), 0))

function rowValid(row: OrderItemRow): boolean {
  if (!row.productId || !row.materialId) return false
  if (row.widthCm <= 0 || row.heightCm <= 0 || row.quantity < 1) return false
  if (!row.quote) return false
  if (row.designSource === 'request' && !row.designBrief.trim()) return false
  return true
}

const canSubmit = computed(() => {
  if (submitting.value) return false
  if (items.value.length === 0) return false
  if (!items.value.every(rowValid)) return false
  if (isKirim.value) {
    if (!form.shippingAddress.trim()) return false
    if (!form.shippingRecipientName.trim()) return false
    if (!form.shippingRecipientPhone.trim()) return false
  }
  // Guest identity (kecuali sudah login sebagai customer):
  if (!auth.isCustomer) {
    if (!form.guestName.trim() || !form.guestPhone.trim()) return false
  }
  return true
})

// -------------------- helpers --------------------
function fmtIDR(v?: number | null): string {
  if (v == null) return '—'
  return new Intl.NumberFormat('id-ID', {
    style: 'currency',
    currency: 'IDR',
    maximumFractionDigits: 0,
  }).format(v)
}

// -------------------- submit --------------------
async function onSubmit() {
  if (!canSubmit.value) return
  submitting.value = true
  submitError.value = null
  try {
    const res = await orderApi.createOnline({
      items: items.value.map((r) => ({
        product_id: r.productId,
        material_id: r.materialId,
        width_cm: r.widthCm,
        height_cm: r.heightCm,
        quantity: r.quantity,
        design_source: r.designSource,
        design_brief: r.designBrief.trim() || undefined,
      })),
      metode_ambil: form.metodeAmbil,
      shipping_address: isKirim.value ? form.shippingAddress.trim() : undefined,
      shipping_recipient_name: isKirim.value ? form.shippingRecipientName.trim() : undefined,
      shipping_recipient_phone: isKirim.value ? form.shippingRecipientPhone.trim() : undefined,
      guest_name: auth.isCustomer ? undefined : form.guestName.trim(),
      guest_phone: auth.isCustomer ? undefined : form.guestPhone.trim(),
      notes: form.notes.trim() || undefined,
    })
    successResult.value = res
    // Scroll ke atas supaya success panel keliatan.
    if (import.meta.client) window.scrollTo({ top: 0, behavior: 'smooth' })
  } catch (e) {
    submitError.value = e instanceof ApiError ? e.message : 'Gagal membuat order'
  } finally {
    submitting.value = false
  }
}

function resetForm() {
  successResult.value = null
  items.value = [makeRow()]
  form.metodeAmbil = 'pickup'
  form.shippingAddress = ''
  form.shippingRecipientName = ''
  form.shippingRecipientPhone = ''
  form.notes = ''
  submitError.value = null
}
</script>

<template>
  <main class="mx-auto max-w-5xl px-4 py-10 md:py-16">
    <!-- ============================ Success panel ============================ -->
    <div v-if="successResult" class="space-y-6">
      <div class="rounded-lg border border-emerald-200 bg-emerald-50 p-6">
        <div class="flex items-start gap-3">
          <CheckCircle2 class="h-6 w-6 text-emerald-600 flex-none mt-0.5" :stroke-width="1.75" />
          <div class="flex-1">
            <h1 class="font-serif text-xl md:text-2xl font-semibold text-emerald-900">Terima kasih! Order Anda tercatat.</h1>
            <p class="mt-1 text-sm text-emerald-800 leading-relaxed">
              Nomor resi Anda: <strong class="font-mono">{{ successResult.resi }}</strong>.
              Simpan resi ini untuk lacak status kapan saja.
            </p>
          </div>
        </div>
      </div>

      <div class="rounded-lg border border-hairline bg-canvas p-6 md:p-8">
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-3">
          Ringkasan order · {{ successResult.items.length }} banner
        </p>
        <ul class="divide-y divide-hairline">
          <li v-for="it in successResult.items" :key="it.line_no" class="py-3 first:pt-0 last:pb-0">
            <div class="flex flex-wrap items-baseline justify-between gap-2">
              <p class="text-sm font-medium text-ink-900">{{ it.product_name }}</p>
              <p class="text-sm text-ink-900">{{ fmtIDR(it.subtotal) }}</p>
            </div>
            <p class="mt-0.5 text-xs text-ink-500">
              {{ it.material_name }}
              <span class="text-ink-400">·</span>
              <span class="font-mono">{{ it.width_cm }} × {{ it.height_cm }} cm</span>
              <span class="text-ink-400">·</span>
              {{ it.quantity }} pcs
            </p>
          </li>
        </ul>
        <div class="mt-3 flex justify-between border-t border-hairline pt-3 text-sm">
          <span class="text-ink-500">Ambil</span>
          <span class="text-ink-900 capitalize">{{ successResult.metode_ambil }}</span>
        </div>
        <div class="mt-2 flex justify-between">
          <span class="text-sm font-semibold text-ink-950">Subtotal</span>
          <span class="font-serif text-lg font-semibold text-ink-950">{{ fmtIDR(successResult.subtotal) }}</span>
        </div>
      </div>

      <div class="rounded-lg bg-ink-950 p-6 md:p-8 text-canvas">
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-gold-400 mb-3">Langkah berikutnya</p>
        <ol class="space-y-3 text-sm leading-relaxed">
          <li class="flex gap-3">
            <span class="font-serif text-lg font-semibold text-gold-400 leading-none">1.</span>
            <span>Tim kami akan cek order Anda & mengabari via WhatsApp
              <strong v-if="!auth.isCustomer" class="font-mono">({{ form.guestPhone || auth.user?.phone }})</strong>.
              <span v-if="successResult.metode_ambil === 'kirim'">Ongkir akan dihitung manual & diinfokan sekalian.</span>
            </span>
          </li>
          <li class="flex gap-3">
            <span class="font-serif text-lg font-semibold text-gold-400 leading-none">2.</span>
            <span>Setelah total fix, Anda transfer / bayar QRIS dan upload bukti transfer.</span>
          </li>
          <li class="flex gap-3">
            <span class="font-serif text-lg font-semibold text-gold-400 leading-none">3.</span>
            <span v-if="successResult.design_source !== 'upload'">
              Tim desainer kami akan kirim draft untuk approval Anda sebelum masuk cetak (untuk banner yang Anda minta dibuatkan).
            </span>
            <!-- Halaman detail pesanan dijaga middleware customer-only, jadi link
                 hanya relevan untuk customer terdaftar. Order guest belum punya
                 jalur upload mandiri — arahkan ke WA supaya tidak mentok di login. -->
            <span v-else-if="auth.isCustomer">
              Setelah pembayaran terverifikasi, upload file desain Anda (CDR / AI / PDF / JPG / PNG)
              di halaman <NuxtLink :to="`/akun/pesanan/${successResult.resi}`" class="underline rounded-sm hover:text-gold-400 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold-400/60 focus-visible:ring-offset-2 focus-visible:ring-offset-ink-950 transition-colors">detail pesanan</NuxtLink> — tim cek lalu langsung cetak.
            </span>
            <span v-else>
              Kirimkan file desain Anda (CDR / AI / PDF / JPG / PNG) via WA — tim cek lalu langsung cetak.
            </span>
          </li>
          <li class="flex gap-3">
            <span class="font-serif text-lg font-semibold text-gold-400 leading-none">4.</span>
            <span>Setelah cetak & QC selesai, banner siap diambil / dikirim. Lacak progres kapan saja.</span>
          </li>
        </ol>
      </div>

      <div class="flex flex-wrap gap-2">
        <NuxtLink
          :to="`/lacak/${successResult.resi}`"
          class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors"
        >
          <ExternalLink class="h-4 w-4" :stroke-width="1.75" />
          Lacak resi
        </NuxtLink>
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-md border border-hairline bg-canvas px-4 py-2 text-sm font-semibold text-ink-900 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
          @click="resetForm"
        >
          <RotateCcw class="h-4 w-4" :stroke-width="1.75" />
          Order lagi
        </button>
      </div>
    </div>

    <!-- ============================ Form ============================ -->
    <div v-else>
      <div class="mb-8">
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2">Order Banner</p>
        <h1 class="font-serif text-3xl md:text-5xl font-semibold tracking-tight text-ink-950">
          Cetak banner Anda, dari sini.
        </h1>
        <p class="mt-3 text-sm md:text-base text-ink-500 leading-relaxed max-w-2xl">
          Pilih bahan, tentukan ukuran, hitung harga langsung. Butuh beberapa ukuran sekaligus? Tambah banner
          dalam satu pesanan yang sama. Tim kami hubungi via WA setelah order masuk.
        </p>
      </div>

      <form class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_340px]" @submit.prevent="onSubmit">
        <div class="space-y-6">
          <AlertMessage v-if="submitError" variant="error" :message="submitError" />

          <!-- Baris banner (§32) — kartu bertumpuk di semua breakpoint, BUKAN
               tabel yang menggulir ke samping (§18: mobile bukan cuma resize). -->
          <div class="space-y-4">
            <div
              v-for="(row, idx) in items"
              :key="row.key"
              class="rounded-lg border border-hairline bg-canvas p-6"
            >
              <div class="flex items-center justify-between gap-2 mb-4">
                <p class="flex items-center gap-2 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
                  <Package class="h-3.5 w-3.5" :stroke-width="1.75" />
                  Banner {{ idx + 1 }}
                </p>
                <button
                  v-if="idx > 0"
                  type="button"
                  class="inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-xs font-medium text-ink-500 hover:text-brand-600 hover:bg-brand-50 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
                  @click="removeItem(row.key)"
                >
                  <Trash2 class="h-3.5 w-3.5" :stroke-width="1.75" />
                  Hapus
                </button>
              </div>

              <div class="grid gap-4 sm:grid-cols-2">
                <div>
                  <label :for="`ord-product-${row.key}`" class="block text-sm font-medium text-ink-900">
                    Produk <span class="text-brand-500">*</span>
                  </label>
                  <select
                    :id="`ord-product-${row.key}`"
                    v-model="row.productId"
                    required
                    :disabled="loadingProducts"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:bg-canvas-alt disabled:text-ink-500"
                    @change="onProductChange(row)"
                  >
                    <option value="">{{ loadingProducts ? 'Memuat…' : 'Pilih produk' }}</option>
                    <option v-for="p in products" :key="p.id" :value="p.id">
                      {{ p.name }}
                    </option>
                  </select>
                  <p v-if="row.productDetail?.description" class="mt-1 text-xs text-ink-500 leading-relaxed">{{ row.productDetail.description }}</p>
                </div>

                <div>
                  <label :for="`ord-material-${row.key}`" class="block text-sm font-medium text-ink-900">
                    Bahan <span class="text-brand-500">*</span>
                  </label>
                  <select
                    :id="`ord-material-${row.key}`"
                    v-model="row.materialId"
                    required
                    :disabled="!row.productDetail || row.loadingDetail"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:bg-canvas-alt disabled:text-ink-500"
                    @change="onMaterialChange(row)"
                  >
                    <option value="">{{ row.loadingDetail ? 'Memuat…' : 'Pilih bahan' }}</option>
                    <option v-for="m in availableMaterials(row)" :key="m.id" :value="m.id">{{ m.label }}</option>
                  </select>
                  <p v-if="isPaket(row)" class="mt-1 text-xs text-ink-500">Ukuran otomatis mengikuti paket terpilih.</p>
                  <p v-else-if="isPerM2(row) && row.productDetail" class="mt-1 text-xs text-ink-500">
                    <template v-if="row.productDetail.min_width_cm && row.productDetail.max_width_cm">
                      Ukuran custom · {{ row.productDetail.min_width_cm }}–{{ row.productDetail.max_width_cm }} cm (W)
                      × {{ row.productDetail.min_height_cm }}–{{ row.productDetail.max_height_cm }} cm (H)
                    </template>
                  </p>
                </div>
              </div>

              <div class="mt-4 grid gap-4 sm:grid-cols-3">
                <div>
                  <label :for="`ord-width-${row.key}`" class="block text-sm font-medium text-ink-900">
                    Lebar (cm) <span class="text-brand-500">*</span>
                  </label>
                  <input
                    :id="`ord-width-${row.key}`"
                    v-model.number="row.widthCm"
                    type="number"
                    min="1"
                    required
                    :disabled="isPaket(row)"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:bg-canvas-alt disabled:text-ink-500"
                    @input="scheduleQuote(row)"
                  >
                </div>
                <div>
                  <label :for="`ord-height-${row.key}`" class="block text-sm font-medium text-ink-900">
                    Tinggi (cm) <span class="text-brand-500">*</span>
                  </label>
                  <input
                    :id="`ord-height-${row.key}`"
                    v-model.number="row.heightCm"
                    type="number"
                    min="1"
                    required
                    :disabled="isPaket(row)"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:bg-canvas-alt disabled:text-ink-500"
                    @input="scheduleQuote(row)"
                  >
                </div>
                <div>
                  <label :for="`ord-qty-${row.key}`" class="block text-sm font-medium text-ink-900">
                    Kuantitas <span class="text-brand-500">*</span>
                  </label>
                  <input
                    :id="`ord-qty-${row.key}`"
                    v-model.number="row.quantity"
                    type="number"
                    min="1"
                    required
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  >
                </div>
              </div>

              <!-- Desain per baris (§32.5) -->
              <div class="mt-4">
                <p class="text-sm font-medium text-ink-900 mb-2">Desain banner ini</p>
                <div class="grid gap-2 sm:grid-cols-2">
                  <label
                    :class="[
                      'flex cursor-pointer items-start gap-2 rounded-md border p-3 text-sm transition-colors',
                      row.designSource === 'upload'
                        ? 'border-brand-500 bg-brand-50/50 text-ink-950'
                        : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300',
                    ]"
                  >
                    <input v-model="row.designSource" type="radio" :name="`design-source-${row.key}`" value="upload" class="mt-0.5 accent-brand-500">
                    <span>
                      <span class="block font-semibold">Saya sudah punya desain</span>
                      <span class="mt-0.5 block text-xs text-ink-500">Upload file (CDR/AI/PDF/JPG/PNG) via WA setelah order tercatat.</span>
                    </span>
                  </label>
                  <label
                    :class="[
                      'flex cursor-pointer items-start gap-2 rounded-md border p-3 text-sm transition-colors',
                      row.designSource === 'request'
                        ? 'border-brand-500 bg-brand-50/50 text-ink-950'
                        : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300',
                    ]"
                  >
                    <input v-model="row.designSource" type="radio" :name="`design-source-${row.key}`" value="request" class="mt-0.5 accent-brand-500">
                    <span>
                      <span class="block font-semibold">Minta jasa desain</span>
                      <span class="mt-0.5 block text-xs text-ink-500">Tim desainer buat draft, Anda approve dulu sebelum cetak.</span>
                    </span>
                  </label>
                </div>

                <div v-if="row.designSource === 'request'" class="mt-3">
                  <label :for="`ord-brief-${row.key}`" class="block text-sm font-medium text-ink-900">
                    Brief singkat <span class="text-brand-500">*</span>
                  </label>
                  <textarea
                    :id="`ord-brief-${row.key}`"
                    v-model="row.designBrief"
                    rows="2"
                    required
                    placeholder="Contoh: Banner ucapan syukuran, warna dominan biru, teks 'Aqiqah Naura'."
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  />
                </div>
              </div>

              <!-- Harga baris ini -->
              <div class="mt-4 flex items-center justify-between border-t border-hairline pt-3 text-sm">
                <span class="text-ink-500">
                  <Loader2 v-if="row.quoteLoading" class="inline h-3 w-3 animate-spin align-[-1px]" :stroke-width="1.75" />
                  <template v-else-if="row.quote">{{ fmtIDR(row.quote.total_price) }} / pcs</template>
                  <template v-else>Lengkapi produk, bahan & ukuran</template>
                </span>
                <span class="font-serif text-base font-semibold text-ink-950">{{ fmtIDR(rowSubtotal(row)) }}</span>
              </div>
              <p v-if="row.quoteError" class="mt-1 text-xs text-brand-700">{{ row.quoteError }}</p>
            </div>
          </div>

          <div>
            <button
              type="button"
              :disabled="atMaxItems"
              class="inline-flex items-center gap-2 rounded-md border border-hairline bg-canvas px-4 py-2 text-sm font-semibold text-ink-900 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              @click="addItem"
            >
              <Plus class="h-4 w-4" :stroke-width="1.75" />
              Tambah banner
            </button>
            <p class="mt-1.5 text-xs text-ink-500">
              {{ atMaxItems
                ? `Maksimal ${MAX_ITEMS} banner per pesanan sudah tercapai. Buat pesanan terpisah untuk banner tambahan.`
                : `Bisa ditambah sampai ${MAX_ITEMS} banner dalam satu pesanan (${items.length}/${MAX_ITEMS}).` }}
            </p>
          </div>

          <!-- Fulfillment -->
          <fieldset class="rounded-lg border border-hairline bg-canvas p-6">
            <legend class="flex items-center gap-2 px-2 -ml-2 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
              <Truck class="h-3.5 w-3.5" :stroke-width="1.75" />
              Pengambilan
            </legend>

            <div class="inline-flex rounded-md border border-hairline overflow-hidden">
              <button
                type="button"
                :class="[
                  'px-4 py-2 text-sm font-medium transition-colors',
                  form.metodeAmbil === 'pickup'
                    ? 'bg-ink-950 text-canvas'
                    : 'bg-canvas text-ink-700 hover:bg-canvas-alt',
                ]"
                @click="form.metodeAmbil = 'pickup'"
              >
                Ambil di tempat
              </button>
              <button
                type="button"
                :class="[
                  'px-4 py-2 text-sm font-medium border-l border-hairline transition-colors',
                  form.metodeAmbil === 'kirim'
                    ? 'bg-ink-950 text-canvas'
                    : 'bg-canvas text-ink-700 hover:bg-canvas-alt',
                ]"
                @click="form.metodeAmbil = 'kirim'"
              >
                Kirim
              </button>
            </div>
            <p v-if="isKirim" class="mt-2 text-xs text-ink-500">
              Ongkir dihitung manual oleh admin & diinfokan via WA sebelum bayar.
            </p>

            <div v-if="isKirim" class="mt-4 grid gap-4 sm:grid-cols-2">
              <div class="sm:col-span-2">
                <label for="ord-ship-addr" class="block text-sm font-medium text-ink-900">
                  Alamat pengiriman <span class="text-brand-500">*</span>
                </label>
                <textarea
                  id="ord-ship-addr"
                  v-model="form.shippingAddress"
                  rows="2"
                  required
                  placeholder="Jl. Merdeka No. 12, Trenggalek, Jawa Timur"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                />
              </div>
              <div>
                <label for="ord-ship-name" class="block text-sm font-medium text-ink-900">
                  Nama penerima <span class="text-brand-500">*</span>
                </label>
                <input
                  id="ord-ship-name"
                  v-model="form.shippingRecipientName"
                  type="text"
                  required
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                >
              </div>
              <div>
                <label for="ord-ship-phone" class="block text-sm font-medium text-ink-900">
                  No. HP penerima <span class="text-brand-500">*</span>
                </label>
                <input
                  id="ord-ship-phone"
                  v-model="form.shippingRecipientPhone"
                  type="tel"
                  required
                  inputmode="numeric"
                  placeholder="0812 3456 7890"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                >
              </div>
            </div>
          </fieldset>

          <!-- Kontak -->
          <fieldset v-if="!auth.isCustomer" class="rounded-lg border border-hairline bg-canvas p-6">
            <legend class="flex items-center gap-2 px-2 -ml-2 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
              <User class="h-3.5 w-3.5" :stroke-width="1.75" />
              Kontak Anda
            </legend>
            <p class="mb-3 text-xs text-ink-500">
              Belum punya akun? Order tetap tercatat sebagai <strong class="text-ink-900">guest</strong> — nomor WA jadi identitas.
              <NuxtLink to="/register" class="text-brand-700 hover:underline">Daftar</NuxtLink> juga bisa untuk riwayat lengkap.
            </p>

            <div class="grid gap-4 sm:grid-cols-2">
              <div>
                <label for="ord-name" class="block text-sm font-medium text-ink-900">
                  Nama <span class="text-brand-500">*</span>
                </label>
                <input
                  id="ord-name"
                  v-model="form.guestName"
                  type="text"
                  required
                  placeholder="Nama lengkap"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                >
              </div>
              <div>
                <label for="ord-phone" class="block text-sm font-medium text-ink-900">
                  No. WhatsApp <span class="text-brand-500">*</span>
                </label>
                <input
                  id="ord-phone"
                  v-model="form.guestPhone"
                  type="tel"
                  required
                  inputmode="numeric"
                  autocomplete="tel"
                  placeholder="0812 3456 7890"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                >
                <p class="mt-1 text-xs text-ink-500">Auto-format ke <span class="font-mono">62xxx</span>.</p>
              </div>
            </div>
          </fieldset>

          <!-- Notes -->
          <fieldset class="rounded-lg border border-hairline bg-canvas p-6">
            <legend class="flex items-center gap-2 px-2 -ml-2 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
              <StickyNote class="h-3.5 w-3.5" :stroke-width="1.75" />
              Catatan (opsional, untuk seluruh pesanan)
            </legend>
            <textarea
              v-model="form.notes"
              rows="2"
              placeholder="Contoh: minta finishing mata ayam 4 pojok"
              class="block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
            />
          </fieldset>
        </div>

        <!-- Summary sticky -->
        <aside class="lg:sticky lg:top-6 lg:self-start space-y-4">
          <div class="rounded-lg border border-hairline bg-canvas p-6">
            <div class="flex items-center gap-2">
              <Receipt class="h-4 w-4 text-ink-700" :stroke-width="1.75" />
              <h2 class="text-sm font-semibold text-ink-900">Ringkasan · {{ items.length }} banner</h2>
            </div>

            <ul class="mt-4 space-y-3 divide-y divide-hairline">
              <li v-for="(row, idx) in items" :key="row.key" class="pt-3 first:pt-0 text-sm">
                <div class="flex justify-between gap-2">
                  <span class="text-ink-500">Banner {{ idx + 1 }}</span>
                  <span class="text-ink-900 font-medium">{{ fmtIDR(rowSubtotal(row)) }}</span>
                </div>
                <p class="mt-0.5 text-xs text-ink-500 truncate">
                  {{ row.productDetail?.name || 'Belum dipilih' }}
                  <template v-if="row.widthCm > 0 && row.heightCm > 0">
                    <span class="text-ink-400">·</span> <span class="font-mono">{{ row.widthCm }}×{{ row.heightCm }}cm</span>
                  </template>
                  <span class="text-ink-400">·</span> {{ row.quantity }} pcs
                </p>
              </li>
            </ul>

            <div class="mt-4 flex justify-between border-t border-hairline pt-3">
              <span class="font-semibold text-ink-950">Subtotal</span>
              <span class="font-serif text-lg font-semibold text-ink-950">{{ fmtIDR(grandSubtotal) }}</span>
            </div>
            <p v-if="isKirim" class="mt-2 text-[11px] text-ink-500 leading-relaxed">
              + ongkir (dihitung admin, diinfokan via WA).
            </p>

            <!--
              type="button" + @click supaya submit tetap fire di kondisi hydration
              agresif (mis. user klik sesaat setelah SSR nyala). Form @submit.prevent
              tetap ada untuk Enter-key path.
            -->
            <button
              type="button"
              :disabled="!canSubmit"
              class="mt-5 inline-flex w-full items-center justify-center gap-2 rounded-md bg-brand-500 px-4 py-2.5 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              @click="onSubmit"
            >
              <Loader2 v-if="submitting" class="h-4 w-4 animate-spin" :stroke-width="1.75" />
              <Receipt v-else class="h-4 w-4" :stroke-width="1.75" />
              {{ submitting ? 'Mengirim…' : 'Kirim pesanan' }}
            </button>

            <p class="mt-3 text-xs text-ink-500 leading-relaxed">
              Dengan mengirim, Anda setuju admin menghubungi via WhatsApp untuk konfirmasi & pembayaran.
            </p>
          </div>
        </aside>
      </form>
    </div>
  </main>
</template>
