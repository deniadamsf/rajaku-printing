<script setup lang="ts">
/**
 * /order — Halaman order publik (§17 CTA utama).
 *
 * Flow:
 *   1. Customer pilih produk + bahan + ukuran → live quote via /catalog/quote.
 *   2. Pilih ambil (pickup / kirim + alamat).
 *   3. Pilih sumber desain (upload sendiri / minta desain).
 *   4. Isi identitas (auto-fill kalau login; guest → nama + WA).
 *   5. Submit → order dibuat status `order_masuk`, dapat resi RJK-xxxxxxxx.
 *   6. Success panel: resi + link `/lacak/:resi` + step-by-step apa berikutnya.
 *
 * Auth opsional — Bearer token auto-attach kalau ada; kalau tidak, guest path
 * pakai nama + phone. Redirect POST-nya sama.
 *
 * Design: patuh CLAUDE.md §26 (Fraunces + Inter + Lucide, brand/ink/hairline).
 */
import {
  User,
  Package,
  Truck,
  Palette as PaletteIcon,
  Receipt,
  StickyNote,
  ExternalLink,
  RotateCcw,
  CheckCircle2,
  Loader2,
} from '@lucide/vue'
import type { CatalogProduct, CatalogProductDetail, CatalogQuote } from '~/types/catalog'
import type { Order, DesignSource, MetodeAmbil } from '~/types/order'
import { ApiError } from '~/composables/useApi'

definePageMeta({ layout: 'default' })

useSeoMeta({
  title: 'Order Banner — Rajaku Printing',
  description:
    'Order banner cetak custom online. Upload desain sendiri atau minta jasa desain. Ambil di tempat atau dikirim ke alamat Anda.',
  ogTitle: 'Order Banner — Rajaku Printing',
})

const catalog = useCatalog()
const orderApi = useOrder()
const auth = useAuthStore()

// -------------------- state --------------------
const products = ref<CatalogProduct[]>([])
const productDetail = ref<CatalogProductDetail | null>(null)
const loadingProducts = ref(false)
const loadingDetail = ref(false)

