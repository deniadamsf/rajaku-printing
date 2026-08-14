<script setup lang="ts">
/**
 * /admin/pos — Form kasir walk-in (§11).
 *
 * Flow ringkas:
 *   1. Kasir input nama + WA pelanggan (auto-normalize ke 62xxx di backend).
 *   2. Pilih produk & bahan → dimensi (per_m2 custom / paket fixed).
 *   3. Kalkulator harga live via /catalog/quote (debounced).
 *   4. Pilih metode ambil (pickup/kirim) + metode bayar (cash/qris_pos).
 *   5. Submit → backend resolve/create customer, buat order status "dibayar",
 *      trigger WA + invoice async, kembalikan resi + tracking URL.
 *   6. Panel sukses: cetak struk (window.print) + link invoice + reset form.
 *
 * Design: patuh CLAUDE.md §26 (brand crimson + ink warm neutral + Lucide monoline).
 */
import {
  User,
  Package,
  Truck,
  Palette as PaletteIcon,
  Wallet,
  StickyNote,
  Receipt,
  Printer,
  RotateCcw,
  ExternalLink,
  CheckCircle2,
  Loader2,
} from '@lucide/vue'
import type { CatalogProduct, CatalogProductDetail, CatalogQuote } from '~/types/catalog'
import type {
  PosCreateOrderResult,
  PosDesignApprovalMode,
  PosDesignSource,
  PosMetodeAmbil,
  PosMetodeBayar,
} from '~/types/pos'
import { ApiError } from '~/composables/useApi'

definePageMeta({
  middleware: ['staff-only'],
  layout: 'admin',
})

useSeoMeta({ title: 'POS / Kasir — Rajaku Admin' })

const catalog = useCatalog()
const pos = usePos()

// -------------------- state --------------------
const products = ref<CatalogProduct[]>([])
const productDetail = ref<CatalogProductDetail | null>(null)
const loadingProducts = ref(false)
const loadingDetail = ref(false)

const form = reactive({
  customerName: '',
  customerPhone: '',
  productId: '',
  materialId: '',
  widthCm: 0,
  heightCm: 0,
  quantity: 1,
  metodeAmbil: 'pickup' as PosMetodeAmbil,
  shippingAddress: '',
  shippingRecipientName: '',
  shippingRecipientPhone: '',
  shippingCost: 0,
  metodeBayar: 'cash' as PosMetodeBayar,
  designSource: 'upload' as PosDesignSource,
  designApprovalMode: 'instant_walkin' as PosDesignApprovalMode,
  designBrief: '',
  notes: '',
})

const quote = ref<CatalogQuote | null>(null)
const quoteLoading = ref(false)
const quoteError = ref<string | null>(null)

const submitting = ref(false)
const submitError = ref<string | null>(null)
const successResult = ref<PosCreateOrderResult | null>(null)

// -------------------- data load --------------------
onMounted(async () => {
  loadingProducts.value = true
  try {
    const res = await catalog.listProducts()
    products.value = res.products
  } catch (e: unknown) {
    submitError.value = e instanceof ApiError ? e.message : 'Gagal memuat produk'
  } finally {
    loadingProducts.value = false
  }
})

watch(
  () => form.productId,
  async (slug) => {
    productDetail.value = null
    form.materialId = ''
    form.widthCm = 0
    form.heightCm = 0
    quote.value = null
    quoteError.value = null
    if (!slug) return
    // find product by id → fetch by slug (backend detail endpoint pakai slug)
    const p = products.value.find((x) => x.id === slug)
    if (!p) return
    loadingDetail.value = true
    try {
      productDetail.value = await catalog.getProduct(p.slug)
      // preselect first material if only one
      if (productDetail.value.pricings.length === 1) {
        form.materialId = productDetail.value.pricings[0].material_id
      }
      // for paket, dimensions locked from pricing row → set when material chosen
    } catch (e: unknown) {
      submitError.value = e instanceof ApiError ? e.message : 'Gagal memuat detail produk'
    } finally {
      loadingDetail.value = false
    }
  },
)

// When material changes and pricing type is paket, lock dimensions from pricing row.
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

// Quote debounce
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
  } catch (e: unknown) {
    quote.value = null
    quoteError.value = e instanceof ApiError ? e.message : 'Gagal menghitung harga'
  } finally {
    quoteLoading.value = false
  }
}

