<script setup lang="ts">
/**
 * /admin/order/[resi] — Detail order + operational sidebar untuk staff (§4/§10).
 *
 * Layout 2-kolom:
 *   - Kiri (2/3): Header, Timeline (riwayat status), Produk, Nominal, Customer,
 *     Alamat/Pickup, Desain (+ files list), Pembayaran (+ proofs list), Notes.
 *   - Kanan (1/3, sticky): Set ongkir (kirim), Confirm pickup, Cross-module
 *     shortcut, Meta.
 *
 * Data:
 *   - Utama: `GET /admin/orders/:resi` → order + customer + history rows.
 *   - Payment proofs: `GET /admin/payment-proofs?order_id=…` (via usePayment).
 *   - Design files:   `GET /orders/:resi/design-files` (via useDesign).
 *
 * Design: patuh CLAUDE.md §26 (Fraunces + Inter + Lucide, brand/ink/hairline).
 */
import type { Order, OrderStatus, DesignSource } from '~/types/order'
import type { PaymentProof } from '~/types/payment'
import type { DesignFile } from '~/types/design'
import type { AuditLogEntry } from '~/types/audit'
import type { CatalogProduct, CatalogProductDetail, CatalogQuote } from '~/types/catalog'
import type {
  AdminOrderCustomer,
  AdminOrderHistoryRow,
  AdminOrderEditInput,
  AdminOrderEditItemInput,
} from '~/composables/useOrder'
import { ApiError } from '~/composables/useApi'
import {
  CreditCard,
  Palette,
  Printer,
  XCircle,
  User,
  Package as PackageIcon,
  Truck,
  Wallet,
  Image as ImageIcon,
  FileText,
  Copy,
  Check,
  CircleDot,
  Circle,
  CheckCircle2,
  Loader2,
  Phone,
  Pencil,
  RefreshCw,
  Trash2,
  History,
  AlertTriangle,
  Plus,
} from '@lucide/vue'

definePageMeta({
  middleware: ['staff-only'],
  layout: 'admin',
})

const route = useRoute()
const config = useRuntimeConfig()
const auth = useAuthStore()
const orderSvc = useOrder()
const paymentSvc = usePayment()
const designSvc = useDesign()
const pos = usePos()
const auditSvc = useAuditLog()
const catalog = useCatalog()

const resi = computed(() => String(route.params.resi))

// -------------------- state --------------------
const order = ref<Order | null>(null)
const customer = ref<AdminOrderCustomer | null>(null)
const history = ref<AdminOrderHistoryRow[]>([])
const loading = ref(true)

const proofs = ref<PaymentProof[]>([])
const proofsLoading = ref(false)

const designFiles = ref<DesignFile[]>([])
const designLoading = ref(false)

const auditLog = ref<AuditLogEntry[]>([])
const auditLoading = ref(false)

const errorMsg = ref<string | null>(null)
const successMsg = ref<string | null>(null)

/**
 * Error milik modal yang sedang terbuka (cancel/edit/override/hapus). Dipisah
 * dari `errorMsg` (banner halaman) karena banner ada di belakang overlay
 * modal — kegagalan submit jadi tidak terlihat sampai modal ditutup manual.
 * Halaman ini punya 4 modal tapi cuma satu yang bisa terbuka pada satu waktu,
 * jadi cukup satu ref. Aksi NON-modal (set ongkir, confirm pickup, fetch
 * detail) tetap pakai `errorMsg` seperti biasa.
 */
const modalError = ref<string | null>(null)

useSeoMeta({ title: () => `Order ${resi.value} — Admin` })

function showSuccess(msg: string) {
  successMsg.value = msg
  setTimeout(() => (successMsg.value = null), 3000)
}

/**
 * Beberapa error code backend butuh pesan yang lebih ramah daripada
 * `e.message` mentah:
 *   - `STALE_STATE`   — status pesanan berubah di tempat lain saat form ini
 *     terbuka (mis. staff lain memproses order). Jangan tampilkan pesan
 *     teknis, suruh admin muat ulang.
 *   - `DELETE_NOT_ALLOWED_PAID` — pesanan sudah dibayar, backend menolak
 *     hapus (409). Tombol hapus sudah di-disable duluan untuk kasus ini,
 *     tapi status bisa berubah di tab lain sehingga tetap perlu ditangani.
 */
function toApiError(e: unknown, fallback: string): string {
  if (e instanceof ApiError) {
    if (e.code === 'STALE_STATE') {
      return 'Status pesanan ini berubah sejak halaman dibuka (mungkin diproses staff lain). Muat ulang halaman, lalu coba lagi.'
    }
    if (e.code === 'DELETE_NOT_ALLOWED_PAID') {
      return 'Pesanan ini sudah dibayar sehingga tidak bisa dihapus. Batalkan pesanan terlebih dahulu, baru bisa dihapus.'
    }
    return e.message
  }
  return fallback
}

// -------------------- fetch --------------------
async function loadDetail() {
  loading.value = true
  errorMsg.value = null
  try {
    const res = await orderSvc.adminGetDetail(resi.value)
    order.value = res.order
    customer.value = res.customer ?? null
    history.value = res.history ?? []
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal memuat order')
    order.value = null
  } finally {
    loading.value = false
  }
}

async function loadProofs() {
  if (!order.value) return
  proofsLoading.value = true
  try {
    const res = await paymentSvc.listProofs({ orderId: order.value.id, pageSize: 50 })
    proofs.value = res.items ?? []
  } catch (e) {
    // Non-blocking — tampilkan error lokal doang.
    console.error('Gagal load proofs', e)
    proofs.value = []
  } finally {
    proofsLoading.value = false
  }
}

async function loadDesignFiles() {
  designLoading.value = true
  try {
    const res = await designSvc.listByResi(resi.value)
    designFiles.value = res.items ?? []
  } catch (e) {
    console.error('Gagal load design files', e)
    designFiles.value = []
  } finally {
    designLoading.value = false
  }
}

async function loadAuditLog() {
  if (!order.value || !auth.hasPermission('audit.view')) return
  auditLoading.value = true
  try {
    auditLog.value = await auditSvc.listByEntity(order.value.id, 50)
  } catch (e) {
    // Non-blocking — jejak audit gagal load tidak boleh menggagalkan detail order.
    console.error('Gagal load audit log', e)
    auditLog.value = []
  } finally {
    auditLoading.value = false
  }
}

onMounted(async () => {
  await loadDetail()
  if (order.value) {
    // Kick off attachments fetch in parallel — tidak menahan render.
    loadProofs()
    loadDesignFiles()
    loadAuditLog()
  }
  loadReceiptConfig()
})

// -------------------- cetak ulang struk (§12) --------------------
/**
 * Lebar kertas struk aktif, dipakai `ReceiptStruk` untuk `@page` dinamis.
 *
 * `null` = BELUM DIKETAHUI, sengaja BUKAN default diam-diam ke 58mm. Kalau
 * request config ini gagal dan kita tebak 58mm, staff bisa mencetak struk di
 * lebar kertas yang SALAH tanpa peringatan apa pun — dan kertas thermal yang
 * sudah tercetak tidak bisa ditarik kembali (§22 CLAUDE.md: dilarang gagal
 * diam-diam). Jangan "sederhanakan" ini balik jadi `ref<58 | 80>(58)` —
 * itu justru bug yang sedang diperbaiki. Selama nilainya `null`, tombol
 * "Cetak struk" DIHARUSKAN disabled (lihat template) dan `printStruk()`
 * punya jaring pengaman `return` kalau tetap terpicu.
 */
const receiptWidthMm = ref<58 | 80 | null>(null)
// true kalau request (+ satu kali retry) gagal — dipakai menampilkan pesan
// & tombol "Coba lagi" di dekat tombol cetak, bukan cuma console.error yang
// tak pernah dibaca kasir.
const receiptConfigFailed = ref(false)

async function loadReceiptConfig() {
  receiptConfigFailed.value = false
  try {
    const cfg = await pos.receiptConfig()
    receiptWidthMm.value = cfg.width_mm === 80 ? 80 : 58
    return
  } catch (e) {
    console.error('Gagal memuat receipt-config, mencoba ulang sekali…', e)
  }
  // Kegagalan jaringan sesaat tidak perlu langsung memblokir kasir — coba
  // sekali lagi setelah jeda singkat sebelum benar-benar menyerah.
  await new Promise((resolve) => setTimeout(resolve, 800))
  try {
    const cfg = await pos.receiptConfig()
    receiptWidthMm.value = cfg.width_mm === 80 ? 80 : 58
  } catch (e) {
    console.error('Gagal memuat receipt-config setelah retry — cetak struk dinonaktifkan', e)
    receiptConfigFailed.value = true
  }
}

/**
 * Tracking URL dibangun dari `appBaseUrl` terpusat (CLAUDE.md §2) — pola
 * sama dengan `layouts/default.vue`/`pages/tentang-kami.vue`, bukan
 * konstanta baru.
 */
const trackingUrl = computed(() =>
  order.value ? `${config.public.appBaseUrl.replace(/\/$/, '')}/lacak/${order.value.resi}` : '',
)

/**
 * Struk tidak dirender di layar — semua datanya (nama pelanggan, produk,
 * nominal) sudah tampil di section lain halaman ini, jadi preview duplikat
 * cuma menambah scroll tanpa info baru.
 *
 * `receiptMounted` ADA supaya struk hanya ter-mount selama proses cetak
 * struk berlangsung, bukan sepanjang halaman terbuka. Alasannya CSS print di
 * ReceiptStruk.vue menyembunyikan SELURUH isi halaman (`body *`) tanpa
 * syarat dan memaksa `@page` jadi 58/80mm. Kalau komponen itu ter-mount
 * permanen, staff yang menekan Ctrl+P untuk mencetak lembar detail order
 * (mis. sebagai surat jalan produksi) akan diam-diam mendapat struk thermal
 * di ukuran kertas yang salah — mereka tidak menekan tombol apa pun yang
 * meminta struk. Dengan gerbang ini, Ctrl+P biasa tetap mencetak halaman.
 */
const receiptMounted = ref(false)

/**
 * Status di mana uangnya SUDAH benar-benar diterima — menentukan stempel
 * LUNAS / BELUM LUNAS di struk.
 *
 * Tombol "Cetak struk" di halaman ini sengaja tersedia untuk order status apa
 * pun (staff kadang perlu lembar rincian sebelum bayar), jadi struk TIDAK
 * boleh mengasumsikan pesanan sudah lunas. Order online yang masih
 * `menunggu_pembayaran` lalu dicetak bertuliskan LUNAS = bukti bayar palsu.
 *
 * Daftar ini mengikuti §4: `dibayar` dan seluruh status sesudahnya. Sengaja
 * TIDAK memasukkan `ditolak` (bukti bayar ditolak), `dibatalkan` (bisa sudah
 * direfund), dan semua status sebelum pembayaran diverifikasi.
 */