const form = reactive({
  productId: '',
  materialId: '',
  widthCm: 0,
  heightCm: 0,
  quantity: 1,
  metodeAmbil: 'pickup' as MetodeAmbil,
  shippingAddress: '',
  shippingRecipientName: '',
  shippingRecipientPhone: '',
  designSource: 'upload' as DesignSource,
  designBrief: '',
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

const quote = ref<CatalogQuote | null>(null)
const quoteLoading = ref(false)
const quoteError = ref<string | null>(null)

const submitting = ref(false)
const submitError = ref<string | null>(null)
const successResult = ref<Order | null>(null)

// -------------------- data load --------------------
onMounted(async () => {
  loadingProducts.value = true
  try {
    const res = await catalog.listProducts()
    products.value = res.products
  } catch (e) {
    submitError.value = e instanceof ApiError ? e.message : 'Gagal memuat katalog'
  } finally {
    loadingProducts.value = false
  }
})

watch(
  () => form.productId,
  async (pid) => {
    productDetail.value = null
    form.materialId = ''
    form.widthCm = 0
    form.heightCm = 0
    quote.value = null
    quoteError.value = null
    if (!pid) return
    const p = products.value.find((x) => x.id === pid)
    if (!p) return
    loadingDetail.value = true
    try {
      productDetail.value = await catalog.getProduct(p.slug)
      if (productDetail.value.pricings.length === 1) {
        form.materialId = productDetail.value.pricings[0].material_id
      }
    } catch (e) {
      submitError.value = e instanceof ApiError ? e.message : 'Gagal memuat detail produk'
    } finally {
      loadingDetail.value = false
    }
  },
)

watch(
  () => form.materialId,
  (matId) => {
    const p = productDetail.value
    if (!p || !matId) return
    if (p.pricing_type === 'paket') {
      const row = p.pricings.find((r) => r.material_id === matId)
      if (row?.width_cm) form.widthCm = row.width_cm
      if (row?.height_cm) form.heightCm = row.height_cm
    }
  },
)

let quoteTimer: ReturnType<typeof setTimeout> | null = null
function scheduleQuote() {
  if (quoteTimer) clearTimeout(quoteTimer)
  quoteTimer = setTimeout(runQuote, 400)
}

async function runQuote() {
  const p = productDetail.value
  if (!p || !form.materialId || form.widthCm <= 0 || form.heightCm <= 0) {
    quote.value = null
    quoteError.value = null
    return
  }
  quoteLoading.value = true
  quoteError.value = null
  try {
    quote.value = await catalog.quote({
      product_id: p.id,
      material_id: form.materialId,
      width_cm: form.widthCm,
      height_cm: form.heightCm,
    })
  } catch (e) {
    quote.value = null
    quoteError.value = e instanceof ApiError ? e.message : 'Gagal menghitung harga'
  } finally {
    quoteLoading.value = false
  }
}

watch(() => [form.materialId, form.widthCm, form.heightCm], scheduleQuote)

// -------------------- derived --------------------
const isPerM2 = computed(() => productDetail.value?.pricing_type === 'per_m2')
const isPaket = computed(() => productDetail.value?.pricing_type === 'paket')
const isKirim = computed(() => form.metodeAmbil === 'kirim')

const subtotal = computed(() => (quote.value ? quote.value.total_price * form.quantity : 0))

const availableMaterials = computed(() => {
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

const canSubmit = computed(() => {
  if (submitting.value) return false
  if (!form.productId || !form.materialId) return false
  if (form.widthCm <= 0 || form.heightCm <= 0 || form.quantity < 1) return false
  if (!quote.value) return false
  if (isKirim.value) {
    if (!form.shippingAddress.trim()) return false
    if (!form.shippingRecipientName.trim()) return false
    if (!form.shippingRecipientPhone.trim()) return false
  }
  if (form.designSource === 'request' && !form.designBrief.trim()) return false
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
      product_id: form.productId,
      material_id: form.materialId,
      width_cm: form.widthCm,
      height_cm: form.heightCm,
      quantity: form.quantity,
      metode_ambil: form.metodeAmbil,
      shipping_address: isKirim.value ? form.shippingAddress.trim() : undefined,
      shipping_recipient_name: isKirim.value ? form.shippingRecipientName.trim() : undefined,
      shipping_recipient_phone: isKirim.value ? form.shippingRecipientPhone.trim() : undefined,
      design_source: form.designSource,
      design_brief: form.designBrief.trim() || undefined,
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
  form.productId = ''
  form.materialId = ''
  form.widthCm = 0
  form.heightCm = 0
  form.quantity = 1
  form.metodeAmbil = 'pickup'
  form.shippingAddress = ''
  form.shippingRecipientName = ''
  form.shippingRecipientPhone = ''
  form.designSource = 'upload'
  form.designBrief = ''
  form.notes = ''
  productDetail.value = null
  quote.value = null
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
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2">Ringkasan order</p>
        <div class="grid gap-x-6 gap-y-2 text-sm md:grid-cols-2">
          <div class="flex justify-between md:justify-start md:gap-2">
            <span class="text-ink-500">Produk</span>
            <span class="text-ink-900">{{ successResult.product_name }}</span>
          </div>
          <div class="flex justify-between md:justify-start md:gap-2">
            <span class="text-ink-500">Bahan</span>
            <span class="text-ink-900">{{ successResult.material_name }}</span>
          </div>
          <div class="flex justify-between md:justify-start md:gap-2">
            <span class="text-ink-500">Ukuran</span>
            <span class="text-ink-900 font-mono text-xs">{{ successResult.width_cm }} × {{ successResult.height_cm }} cm</span>
          </div>
          <div class="flex justify-between md:justify-start md:gap-2">
            <span class="text-ink-500">Kuantitas</span>
            <span class="text-ink-900">{{ successResult.quantity }} pcs</span>
          </div>
          <div class="flex justify-between md:justify-start md:gap-2">
            <span class="text-ink-500">Ambil</span>
            <span class="text-ink-900 capitalize">{{ successResult.metode_ambil }}</span>
          </div>
          <div class="flex justify-between md:justify-start md:gap-2">
            <span class="text-ink-500">Subtotal</span>
            <span class="font-serif text-lg font-semibold text-ink-950">{{ fmtIDR(successResult.subtotal) }}</span>
          </div>
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
            <span v-if="successResult.design_source === 'request'">
              Tim desainer kami akan kirim draft untuk approval Anda sebelum masuk cetak.
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
          Pilih bahan, tentukan ukuran, hitung harga langsung. Tim kami hubungi via WA setelah order masuk —
          bisa bayar transfer atau QRIS, upload desain kapan pun setelah pesanan tercatat.
        </p>
      </div>

      <form class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_340px]" @submit.prevent="onSubmit">
        <div class="space-y-6">
          <AlertMessage v-if="submitError" variant="error" :message="submitError" />

          <!-- Produk & ukuran -->
          <fieldset class="rounded-lg border border-hairline bg-canvas p-6">
            <legend class="flex items-center gap-2 px-2 -ml-2 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
              <Package class="h-3.5 w-3.5" :stroke-width="1.75" />
              Produk & Ukuran
            </legend>

            <div class="grid gap-4 sm:grid-cols-2">
              <div>
                <label for="ord-product" class="block text-sm font-medium text-ink-900">
                  Produk <span class="text-brand-500">*</span>
                </label>
                <select
                  id="ord-product"
                  v-model="form.productId"
                  required
                  :disabled="loadingProducts"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:bg-canvas-alt disabled:text-ink-500"
                >
                  <option value="">{{ loadingProducts ? 'Memuat…' : 'Pilih produk' }}</option>
                  <option v-for="p in products" :key="p.id" :value="p.id">
                    {{ p.name }}
                  </option>
                </select>
                <p v-if="productDetail?.description" class="mt-1 text-xs text-ink-500 leading-relaxed">{{ productDetail.description }}</p>
              </div>

              <div>
                <label for="ord-material" class="block text-sm font-medium text-ink-900">
                  Bahan <span class="text-brand-500">*</span>
                </label>
                <select
                  id="ord-material"
                  v-model="form.materialId"
                  required
                  :disabled="!productDetail || loadingDetail"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:bg-canvas-alt disabled:text-ink-500"
                >
                  <option value="">{{ loadingDetail ? 'Memuat…' : 'Pilih bahan' }}</option>
                  <option v-for="m in availableMaterials" :key="m.id" :value="m.id">{{ m.label }}</option>
                </select>
                <p v-if="isPaket" class="mt-1 text-xs text-ink-500">Ukuran otomatis mengikuti paket terpilih.</p>
                <p v-else-if="isPerM2 && productDetail" class="mt-1 text-xs text-ink-500">
                  <template v-if="productDetail.min_width_cm && productDetail.max_width_cm">
                    Ukuran custom · {{ productDetail.min_width_cm }}–{{ productDetail.max_width_cm }} cm (W)
                    × {{ productDetail.min_height_cm }}–{{ productDetail.max_height_cm }} cm (H)
                  </template>
                </p>
              </div>
            </div>

            <div class="mt-4 grid gap-4 sm:grid-cols-3">
              <div>
                <label for="ord-width" class="block text-sm font-medium text-ink-900">
                  Lebar (cm) <span class="text-brand-500">*</span>
                </label>
                <input
                  id="ord-width"
                  v-model.number="form.widthCm"
                  type="number"
                  min="1"
                  required
                  :disabled="isPaket"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:bg-canvas-alt disabled:text-ink-500"
                >
              </div>
              <div>
                <label for="ord-height" class="block text-sm font-medium text-ink-900">
                  Tinggi (cm) <span class="text-brand-500">*</span>
                </label>
                <input
                  id="ord-height"
                  v-model.number="form.heightCm"
                  type="number"
                  min="1"
                  required
                  :disabled="isPaket"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:bg-canvas-alt disabled:text-ink-500"
                >
              </div>
              <div>
                <label for="ord-qty" class="block text-sm font-medium text-ink-900">
                  Kuantitas <span class="text-brand-500">*</span>
                </label>
                <input
                  id="ord-qty"
                  v-model.number="form.quantity"
                  type="number"
                  min="1"
                  required
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                >
              </div>
            </div>
          </fieldset>

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

          <!-- Design -->
          <fieldset class="rounded-lg border border-hairline bg-canvas p-6">
            <legend class="flex items-center gap-2 px-2 -ml-2 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
              <PaletteIcon class="h-3.5 w-3.5" :stroke-width="1.75" />
              Desain
            </legend>

            <div class="grid gap-2 sm:grid-cols-2">
              <label
                :class="[
                  'flex cursor-pointer items-start gap-2 rounded-md border p-3 text-sm transition-colors',
                  form.designSource === 'upload'
                    ? 'border-brand-500 bg-brand-50/50 text-ink-950'
                    : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300',
                ]"
              >
                <input v-model="form.designSource" type="radio" value="upload" class="mt-0.5 accent-brand-500">
                <span>
                  <span class="block font-semibold">Saya sudah punya desain</span>
                  <span class="mt-0.5 block text-xs text-ink-500">Upload file (CDR/AI/PDF/JPG/PNG) via WA setelah order tercatat.</span>
                </span>
              </label>
              <label
                :class="[
                  'flex cursor-pointer items-start gap-2 rounded-md border p-3 text-sm transition-colors',
                  form.designSource === 'request'
                    ? 'border-brand-500 bg-brand-50/50 text-ink-950'
                    : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300',
                ]"
              >
                <input v-model="form.designSource" type="radio" value="request" class="mt-0.5 accent-brand-500">
                <span>
                  <span class="block font-semibold">Minta jasa desain</span>
                  <span class="mt-0.5 block text-xs text-ink-500">Tim desainer buat draft, Anda approve dulu sebelum cetak.</span>
                </span>
              </label>
            </div>

            <div v-if="form.designSource === 'request'" class="mt-4">
              <label for="ord-brief" class="block text-sm font-medium text-ink-900">
                Brief singkat <span class="text-brand-500">*</span>
              </label>
              <textarea
                id="ord-brief"
                v-model="form.designBrief"
                rows="3"
                required
                placeholder="Contoh: Banner ucapan syukuran, warna dominan biru, teks 'Aqiqah Naura'. Logo boleh saya kirim via WA nanti."
                class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
              />
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
              Catatan (opsional)
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
              <h2 class="text-sm font-semibold text-ink-900">Ringkasan</h2>
            </div>

            <dl class="mt-4 space-y-2 text-sm">
              <div class="flex justify-between">
                <dt class="text-ink-500">Produk</dt>
                <dd class="text-ink-900 text-right max-w-[60%] truncate">{{ productDetail?.name || '—' }}</dd>
              </div>
              <div class="flex justify-between">
                <dt class="text-ink-500">Bahan</dt>
                <dd class="text-ink-900 text-right max-w-[60%] truncate">{{ quote?.material_name || '—' }}</dd>
              </div>
              <div class="flex justify-between">
                <dt class="text-ink-500">Ukuran</dt>
                <dd class="text-ink-900 font-mono text-xs">
                  <template v-if="form.widthCm > 0 && form.heightCm > 0">
                    {{ form.widthCm }} × {{ form.heightCm }} cm
                  </template>
                  <template v-else>—</template>
                </dd>
              </div>
              <div class="flex justify-between">
                <dt class="text-ink-500">Kuantitas</dt>
                <dd class="text-ink-900">{{ form.quantity }} pcs</dd>
              </div>
              <div v-if="quote?.area_m2 != null" class="flex justify-between">
                <dt class="text-ink-500">Luas / pcs</dt>
                <dd class="text-ink-900 font-mono text-xs">{{ quote.area_m2.toFixed(2) }} m²</dd>
              </div>
              <div class="flex justify-between border-t border-hairline pt-2 mt-1">
                <dt class="text-ink-500">Harga / pcs</dt>
                <dd class="text-ink-900">
                  <span v-if="quoteLoading" class="inline-flex items-center text-ink-500">
                    <Loader2 class="h-3 w-3 animate-spin" :stroke-width="1.75" />
                  </span>
                  <span v-else>{{ fmtIDR(quote?.total_price) }}</span>
                </dd>
              </div>
              <div class="flex justify-between border-t border-hairline pt-3 mt-1">
                <dt class="font-semibold text-ink-950">Subtotal</dt>
                <dd class="font-serif text-lg font-semibold text-ink-950">{{ fmtIDR(subtotal) }}</dd>
              </div>
            </dl>
            <p v-if="isKirim" class="mt-2 text-[11px] text-ink-500 leading-relaxed">
              + ongkir (dihitung admin, diinfokan via WA).
            </p>
            <p v-if="quoteError" class="mt-3 text-xs text-brand-700">{{ quoteError }}</p>

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

            <p class="mt-3 text-[11px] text-ink-500 leading-relaxed">
              Dengan mengirim, Anda setuju admin menghubungi via WhatsApp untuk konfirmasi & pembayaran.
            </p>
          </div>
        </aside>
      </form>
    </div>
  </main>
</template>
