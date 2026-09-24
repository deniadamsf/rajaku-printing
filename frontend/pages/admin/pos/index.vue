<script setup lang="ts">
/**
 * /admin/pos — Form kasir walk-in (§11, §32 Order Multi-Item).
 *
 * Flow ringkas:
 *   1. Kasir input nama + WA pelanggan (auto-normalize ke 62xxx di backend).
 *   2. Isi keranjang — 1..20 baris barang (§32.4), tiap baris: produk, bahan,
 *      dimensi (per_m2 custom / paket fixed), kuantitas, sumber desain +
 *      brief, catatan item. Kalkulator harga live per baris via /catalog/quote
 *      (debounced) — total keranjang = penjumlahan quote tiap baris, TIDAK
 *      dihitung ulang di frontend.
 *   3. Pilih metode ambil (pickup/kirim) + metode bayar (cash/qris_pos).
 *   4. (Opsional) diskon — promo master atau manual, dihitung dari SELURUH
 *      baris keranjang (§32.3).
 *   5. Submit → backend resolve/create customer, buat order status "dibayar",
 *      trigger WA + invoice async, kembalikan resi + tracking URL + rincian
 *      per item untuk struk.
 *   6. Panel sukses: cetak struk (Print Agent §29, fallback window.print) +
 *      link invoice + reset form.
 *
 * Design: patuh CLAUDE.md §26 (brand crimson + ink warm neutral + Lucide monoline).
 * Ergonomi kasir (§32 batch 2 brief): tiap baris keranjang bisa
 * dilipat/dibuka supaya layar tetap terbaca tanpa banyak menggulir saat
 * melayani pelanggan di depan konter; ringkasan total + tombol "Buat
 * pesanan" tetap sticky di kolom kanan.
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
  TicketPercent,
  Search,
  Crown,
  Plus,
  Trash2,
  ChevronDown,
} from '@lucide/vue'
import { onClickOutside } from '@vueuse/core'
import type { CatalogProduct, CatalogProductDetail, CatalogQuote } from '~/types/catalog'
import type {
  PosCreateOrderResult,
  PosDesignApprovalMode,
  PosDesignSource,
  PosMetodeAmbil,
  PosMetodeBayar,
  PosReceiptItem,
} from '~/types/pos'
import type { PosCustomerSearchResult } from '~/composables/usePos'
import type { ApplicableCartItem } from '~/composables/useDiscount'
import type { ApplicableDiscount } from '~/types/discount'
import type { ReceiptItemRow } from '~/components/admin/ReceiptStruk.vue'
import { ApiError } from '~/composables/useApi'

definePageMeta({
  middleware: ['staff-only'],
  layout: 'admin',
})

useSeoMeta({ title: 'POS / Kasir — Rajaku Admin' })

const catalog = useCatalog()
const pos = usePos()
const discountSvc = useDiscount()
const auth = useAuthStore()

/** Blok diskon (pilih promo / diskon manual) hanya untuk kasir yang punya izin — §28. */
const canApplyDiscount = computed(() => auth.hasPermission('discount.apply'))

// -------------------- state --------------------
const products = ref<CatalogProduct[]>([])
const loadingProducts = ref(false)

const form = reactive({
  customerName: '',
  customerPhone: '',
  metodeAmbil: 'pickup' as PosMetodeAmbil,
  shippingAddress: '',
  shippingRecipientName: '',
  shippingRecipientPhone: '',
  shippingCost: 0,
  metodeBayar: 'cash' as PosMetodeBayar,
  designApprovalMode: 'instant_walkin' as PosDesignApprovalMode,
  notes: '',
})

// -------------------- keranjang (§32) --------------------
const MAX_ITEMS = 20

interface PosCartRow {
  /** ID lokal stabil untuk :key & timer debounce — TIDAK dikirim ke backend. */
  key: number
  productId: string
  materialId: string
  widthCm: number
  heightCm: number
  quantity: number
  designSource: PosDesignSource
  designBrief: string
  itemNotes: string
  productDetail: CatalogProductDetail | null
  loadingDetail: boolean
  quote: CatalogQuote | null
  quoteLoading: boolean
  quoteError: string | null
  /** Ergonomi kasir — baris yang sudah lengkap bisa dilipat supaya layar tetap ringkas. */
  collapsed: boolean
}

let rowKeySeq = 0
function makeRow(): PosCartRow {
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
    itemNotes: '',
    productDetail: null,
    loadingDetail: false,
    quote: null,
    quoteLoading: false,
    quoteError: null,
    collapsed: false,
  }
}

const cart = ref<PosCartRow[]>([makeRow()])
const atMaxItems = computed(() => cart.value.length >= MAX_ITEMS)

function addRow() {
  if (atMaxItems.value) return
  cart.value.push(makeRow())
}

function removeRow(key: number) {
  // Baris pertama tidak bisa dihapus — order selalu butuh minimal 1 item.
  if (cart.value.length <= 1) return
  const timer = quoteTimers.get(key)
  if (timer) {
    clearTimeout(timer)
    quoteTimers.delete(key)
  }
  cart.value = cart.value.filter((r) => r.key !== key)
}

function toggleCollapse(row: PosCartRow) {
  row.collapsed = !row.collapsed
}