const PAID_STATUSES = new Set([
  'dibayar',
  'desain_dikerjakan',
  'menunggu_approval_desain',
  'desain_diverifikasi',
  'proses_cetak',
  'qc',
  'siap_kirim',
  'siap_ambil',
  'dikirim',
  'selesai',
])
const orderIsPaid = computed(() => !!order.value && PAID_STATUSES.has(order.value.status))

// Template ref ke instance ReceiptStruk — dipakai memanggil `printNow()`
// (lihat kontrak pemakaian di kepala `ReceiptStruk.vue`). Cuma terisi
// selama `receiptMounted` true (component-nya memang cuma mount saat itu,
// lihat wrapper `hidden print:block` di template).
const receiptRef = ref<{ printNow: () => Promise<void> } | null>(null)

/**
 * Urutan wajib: mount komponen dulu (`receiptMounted = true`), tunggu DOM
 * ter-patch (`nextTick()`) supaya `receiptRef`/`#struk` sudah terisi, BARU
 * coba cetak. Dua jalur cetak (§ useThermalPrint.ts):
 *
 *   1. Print Agent lokal (`services/print-agent`) — jalur UTAMA untuk printer
 *      thermal EPPOS EP8081/RPP02. `window.print()` TERBUKTI merusak struk di
 *      printer itu (lihat README print-agent), jadi kalau agennya hidup, ini
 *      yang dipakai.
 *   2. `window.print()` lewat `ReceiptStruk.printNow()` — jalur CADANGAN,
 *      dipakai kalau agen mati atau gagal mencetak. `printNow()` menangani
 *      detail teknis pencetakan (teleport ke `<body>`, timing `window.print()`,
 *      lihat `ReceiptStruk.vue`) — halaman ini tidak perlu tahu-menahu lagi
 *      soal itu.
 *
 * Dulu di sini ada loop verifikasi manual (`waitForReceiptReady`, dua rAF +
 * polling `document.getElementById('struk')`) karena isolasi cetak lama
 * bergantung pada urutan `<style>` `@page` tersuntik & CSS visibility aktif
 * di waktu yang tepat — kalau meleset, browser mencetak seluruh halaman
 * admin. Sekarang tanggung jawab itu pindah ke dalam `printNow()`
 * (teleport + `nextTick()` sebelum `window.print()`), jadi cukup `nextTick()`
 * biasa di sini untuk menunggu komponennya mount.
 */
const thermalPrint = useThermalPrint()

async function printStruk() {
  // Jaring pengaman kalau tombol tetap terpicu (mis. race kondisi disabled
  // belum ter-render): lebar kertas belum diketahui = jangan cetak sama
  // sekali, daripada menebak dan salah ukuran (lihat komentar `receiptWidthMm`).
  if (receiptWidthMm.value === null) return
  const widthMm = receiptWidthMm.value
  receiptMounted.value = true
  await nextTick()
  try {
    if (await thermalPrint.isAgentAvailable()) {
      try {
        await thermalPrint.printViaAgent(widthMm)
        showSuccess('Struk terkirim ke printer.')
        return
      } catch (e: unknown) {
        const msg = e instanceof Error ? e.message : 'Gagal mencetak lewat Print Agent.'
        errorMsg.value = `Print Agent aktif tapi gagal mencetak (${msg}). Dialihkan ke cetak biasa — hasil bisa rusak di printer thermal.`
        await receiptRef.value?.printNow()
        return
      }
    }
    errorMsg.value =
      'Print Agent cetak thermal tidak aktif di komputer ini. Dialihkan ke cetak biasa — hasil cetak BISA RUSAK pada printer thermal EPPOS. Jalankan print-agent.ps1 lalu coba lagi.'
    await receiptRef.value?.printNow()
  } finally {
    // window.print() memblokir sampai dialog ditutup di sebagian besar
    // browser, tapi tidak dijamin — `finally` + afterprint dua-duanya
    // dipasang supaya struk tidak pernah tertinggal ter-mount. Ini juga
    // menutup jalur Print Agent (return awal di atas tetap melewati blok
    // finally ini), jadi struk selalu di-unmount lagi setelah selesai.
    receiptMounted.value = false
  }
}

if (import.meta.client) {
  useEventListener(window, 'afterprint', () => {
    receiptMounted.value = false
  })
}

// -------------------- permissions --------------------
const canSetOngkir = computed(() => auth.hasPermission('shipping.set_cost'))
const canUpdateStatus = computed(() => auth.hasPermission('order.update_status'))
const canCancel = computed(() => auth.hasPermission('order.cancel'))

// Aksi super admin (edit data, override status paksa, hapus) — §26 admin override.
const canEditOrder = computed(() => auth.hasPermission('order.edit'))
const canOverrideStatus = computed(() => auth.hasPermission('order.override_status'))
const canDeleteOrder = computed(() => auth.hasPermission('order.delete'))
const canViewAudit = computed(() => auth.hasPermission('audit.view'))

// State flags (§4)
const preCetakStates = new Set([
  'order_masuk',
  'menunggu_ongkir',
  'menunggu_pembayaran',
  'menunggu_verifikasi',
  'ditolak',
  'dibayar',
  'desain_dikerjakan',
  'menunggu_approval_desain',
  'desain_diverifikasi',
])
const canCancelNow = computed(() =>
  canCancel.value && order.value !== null && preCetakStates.has(order.value.status),
)
const isMenungguOngkir = computed(() => order.value?.status === 'menunggu_ongkir')
const isOrderMasuk = computed(() => order.value?.status === 'order_masuk')
const isPickup = computed(() => order.value?.metode_ambil === 'pickup')

/*
 * Guard dua panel di kolom kanan HARUS sama persis dengan prasyarat backend —
 * kalau lebih sempit, order mandek tanpa aksi apa pun di layar (bug 7 Sep 2026:
 * keduanya digate `menunggu_ongkir` saja, padahal order baru selalu lahir di
 * `order_masuk` dan pickup TIDAK PERNAH melewati `menunggu_ongkir`, jadi kedua
 * tombolnya tidak pernah muncul).
 *
 *   SetShippingCost   — kirim  + (order_masuk | menunggu_ongkir)
 *   ConfirmPickupTotal— pickup + order_masuk saja
 */
const canFillOngkirNow = computed(
  () => (isOrderMasuk.value || isMenungguOngkir.value) && !isPickup.value && canSetOngkir.value,
)
const canConfirmPickupNow = computed(
  () => isOrderMasuk.value && isPickup.value && canUpdateStatus.value,
)

// -------------------- set ongkir --------------------
const ongkirValue = ref<number>(0)
const ongkirNote = ref('')
const ongkirBusy = ref(false)

async function submitOngkir() {
  if (ongkirValue.value < 0) return
  ongkirBusy.value = true
  errorMsg.value = null
  try {
    order.value = await orderSvc.setShippingCost(resi.value, {
      shipping_cost: ongkirValue.value,
      note: ongkirNote.value.trim() || undefined,
    })
    showSuccess(`Ongkir ${fmtIDR(ongkirValue.value)} tersimpan. Customer akan dinotifikasi via WA.`)
    ongkirNote.value = ''
    // Reload detail supaya history + status ikut update.
    await loadDetail()
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal set ongkir')
  } finally {
    ongkirBusy.value = false
  }
}

// -------------------- confirm pickup --------------------
const pickupBusy = ref(false)
async function confirmPickup() {
  pickupBusy.value = true
  errorMsg.value = null
  try {
    order.value = await orderSvc.confirmPickup(resi.value)
    showSuccess('Total pickup dikonfirmasi. Customer dinotifikasi untuk bayar.')
    await loadDetail()
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal confirm pickup')
  } finally {
    pickupBusy.value = false
  }
}

// -------------------- cancel --------------------
const cancelOpen = ref(false)
const cancelReason = ref('')
const cancelBusy = ref(false)

function openCancel() {
  cancelReason.value = ''
  modalError.value = null
  cancelOpen.value = true
}

async function submitCancel() {
  if (cancelReason.value.trim().length < 3) {
    modalError.value = 'Alasan pembatalan minimal 3 karakter.'
    return
  }
  cancelBusy.value = true
  modalError.value = null
  try {
    order.value = await orderSvc.cancelOrder(resi.value, cancelReason.value.trim())
    cancelOpen.value = false
    cancelReason.value = ''
    showSuccess('Order dibatalkan.')
    await loadDetail()
  } catch (e) {
    modalError.value = toApiError(e, 'Gagal cancel order')
  } finally {
    cancelBusy.value = false
  }
}

// -------------------- edit pesanan (super admin, order.edit) --------------------
/**
 * `shipping_recipient_name`/`phone`/`shipping_address`/`notes` selalu boleh
 * dikirim apa pun statusnya — itu data pengiriman KHUSUS pesanan ini, bukan
 * identitas global pelanggan (lihat catatan di `AdminOrderEditInput`).
 *
 * `shipping_cost` juga selalu boleh diedit, TAPI begitu order sudah
 * `dibayar` (atau status sesudahnya, lihat `orderIsPaid`), backend
 * mewajibkan `reason` (min 10 karakter) kalau nominal itu berubah — atau
 * kalau baris item diubah lewat editor di bawah (§32.9) — karena invoice
 * yang sudah terkirim ke pelanggan bisa jadi tidak cocok lagi. `total` dan
 * `subtotal` TIDAK ADA di form — keduanya selalu dihitung backend
 * (`subtotal` = Σ item, `total` = subtotal - diskon + ongkir), ditampilkan
 * read-only/perkiraan di bawah.
 */
const editOpen = ref(false)
const editBusy = ref(false)
const editForm = reactive({
  shipping_recipient_name: '',
  shipping_recipient_phone: '',
  shipping_address: '',
  notes: '',
  shipping_cost: 0,
  reason: '',
})

function openEdit() {
  if (!order.value) return
  editForm.shipping_recipient_name = order.value.shipping_recipient_name ?? ''
  editForm.shipping_recipient_phone = order.value.shipping_recipient_phone ?? ''
  editForm.shipping_address = order.value.shipping_address ?? ''
  editForm.notes = order.value.notes ?? ''
  editForm.shipping_cost = order.value.shipping_cost ?? 0
  editForm.reason = ''
  closeItemsEditor()
  modalError.value = null
  editOpen.value = true
}