watch(
  () => [form.materialId, form.widthCm, form.heightCm],
  scheduleQuote,
)

// -------------------- derived --------------------
const isPerM2 = computed(() => productDetail.value?.pricing_type === 'per_m2')
const isPaket = computed(() => productDetail.value?.pricing_type === 'paket')
const isKirim = computed(() => form.metodeAmbil === 'kirim')

const subtotal = computed(() => (quote.value ? quote.value.total_price * form.quantity : 0))
const shippingCost = computed(() => (isKirim.value ? form.shippingCost : 0))
const grandTotal = computed(() => subtotal.value + shippingCost.value)

const canSubmit = computed(() => {
  if (submitting.value) return false
  if (!form.customerName.trim() || !form.customerPhone.trim()) return false
  if (!form.productId || !form.materialId) return false
  if (form.widthCm <= 0 || form.heightCm <= 0 || form.quantity < 1) return false
  if (!quote.value) return false
  if (isKirim.value) {
    if (!form.shippingAddress.trim()) return false
    if (!form.shippingRecipientName.trim()) return false
    if (!form.shippingRecipientPhone.trim()) return false
    if (form.shippingCost <= 0) return false
  }
  if (form.designSource === 'request' && !form.designBrief.trim()) return false
  return true
})

const availableMaterials = computed(() => {
  const p = productDetail.value
  if (!p) return []
  return p.pricings.map((r) => ({
    id: r.material_id,
    name: r.material_name,
    code: r.material_code,
    label:
      p.pricing_type === 'paket'
        ? `${r.material_name}${r.package_label ? ' · ' + r.package_label : ''}${r.price_total != null ? ' · ' + fmtIDR(r.price_total) : ''}`
        : `${r.material_name}${r.price_per_m2 != null ? ' · ' + fmtIDR(r.price_per_m2) + '/m²' : ''}`,
  }))
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

function fmtDateTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString('id-ID', {
      day: '2-digit',
      month: 'short',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    })
  } catch {
    return iso
  }
}

// -------------------- submit --------------------
async function onSubmit() {
  if (!canSubmit.value) return
  submitting.value = true
  submitError.value = null
  try {
    const res = await pos.createOrder({
      customer_name: form.customerName.trim(),
      customer_phone: form.customerPhone.trim(),
      product_id: form.productId,
      material_id: form.materialId,
      width_cm: form.widthCm,
      height_cm: form.heightCm,
      quantity: form.quantity,
      metode_ambil: form.metodeAmbil,
      shipping_address: isKirim.value ? form.shippingAddress.trim() : undefined,
      shipping_recipient_name: isKirim.value ? form.shippingRecipientName.trim() : undefined,
      shipping_recipient_phone: isKirim.value ? form.shippingRecipientPhone.trim() : undefined,
      shipping_cost: isKirim.value ? form.shippingCost : undefined,
      metode_bayar: form.metodeBayar,
      design_source: form.designSource,
      design_approval_mode: form.designApprovalMode,
      design_brief: form.designBrief.trim() || undefined,
      notes: form.notes.trim() || undefined,
    })
    successResult.value = res
  } catch (e: unknown) {
    submitError.value = e instanceof ApiError ? e.message : 'Gagal membuat order'
  } finally {
    submitting.value = false
  }
}

function resetForm() {
  successResult.value = null
  form.customerName = ''
  form.customerPhone = ''
  form.productId = ''
  form.materialId = ''
  form.widthCm = 0
  form.heightCm = 0
  form.quantity = 1
  form.metodeAmbil = 'pickup'
  form.shippingAddress = ''
  form.shippingRecipientName = ''
  form.shippingRecipientPhone = ''
  form.shippingCost = 0
  form.metodeBayar = 'cash'
  form.designSource = 'upload'
  form.designApprovalMode = 'instant_walkin'
  form.designBrief = ''
  form.notes = ''
  productDetail.value = null
  quote.value = null
  submitError.value = null
}

function printStruk() {
  window.print()
}
</script>