// -------------------- pencarian pelanggan (§11) --------------------
// WA jadi matching key lintas channel — kasir cari pelanggan yang sudah
// pernah order (online/walk-in) supaya tidak mengetik ulang & berisiko
// membuat identitas mendekati-duplikat. Tidak ada state "customer id
// terpilih" yang disimpan — cukup autofill nama+WA, backend tetap
// resolve/create customer berdasarkan nomor WA saat submit.
const customerSearchRoot = ref<HTMLElement | null>(null)
const customerSearchQuery = ref('')
const customerSearchResults = ref<PosCustomerSearchResult[]>([])
const customerSearchLoading = ref(false)
const customerSearchError = ref<string | null>(null)
const customerSearchOpen = ref(false)
// Pelanggan yang di-resolve dari hasil pencarian (§30.3) — id-nya dikirim ke
// discountSvc.applicable() supaya diskon audience_scope='member' ikut
// tersaring. Disimpan berpasangan dengan name/phone SAAT dipilih (bukan
// watcher terpisah) supaya perbandingan di `selectedCustomerId` di bawah
// otomatis membatalkan diri kalau kasir mengedit nama/WA manual sesudahnya —
// customer_id lama bisa jadi identitas orang lain kalau tetap dikirim.
const resolvedCustomer = ref<{ id: string; name: string; phone: string } | null>(null)
const selectedCustomerId = computed(() =>
  resolvedCustomer.value
  && resolvedCustomer.value.name === form.customerName
  && resolvedCustomer.value.phone === form.customerPhone
    ? resolvedCustomer.value.id
    : null,
)

let customerSearchTimer: ReturnType<typeof setTimeout> | null = null
function scheduleCustomerSearch() {
  if (customerSearchTimer) clearTimeout(customerSearchTimer)
  const q = customerSearchQuery.value.trim()
  if (q.length < 2) {
    customerSearchResults.value = []
    customerSearchError.value = null
    customerSearchOpen.value = false
    return
  }
  customerSearchTimer = setTimeout(runCustomerSearch, 400)
}

// Penjaga urutan request: pencarian di-debounce tapi tidak dijamin
// resolve berurutan — tanpa ini, request lama yang lambat bisa menimpa
// hasil request baru yang lebih relevan dengan isi kotak pencarian saat ini.
let customerSearchSeq = 0

async function runCustomerSearch() {
  const q = customerSearchQuery.value.trim()
  if (q.length < 2) return
  const seq = ++customerSearchSeq
  customerSearchLoading.value = true
  customerSearchError.value = null
  try {
    const results = await pos.searchCustomers(q)
    if (seq !== customerSearchSeq) return
    customerSearchResults.value = results
    customerSearchOpen.value = true
  } catch (e: unknown) {
    if (seq !== customerSearchSeq) return
    customerSearchResults.value = []
    customerSearchError.value = e instanceof ApiError ? e.message : 'Gagal mencari pelanggan'
  } finally {
    if (seq === customerSearchSeq) customerSearchLoading.value = false
  }
}

function selectCustomer(c: PosCustomerSearchResult) {
  form.customerName = c.name
  form.customerPhone = c.phone
  resolvedCustomer.value = { id: c.id, name: c.name, phone: c.phone }
  customerSearchQuery.value = ''
  customerSearchResults.value = []
  customerSearchOpen.value = false
}

onClickOutside(customerSearchRoot, () => {
  customerSearchOpen.value = false
})

// -------------------- diskon (§28, basis §32.3) --------------------
type DiscountMode = 'none' | 'master' | 'manual'
const discountMode = ref<DiscountMode>('none')
const selectedDiscountId = ref('')
const manualDiscountAmount = ref<number>(0)
const discountNote = ref('')

const applicableDiscounts = ref<ApplicableDiscount[]>([])
const discountsLoading = ref(false)
const discountsError = ref<string | null>(null)

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

// -------------------- per-baris: produk / bahan / quote --------------------
async function onProductChange(row: PosCartRow) {
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
  } catch (e: unknown) {
    submitError.value = e instanceof ApiError ? e.message : 'Gagal memuat detail produk'
  } finally {
    row.loadingDetail = false
  }
}

function onMaterialChange(row: PosCartRow) {
  const p = row.productDetail
  if (p && row.materialId && p.pricing_type === 'paket') {
    const found = p.pricings.find((r) => r.material_id === row.materialId)
    if (found?.width_cm) row.widthCm = found.width_cm
    if (found?.height_cm) row.heightCm = found.height_cm
  }
  scheduleQuote(row)
}

const quoteTimers = new Map<number, ReturnType<typeof setTimeout>>()
function scheduleQuote(row: PosCartRow) {
  const existing = quoteTimers.get(row.key)
  if (existing) clearTimeout(existing)
  quoteTimers.set(
    row.key,
    setTimeout(() => runQuote(row), 400),
  )
}

async function runQuote(row: PosCartRow) {
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
  } catch (e: unknown) {
    row.quote = null
    row.quoteError = e instanceof ApiError ? e.message : 'Gagal menghitung harga'
  } finally {
    row.quoteLoading = false
  }
}

// -------------------- derived --------------------
function isPerM2(row: PosCartRow): boolean {
  return row.productDetail?.pricing_type === 'per_m2'
}
function isPaket(row: PosCartRow): boolean {
  return row.productDetail?.pricing_type === 'paket'
}
const isKirim = computed(() => form.metodeAmbil === 'kirim')
/** Mode approval desain (§11/§32.5) hanya relevan kalau minimal satu baris minta dibuatkan. */
const anyRequestDesign = computed(() => cart.value.some((r) => r.designSource === 'request'))