// ---- Koreksi baris item (§32.9) ----
// Editor di bawah mengandalkan `item.id` asli (order_items.id, dipetakan
// backend sejak 1 September 2026 — lihat doc comment `OrderItem.id` di
// `types/order.ts`) supaya baris "existing" bisa dikirim balik ke
// `PATCH /admin/orders/:resi` sebagai EDIT, bukan disalahartikan backend
// sebagai baris BARU (yang bisa menduplikasi item).
interface EditItemRow {
  key: number
  /** Terisi = baris existing (id order_items asli). Kosong = baris baru. */
  id?: string
  productName: string
  materialName: string
  widthCm: number
  heightCm: number
  quantity: number
  unitPrice: number
  itemNotes: string
  // Baris baru saja:
  productId: string
  materialId: string
  designSource: DesignSource | ''
  designBrief: string
  productDetail: CatalogProductDetail | null
  loadingDetail: boolean
  quote: CatalogQuote | null
  quoteLoading: boolean
  quoteError: string | null
}

let editItemKeySeq = 0
function makeNewEditRow(): EditItemRow {
  editItemKeySeq += 1
  return {
    key: editItemKeySeq,
    productName: '',
    materialName: '',
    widthCm: 0,
    heightCm: 0,
    quantity: 1,
    unitPrice: 0,
    itemNotes: '',
    productId: '',
    materialId: '',
    designSource: '',
    designBrief: '',
    productDetail: null,
    loadingDetail: false,
    quote: null,
    quoteLoading: false,
    quoteError: null,
  }
}

const itemsEditMode = ref(false)
const editItems = ref<EditItemRow[]>([])
const editProducts = ref<CatalogProduct[]>([])
const editProductsLoading = ref(false)

async function openItemsEditor() {
  if (!order.value) return
  editItemKeySeq = 0
  editItems.value = order.value.items.map((it) => ({
    ...makeNewEditRow(),
    id: it.id,
    productName: it.product_name,
    materialName: it.material_name,
    widthCm: it.width_cm,
    heightCm: it.height_cm,
    quantity: it.quantity,
    unitPrice: it.unit_price,
    itemNotes: it.item_notes ?? '',
  }))
  itemsEditMode.value = true
  if (editProducts.value.length === 0) {
    editProductsLoading.value = true
    try {
      const res = await catalog.listProducts()
      editProducts.value = res.products
    } catch (e) {
      modalError.value = toApiError(e, 'Gagal memuat daftar produk')
    } finally {
      editProductsLoading.value = false
    }
  }
}

function closeItemsEditor() {
  itemsEditMode.value = false
  editItems.value = []
}

const MAX_EDIT_ITEMS = 20
function addEditItemRow() {
  if (editItems.value.length >= MAX_EDIT_ITEMS) return
  editItems.value.push(makeNewEditRow())
}
function removeEditItemRow(key: number) {
  if (editItems.value.length <= 1) return
  editItems.value = editItems.value.filter((r) => r.key !== key)
}

async function onEditRowProductChange(row: EditItemRow) {
  row.productDetail = null
  row.materialId = ''
  row.widthCm = 0
  row.heightCm = 0
  row.quote = null
  row.quoteError = null
  if (!row.productId) return
  const p = editProducts.value.find((x) => x.id === row.productId)
  if (!p) return
  row.loadingDetail = true
  try {
    row.productDetail = await catalog.getProduct(p.slug)
    if (row.productDetail.pricings.length === 1) {
      row.materialId = row.productDetail.pricings[0].material_id
      onEditRowMaterialChange(row)
    }
  } catch (e) {
    modalError.value = toApiError(e, 'Gagal memuat detail produk')
  } finally {
    row.loadingDetail = false
  }
}

function onEditRowMaterialChange(row: EditItemRow) {
  const p = row.productDetail
  if (p && row.materialId && p.pricing_type === 'paket') {
    const found = p.pricings.find((r) => r.material_id === row.materialId)
    if (found?.width_cm) row.widthCm = found.width_cm
    if (found?.height_cm) row.heightCm = found.height_cm
  }
  scheduleEditRowQuote(row)
}

const editQuoteTimers = new Map<number, ReturnType<typeof setTimeout>>()
function scheduleEditRowQuote(row: EditItemRow) {
  const existing = editQuoteTimers.get(row.key)
  if (existing) clearTimeout(existing)
  editQuoteTimers.set(
    row.key,
    setTimeout(() => runEditRowQuote(row), 400),
  )
}

async function runEditRowQuote(row: EditItemRow) {
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
    row.quoteError = toApiError(e, 'Gagal menghitung harga')
  } finally {
    row.quoteLoading = false
  }
}

function editRowIsPaket(row: EditItemRow): boolean {
  return row.productDetail?.pricing_type === 'paket'
}
function editRowMaterials(row: EditItemRow) {
  const p = row.productDetail
  if (!p) return []
  return p.pricings.map((r) => ({
    id: r.material_id,
    label:
      p.pricing_type === 'paket'
        ? `${r.material_name}${r.package_label ? ' · ' + r.package_label : ''}`
        : r.material_name,
  }))
}

/** Subtotal baris — existing pakai harga yang diketik admin; baru pakai quote katalog (final tetap dihitung ulang backend). */
function editRowSubtotal(row: EditItemRow): number {
  if (row.id) return (row.unitPrice || 0) * (row.quantity || 0)
  return row.quote ? row.quote.total_price * (row.quantity || 0) : 0
}

function editItemRowValid(row: EditItemRow): boolean {
  if (row.widthCm <= 0 || row.heightCm <= 0 || row.quantity < 1) return false
  if (row.id) return row.unitPrice >= 0
  if (!row.productId || !row.materialId || !row.designSource) return false
  if (row.designSource === 'request' && !row.designBrief.trim()) return false
  return true
}

const editItemsValid = computed(
  () =>
    editItems.value.length >= 1 &&
    editItems.value.length <= MAX_EDIT_ITEMS &&
    editItems.value.every(editItemRowValid),
)

// ---- Nominal (perkiraan — angka final dari respons backend, §32.9) ----
const estimatedSubtotal = computed(() => {
  if (!order.value) return 0
  if (!itemsEditMode.value) return order.value.subtotal
  return editItems.value.reduce((sum, row) => sum + editRowSubtotal(row), 0)
})
const currentDiscountAmount = computed(() => order.value?.discount_amount ?? 0)
const discountExceedsSubtotal = computed(() => currentDiscountAmount.value > estimatedSubtotal.value)
const editComputedTotal = computed(
  () => estimatedSubtotal.value - currentDiscountAmount.value + (editForm.shipping_cost || 0),
)

const editFinancialChanged = computed(() => {
  if (!order.value) return false
  const shippingChanged = editForm.shipping_cost !== (order.value.shipping_cost ?? 0)
  return shippingChanged || itemsEditMode.value
})
// Alasan wajib hanya kalau order sudah lunas DAN nominal (ongkir/item) diubah.
const editReasonRequired = computed(() => orderIsPaid.value && editFinancialChanged.value)
const editCanSubmit = computed(() => {
  if (itemsEditMode.value && (!editItemsValid.value || discountExceedsSubtotal.value)) return false
  return !editReasonRequired.value || editForm.reason.trim().length >= 10
})

function buildEditItemsPayload(): AdminOrderEditItemInput[] {
  return editItems.value.map((row) => {
    if (row.id) {
      return {
        id: row.id,
        width_cm: row.widthCm,
        height_cm: row.heightCm,
        quantity: row.quantity,
        unit_price: row.unitPrice,
        item_notes: row.itemNotes.trim() || undefined,
      }
    }
    return {
      product_id: row.productId,
      material_id: row.materialId,
      width_cm: row.widthCm,
      height_cm: row.heightCm,
      quantity: row.quantity,
      design_source: row.designSource || undefined,
      design_brief: row.designSource === 'request' ? row.designBrief.trim() || undefined : undefined,
      item_notes: row.itemNotes.trim() || undefined,
    }
  })
}

async function submitEdit() {
  if (!order.value || !editCanSubmit.value) return
  editBusy.value = true
  modalError.value = null
  try {
    const body: AdminOrderEditInput = {
      shipping_recipient_name: editForm.shipping_recipient_name.trim(),
      shipping_recipient_phone: editForm.shipping_recipient_phone.trim(),
      shipping_address: editForm.shipping_address.trim(),
      notes: editForm.notes.trim(),
      shipping_cost: editForm.shipping_cost,
    }
    if (itemsEditMode.value) {
      body.items = buildEditItemsPayload()
    }
    if (editReasonRequired.value) {
      body.reason = editForm.reason.trim()
    }
    order.value = await orderSvc.adminEditOrder(resi.value, body)
    editOpen.value = false
    closeItemsEditor()
    showSuccess('Perubahan pesanan tersimpan.')
    await loadDetail()
    await loadAuditLog()
  } catch (e) {
    modalError.value = toApiError(e, 'Gagal menyimpan perubahan pesanan')
  } finally {
    editBusy.value = false
  }
}

// -------------------- override status (super admin, order.override_status) --------------------
const ALL_ORDER_STATUSES: OrderStatus[] = [
  'order_masuk',
  'menunggu_ongkir',
  'menunggu_pembayaran',
  'menunggu_verifikasi',
  'dibayar',
  'ditolak',
  'desain_dikerjakan',
  'menunggu_approval_desain',
  'desain_diverifikasi',
  'proses_cetak',
  'qc',
  'siap_kirim',
  'siap_ambil',
  'dikirim',
  'selesai',
  'dibatalkan',
]

const overrideOpen = ref(false)
const overrideBusy = ref(false)
const overrideToStatus = ref<OrderStatus | ''>('')
const overrideReason = ref('')
const overrideCanSubmit = computed(
  () => overrideToStatus.value !== '' && overrideReason.value.trim().length >= 10,
)

function openOverride() {
  if (!order.value) return
  overrideToStatus.value = order.value.status as OrderStatus
  overrideReason.value = ''
  modalError.value = null
  overrideOpen.value = true
}

async function submitOverride() {
  if (!order.value || !overrideCanSubmit.value) return
  overrideBusy.value = true
  modalError.value = null
  try {
    order.value = await orderSvc.adminOverrideStatus(resi.value, {
      to_status: overrideToStatus.value,
      reason: overrideReason.value.trim(),
    })
    overrideOpen.value = false
    showSuccess(`Status diubah paksa ke "${statusLabel(overrideToStatus.value)}". Tidak ada notifikasi WA terkirim.`)
    await loadDetail()
    await loadAuditLog()
  } catch (e) {
    modalError.value = toApiError(e, 'Gagal mengubah status pesanan')
  } finally {
    overrideBusy.value = false
  }
}