<template>
  <section>
    <AdminPageHeader
      title="POS / Kasir"
      subtitle="Buat order walk-in — pelanggan tidak perlu daftar. Nomor WA jadi identitas, bayar langsung di tempat."
    />

    <!-- ============================ Sukses panel ============================ -->
    <div v-if="successResult" class="space-y-6">
      <div class="rounded-lg border border-emerald-200 bg-emerald-50 p-6 print:hidden">
        <div class="flex items-start gap-3">
          <CheckCircle2 class="h-6 w-6 text-emerald-600 flex-none mt-0.5" :stroke-width="1.75" />
          <div class="flex-1">
            <h2 class="font-serif text-xl font-semibold text-emerald-900">Order berhasil dibuat</h2>
            <p class="mt-1 text-sm text-emerald-800 leading-relaxed">
              WA konfirmasi & invoice ke pelanggan dikirim otomatis di background.
              Cetak struk untuk pelanggan lalu buka form baru untuk order berikutnya.
            </p>
          </div>
        </div>
      </div>

      <!-- Struk ringkas (printable) -->
      <div id="struk" class="rounded-lg border border-hairline bg-canvas p-6 max-w-md print:border-0 print:shadow-none print:p-0">
        <div class="text-center">
          <p class="font-serif text-lg font-semibold text-ink-950">Rajaku Printing</p>
          <p class="mt-0.5 text-xs text-ink-500">Struk Order Walk-in</p>
        </div>
        <div class="mt-4 border-t border-dashed border-hairline pt-4 space-y-1.5 text-sm">
          <div class="flex justify-between">
            <span class="text-ink-500">Resi</span>
            <span class="font-mono font-semibold text-ink-950">{{ successResult.resi }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-ink-500">Tanggal</span>
            <span class="text-ink-900">{{ fmtDateTime(successResult.created_at) }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-ink-500">Ambil</span>
            <span class="text-ink-900 capitalize">{{ successResult.metode_ambil }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-ink-500">Bayar</span>
            <span class="text-ink-900 uppercase">{{ successResult.metode_bayar }}</span>
          </div>
          <div class="flex justify-between border-t border-dashed border-hairline pt-2 mt-2">
            <span class="font-semibold text-ink-950">Total</span>
            <span class="font-semibold text-ink-950">{{ fmtIDR(successResult.total) }}</span>
          </div>
        </div>
        <div class="mt-4 border-t border-dashed border-hairline pt-4 text-center">
          <p class="text-xs text-ink-500">Lacak pesanan Anda:</p>
          <p class="mt-1 font-mono text-xs text-ink-700 break-all">{{ successResult.tracking_url }}</p>
        </div>
        <p class="mt-4 text-center text-xs text-ink-500">Terima kasih 🙏</p>
      </div>

      <div class="flex flex-wrap gap-2 print:hidden">
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors"
          @click="printStruk"
        >
          <Printer class="h-4 w-4" :stroke-width="1.75" />
          Cetak struk
        </button>
        <a
          v-if="successResult.invoice_url"
          :href="successResult.invoice_url"
          target="_blank"
          rel="noopener"
          class="inline-flex items-center gap-2 rounded-md border border-hairline bg-canvas px-4 py-2 text-sm font-semibold text-ink-900 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
        >
          <ExternalLink class="h-4 w-4" :stroke-width="1.75" />
          Invoice PDF
        </a>
        <NuxtLink
          :to="`/admin/order/${successResult.resi}`"
          class="inline-flex items-center gap-2 rounded-md border border-hairline bg-canvas px-4 py-2 text-sm font-semibold text-ink-900 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
        >
          <Receipt class="h-4 w-4" :stroke-width="1.75" />
          Detail order
        </NuxtLink>
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-md border border-hairline bg-canvas px-4 py-2 text-sm font-semibold text-ink-900 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
          @click="resetForm"
        >
          <RotateCcw class="h-4 w-4" :stroke-width="1.75" />
          Order baru
        </button>
      </div>
    </div>

    <!-- ============================ Form ============================ -->
    <form v-else class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_360px] print:hidden" @submit.prevent="onSubmit">
      <div class="space-y-6">
        <AlertMessage v-if="submitError" variant="error" :message="submitError" />

        <!-- Pelanggan -->
        <fieldset class="rounded-lg border border-hairline bg-canvas p-6">
          <legend class="flex items-center gap-2 px-2 -ml-2 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
            <User class="h-3.5 w-3.5" :stroke-width="1.75" />
            Pelanggan
          </legend>
          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <label for="pos-name" class="block text-sm font-medium text-ink-900">
                Nama <span class="text-brand-500">*</span>
              </label>
              <input
                id="pos-name"
                v-model="form.customerName"
                type="text"
                required
                autocomplete="off"
                placeholder="Bu Sari"
                class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
              >
            </div>
            <div>
              <label for="pos-phone" class="block text-sm font-medium text-ink-900">
                No. WhatsApp <span class="text-brand-500">*</span>
              </label>
              <input
                id="pos-phone"
                v-model="form.customerPhone"
                type="tel"
                required
                inputmode="numeric"
                autocomplete="off"
                placeholder="0812 3456 7890"
                class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
              >
              <p class="mt-1 text-xs text-ink-500">Auto-format ke <span class="font-mono">62xxx</span>. 1 nomor = 1 identitas pelanggan.</p>
            </div>
          </div>
        </fieldset>

        <!-- Produk -->
        <fieldset class="rounded-lg border border-hairline bg-canvas p-6">
          <legend class="flex items-center gap-2 px-2 -ml-2 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
            <Package class="h-3.5 w-3.5" :stroke-width="1.75" />
            Produk & Ukuran
          </legend>

          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <label for="pos-product" class="block text-sm font-medium text-ink-900">
                Produk <span class="text-brand-500">*</span>
              </label>
              <select
                id="pos-product"
                v-model="form.productId"
                required
                :disabled="loadingProducts"
                class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:bg-canvas-alt disabled:text-ink-500"
              >
                <option value="">{{ loadingProducts ? 'Memuat…' : 'Pilih produk' }}</option>
                <option v-for="p in products" :key="p.id" :value="p.id">
                  {{ p.name }} <span v-if="p.category">· {{ p.category }}</span>
                </option>
              </select>
            </div>

            <div>
              <label for="pos-material" class="block text-sm font-medium text-ink-900">
                Bahan <span class="text-brand-500">*</span>
              </label>
              <select
                id="pos-material"
                v-model="form.materialId"
                required
                :disabled="!productDetail || loadingDetail"
                class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:bg-canvas-alt disabled:text-ink-500"
              >
                <option value="">{{ loadingDetail ? 'Memuat…' : 'Pilih bahan' }}</option>
                <option v-for="m in availableMaterials" :key="m.id" :value="m.id">
                  {{ m.label }}
                </option>
              </select>
              <p v-if="isPaket" class="mt-1 text-xs text-ink-500">Ukuran otomatis mengikuti paket terpilih.</p>
              <p v-else-if="isPerM2 && productDetail" class="mt-1 text-xs text-ink-500">
                Ukuran custom · rentang
                <template v-if="productDetail.min_width_cm && productDetail.max_width_cm">
                  {{ productDetail.min_width_cm }}–{{ productDetail.max_width_cm }} cm (W)
                </template>
                ×
                <template v-if="productDetail.min_height_cm && productDetail.max_height_cm">
                  {{ productDetail.min_height_cm }}–{{ productDetail.max_height_cm }} cm (H)
                </template>
              </p>
            </div>
          </div>

          <div class="mt-4 grid gap-4 sm:grid-cols-3">
            <div>
              <label for="pos-width" class="block text-sm font-medium text-ink-900">
                Lebar (cm) <span class="text-brand-500">*</span>
              </label>
              <input
                id="pos-width"
                v-model.number="form.widthCm"
                type="number"
                min="1"
                required
                :disabled="isPaket"
                :placeholder="isPerM2 ? '100' : '—'"
                class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:bg-canvas-alt disabled:text-ink-500"
              >
            </div>
            <div>
              <label for="pos-height" class="block text-sm font-medium text-ink-900">
                Tinggi (cm) <span class="text-brand-500">*</span>
              </label>
              <input
                id="pos-height"
                v-model.number="form.heightCm"
                type="number"
                min="1"
                required
                :disabled="isPaket"
                :placeholder="isPerM2 ? '200' : '—'"
                class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:bg-canvas-alt disabled:text-ink-500"
              >
            </div>
            <div>
              <label for="pos-qty" class="block text-sm font-medium text-ink-900">
                Kuantitas <span class="text-brand-500">*</span>
              </label>
              <input
                id="pos-qty"
                v-model.number="form.quantity"
                type="number"
                min="1"
                required
                class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
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

          <div v-if="isKirim" class="mt-4 grid gap-4 sm:grid-cols-2">
            <div class="sm:col-span-2">
              <label for="pos-ship-addr" class="block text-sm font-medium text-ink-900">
                Alamat pengiriman <span class="text-brand-500">*</span>
              </label>
              <textarea
                id="pos-ship-addr"
                v-model="form.shippingAddress"
                rows="2"
                required
                placeholder="Jl. Merdeka No. 12, Trenggalek"
                class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
              />
            </div>
            <div>
              <label for="pos-ship-name" class="block text-sm font-medium text-ink-900">
                Nama penerima <span class="text-brand-500">*</span>
              </label>
              <input
                id="pos-ship-name"
                v-model="form.shippingRecipientName"
                type="text"
                required
                class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
              >
            </div>
            <div>
              <label for="pos-ship-phone" class="block text-sm font-medium text-ink-900">
                No. HP penerima <span class="text-brand-500">*</span>
              </label>
              <input
                id="pos-ship-phone"
                v-model="form.shippingRecipientPhone"
                type="tel"
                required
                inputmode="numeric"
                class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
              >
            </div>
            <div>
              <label for="pos-ship-cost" class="block text-sm font-medium text-ink-900">
                Ongkir (Rp) <span class="text-brand-500">*</span>
              </label>
              <input
                id="pos-ship-cost"
                v-model.number="form.shippingCost"
                type="number"
                min="0"
                required
                placeholder="15000"
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

          <div class="space-y-4">
            <div>
              <p class="mb-2 text-sm font-medium text-ink-900">Sumber desain</p>
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
                    <span class="block font-semibold">Bawa desain siap cetak</span>
                    <span class="mt-0.5 block text-xs text-ink-500">Kasir upload file ke order setelah dibuat.</span>
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
                    <span class="block font-semibold">Minta desain</span>
                    <span class="mt-0.5 block text-xs text-ink-500">Desainer buat di tempat / follow-up nanti.</span>
                  </span>
                </label>
              </div>
            </div>

            <div>
              <p class="mb-2 text-sm font-medium text-ink-900">Mode approval</p>
              <div class="grid gap-2 sm:grid-cols-2">
                <label
                  :class="[
                    'flex cursor-pointer items-start gap-2 rounded-md border p-3 text-sm transition-colors',
                    form.designApprovalMode === 'instant_walkin'
                      ? 'border-brand-500 bg-brand-50/50 text-ink-950'
                      : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300',
                  ]"
                >
                  <input v-model="form.designApprovalMode" type="radio" value="instant_walkin" class="mt-0.5 accent-brand-500">
                  <span>
                    <span class="block font-semibold">Approve di tempat</span>
                    <span class="mt-0.5 block text-xs text-ink-500">Skip loop notifikasi — pelanggan setuju verbal.</span>
                  </span>
                </label>
                <label
                  :class="[
                    'flex cursor-pointer items-start gap-2 rounded-md border p-3 text-sm transition-colors',
                    form.designApprovalMode === 'async_notify'
                      ? 'border-brand-500 bg-brand-50/50 text-ink-950'
                      : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300',
                  ]"
                >
                  <input v-model="form.designApprovalMode" type="radio" value="async_notify" class="mt-0.5 accent-brand-500">
                  <span>
                    <span class="block font-semibold">Follow-up via WA</span>
                    <span class="mt-0.5 block text-xs text-ink-500">Kirim approval link kalau butuh waktu lebih.</span>
                  </span>
                </label>
              </div>
            </div>

            <div v-if="form.designSource === 'request'">
              <label for="pos-brief" class="block text-sm font-medium text-ink-900">
                Brief singkat <span class="text-brand-500">*</span>
              </label>
              <textarea
                id="pos-brief"
                v-model="form.designBrief"
                rows="3"
                required
                placeholder="Contoh: Banner ucapan syukuran, warna dominan biru, teks 'Aqiqah Naura'…"
                class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
              />
            </div>
          </div>
        </fieldset>

        <!-- Payment -->
        <fieldset class="rounded-lg border border-hairline bg-canvas p-6">
          <legend class="flex items-center gap-2 px-2 -ml-2 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
            <Wallet class="h-3.5 w-3.5" :stroke-width="1.75" />
            Pembayaran
          </legend>
          <p class="mb-3 text-xs text-ink-500">
            Uang sudah di tangan kasir — order langsung berstatus <strong class="text-ink-900">dibayar</strong>, skip verifikasi.
          </p>
          <div class="grid gap-2 sm:grid-cols-2">
            <label
              :class="[
                'flex cursor-pointer items-center gap-2 rounded-md border p-3 text-sm transition-colors',
                form.metodeBayar === 'cash'
                  ? 'border-brand-500 bg-brand-50/50 text-ink-950'
                  : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300',
              ]"
            >
              <input v-model="form.metodeBayar" type="radio" value="cash" class="accent-brand-500">
              <span class="font-semibold">Cash</span>
            </label>
            <label
              :class="[
                'flex cursor-pointer items-center gap-2 rounded-md border p-3 text-sm transition-colors',
                form.metodeBayar === 'qris_pos'
                  ? 'border-brand-500 bg-brand-50/50 text-ink-950'
                  : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300',
              ]"
            >
              <input v-model="form.metodeBayar" type="radio" value="qris_pos" class="accent-brand-500">
              <span class="font-semibold">QRIS di tempat</span>
            </label>
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

      <!-- ============================ Right column: summary ============================ -->
      <aside class="lg:sticky lg:top-6 lg:self-start space-y-4">
        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <div class="flex items-center gap-2">
            <Receipt class="h-4 w-4 text-ink-700" :stroke-width="1.75" />
            <h2 class="text-sm font-semibold text-ink-900">Ringkasan</h2>
          </div>

          <dl class="mt-4 space-y-2 text-sm">
            <div class="flex justify-between">
              <dt class="text-ink-500">Produk</dt>
              <dd class="text-ink-900 text-right max-w-[60%] truncate">
                {{ productDetail?.name || '—' }}
              </dd>
            </div>
            <div class="flex justify-between">
              <dt class="text-ink-500">Bahan</dt>
              <dd class="text-ink-900 text-right max-w-[60%] truncate">
                {{ quote?.material_name || '—' }}
              </dd>
            </div>
            <div class="flex justify-between">
              <dt class="text-ink-500">Ukuran</dt>
              <dd class="text-ink-900">
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
            <div v-if="quote?.package_label" class="flex justify-between">
              <dt class="text-ink-500">Paket</dt>
              <dd class="text-ink-900">{{ quote.package_label }}</dd>
            </div>
            <div class="flex justify-between border-t border-hairline pt-2 mt-1">
              <dt class="text-ink-500">Harga / pcs</dt>
              <dd class="text-ink-900">
                <span v-if="quoteLoading" class="inline-flex items-center gap-1 text-ink-500">
                  <Loader2 class="h-3 w-3 animate-spin" :stroke-width="1.75" />
                </span>
                <span v-else>{{ fmtIDR(quote?.total_price) }}</span>
              </dd>
            </div>
            <div class="flex justify-between">
              <dt class="text-ink-500">Subtotal</dt>
              <dd class="text-ink-900">{{ fmtIDR(subtotal) }}</dd>
            </div>
            <div v-if="isKirim" class="flex justify-between">
              <dt class="text-ink-500">Ongkir</dt>
              <dd class="text-ink-900">{{ fmtIDR(shippingCost) }}</dd>
            </div>
            <div class="flex justify-between border-t border-hairline pt-3 mt-1">
              <dt class="font-semibold text-ink-950">Total</dt>
              <dd class="font-serif text-lg font-semibold text-ink-950">{{ fmtIDR(grandTotal) }}</dd>
            </div>
          </dl>

          <p v-if="quoteError" class="mt-3 text-xs text-brand-700">{{ quoteError }}</p>

          <button
            type="submit"
            :disabled="!canSubmit"
            class="mt-5 inline-flex w-full items-center justify-center gap-2 rounded-md bg-brand-500 px-4 py-2.5 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <Loader2 v-if="submitting" class="h-4 w-4 animate-spin" :stroke-width="1.75" />
            <Receipt v-else class="h-4 w-4" :stroke-width="1.75" />
            {{ submitting ? 'Memproses…' : 'Buat pesanan' }}
          </button>

          <p class="mt-3 text-[11px] text-ink-500 leading-relaxed">
            Klik <strong class="text-ink-700">Buat pesanan</strong> untuk simpan order & cetak struk.
            Pelanggan dapat notifikasi WA otomatis dengan link tracking.
          </p>
        </div>
      </aside>
    </form>
  </section>
</template>

<style scoped>
@media print {
  :global(body) {
    background: white;
  }
}
</style>