function availableMaterials(row: PosCartRow) {
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

function rowSubtotal(row: PosCartRow): number {
  return row.quote ? row.quote.total_price * row.quantity : 0
}
/** Basis hitung diskon & angka yang dikirim ke backend — SELALU penjumlahan quote per baris, tidak pernah dihitung ulang di frontend (§32.2). */
const cartSubtotal = computed(() => cart.value.reduce((sum, r) => sum + rowSubtotal(r), 0))
const shippingCost = computed(() => (isKirim.value ? form.shippingCost : 0))

/**
 * Satu entri per baris keranjang yang punya produk & harga terhitung (§32.3)
 * — dikirim ke `discountSvc.applicable()` supaya diskon `applies_to='selected'`
 * tersaring berdasarkan `eligible_subtotal` yang benar, dan supaya kasir tidak
 * pernah melihat promo yang bakal ditolak backend saat submit.
 */
const cartDiscountItems = computed<ApplicableCartItem[]>(() =>
  cart.value
    .filter((r) => r.productId && rowSubtotal(r) > 0)
    .map((r) => ({
      product_id: r.productId,
      subtotal: rowSubtotal(r),
      pricing_type: r.productDetail?.pricing_type,
      chargeable_m2: (r.quote?.chargeable_m2 ?? 0) * r.quantity,
      width_cm: r.widthCm,
      height_cm: r.heightCm,
      quantity: r.quantity,
    })),
)

const selectedDiscount = computed(() =>
  applicableDiscounts.value.find((d) => d.id === selectedDiscountId.value) ?? null,
)
const discountAmount = computed(() => {
  if (!canApplyDiscount.value) return 0
  if (discountMode.value === 'master') return selectedDiscount.value?.preview_amount ?? 0
  if (discountMode.value === 'manual') return Math.min(manualDiscountAmount.value || 0, cartSubtotal.value)
  return 0
})
const discountLabelPreview = computed(() => {
  if (discountMode.value === 'master') return selectedDiscount.value?.name ?? ''
  if (discountMode.value === 'manual') return discountNote.value.trim()
  return ''
})

const grandTotal = computed(() => Math.max(0, cartSubtotal.value - discountAmount.value) + shippingCost.value)

// Ambil diskon yang berlaku setiap isi keranjang (produk/subtotal per baris)
// ATAU pelanggan terpilih berubah (debounced) — hanya kalau kasir punya izin
// & ada minimal satu baris dengan subtotal > 0. Reset pilihan diskon yang
// sedang aktif kalau daftar baru tidak lagi memuatnya — jangan biarkan kasir
// menekan "Buat Pesanan" dengan diskon yang akan ditolak backend.
let discountTimer: ReturnType<typeof setTimeout> | null = null
function scheduleDiscountFetch() {
  if (discountTimer) clearTimeout(discountTimer)
  discountTimer = setTimeout(fetchApplicableDiscounts, 400)
}
async function fetchApplicableDiscounts() {
  if (!canApplyDiscount.value || cartDiscountItems.value.length === 0) {
    applicableDiscounts.value = []
    if (discountMode.value === 'master') {
      selectedDiscountId.value = ''
      discountMode.value = 'none'
    }
    return
  }
  discountsLoading.value = true
  discountsError.value = null
  try {
    applicableDiscounts.value = await discountSvc.applicable({
      channel: 'pos',
      items: cartDiscountItems.value,
      customer_id: selectedCustomerId.value || undefined,
    })
    if (selectedDiscountId.value && !applicableDiscounts.value.some((d) => d.id === selectedDiscountId.value)) {
      selectedDiscountId.value = ''
      if (discountMode.value === 'master') discountMode.value = 'none'
    }
  } catch (e: unknown) {
    discountsError.value = e instanceof ApiError ? e.message : 'Gagal memuat diskon yang berlaku'
    applicableDiscounts.value = []
  } finally {
    discountsLoading.value = false
  }
}
watch([cartDiscountItems, selectedCustomerId], scheduleDiscountFetch, { deep: true })

function rowValid(row: PosCartRow): boolean {
  if (!row.productId || !row.materialId) return false
  if (row.widthCm <= 0 || row.heightCm <= 0 || row.quantity < 1) return false
  if (!row.quote) return false
  if (row.designSource === 'request' && !row.designBrief.trim()) return false
  return true
}

const canSubmit = computed(() => {
  if (submitting.value) return false
  if (!form.customerName.trim() || !form.customerPhone.trim()) return false
  if (cart.value.length === 0) return false
  if (!cart.value.every(rowValid)) return false
  if (isKirim.value) {
    if (!form.shippingAddress.trim()) return false
    if (!form.shippingRecipientName.trim()) return false
    if (!form.shippingRecipientPhone.trim()) return false
    if (form.shippingCost <= 0) return false
  }
  if (discountMode.value === 'master' && !selectedDiscountId.value) return false
  if (discountMode.value === 'manual') {
    if (manualDiscountAmount.value <= 0) return false
    if (!discountNote.value.trim()) return false
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
    const res = await pos.createOrder({
      customer_name: form.customerName.trim(),
      customer_phone: form.customerPhone.trim(),
      items: cart.value.map((r) => ({
        product_id: r.productId,
        material_id: r.materialId,
        width_cm: r.widthCm,
        height_cm: r.heightCm,
        quantity: r.quantity,
        design_source: r.designSource,
        design_brief: r.designSource === 'request' ? r.designBrief.trim() || undefined : undefined,
        item_notes: r.itemNotes.trim() || undefined,
      })),
      metode_ambil: form.metodeAmbil,
      shipping_address: isKirim.value ? form.shippingAddress.trim() : undefined,
      shipping_recipient_name: isKirim.value ? form.shippingRecipientName.trim() : undefined,
      shipping_recipient_phone: isKirim.value ? form.shippingRecipientPhone.trim() : undefined,
      shipping_cost: isKirim.value ? form.shippingCost : undefined,
      metode_bayar: form.metodeBayar,
      design_approval_mode: anyRequestDesign.value ? form.designApprovalMode : undefined,
      notes: form.notes.trim() || undefined,
      discount_id: discountMode.value === 'master' ? selectedDiscountId.value : undefined,
      manual_discount_amount: discountMode.value === 'manual' ? discountAmount.value : undefined,
      discount_note: discountMode.value === 'manual' ? discountNote.value.trim() : undefined,
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
  resolvedCustomer.value = null
  cart.value = [makeRow()]
  form.metodeAmbil = 'pickup'
  form.shippingAddress = ''
  form.shippingRecipientName = ''
  form.shippingRecipientPhone = ''
  form.shippingCost = 0
  form.metodeBayar = 'cash'
  form.designApprovalMode = 'instant_walkin'
  form.notes = ''
  discountMode.value = 'none'
  selectedDiscountId.value = ''
  manualDiscountAmount.value = 0
  discountNote.value = ''
  applicableDiscounts.value = []
  discountsError.value = null
  submitError.value = null
}

/**
 * Baris struk (§32.7) — backend `ReceiptItem` (`PosReceiptItem`) tidak
 * membawa `line_no` eksplisit (urutan array-nya sendiri sudah final,
 * line_no ASC), tapi `ReceiptStruk.vue` butuh field itu untuk `:key`. Diisi
 * dari posisi array, bukan dikirim ulang ke backend.
 */
const receiptItems = computed<ReceiptItemRow[]>(() =>
  (successResult.value?.items ?? []).map((it: PosReceiptItem, idx: number) => ({
    line_no: idx + 1,
    ...it,
  })),
)

// Template ref ke ReceiptStruk — jalur cadangan (window.print()) WAJIB lewat
// method yang diekspos komponen itu (`printNow()`), bukan `window.print()`
// langsung dari halaman ini. `printNow()` yang men-teleport struk ke `<body>`
// sebelum mencetak (lihat kontrak pemakaian di kepala `ReceiptStruk.vue`);
// memanggil `window.print()` di sini akan mencetak seluruh halaman POS,
// bukan struk.
const receiptRef = ref<{ printNow: () => Promise<void> } | null>(null)

const thermalPrint = useThermalPrint()
const printNotice = ref<{ variant: 'success' | 'error'; message: string } | null>(null)
let printNoticeTimer: ReturnType<typeof setTimeout> | null = null
function showPrintNotice(variant: 'success' | 'error', message: string) {
  printNotice.value = { variant, message }
  if (printNoticeTimer) clearTimeout(printNoticeTimer)
  // Notifikasi sukses cukup sekilas; peringatan (agen mati/gagal) dibiarkan
  // lebih lama supaya kasir sempat membaca sebelum lanjut ke order berikutnya.
  printNoticeTimer = setTimeout(() => (printNotice.value = null), variant === 'success' ? 3000 : 8000)
}

/**
 * Struk POS selalu ter-mount (bukan seperti halaman Detail Order yang
 * mem-mount on-demand), jadi urutannya lebih sederhana: coba Print Agent
 * lokal dulu (§ useThermalPrint), kalau tidak tersedia atau gagal → jatuh
 * balik ke `window.print()` lewat `ReceiptStruk.printNow()` seperti semula,
 * plus peringatan terlihat bahwa hasil cetaknya tidak terjamin di printer
 * thermal (§ README print-agent — window.print() merusak struk EPPOS).
 */
async function printStruk() {
  const widthMm = successResult.value?.receipt_width_mm === 80 ? 80 : 58
  if (await thermalPrint.isAgentAvailable()) {
    try {
      await thermalPrint.printViaAgent(widthMm)
      showPrintNotice('success', 'Struk terkirim ke printer.')
      return
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : 'Gagal mencetak lewat Print Agent.'
      showPrintNotice(
        'error',
        `Print Agent aktif tapi gagal mencetak (${msg}). Dialihkan ke cetak biasa — hasil bisa rusak di printer thermal.`,
      )
      await receiptRef.value?.printNow()
      return
    }
  }
  showPrintNotice(
    'error',
    'Print Agent cetak thermal tidak aktif di komputer ini. Dialihkan ke cetak biasa — hasil cetak BISA RUSAK pada printer thermal EPPOS. Jalankan print-agent.ps1 lalu coba lagi.',
  )
  await receiptRef.value?.printNow()
}
</script>

<template>
  <section>
    <AdminPageHeader
      title="POS / Kasir"
      subtitle="Buat order walk-in — pelanggan tidak perlu daftar. Nomor WA jadi identitas, bayar langsung di tempat."
      class="print:hidden"
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

      <!-- Struk ringkas (printable) — markup + CSS print + `@page` dinamis
           hidup di ReceiptStruk.vue (dipakai bersama halaman Detail Order
           untuk cetak ulang, lihat §12).

           `is-paid` hardcoded true dan itu benar di sini: order POS baru
           dibuat SETELAH kasir menerima uang tunai/QRIS di tempat, dan
           backend langsung menyetelnya ke status `dibayar` (§11). Panel ini
           cuma muncul kalau order itu sudah jadi. -->
      <AdminReceiptStruk
        ref="receiptRef"
        :resi="successResult.resi"
        :created-at="successResult.created_at"
        :customer-name="successResult.customer_name"
        :customer-phone="successResult.customer_phone"
        :items="receiptItems"
        :subtotal="successResult.subtotal"
        :shipping-cost="successResult.shipping_cost"
        :discount-amount="successResult.discount_amount"
        :discount-label="successResult.discount_label"
        :total="successResult.total"
        :metode-ambil="successResult.metode_ambil"
        :metode-bayar="successResult.metode_bayar"
        :tracking-url="successResult.tracking_url"
        :is-paid="true"
        :width-mm="successResult.receipt_width_mm"
      />

      <AlertMessage
        v-if="printNotice"
        :variant="printNotice.variant"
        :message="printNotice.message"
        class="print:hidden"
      />

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

          <div ref="customerSearchRoot" class="relative mb-4">
            <label for="pos-customer-search" class="block text-sm font-medium text-ink-900">
              Cari pelanggan (opsional)
            </label>
            <div class="relative mt-1">
              <Search class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-400" :stroke-width="1.75" />
              <input
                id="pos-customer-search"
                v-model="customerSearchQuery"
                type="text"
                autocomplete="off"
                placeholder="Nama atau nomor WA…"
                class="block w-full rounded-md border border-hairline bg-canvas py-2 pl-9 pr-9 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                @input="scheduleCustomerSearch"
                @focus="customerSearchQuery.trim().length >= 2 && (customerSearchOpen = true)"
              >
              <Loader2
                v-if="customerSearchLoading"
                class="absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 animate-spin text-ink-400"
                :stroke-width="1.75"
              />
            </div>
            <p class="mt-1 text-xs text-ink-500">
              Ketik nama atau nomor WA pelanggan yang sudah pernah order. Kosongkan untuk membuat pelanggan baru.
            </p>
            <p v-if="customerSearchError" class="mt-1 text-xs text-brand-700">{{ customerSearchError }}</p>

            <div
              v-if="customerSearchOpen && customerSearchQuery.trim().length >= 2 && !customerSearchLoading && customerSearchResults.length > 0"
              class="absolute z-10 mt-1 w-full rounded-md border border-hairline bg-canvas shadow-lg ring-1 ring-black/5"
            >
              <ul class="max-h-56 overflow-y-auto py-1">
                <li v-for="c in customerSearchResults" :key="c.id">
                  <button
                    type="button"
                    class="flex w-full flex-col items-start gap-0.5 px-3 py-2 text-left cursor-pointer hover:bg-canvas-alt transition-colors"
                    @click="selectCustomer(c)"
                  >
                    <span class="flex items-center gap-1.5">
                      <span class="text-sm font-medium text-ink-900">{{ c.name }}</span>
                      <span
                        v-if="c.membership_status === 'active'"
                        class="inline-flex items-center gap-1 rounded-full bg-gold-50 px-1.5 py-0.5 text-[10px] font-medium text-gold-900 ring-1 ring-inset ring-gold-200"
                      >
                        <Crown class="h-2.5 w-2.5" :stroke-width="1.75" />
                        Member
                      </span>
                    </span>
                    <span class="text-xs font-mono text-ink-500">{{ c.phone }}</span>
                  </button>
                </li>
              </ul>
            </div>
            <p
              v-else-if="customerSearchOpen && customerSearchQuery.trim().length >= 2 && !customerSearchLoading && customerSearchResults.length === 0"
              class="mt-1 text-xs text-ink-500"
            >
              Tidak ditemukan — akan dibuat pelanggan baru saat disimpan.
            </p>
          </div>

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

        <!-- Keranjang (§32) — kartu bertumpuk, bisa dilipat/dibuka per baris
             supaya layar tetap ringkas sambil melayani pelanggan di konter. -->
        <fieldset class="rounded-lg border border-hairline bg-canvas p-6">
          <legend class="flex items-center gap-2 px-2 -ml-2 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
            <Package class="h-3.5 w-3.5" :stroke-width="1.75" />
            Keranjang & Desain
          </legend>

          <div class="space-y-3">
            <div
              v-for="(row, idx) in cart"
              :key="row.key"
              class="rounded-md border border-hairline overflow-hidden"
            >
              <!-- Header baris — selalu terlihat, jadi kasir tetap tahu isi
                   baris tanpa membuka semua form sekaligus. -->
              <div class="flex items-center gap-2 bg-canvas-alt px-3 py-2">
                <button
                  type="button"
                  class="flex-none rounded-md p-1 text-ink-500 hover:text-ink-900 hover:bg-canvas transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas-alt"
                  :aria-label="row.collapsed ? 'Buka baris' : 'Lipat baris'"
                  @click="toggleCollapse(row)"
                >
                  <ChevronDown class="h-4 w-4 transition-transform" :class="row.collapsed ? '-rotate-90' : ''" :stroke-width="1.75" />
                </button>
                <div class="min-w-0 flex-1">
                  <p class="truncate text-sm font-semibold text-ink-900">
                    {{ idx + 1 }}. {{ row.productDetail?.name || 'Barang belum dipilih' }}
                  </p>
                  <p v-if="row.widthCm > 0 && row.heightCm > 0" class="truncate text-xs text-ink-500">
                    <span class="font-mono">{{ row.widthCm }}×{{ row.heightCm }}cm</span> · {{ row.quantity }} pcs
                  </p>
                </div>
                <span class="flex-none text-sm font-medium tabular-nums text-ink-900">
                  <Loader2 v-if="row.quoteLoading" class="inline h-3.5 w-3.5 animate-spin align-[-2px] text-ink-400" :stroke-width="1.75" />
                  <template v-else>{{ fmtIDR(rowSubtotal(row)) }}</template>
                </span>
                <button
                  v-if="idx > 0"
                  type="button"
                  class="flex-none rounded-md p-1.5 text-ink-500 hover:text-brand-600 hover:bg-brand-50 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas-alt"
                  aria-label="Hapus baris"
                  @click="removeRow(row.key)"
                >
                  <Trash2 class="h-4 w-4" :stroke-width="1.75" />
                </button>
              </div>

              <!-- Isi baris -->
              <div v-show="!row.collapsed" class="space-y-4 p-4">
                <div class="grid gap-4 sm:grid-cols-2">
                  <div>
                    <label :for="`pos-product-${row.key}`" class="block text-sm font-medium text-ink-900">
                      Produk <span class="text-brand-500">*</span>
                    </label>
                    <select
                      :id="`pos-product-${row.key}`"
                      v-model="row.productId"
                      required
                      :disabled="loadingProducts"
                      class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:bg-canvas-alt disabled:text-ink-500"
                      @change="onProductChange(row)"
                    >
                      <option value="">{{ loadingProducts ? 'Memuat…' : 'Pilih produk' }}</option>
                      <option v-for="p in products" :key="p.id" :value="p.id">
                        {{ p.name }} <span v-if="p.category">· {{ p.category }}</span>
                      </option>
                    </select>
                  </div>

                  <div>
                    <label :for="`pos-material-${row.key}`" class="block text-sm font-medium text-ink-900">
                      Bahan <span class="text-brand-500">*</span>
                    </label>
                    <select
                      :id="`pos-material-${row.key}`"
                      v-model="row.materialId"
                      required
                      :disabled="!row.productDetail || row.loadingDetail"
                      class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:bg-canvas-alt disabled:text-ink-500"
                      @change="onMaterialChange(row)"
                    >
                      <option value="">{{ row.loadingDetail ? 'Memuat…' : 'Pilih bahan' }}</option>
                      <option v-for="m in availableMaterials(row)" :key="m.id" :value="m.id">
                        {{ m.label }}
                      </option>
                    </select>
                    <p v-if="isPaket(row)" class="mt-1 text-xs text-ink-500">Ukuran otomatis mengikuti paket terpilih.</p>
                    <p v-else-if="isPerM2(row) && row.productDetail" class="mt-1 text-xs text-ink-500">
                      Ukuran custom · rentang
                      <template v-if="row.productDetail.min_width_cm && row.productDetail.max_width_cm">
                        {{ row.productDetail.min_width_cm }}–{{ row.productDetail.max_width_cm }} cm (W)
                      </template>
                      ×
                      <template v-if="row.productDetail.min_height_cm && row.productDetail.max_height_cm">
                        {{ row.productDetail.min_height_cm }}–{{ row.productDetail.max_height_cm }} cm (H)
                      </template>
                    </p>
                  </div>
                </div>

                <div class="grid gap-4 sm:grid-cols-3">
                  <div>
                    <label :for="`pos-width-${row.key}`" class="block text-sm font-medium text-ink-900">
                      Lebar (cm) <span class="text-brand-500">*</span>
                    </label>
                    <input
                      :id="`pos-width-${row.key}`"
                      v-model.number="row.widthCm"
                      type="number"
                      min="1"
                      required
                      :disabled="isPaket(row)"
                      :placeholder="isPerM2(row) ? '100' : '—'"
                      class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:bg-canvas-alt disabled:text-ink-500"
                      @input="scheduleQuote(row)"
                    >
                  </div>
                  <div>
                    <label :for="`pos-height-${row.key}`" class="block text-sm font-medium text-ink-900">
                      Tinggi (cm) <span class="text-brand-500">*</span>
                    </label>
                    <input
                      :id="`pos-height-${row.key}`"
                      v-model.number="row.heightCm"
                      type="number"
                      min="1"
                      required
                      :disabled="isPaket(row)"
                      :placeholder="isPerM2(row) ? '200' : '—'"
                      class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:bg-canvas-alt disabled:text-ink-500"
                      @input="scheduleQuote(row)"
                    >
                  </div>
                  <div>
                    <label :for="`pos-qty-${row.key}`" class="block text-sm font-medium text-ink-900">
                      Kuantitas <span class="text-brand-500">*</span>
                    </label>
                    <input
                      :id="`pos-qty-${row.key}`"
                      v-model.number="row.quantity"
                      type="number"
                      min="1"
                      required
                      class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                    >
                  </div>
                </div>

                <!-- Desain per baris (§32.5) -->
                <div>
                  <p class="mb-2 text-sm font-medium text-ink-900">Sumber desain baris ini</p>
                  <div class="grid gap-2 sm:grid-cols-2">
                    <label
                      :class="[
                        'flex cursor-pointer items-start gap-2 rounded-md border p-3 text-sm transition-colors',
                        row.designSource === 'upload'
                          ? 'border-brand-500 bg-brand-50/50 text-ink-950'
                          : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300',
                      ]"
                    >
                      <input v-model="row.designSource" type="radio" :name="`pos-design-source-${row.key}`" value="upload" class="mt-0.5 accent-brand-500">
                      <span>
                        <span class="block font-semibold">Bawa desain siap cetak</span>
                        <span class="mt-0.5 block text-xs text-ink-500">Kasir upload file ke order setelah dibuat.</span>
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
                      <input v-model="row.designSource" type="radio" :name="`pos-design-source-${row.key}`" value="request" class="mt-0.5 accent-brand-500">
                      <span>
                        <span class="block font-semibold">Minta desain</span>
                        <span class="mt-0.5 block text-xs text-ink-500">Desainer buat di tempat / follow-up nanti.</span>
                      </span>
                    </label>
                  </div>

                  <div v-if="row.designSource === 'request'" class="mt-3">
                    <label :for="`pos-brief-${row.key}`" class="block text-sm font-medium text-ink-900">
                      Brief singkat <span class="text-brand-500">*</span>
                    </label>
                    <textarea
                      :id="`pos-brief-${row.key}`"
                      v-model="row.designBrief"
                      rows="2"
                      required
                      placeholder="Contoh: Banner ucapan syukuran, warna dominan biru, teks 'Aqiqah Naura'…"
                      class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                    />
                  </div>
                </div>

                <div>
                  <label :for="`pos-item-notes-${row.key}`" class="block text-sm font-medium text-ink-900">
                    Catatan item (opsional)
                  </label>
                  <input
                    :id="`pos-item-notes-${row.key}`"
                    v-model="row.itemNotes"
                    type="text"
                    placeholder="Contoh: finishing mata ayam 4 pojok"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  >
                </div>

                <div class="flex items-center justify-between border-t border-hairline pt-3 text-sm">
                  <span class="text-ink-500">
                    <Loader2 v-if="row.quoteLoading" class="inline h-3 w-3 animate-spin align-[-1px]" :stroke-width="1.75" />
                    <template v-else-if="row.quote">{{ fmtIDR(row.quote.total_price) }} / pcs</template>
                    <template v-else>Lengkapi produk, bahan & ukuran</template>
                  </span>
                  <span class="font-serif text-base font-semibold text-ink-950">{{ fmtIDR(rowSubtotal(row)) }}</span>
                </div>
                <p v-if="row.quoteError" class="text-xs text-brand-700">{{ row.quoteError }}</p>
              </div>
            </div>
          </div>

          <div class="mt-3">
            <button
              type="button"
              :disabled="atMaxItems"
              class="inline-flex items-center gap-2 rounded-md border border-hairline bg-canvas px-4 py-2 text-sm font-semibold text-ink-900 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-50 disabled:cursor-not-allowed focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
              @click="addRow"
            >
              <Plus class="h-4 w-4" :stroke-width="1.75" />
              Tambah barang
            </button>
            <p class="mt-1.5 text-xs text-ink-500">
              {{ atMaxItems
                ? `Maksimal ${MAX_ITEMS} barang per pesanan sudah tercapai. Buat pesanan terpisah untuk barang tambahan.`
                : `Bisa ditambah sampai ${MAX_ITEMS} barang dalam satu pesanan (${cart.length}/${MAX_ITEMS}).` }}
            </p>
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

        <!-- Mode approval desain (§11, per order — tetap satu untuk seluruh keranjang) -->
        <fieldset v-if="anyRequestDesign" class="rounded-lg border border-hairline bg-canvas p-6">
          <legend class="flex items-center gap-2 px-2 -ml-2 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
            <PaletteIcon class="h-3.5 w-3.5" :stroke-width="1.75" />
            Mode approval desain
          </legend>
          <p class="-mt-1 mb-2 text-xs text-ink-500">
            Berlaku untuk seluruh pesanan — minimal satu barang di keranjang minta dibuatkan desain (§11).
          </p>
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

        <!-- Diskon (§28, §32.3) -->
        <fieldset v-if="canApplyDiscount" class="rounded-lg border border-hairline bg-canvas p-6">
          <legend class="flex items-center gap-2 px-2 -ml-2 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
            <TicketPercent class="h-3.5 w-3.5" :stroke-width="1.75" />
            Diskon (opsional)
          </legend>

          <div class="grid gap-2 sm:grid-cols-3">
            <label
              :class="[
                'flex cursor-pointer items-center gap-2 rounded-md border p-3 text-sm transition-colors',
                discountMode === 'none' ? 'border-brand-500 bg-brand-50/50 text-ink-950' : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300',
              ]"
            >
              <input v-model="discountMode" type="radio" value="none" class="accent-brand-500">
              <span class="font-semibold">Tanpa diskon</span>
            </label>
            <label
              :class="[
                'flex cursor-pointer items-center gap-2 rounded-md border p-3 text-sm transition-colors',
                discountMode === 'master' ? 'border-brand-500 bg-brand-50/50 text-ink-950' : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300',
                applicableDiscounts.length === 0 && 'opacity-50',
              ]"
            >
              <input v-model="discountMode" type="radio" value="master" class="accent-brand-500" :disabled="applicableDiscounts.length === 0">
              <span class="font-semibold">Pilih promo</span>
            </label>
            <label
              :class="[
                'flex cursor-pointer items-center gap-2 rounded-md border p-3 text-sm transition-colors',
                discountMode === 'manual' ? 'border-brand-500 bg-brand-50/50 text-ink-950' : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300',
              ]"
            >
              <input v-model="discountMode" type="radio" value="manual" class="accent-brand-500">
              <span class="font-semibold">Diskon manual</span>
            </label>
          </div>

          <p v-if="discountsLoading" class="mt-3 flex items-center gap-2 text-xs text-ink-500">
            <Loader2 class="h-3.5 w-3.5 animate-spin" :stroke-width="1.75" />
            Memeriksa promo yang berlaku…
          </p>
          <p v-else-if="discountsError" class="mt-3 text-xs text-brand-700">{{ discountsError }}</p>
          <p v-else-if="cartSubtotal > 0 && applicableDiscounts.length === 0" class="mt-3 text-xs text-ink-500">
            Tidak ada promo yang berlaku untuk keranjang ini.
          </p>

          <div v-if="discountMode === 'master'" class="mt-3 space-y-2">
            <label
              v-for="d in applicableDiscounts"
              :key="d.id"
              :class="[
                'flex cursor-pointer items-start justify-between gap-3 rounded-md border p-3 text-sm transition-colors',
                selectedDiscountId === d.id ? 'border-brand-500 bg-brand-50/50 text-ink-950' : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300',
              ]"
            >
              <span class="flex items-start gap-2">
                <input v-model="selectedDiscountId" type="radio" :value="d.id" class="mt-0.5 accent-brand-500">
                <span>
                  <span class="block font-semibold">{{ d.name }}</span>
                  <span class="mt-0.5 block font-mono text-xs text-ink-500">
                    {{ d.code }} · {{ d.type === 'percent' ? `${d.value_percent}%` : d.type === 'nominal_per_m2' ? `${fmtIDR(d.value_amount)}/m²` : fmtIDR(d.value_amount) }}
                  </span>
                </span>
              </span>
              <span class="shrink-0 font-medium text-brand-600">-{{ fmtIDR(d.preview_amount) }}</span>
            </label>
          </div>

          <div v-if="discountMode === 'manual'" class="mt-3 grid gap-4 sm:grid-cols-2">
            <div>
              <label for="pos-discount-amount" class="block text-sm font-medium text-ink-900">
                Nominal diskon (Rp) <span class="text-brand-500">*</span>
              </label>
              <input
                id="pos-discount-amount"
                v-model.number="manualDiscountAmount"
                type="number"
                min="1"
                :max="cartSubtotal"
                required
                placeholder="10000"
                class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
              >
            </div>
            <div>
              <label for="pos-discount-note" class="block text-sm font-medium text-ink-900">
                Alasan <span class="text-brand-500">*</span>
              </label>
              <input
                id="pos-discount-note"
                v-model="discountNote"
                type="text"
                required
                placeholder="Contoh: kompensasi keterlambatan"
                class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
              >
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
            placeholder="Contoh: ambil sore hari"
            class="block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
          />
        </fieldset>
      </div>

      <!-- ============================ Right column: summary ============================ -->
      <aside class="lg:sticky lg:top-6 lg:self-start space-y-4">
        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <div class="flex items-center gap-2">
            <Receipt class="h-4 w-4 text-ink-700" :stroke-width="1.75" />
            <h2 class="text-sm font-semibold text-ink-900">Ringkasan · {{ cart.length }} barang</h2>
          </div>

          <ul class="mt-4 max-h-56 space-y-3 divide-y divide-hairline overflow-y-auto pr-1">
            <li v-for="(row, idx) in cart" :key="row.key" class="pt-3 first:pt-0 text-sm">
              <div class="flex justify-between gap-2">
                <span class="text-ink-500">Barang {{ idx + 1 }}</span>
                <span class="text-ink-900 font-medium">{{ fmtIDR(rowSubtotal(row)) }}</span>
              </div>
              <p class="mt-0.5 truncate text-xs text-ink-500">
                {{ row.productDetail?.name || 'Belum dipilih' }}
                <template v-if="row.widthCm > 0 && row.heightCm > 0">
                  <span class="text-ink-400">·</span> <span class="font-mono">{{ row.widthCm }}×{{ row.heightCm }}cm</span>
                </template>
                <span class="text-ink-400">·</span> {{ row.quantity }} pcs
              </p>
            </li>
          </ul>

          <dl class="mt-4 space-y-2 border-t border-hairline pt-3 text-sm">
            <div class="flex justify-between">
              <dt class="text-ink-500">Subtotal</dt>
              <dd class="text-ink-900">{{ fmtIDR(cartSubtotal) }}</dd>
            </div>
            <div v-if="discountAmount > 0" class="flex justify-between">
              <dt class="text-ink-500">
                Diskon
                <span v-if="discountLabelPreview" class="block max-w-[10rem] truncate text-xs text-ink-400">{{ discountLabelPreview }}</span>
              </dt>
              <dd class="font-medium text-brand-600">-{{ fmtIDR(discountAmount) }}</dd>
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

          <button
            type="submit"
            :disabled="!canSubmit"
            class="mt-5 inline-flex w-full items-center justify-center gap-2 rounded-md bg-brand-500 px-4 py-2.5 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <Loader2 v-if="submitting" class="h-4 w-4 animate-spin" :stroke-width="1.75" />
            <Receipt v-else class="h-4 w-4" :stroke-width="1.75" />
            {{ submitting ? 'Memproses…' : 'Buat pesanan' }}
          </button>

          <p class="mt-3 text-xs text-ink-500 leading-relaxed">
            Klik <strong class="text-ink-700">Buat pesanan</strong> untuk simpan order & cetak struk.
            Pelanggan dapat notifikasi WA otomatis dengan link tracking.
          </p>
        </div>
      </aside>
    </form>
  </section>
</template>

<!-- CSS print & isolasi #struk sekarang hidup di components/admin/ReceiptStruk.vue
     (dipakai bersama halaman Detail Order untuk cetak ulang, §12). -->