// -------------------- hapus pesanan (super admin, order.delete) --------------------
/**
 * Backend menolak (409 `DELETE_NOT_ALLOWED_PAID`) menghapus pesanan yang
 * sudah `dibayar` atau setelahnya (kecuali `dibatalkan`) — supaya order
 * berbayar tidak lenyap diam-diam dari rekap kas harian. `orderIsPaid` (di
 * atas) sudah persis mendefinisikan set status itu, jadi dipakai ulang di
 * sini untuk menonaktifkan tombol Hapus DI UI SEBELUM admin sempat membuka
 * modal — bukan cuma menunggu ditolak server.
 */
const canDeleteNow = computed(() => canDeleteOrder.value && !orderIsPaid.value)
const deleteDisabledReason =
  'Pesanan sudah dibayar/diproses dan tidak bisa dihapus. Batalkan pesanan terlebih dahulu (tombol Cancel), baru bisa dihapus.'

const deleteOpen = ref(false)
const deleteBusy = ref(false)
const deleteReason = ref('')
const deleteCanSubmit = computed(() => deleteReason.value.trim().length >= 10)

function openDelete() {
  deleteReason.value = ''
  modalError.value = null
  deleteOpen.value = true
}

async function submitDelete() {
  if (!order.value || !deleteCanSubmit.value) return
  deleteBusy.value = true
  modalError.value = null
  try {
    await orderSvc.adminDeleteOrder(resi.value, deleteReason.value.trim())
    await navigateTo('/admin/order')
  } catch (e) {
    modalError.value = toApiError(e, 'Gagal menghapus pesanan')
    deleteBusy.value = false
  }
}

// -------------------- audit log rendering helpers --------------------
const auditActionLabelMap: Record<string, string> = {
  'order.edit': 'Edit data pesanan',
  'order.override_status': 'Override status',
  'order.delete': 'Hapus pesanan',
}
function auditActionLabel(action: string): string {
  return auditActionLabelMap[action] ?? action.replace(/_/g, ' ')
}
function isChangeEntry(v: unknown): v is { old?: unknown; new?: unknown } {
  return typeof v === 'object' && v !== null && ('old' in v || 'new' in v)
}
function formatChangeValue(v: unknown): string {
  if (v === null || v === undefined || v === '') return '—'
  if (typeof v === 'object') return JSON.stringify(v)
  return String(v)
}

// -------------------- copy resi --------------------
const copiedResi = ref(false)
async function copyResi() {
  if (!order.value) return
  try {
    await navigator.clipboard.writeText(order.value.resi)
    copiedResi.value = true
    setTimeout(() => (copiedResi.value = false), 1500)
  } catch {
    // ignore
  }
}

// -------------------- helpers --------------------
function fmtIDR(n: number | null | undefined): string {
  if (n == null) return '—'
  return new Intl.NumberFormat('id-ID', {
    style: 'currency',
    currency: 'IDR',
    maximumFractionDigits: 0,
  }).format(n)
}

function fmtDate(s: string): string {
  if (!s) return '—'
  try {
    return new Date(s).toLocaleString('id-ID', {
      day: '2-digit',
      month: 'short',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    })
  } catch {
    return s
  }
}

const statusLabelMap: Record<string, string> = {
  order_masuk: 'Order masuk',
  menunggu_ongkir: 'Menunggu ongkir',
  menunggu_pembayaran: 'Menunggu pembayaran',
  menunggu_verifikasi: 'Menunggu verifikasi bukti',
  dibayar: 'Pembayaran diverifikasi',
  ditolak: 'Bukti transfer ditolak',
  desain_dikerjakan: 'Desain dikerjakan',
  menunggu_approval_desain: 'Menunggu approval desain',
  desain_diverifikasi: 'Desain siap',
  proses_cetak: 'Proses cetak',
  qc: 'Quality check',
  siap_kirim: 'Siap dikirim',
  siap_ambil: 'Siap diambil',
  dikirim: 'Dalam pengiriman',
  selesai: 'Selesai',
  dibatalkan: 'Dibatalkan',
}
function statusLabel(s: string): string {
  return statusLabelMap[s] ?? s.replace(/_/g, ' ')
}

function proofStatusTone(status: string): 'green' | 'amber' | 'rose' {
  if (status === 'approved') return 'green'
  if (status === 'rejected') return 'rose'
  return 'amber'
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

function waLink(phone: string): string {
  const p = phone.replace(/[^0-9]/g, '')
  return `https://wa.me/${p}`
}
</script>

<template>
  <section>
    <AdminPageHeader
      :title="loading ? 'Memuat…' : order ? `Order ${order.resi}` : 'Order tidak ditemukan'"
      :breadcrumb="[{ label: 'Order', to: '/admin/order' }, { label: resi }]"
    >
      <template #actions>
        <AdminStatusBadge v-if="order" :status="statusLabel(order.status)" />
        <button
          v-if="order"
          type="button"
          :disabled="receiptWidthMm === null"
          :title="receiptWidthMm === null ? 'Lebar kertas struk belum diketahui — cetak dinonaktifkan agar tidak salah ukuran.' : undefined"
          class="inline-flex items-center gap-1.5 rounded-md border border-hairline bg-canvas px-2.5 py-1.5 text-xs font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas disabled:opacity-50 disabled:cursor-not-allowed disabled:hover:bg-canvas disabled:hover:border-hairline"
          @click="printStruk"
        >
          <Printer class="h-3.5 w-3.5" :stroke-width="1.75" />
          Cetak struk
        </button>
        <button
          v-if="order"
          type="button"
          class="inline-flex items-center gap-1.5 rounded-md border border-hairline bg-canvas px-2.5 py-1.5 text-xs font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
          @click="copyResi"
        >
          <Check v-if="copiedResi" class="h-3.5 w-3.5 text-emerald-600" :stroke-width="1.75" />
          <Copy v-else class="h-3.5 w-3.5" :stroke-width="1.75" />
          {{ copiedResi ? 'Disalin' : 'Salin resi' }}
        </button>
        <button
          v-if="canCancelNow"
          type="button"
          class="inline-flex items-center gap-2 rounded-md border border-brand-200 bg-canvas px-3 py-1.5 text-sm font-semibold text-brand-700 hover:bg-brand-50 hover:border-brand-300 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
          @click="openCancel"
        >
          <XCircle class="h-4 w-4" :stroke-width="1.75" />
          Cancel
        </button>
        <!-- Aksi super admin: edit data, override status paksa, hapus (soft delete). -->
        <button
          v-if="order && canEditOrder"
          type="button"
          class="inline-flex items-center gap-1.5 rounded-md border border-hairline bg-canvas px-2.5 py-1.5 text-xs font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
          @click="openEdit"
        >
          <Pencil class="h-3.5 w-3.5" :stroke-width="1.75" />
          Edit pesanan
        </button>
        <button
          v-if="order && canOverrideStatus"
          type="button"
          class="inline-flex items-center gap-1.5 rounded-md border border-hairline bg-canvas px-2.5 py-1.5 text-xs font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
          @click="openOverride"
        >
          <RefreshCw class="h-3.5 w-3.5" :stroke-width="1.75" />
          Ubah status paksa
        </button>
        <button
          v-if="order && canDeleteOrder"
          type="button"
          :disabled="!canDeleteNow"
          :title="canDeleteNow ? undefined : deleteDisabledReason"
          class="inline-flex items-center gap-2 rounded-md border border-brand-200 bg-canvas px-3 py-1.5 text-sm font-semibold text-brand-700 hover:bg-brand-50 hover:border-brand-300 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas disabled:opacity-50 disabled:cursor-not-allowed disabled:hover:bg-canvas disabled:hover:border-brand-200"
          @click="openDelete"
        >
          <Trash2 class="h-4 w-4" :stroke-width="1.75" />
          Hapus
        </button>
      </template>
    </AdminPageHeader>

    <p
      v-if="order && canDeleteOrder && !canDeleteNow"
      class="mb-4 flex items-start gap-2 rounded-md border border-hairline bg-canvas-alt/60 px-3 py-2 text-xs text-ink-500 leading-relaxed"
    >
      <AlertTriangle class="h-3.5 w-3.5 flex-none mt-0.5 text-ink-400" :stroke-width="1.75" />
      {{ deleteDisabledReason }}
    </p>

    <p
      v-if="order && receiptConfigFailed"
      class="mb-4 flex items-center gap-2 rounded-md border border-hairline bg-canvas-alt/60 px-3 py-2 text-xs text-ink-500 leading-relaxed"
    >
      <AlertTriangle class="h-3.5 w-3.5 flex-none text-ink-400" :stroke-width="1.75" />
      Lebar kertas struk gagal dimuat. Cetak struk dinonaktifkan agar tidak salah ukuran.
      <button
        type="button"
        class="font-medium text-brand-600 hover:text-brand-700 underline underline-offset-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
        @click="loadReceiptConfig"
      >
        Coba lagi
      </button>
    </p>

    <AlertMessage v-if="errorMsg" variant="error" :message="errorMsg" class="mb-4" />
    <AlertMessage v-if="successMsg" variant="success" :message="successMsg" class="mb-4" />

    <div v-if="loading" class="rounded-lg border border-hairline bg-canvas p-8 text-center">
      <Loader2 class="mx-auto h-5 w-5 animate-spin text-ink-500" :stroke-width="1.75" />
      <p class="mt-3 text-sm text-ink-500">Memuat order…</p>
    </div>

    <div v-else-if="!order" class="rounded-lg border border-hairline bg-canvas p-6 text-sm text-ink-500">
      Order tidak ditemukan.
    </div>

    <div v-else class="grid gap-6 lg:grid-cols-3">
      <!-- ================================ LEFT COLUMN ================================ -->
      <div class="lg:col-span-2 space-y-4">
        <!-- Timeline -->
        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-4">Riwayat status</p>
          <ol v-if="history.length" class="relative border-l border-hairline pl-6 space-y-4">
            <li
              v-for="(row, idx) in history"
              :key="row.id"
              class="relative"
            >
              <span
                :class="[
                  'absolute -left-[27px] flex h-4 w-4 items-center justify-center rounded-full',
                  idx === history.length - 1
                    ? 'bg-brand-500 ring-4 ring-brand-500/15'
                    : 'bg-ink-950/80 ring-4 ring-canvas-alt',
                ]"
              >
                <CheckCircle2
                  v-if="row.to_status === 'selesai'"
                  class="h-3 w-3 text-canvas"
                  :stroke-width="2"
                />
                <CircleDot
                  v-else-if="idx === history.length - 1"
                  class="h-3 w-3 text-canvas"
                  :stroke-width="2"
                />
                <Circle v-else class="h-3 w-3 text-canvas" :stroke-width="2" />
              </span>
              <div class="flex flex-wrap items-baseline justify-between gap-2">
                <div>
                  <p
                    :class="[
                      'text-sm',
                      idx === history.length - 1 ? 'font-semibold text-ink-950' : 'text-ink-700',
                    ]"
                  >
                    {{ statusLabel(row.to_status) }}
                  </p>
                  <p v-if="row.note" class="mt-0.5 text-xs text-ink-500 leading-relaxed max-w-lg">
                    {{ row.note }}
                  </p>
                </div>
                <time class="font-mono text-[11px] text-ink-500">{{ fmtDate(row.changed_at) }}</time>
              </div>
            </li>
          </ol>
          <p v-else class="text-sm text-ink-500">Belum ada history.</p>
        </div>

        <!-- Detail produk (§32 — tabel multi-item) -->
        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <div class="flex items-center justify-between gap-2 mb-3">
            <div class="flex items-center gap-2">
              <PackageIcon class="h-4 w-4 text-ink-500" :stroke-width="1.75" />
              <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Detail produk</p>
            </div>
            <span class="text-xs text-ink-500">{{ order.items.length }} item</span>
          </div>
          <OrderItemsTable :items="order.items" />
          <p v-if="order.notes" class="mt-3 text-sm text-ink-700 leading-relaxed border-l-2 border-gold-300 pl-3">
            "{{ order.notes }}"
          </p>
        </div>

        <!-- Nominal -->
        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <div class="flex items-center gap-2 mb-3">
            <Wallet class="h-4 w-4 text-ink-500" :stroke-width="1.75" />
            <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Nominal</p>
          </div>
          <dl class="space-y-2 text-sm">
            <div class="flex justify-between">
              <dt class="text-ink-500">Subtotal</dt>
              <dd class="text-ink-900">{{ fmtIDR(order.subtotal) }}</dd>
            </div>
            <div v-if="order.discount_amount && order.discount_amount > 0" class="flex justify-between">
              <dt class="text-ink-500">
                Diskon
                <span v-if="order.discount_name_snapshot" class="block text-xs text-ink-400">
                  {{ order.discount_name_snapshot }}
                  <span v-if="order.discount_code_snapshot" class="font-mono">· {{ order.discount_code_snapshot }}</span>
                </span>
              </dt>
              <dd class="text-brand-600 font-medium">-{{ fmtIDR(order.discount_amount) }}</dd>
            </div>
            <div class="flex justify-between">
              <dt class="text-ink-500">Ongkir</dt>
              <dd class="text-ink-900">
                <template v-if="isPickup"><span class="text-ink-500">(pickup, gratis)</span></template>
                <template v-else-if="order.shipping_cost == null"><span class="text-ink-500">belum diinput</span></template>
                <template v-else>{{ fmtIDR(order.shipping_cost) }}</template>
              </dd>
            </div>
            <div class="flex justify-between border-t border-hairline pt-3 mt-1">
              <dt class="font-semibold text-ink-950">Total</dt>
              <dd class="font-serif text-lg font-semibold text-ink-950">{{ fmtIDR(order.total) }}</dd>
            </div>
            <div v-if="order.metode_bayar" class="flex justify-between text-xs text-ink-500 pt-1">
              <dt>Metode bayar</dt>
              <dd class="uppercase text-ink-700">{{ order.metode_bayar }}</dd>
            </div>
          </dl>
          <p
            v-if="order.discount_note"
            class="mt-3 text-xs text-ink-500 leading-relaxed border-l-2 border-gold-300 pl-3"
          >
            Catatan diskon: "{{ order.discount_note }}"
          </p>
        </div>

        <!-- Customer -->
        <div v-if="customer" class="rounded-lg border border-hairline bg-canvas p-6">
          <div class="flex items-center gap-2 mb-3">
            <User class="h-4 w-4 text-ink-500" :stroke-width="1.75" />
            <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Customer</p>
          </div>
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p class="text-base font-medium text-ink-900">{{ customer.name }}</p>
              <p class="mt-0.5 font-mono text-xs text-ink-500">{{ customer.phone }}</p>
              <p v-if="customer.email" class="text-xs text-ink-500">{{ customer.email }}</p>
            </div>
            <div class="flex flex-wrap items-center gap-2">
              <span
                v-if="customer.type"
                class="inline-flex items-center rounded-full bg-canvas-alt px-2 py-0.5 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 ring-1 ring-inset ring-hairline"
              >
                {{ customer.type }}
              </span>
              <a
                :href="waLink(customer.phone)"
                target="_blank"
                rel="noopener"
                class="inline-flex items-center gap-1 rounded-md border border-emerald-300 bg-canvas px-2 py-1 text-xs font-medium text-emerald-800 hover:bg-emerald-50 transition-colors"
              >
                <Phone class="h-3 w-3" :stroke-width="1.75" />
                WhatsApp
              </a>
            </div>
          </div>
        </div>

        <!-- Alamat / pickup -->
        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <div class="flex items-center gap-2 mb-3">
            <Truck class="h-4 w-4 text-ink-500" :stroke-width="1.75" />
            <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
              {{ isPickup ? 'Pickup di toko' : 'Alamat pengiriman' }}
            </p>
          </div>
          <div v-if="isPickup" class="text-sm text-ink-700">
            Customer akan mengambil langsung di toko. Tidak ada ongkir.
          </div>
          <div v-else class="space-y-1 text-sm">
            <p class="text-ink-900 font-medium">{{ order.shipping_recipient_name || '—' }}</p>
            <p class="font-mono text-xs text-ink-500">{{ order.shipping_recipient_phone || '—' }}</p>
            <p class="text-ink-700 leading-relaxed mt-2">
              {{ order.shipping_address || 'Alamat belum diinput.' }}
            </p>
          </div>
        </div>

        <!-- Desain -->
        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <div class="flex items-center justify-between mb-3">
            <div class="flex items-center gap-2">
              <ImageIcon class="h-4 w-4 text-ink-500" :stroke-width="1.75" />
              <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Desain</p>
            </div>
            <NuxtLink
              to="/admin/desain"
              class="text-xs text-ink-500 hover:text-brand-500 transition-colors"
            >
              Buka modul →
            </NuxtLink>
          </div>
          <p class="text-sm">
            <span class="uppercase font-medium text-ink-900">{{ order.design_source }}</span>
            <span class="ml-2 text-ink-500 text-xs">
              {{
                order.design_source === 'upload'
                  ? '(customer upload sendiri)'
                  : order.design_source === 'request'
                    ? '(customer minta jasa desain)'
                    : '(campuran — beberapa item upload sendiri, sebagian minta jasa desain)'
              }}
            </span>
          </p>
          <!-- Per item (§32.5) — sumber desain & brief sekarang menempel di
               baris item, bukan order. Cuma tampilkan item yang punya brief
               atau order-nya multi-item (satu item cukup pakai baris di atas). -->
          <ul v-if="order.items.length > 1" class="mt-3 space-y-2">
            <li
              v-for="it in order.items"
              :key="it.line_no"
              class="rounded-md border border-hairline bg-canvas-alt/60 px-3 py-2"
            >
              <p class="text-xs text-ink-900">
                <span class="font-medium">{{ it.product_name }}</span>
                <span class="ml-2 uppercase text-[10px] text-ink-500">{{ it.design_source }}</span>
              </p>
              <p v-if="it.design_brief" class="mt-1 text-xs text-ink-700 leading-relaxed border-l-2 border-gold-300 pl-2">
                "{{ it.design_brief }}"
              </p>
            </li>
          </ul>
          <p
            v-else-if="order.items[0]?.design_brief"
            class="mt-2 text-sm text-ink-700 leading-relaxed border-l-2 border-gold-300 pl-3"
          >
            "{{ order.items[0].design_brief }}"
          </p>

          <div v-if="designLoading" class="mt-4 text-xs text-ink-500 flex items-center gap-2">
            <Loader2 class="h-3.5 w-3.5 animate-spin" :stroke-width="1.75" />
            Memuat file desain…
          </div>
          <ul v-else-if="designFiles.length" class="mt-4 space-y-2">
            <li
              v-for="f in designFiles"
              :key="f.id"
              class="flex items-start justify-between gap-3 rounded-md border border-hairline bg-canvas-alt/60 px-3 py-2"
            >
              <div class="min-w-0">
                <div class="flex items-center gap-2">
                  <FileText class="h-3.5 w-3.5 text-ink-500 flex-none" :stroke-width="1.75" />
                  <p class="text-xs text-ink-900 truncate">{{ f.file_original_name }}</p>
                </div>
                <p class="mt-0.5 text-[10px] text-ink-500">
                  <span class="uppercase font-medium">{{ f.role.replace('_', ' ') }}</span>
                  · {{ formatBytes(f.file_size_bytes) }} · {{ f.file_mime_type }}
                  · {{ fmtDate(f.uploaded_at) }}
                  <span v-if="f.is_purged" class="ml-1 text-brand-700">· purged</span>
                </p>
              </div>
              <AdminStatusBadge v-if="f.approval_status" :status="f.approval_status" />
            </li>
          </ul>
          <p v-else class="mt-4 text-xs text-ink-500">Belum ada file desain untuk order ini.</p>
        </div>

        <!-- Pembayaran -->
        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <div class="flex items-center justify-between mb-3">
            <div class="flex items-center gap-2">
              <CreditCard class="h-4 w-4 text-ink-500" :stroke-width="1.75" />
              <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Bukti Pembayaran</p>
            </div>
            <NuxtLink
              to="/admin/pembayaran"
              class="text-xs text-ink-500 hover:text-brand-500 transition-colors"
            >
              Buka modul →
            </NuxtLink>
          </div>

          <div v-if="proofsLoading" class="text-xs text-ink-500 flex items-center gap-2">
            <Loader2 class="h-3.5 w-3.5 animate-spin" :stroke-width="1.75" />
            Memuat bukti…
          </div>
          <ul v-else-if="proofs.length" class="space-y-2">
            <li
              v-for="p in proofs"
              :key="p.id"
              class="flex items-start justify-between gap-3 rounded-md border border-hairline bg-canvas-alt/60 px-3 py-2"
            >
              <div class="min-w-0">
                <div class="flex items-center gap-2">
                  <FileText class="h-3.5 w-3.5 text-ink-500 flex-none" :stroke-width="1.75" />
                  <p class="text-xs text-ink-900 truncate">{{ p.file_original_name }}</p>
                </div>
                <p class="mt-0.5 text-[10px] text-ink-500">
                  <span class="uppercase font-medium">{{ p.metode_bayar }}</span>
                  · Nominal claim {{ fmtIDR(p.amount_claimed) }}
                  · {{ fmtDate(p.uploaded_at) }}
                </p>
                <p v-if="p.status === 'rejected' && p.reject_reason" class="mt-0.5 text-[10px] text-brand-700">
                  Alasan tolak: {{ p.reject_reason }}
                </p>
              </div>
              <AdminStatusBadge :status="p.status" :tone="proofStatusTone(p.status)" />
            </li>
          </ul>
          <p v-else class="text-xs text-ink-500">
            Belum ada bukti transfer diupload customer.
            <span v-if="order.channel === 'pos'" class="text-ink-400">(POS bayar langsung — tidak butuh bukti.)</span>
          </p>
        </div>

        <!-- Jejak audit (super admin, audit.view) -->
        <div v-if="canViewAudit" class="rounded-lg border border-hairline bg-canvas p-6">
          <div class="flex items-center gap-2 mb-3">
            <History class="h-4 w-4 text-ink-500" :stroke-width="1.75" />
            <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Jejak audit</p>
          </div>

          <div v-if="auditLoading" class="text-xs text-ink-500 flex items-center gap-2">
            <Loader2 class="h-3.5 w-3.5 animate-spin" :stroke-width="1.75" />
            Memuat jejak audit…
          </div>
          <ol v-else-if="auditLog.length" class="space-y-3">
            <li
              v-for="entry in auditLog"
              :key="entry.id"
              class="rounded-md border border-hairline bg-canvas-alt/60 px-3 py-2.5"
            >
              <div class="flex flex-wrap items-baseline justify-between gap-2">
                <p class="text-sm font-medium text-ink-900">{{ auditActionLabel(entry.action) }}</p>
                <time class="font-mono text-[10px] text-ink-500">{{ fmtDate(entry.created_at) }}</time>
              </div>
              <p class="mt-0.5 text-xs text-ink-500">
                Oleh <span class="text-ink-700">{{ entry.actor_name || 'Staff' }}</span>
                <span class="text-ink-400"> · </span>
                <span class="font-mono text-[10px] text-ink-500">{{ entry.actor_user_id }}</span>
              </p>
              <p v-if="entry.reason" class="mt-2 text-xs text-ink-700 leading-relaxed border-l-2 border-gold-300 pl-3">
                "{{ entry.reason }}"
              </p>
              <div v-if="entry.changes && Object.keys(entry.changes).length" class="mt-2 space-y-0.5">
                <div
                  v-for="(val, key) in entry.changes"
                  :key="key"
                  class="flex flex-wrap items-baseline gap-1.5 font-mono text-[10px] text-ink-500"
                >
                  <span class="text-ink-700">{{ key }}:</span>
                  <template v-if="isChangeEntry(val)">
                    <span>{{ formatChangeValue(val.old) }}</span>
                    <span class="text-ink-400">&rarr;</span>
                    <span class="text-ink-900">{{ formatChangeValue(val.new) }}</span>
                  </template>
                  <span v-else>{{ formatChangeValue(val) }}</span>
                </div>
              </div>
            </li>
          </ol>
          <p v-else class="text-xs text-ink-500">Belum ada aksi super admin tercatat untuk pesanan ini.</p>
        </div>
      </div>

      <!-- ================================ RIGHT COLUMN ================================ -->
      <aside class="space-y-4 lg:sticky lg:top-6 lg:self-start">
        <!-- Set ongkir (kirim + order_masuk/menunggu_ongkir + perm) -->
        <div
          v-if="canFillOngkirNow"
          class="rounded-lg border border-brand-500/25 bg-brand-50 p-5"
        >
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Langkah berikutnya</p>
          <h3 class="mt-1 text-sm font-semibold text-ink-900">Isi ongkir &amp; minta pembayaran</h3>
          <p class="mt-1 text-xs text-ink-500 leading-relaxed">
            Pesanan ini dikirim, jadi totalnya belum final. Setelah ongkir disimpan, status pindah ke
            <strong class="font-medium text-ink-900">Menunggu pembayaran</strong> dan pelanggan langsung
            dapat WA berisi total yang harus dibayar.
          </p>
          <div class="mt-4 space-y-3">
            <div>
              <label for="ongkir-amount" class="block text-xs font-medium text-ink-700">Nominal ongkir (Rp)</label>
              <input
                id="ongkir-amount"
                v-model.number="ongkirValue"
                type="number"
                min="0"
                step="1000"
                placeholder="0"
                class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
              >
            </div>
            <div>
              <label for="ongkir-note" class="block text-xs font-medium text-ink-700">Catatan (opsional)</label>
              <input
                id="ongkir-note"
                v-model="ongkirNote"
                type="text"
                maxlength="500"
                placeholder="Estimasi kurir, ETA, dll."
                class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
              >
            </div>
            <button
              type="button"
              class="w-full inline-flex items-center justify-center gap-2 rounded-md bg-brand-500 px-3 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-60"
              :disabled="ongkirBusy || ongkirValue < 0"
              @click="submitOngkir"
            >
              <Loader2 v-if="ongkirBusy" class="h-3.5 w-3.5 animate-spin" :stroke-width="1.75" />
              Simpan ongkir
            </button>
          </div>
        </div>

        <!-- Confirm pickup -->
        <div
          v-if="canConfirmPickupNow"
          class="rounded-lg border border-brand-500/25 bg-brand-50 p-5"
        >
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Langkah berikutnya</p>
          <h3 class="mt-1 text-sm font-semibold text-ink-900">Konfirmasi total &amp; minta pembayaran</h3>
          <p class="mt-1 text-xs text-ink-500 leading-relaxed">
            Pesanan ini diambil di tempat, jadi tidak ada ongkir — totalnya sudah final. Sekali klik:
            status pindah ke <strong class="font-medium text-ink-900">Menunggu pembayaran</strong> dan
            pelanggan langsung dapat WA berisi total yang harus dibayar.
          </p>
          <div class="mt-4 flex items-baseline justify-between gap-3 rounded-md border border-hairline bg-canvas px-3 py-2">
            <span class="text-xs text-ink-500">Total ditagihkan</span>
            <span class="text-sm font-semibold text-ink-950">{{ fmtIDR(order.total) }}</span>
          </div>
          <button
            type="button"
            class="mt-3 w-full inline-flex items-center justify-center gap-2 rounded-md bg-brand-500 px-3 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-60"
            :disabled="pickupBusy"
            @click="confirmPickup"
          >
            <Loader2 v-if="pickupBusy" class="h-3.5 w-3.5 animate-spin" :stroke-width="1.75" />
            <Wallet v-else class="h-4 w-4" :stroke-width="1.75" />
            Minta pembayaran {{ fmtIDR(order.total) }}
          </button>
        </div>

        <!-- Cross-module links -->
        <div class="rounded-lg border border-hairline bg-canvas p-5">
          <h3 class="text-sm font-semibold text-ink-900">Modul terkait</h3>
          <p class="mt-1 text-xs text-ink-500 leading-relaxed">
            Aksi lanjutan (verifikasi bukti, kerjakan desain, advance status produksi) di modul masing-masing.
          </p>
          <div class="mt-4 space-y-2">
            <NuxtLink
              to="/admin/pembayaran"
              class="flex items-center gap-3 rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-700 hover:border-ink-300 hover:text-brand-500 transition-colors"
            >
              <CreditCard class="h-4 w-4" :stroke-width="1.75" />
              Verifikasi Pembayaran
            </NuxtLink>
            <NuxtLink
              to="/admin/desain"
              class="flex items-center gap-3 rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-700 hover:border-ink-300 hover:text-brand-500 transition-colors"
            >
              <Palette class="h-4 w-4" :stroke-width="1.75" />
              Desain
            </NuxtLink>
            <NuxtLink
              to="/admin/produksi"
              class="flex items-center gap-3 rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-700 hover:border-ink-300 hover:text-brand-500 transition-colors"
            >
              <Printer class="h-4 w-4" :stroke-width="1.75" />
              Produksi
            </NuxtLink>
          </div>
        </div>

        <!-- Meta -->
        <div class="rounded-lg border border-hairline bg-canvas p-5 space-y-2 text-xs">
          <div class="flex justify-between">
            <span class="text-ink-500">Channel</span>
            <span class="uppercase font-medium text-ink-900">{{ order.channel }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-ink-500">Metode ambil</span>
            <span class="uppercase font-medium text-ink-900">{{ order.metode_ambil }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-ink-500">Order masuk</span>
            <span class="text-ink-700">{{ fmtDate(order.created_at) }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-ink-500">Resi</span>
            <span class="font-mono text-[10px] text-ink-700">{{ order.resi }}</span>
          </div>
        </div>
      </aside>
    </div>

    <!-- ============================ Struk (cetak ulang) ============================
         Tersembunyi di layar (semua datanya sudah tampil di section lain di
         atas) — cuma dipakai saat tombol "Cetak struk" di header memanggil
         `printStruk()`. `hidden` mencegah kedipan struk kelihatan sekilas di
         layar selama jeda `receiptMounted = true` → `nextTick()` sebelum
         `printNow()` benar-benar men-teleport-nya ke `<body>` (lihat
         `ReceiptStruk.vue`) — begitu teleport aktif, elemen ini sudah kosong
         karena isinya pindah keluar dari wrapper ini. `print:block` dulu
         krusial untuk teknik `visibility` yang lama; sekarang isolasi cetak
         sudah sepenuhnya ditangani ReceiptStruk sendiri (`display: none`
         pada saudara `#struk-print-root` di `<body>`), jadi kelas ini cuma
         jaring pengaman kalau suatu saat komponennya sempat dirender di sini
         sebelum teleport aktif.

         Soal `:width-mm="receiptWidthMm ?? 58"` di bawah: `receiptMounted`
         hanya pernah di-set `true` oleh `printStruk()`, dan `printStruk()`
         `return` lebih dulu kalau `receiptWidthMm` masih `null` — jadi blok
         ini tidak akan pernah benar-benar ter-render selagi lebar kertas
         belum diketahui. `?? 58` di sini murni supaya `vue-tsc` puas (prop
         `AdminReceiptStruk` bertipe `58 | 80`, bukan `58 | 80 | null`),
         BUKAN jalur tebak-lebar yang sesungguhnya dipakai — itu sudah
         diblok di level pemicu (tombol disabled + guard di `printStruk`). -->
    <div v-if="order && receiptMounted" class="hidden print:block">
      <AdminReceiptStruk
        ref="receiptRef"
        :resi="order.resi"
        :created-at="order.created_at"
        :customer-name="customer?.name ?? '—'"
        :customer-phone="customer?.phone ?? '—'"
        :items="order.items"
        :subtotal="order.subtotal"
        :shipping-cost="order.shipping_cost"
        :discount-amount="order.discount_amount"
        :discount-label="order.discount_name_snapshot"
        :total="order.total"
        :metode-ambil="order.metode_ambil"
        :metode-bayar="order.metode_bayar ?? '-'"
        :tracking-url="trackingUrl"
        :is-paid="orderIsPaid"
        :width-mm="receiptWidthMm ?? 58"
      />
    </div>

    <!-- ============================ Cancel modal ============================ -->
    <Teleport to="body">
      <Transition name="fade">
        <div v-if="cancelOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4">
          <div class="absolute inset-0 bg-ink-950/60" aria-hidden="true" @click="!cancelBusy && (cancelOpen = false)" />
          <div role="dialog" aria-modal="true" class="relative w-full max-w-md rounded-lg bg-canvas shadow-xl ring-1 ring-black/5 p-6">
            <h3 class="font-serif text-lg font-semibold text-ink-950">Cancel order {{ order?.resi }}?</h3>
            <p class="mt-1 text-xs text-ink-500 leading-relaxed">
              Order akan masuk status <strong class="text-ink-900">dibatalkan</strong> dan tidak bisa dikembalikan
              ke flow reguler. Alasan wajib diisi untuk audit.
            </p>
            <AlertMessage v-if="modalError" variant="error" :message="modalError" class="mt-3" />
            <textarea
              v-model="cancelReason"
              rows="3"
              maxlength="500"
              placeholder="Contoh: Customer minta batal via WA; produk out-of-stock; timeout pembayaran…"
              class="mt-3 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
            />
            <p class="mt-1 text-xs text-ink-400 text-right">{{ cancelReason.length }} / 500</p>
            <div class="mt-4 flex justify-end gap-2">
              <button
                type="button"
                class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-50"
                :disabled="cancelBusy"
                @click="cancelOpen = false"
              >
                Batal
              </button>
              <button
                type="button"
                class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-1.5 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-60"
                :disabled="cancelBusy || cancelReason.trim().length < 3"
                @click="submitCancel"
              >
                <Loader2 v-if="cancelBusy" class="h-3.5 w-3.5 animate-spin" :stroke-width="1.75" />
                Ya, cancel order
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- ============================ Edit pesanan modal (super admin) ============================ -->
    <Teleport to="body">
      <Transition name="fade">
        <div v-if="editOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4">
          <div class="absolute inset-0 bg-ink-950/60" aria-hidden="true" @click="!editBusy && (editOpen = false)" />
          <div role="dialog" aria-modal="true" class="relative w-full max-w-lg rounded-lg bg-canvas shadow-xl ring-1 ring-black/5 p-6 max-h-[90vh] overflow-y-auto">
            <h3 class="font-serif text-lg font-semibold text-ink-950">Edit pesanan {{ order?.resi }}</h3>
            <p class="mt-1 text-xs text-ink-500 leading-relaxed">
              Perubahan tercatat di jejak audit. Data penerima &amp; alamat selalu bisa diedit; subtotal &amp; ongkir
              butuh alasan kalau pesanan sudah dibayar.
            </p>

            <AlertMessage v-if="modalError" variant="error" :message="modalError" class="mt-4" />

            <div class="mt-4 space-y-3">
              <div>
                <label for="edit-recipient-name" class="block text-xs font-medium text-ink-700">Nama Penerima</label>
                <input
                  id="edit-recipient-name"
                  v-model="editForm.shipping_recipient_name"
                  type="text"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                >
                <p class="mt-1 text-[11px] text-ink-400 leading-relaxed">
                  Berlaku untuk pesanan ini saja — tidak mengubah data akun pelanggan.
                </p>
              </div>
              <div>
                <label for="edit-recipient-phone" class="block text-xs font-medium text-ink-700">No. WA Penerima</label>
                <input
                  id="edit-recipient-phone"
                  v-model="editForm.shipping_recipient_phone"
                  type="text"
                  placeholder="62xxxxxxxxxx"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm font-mono placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                >
                <p class="mt-1 text-[11px] text-ink-400 leading-relaxed">
                  Berlaku untuk pesanan ini saja — tidak mengubah data akun pelanggan.
                </p>
              </div>
              <div>
                <label for="edit-shipping-address" class="block text-xs font-medium text-ink-700">Alamat pengiriman</label>
                <textarea
                  id="edit-shipping-address"
                  v-model="editForm.shipping_address"
                  rows="2"
                  placeholder="Kosongkan kalau pickup"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                />
              </div>
              <div>
                <label for="edit-notes" class="block text-xs font-medium text-ink-700">Catatan</label>
                <textarea
                  id="edit-notes"
                  v-model="editForm.notes"
                  rows="2"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                />
              </div>
              <!-- Baris item pesanan (§32.9) -->
              <div class="rounded-md border border-hairline p-3">
                <div class="flex items-center justify-between gap-2">
                  <label class="block text-xs font-medium text-ink-700">Baris item pesanan</label>
                  <button
                    v-if="!itemsEditMode"
                    type="button"
                    class="text-xs font-medium text-brand-600 hover:text-brand-700 underline underline-offset-2 transition-colors"
                    @click="openItemsEditor"
                  >
                    Ubah baris item
                  </button>
                  <button
                    v-else
                    type="button"
                    class="text-xs font-medium text-ink-500 hover:text-ink-700 transition-colors"
                    @click="closeItemsEditor"
                  >
                    Batalkan perubahan item
                  </button>
                </div>

                <!-- Read-only view -->
                <div v-if="!itemsEditMode" class="mt-2">
                  <OrderItemsTable :items="order?.items ?? []" />
                </div>

                <!-- Editor -->
                <div v-else class="mt-2 space-y-3">
                  <p v-if="editProductsLoading" class="text-xs text-ink-500">Memuat daftar produk…</p>
                  <div
                    v-for="(row, idx) in editItems"
                    :key="row.key"
                    class="rounded-md border border-hairline bg-canvas-alt/40 p-3"
                  >
                    <div class="flex items-center justify-between gap-2">
                      <p class="text-xs font-medium text-ink-900">
                        Baris {{ idx + 1 }}
                        <span
                          v-if="!row.id"
                          class="ml-1.5 rounded-full bg-emerald-50 px-1.5 py-0.5 text-[10px] font-medium text-emerald-800 ring-1 ring-inset ring-emerald-200"
                        >Baru</span>
                      </p>
                      <button
                        type="button"
                        :disabled="editItems.length <= 1"
                        class="rounded p-1 text-ink-500 hover:text-brand-600 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                        aria-label="Hapus baris"
                        @click="removeEditItemRow(row.key)"
                      >
                        <Trash2 class="h-3.5 w-3.5" :stroke-width="1.75" />
                      </button>
                    </div>

                    <!-- Baris existing: identitas read-only -->
                    <template v-if="row.id">
                      <p class="mt-1.5 text-sm text-ink-900">
                        {{ row.productName }} <span class="text-ink-500">· {{ row.materialName }}</span>
                      </p>
                      <p class="text-[11px] text-ink-400">Ganti produk/bahan? Hapus baris ini, lalu tambah baris baru.</p>
                    </template>

                    <!-- Baris baru: pemilih produk/bahan -->
                    <template v-else>
                      <div class="mt-1.5 grid gap-2 sm:grid-cols-2">
                        <select
                          v-model="row.productId"
                          class="rounded-md border border-hairline bg-canvas px-2.5 py-1.5 text-xs text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                          @change="onEditRowProductChange(row)"
                        >
                          <option value="">Pilih produk</option>
                          <option v-for="p in editProducts" :key="p.id" :value="p.id">{{ p.name }}</option>
                        </select>
                        <select
                          v-model="row.materialId"
                          :disabled="!row.productDetail || row.loadingDetail"
                          class="rounded-md border border-hairline bg-canvas px-2.5 py-1.5 text-xs text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:bg-canvas-alt disabled:text-ink-500"
                          @change="onEditRowMaterialChange(row)"
                        >
                          <option value="">{{ row.loadingDetail ? 'Memuat…' : 'Pilih bahan' }}</option>
                          <option v-for="m in editRowMaterials(row)" :key="m.id" :value="m.id">{{ m.label }}</option>
                        </select>
                      </div>
                    </template>

                    <div class="mt-2 grid grid-cols-3 gap-2">
                      <input
                        v-model.number="row.widthCm"
                        type="number"
                        min="1"
                        placeholder="Lebar cm"
                        :disabled="!row.id && editRowIsPaket(row)"
                        class="rounded-md border border-hairline bg-canvas px-2.5 py-1.5 text-xs text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:bg-canvas-alt disabled:text-ink-500"
                        @input="!row.id && scheduleEditRowQuote(row)"
                      >
                      <input
                        v-model.number="row.heightCm"
                        type="number"
                        min="1"
                        placeholder="Tinggi cm"
                        :disabled="!row.id && editRowIsPaket(row)"
                        class="rounded-md border border-hairline bg-canvas px-2.5 py-1.5 text-xs text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:bg-canvas-alt disabled:text-ink-500"
                        @input="!row.id && scheduleEditRowQuote(row)"
                      >
                      <input
                        v-model.number="row.quantity"
                        type="number"
                        min="1"
                        placeholder="Qty"
                        class="rounded-md border border-hairline bg-canvas px-2.5 py-1.5 text-xs text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                      >
                    </div>

                    <div v-if="row.id" class="mt-2">
                      <label class="text-[11px] text-ink-500">Harga satuan (Rp)</label>
                      <input
                        v-model.number="row.unitPrice"
                        type="number"
                        min="0"
                        step="1000"
                        class="mt-0.5 block w-full rounded-md border border-hairline bg-canvas px-2.5 py-1.5 text-xs text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                      >
                    </div>
                    <div v-else class="mt-2 text-xs text-ink-500">
                      <Loader2 v-if="row.quoteLoading" class="inline h-3 w-3 animate-spin align-[-1px]" :stroke-width="1.75" />
                      <template v-else-if="row.quote">Estimasi {{ fmtIDR(row.quote.total_price) }} / pcs (dihitung ulang backend)</template>
                      <template v-else>Lengkapi produk, bahan &amp; ukuran untuk estimasi harga</template>
                      <p v-if="row.quoteError" class="mt-0.5 text-brand-700">{{ row.quoteError }}</p>

                      <div class="mt-2">
                        <p class="text-[11px] font-medium text-ink-700">Sumber desain</p>
                        <div class="mt-1 flex gap-3 text-xs text-ink-700">
                          <label class="flex items-center gap-1.5">
                            <input v-model="row.designSource" type="radio" value="upload" class="accent-brand-500">
                            Upload sendiri
                          </label>
                          <label class="flex items-center gap-1.5">
                            <input v-model="row.designSource" type="radio" value="request" class="accent-brand-500">
                            Minta jasa desain
                          </label>
                        </div>
                        <textarea
                          v-if="row.designSource === 'request'"
                          v-model="row.designBrief"
                          rows="2"
                          placeholder="Brief singkat"
                          class="mt-1.5 block w-full rounded-md border border-hairline bg-canvas px-2.5 py-1.5 text-xs text-ink-900 placeholder-ink-400 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                        />
                      </div>
                    </div>

                    <input
                      v-model="row.itemNotes"
                      type="text"
                      maxlength="1000"
                      placeholder="Catatan baris (opsional)"
                      class="mt-2 block w-full rounded-md border border-hairline bg-canvas px-2.5 py-1.5 text-xs text-ink-900 placeholder-ink-400 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                    >

                    <div class="mt-2 flex justify-between border-t border-hairline pt-2 text-xs">
                      <span class="text-ink-500">Subtotal baris</span>
                      <span class="font-medium text-ink-900">{{ fmtIDR(editRowSubtotal(row)) }}</span>
                    </div>
                  </div>

                  <button
                    type="button"
                    :disabled="editItems.length >= MAX_EDIT_ITEMS"
                    class="inline-flex items-center gap-1.5 rounded-md border border-hairline bg-canvas px-3 py-1.5 text-xs font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                    @click="addEditItemRow"
                  >
                    <Plus class="h-3.5 w-3.5" :stroke-width="1.75" />
                    Tambah baris ({{ editItems.length }}/{{ MAX_EDIT_ITEMS }})
                  </button>
                </div>
              </div>

              <div>
                <label for="edit-shipping-cost" class="block text-xs font-medium text-ink-700">Ongkir (Rp)</label>
                <input
                  id="edit-shipping-cost"
                  v-model.number="editForm.shipping_cost"
                  type="number"
                  min="0"
                  step="1000"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                >
              </div>

              <div class="rounded-md border border-hairline bg-canvas-alt/60 px-3 py-2.5">
                <dl class="space-y-1 text-xs">
                  <div class="flex justify-between">
                    <dt class="text-ink-500">Subtotal{{ itemsEditMode ? ' (perkiraan)' : '' }}</dt>
                    <dd class="text-ink-900">{{ fmtIDR(estimatedSubtotal) }}</dd>
                  </div>
                  <div v-if="currentDiscountAmount > 0" class="flex justify-between">
                    <dt class="text-ink-500">Diskon</dt>
                    <dd class="text-ink-900">-{{ fmtIDR(currentDiscountAmount) }}</dd>
                  </div>
                  <div class="flex justify-between border-t border-hairline pt-1 mt-1">
                    <dt class="font-medium text-ink-700">Total (perkiraan)</dt>
                    <dd class="font-serif text-base font-semibold text-ink-950">{{ fmtIDR(editComputedTotal) }}</dd>
                  </div>
                </dl>
                <p class="mt-1.5 text-[11px] text-ink-400 leading-relaxed">
                  Angka final dihitung ulang backend setelah disimpan — ini cuma bantuan pratinjau.
                </p>
              </div>

              <p v-if="itemsEditMode && discountExceedsSubtotal" class="flex items-start gap-2 rounded-md border border-brand-200 bg-brand-50 p-3 text-xs text-brand-800 leading-relaxed">
                <AlertTriangle class="h-3.5 w-3.5 flex-none mt-0.5" :stroke-width="1.75" />
                Potongan diskon ({{ fmtIDR(currentDiscountAmount) }}) jadi melebihi nilai barang hasil edit ini
                ({{ fmtIDR(estimatedSubtotal) }}). Kurangi jumlah baris yang dihapus atau naikkan harga/qty
                sebelum menyimpan.
              </p>

              <div v-if="editReasonRequired" class="rounded-md border border-brand-200 bg-brand-50 p-3">
                <p class="flex items-start gap-2 text-xs text-brand-800 leading-relaxed">
                  <AlertTriangle class="h-3.5 w-3.5 flex-none mt-0.5" :stroke-width="1.75" />
                  Pesanan ini sudah berstatus <strong>{{ statusLabel(order?.status ?? '') }}</strong> — invoice mungkin
                  sudah terkirim ke pelanggan. Mengubah ongkir/item di sini TIDAK mengirim invoice baru, jadi bisa
                  jadi tidak cocok lagi dengan yang diterima pelanggan. Jelaskan alasannya di bawah.
                </p>
                <label for="edit-reason" class="mt-2 block text-xs font-medium text-brand-800">
                  Alasan perubahan (wajib, min. 10 karakter)
                </label>
                <textarea
                  id="edit-reason"
                  v-model="editForm.reason"
                  rows="2"
                  maxlength="500"
                  placeholder="Contoh: koreksi ongkir salah input, kurir ganti tarif, dll."
                  class="mt-1 block w-full rounded-md border border-brand-200 bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                />
              </div>
            </div>

            <div class="mt-5 flex justify-end gap-2">
              <button
                type="button"
                class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-50"
                :disabled="editBusy"
                @click="editOpen = false"
              >
                Batal
              </button>
              <button
                type="button"
                class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-1.5 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-60"
                :disabled="editBusy || !editCanSubmit"
                @click="submitEdit"
              >
                <Loader2 v-if="editBusy" class="h-3.5 w-3.5 animate-spin" :stroke-width="1.75" />
                Simpan perubahan
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- ============================ Override status modal (super admin) ============================ -->
    <Teleport to="body">
      <Transition name="fade">
        <div v-if="overrideOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4">
          <div class="absolute inset-0 bg-ink-950/60" aria-hidden="true" @click="!overrideBusy && (overrideOpen = false)" />
          <div role="dialog" aria-modal="true" class="relative w-full max-w-md rounded-lg bg-canvas shadow-xl ring-1 ring-black/5 p-6">
            <h3 class="font-serif text-lg font-semibold text-ink-950">Ubah status paksa</h3>
            <p class="mt-1 text-xs text-ink-500 leading-relaxed">
              Override bisa lompat ke status apa pun di alur pesanan, di luar urutan normal. Tenang — aksi ini
              <strong class="text-ink-900">tidak mengirim notifikasi WA</strong> ke pelanggan, dan tercatat di jejak audit.
            </p>

            <AlertMessage v-if="modalError" variant="error" :message="modalError" class="mt-4" />

            <div class="mt-4 space-y-3">
              <div>
                <label for="override-status" class="block text-xs font-medium text-ink-700">Status baru</label>
                <select
                  id="override-status"
                  v-model="overrideToStatus"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                >
                  <option v-for="s in ALL_ORDER_STATUSES" :key="s" :value="s">
                    {{ statusLabel(s) }}{{ order && s === order.status ? ' (status saat ini)' : '' }}
                  </option>
                </select>
              </div>
              <div>
                <label for="override-reason" class="block text-xs font-medium text-ink-700">
                  Alasan override (wajib, min. 10 karakter)
                </label>
                <textarea
                  id="override-reason"
                  v-model="overrideReason"
                  rows="3"
                  maxlength="500"
                  placeholder="Contoh: sinkronisasi status manual setelah kendala teknis, koreksi input kasir, dll."
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                />
                <p class="mt-1 text-xs text-ink-400 text-right">{{ overrideReason.length }} / 500</p>
              </div>
            </div>

            <div class="mt-4 flex justify-end gap-2">
              <button
                type="button"
                class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-50"
                :disabled="overrideBusy"
                @click="overrideOpen = false"
              >
                Batal
              </button>
              <button
                type="button"
                class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-1.5 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-60"
                :disabled="overrideBusy || !overrideCanSubmit"
                @click="submitOverride"
              >
                <Loader2 v-if="overrideBusy" class="h-3.5 w-3.5 animate-spin" :stroke-width="1.75" />
                Terapkan status
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- ============================ Hapus pesanan modal (super admin) ============================ -->
    <Teleport to="body">
      <Transition name="fade">
        <div v-if="deleteOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4">
          <div class="absolute inset-0 bg-ink-950/60" aria-hidden="true" @click="!deleteBusy && (deleteOpen = false)" />
          <div role="dialog" aria-modal="true" class="relative w-full max-w-md rounded-lg bg-canvas shadow-xl ring-1 ring-black/5 p-6">
            <h3 class="font-serif text-lg font-semibold text-ink-950">Hapus pesanan {{ order?.resi }}?</h3>
            <p class="mt-1 text-xs text-ink-500 leading-relaxed">
              Pesanan akan hilang dari daftar &amp; rekap admin (soft delete). Nomor resi
              <span class="font-mono text-[11px] text-ink-700">{{ order?.resi }}</span> tetap bisa dilacak publik di
              halaman lacak resi. Alasan wajib diisi untuk audit.
            </p>
            <AlertMessage v-if="modalError" variant="error" :message="modalError" class="mt-3" />
            <textarea
              v-model="deleteReason"
              rows="3"
              maxlength="500"
              placeholder="Contoh: order duplikat, dibuat keliru oleh kasir, dll."
              class="mt-3 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
            />
            <p class="mt-1 text-xs text-ink-400 text-right">{{ deleteReason.length }} / 500</p>
            <div class="mt-4 flex justify-end gap-2">
              <button
                type="button"
                class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-50"
                :disabled="deleteBusy"
                @click="deleteOpen = false"
              >
                Batal
              </button>
              <button
                type="button"
                class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-1.5 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-60"
                :disabled="deleteBusy || !deleteCanSubmit"
                @click="submitDelete"
              >
                <Loader2 v-if="deleteBusy" class="h-3.5 w-3.5 animate-spin" :stroke-width="1.75" />
                Ya, hapus pesanan
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </section>
</template>

<style scoped>
.fade-enter-active, .fade-leave-active {
  transition: opacity 0.15s ease;
}
.fade-enter-from, .fade-leave-to {
  opacity: 0;
}
</style>
