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
import type { Order } from '~/types/order'
import type { PaymentProof } from '~/types/payment'
import type { DesignFile } from '~/types/design'
import type { AdminOrderCustomer, AdminOrderHistoryRow } from '~/composables/useOrder'
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

const errorMsg = ref<string | null>(null)
const successMsg = ref<string | null>(null)

useSeoMeta({ title: () => `Order ${resi.value} — Admin` })

function showSuccess(msg: string) {
  successMsg.value = msg
  setTimeout(() => (successMsg.value = null), 3000)
}

function toApiError(e: unknown, fallback: string): string {
  return e instanceof ApiError ? e.message : fallback
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

onMounted(async () => {
  await loadDetail()
  if (order.value) {
    // Kick off attachments fetch in parallel — tidak menahan render.
    loadProofs()
    loadDesignFiles()
  }
  loadReceiptConfig()
})

// -------------------- cetak ulang struk (§12) --------------------
/**
 * Lebar kertas struk aktif, dipakai `ReceiptStruk` untuk `@page` dinamis.
 * Fallback 58mm kalau endpoint config gagal/belum tersedia — kegagalan di
 * sini TIDAK BOLEH menggagalkan render halaman detail order, cuma fitur
 * cetak ulang yang terdampak (tetap jalan, cuma pakai default).
 */
const receiptWidthMm = ref<58 | 80>(58)
async function loadReceiptConfig() {
  try {
    const cfg = await pos.receiptConfig()
    receiptWidthMm.value = cfg.width_mm === 80 ? 80 : 58
  } catch (e) {
    console.error('Gagal memuat receipt-config, pakai default 58mm', e)
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

/**
 * `nextTick()` SAJA TIDAK CUKUP di sini, dan ini pernah menghasilkan cetakan
 * kacau di lapangan: nextTick hanya menjamin DOM ter-patch, sementara dua hal
 * yang menentukan bentuk cetakan justru menyusul setelahnya —
 *
 *   1. `<style>` @page (lebar kertas 58/80mm) disuntik `useHead` secara
 *      ASINKRON, tidak pada tick yang sama dengan mount komponen.
 *   2. CSS isolasi cetak milik ReceiptStruk (`body *` disembunyikan, `#struk`
 *      ditampilkan) baru aktif setelah style komponen benar-benar terpasang.
 *
 * Kalau `window.print()` dipanggil sebelum keduanya siap, browser mencetak
 * SELURUH halaman admin dengan ukuran kertas default — bukan struk. Halaman
 * POS tidak kena karena di sana struk ter-mount permanen di panel sukses.
 *
 * Dua rAF berturut-turut menjamin satu siklus render + style flush penuh
 * selesai. Loop verifikasi di bawahnya adalah jaring pengaman terakhir:
 * lebih baik menunda cetak beberapa milidetik daripada mengeluarkan kertas
 * yang salah — kertas thermal yang sudah tercetak tidak bisa ditarik kembali.
 */
async function waitForReceiptReady() {
  await nextTick()
  await new Promise<void>((resolve) => {
    requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
  })
  for (let i = 0; i < 20 && !document.getElementById('struk'); i++) {
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
  }
}

async function printStruk() {
  receiptMounted.value = true
  await waitForReceiptReady()
  try {
    window.print()
  } finally {
    // window.print() memblokir sampai dialog ditutup di sebagian besar
    // browser, tapi tidak dijamin — `finally` + afterprint dua-duanya
    // dipasang supaya struk tidak pernah tertinggal ter-mount.
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
const isPickup = computed(() => order.value?.metode_ambil === 'pickup')

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

async function submitCancel() {
  if (cancelReason.value.trim().length < 3) {
    errorMsg.value = 'Alasan pembatalan minimal 3 karakter.'
    return
  }
  cancelBusy.value = true
  errorMsg.value = null
  try {
    order.value = await orderSvc.cancelOrder(resi.value, cancelReason.value.trim())
    cancelOpen.value = false
    cancelReason.value = ''
    showSuccess('Order dibatalkan.')
    await loadDetail()
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal cancel order')
  } finally {
    cancelBusy.value = false
  }
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
          class="inline-flex items-center gap-1.5 rounded-md border border-hairline bg-canvas px-2.5 py-1.5 text-xs font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
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
          class="inline-flex items-center gap-2 rounded-md border border-brand-200 bg-canvas px-3 py-1.5 text-sm font-semibold text-brand-700 hover:bg-brand-50 hover:border-brand-300 transition-colors"
          @click="cancelOpen = true"
        >
          <XCircle class="h-4 w-4" :stroke-width="1.75" />
          Cancel
        </button>
      </template>
    </AdminPageHeader>

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

        <!-- Detail produk -->
        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <div class="flex items-center gap-2 mb-3">
            <PackageIcon class="h-4 w-4 text-ink-500" :stroke-width="1.75" />
            <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Detail produk</p>
          </div>
          <p class="font-serif text-xl font-semibold tracking-tight text-ink-950">{{ order.product_name }}</p>
          <p class="mt-1 text-sm text-ink-500">
            {{ order.material_name }}
            <span class="text-ink-400">·</span>
            <span class="font-mono text-xs text-ink-700">{{ order.width_cm }} × {{ order.height_cm }} cm</span>
            <span class="text-ink-400">·</span>
            {{ order.quantity }} pcs
          </p>
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
              <dt class="text-ink-500">Harga satuan</dt>
              <dd class="text-ink-900">{{ fmtIDR(order.unit_price) }}</dd>
            </div>
            <div class="flex justify-between">
              <dt class="text-ink-500">Subtotal</dt>
              <dd class="text-ink-900">{{ fmtIDR(order.subtotal) }}</dd>
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
              {{ order.design_source === 'upload' ? '(customer upload sendiri)' : '(customer minta jasa desain)' }}
            </span>
          </p>
          <p v-if="order.design_brief" class="mt-2 text-sm text-ink-700 leading-relaxed border-l-2 border-gold-300 pl-3">
            "{{ order.design_brief }}"
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
      </div>

      <!-- ================================ RIGHT COLUMN ================================ -->
      <aside class="space-y-4 lg:sticky lg:top-6 lg:self-start">
        <!-- Set ongkir (kirim + menunggu_ongkir + perm) -->
        <div
          v-if="isMenungguOngkir && !isPickup && canSetOngkir"
          class="rounded-lg border border-hairline bg-canvas p-5"
        >
          <h3 class="text-sm font-semibold text-ink-900">Set ongkir</h3>
          <p class="mt-1 text-xs text-ink-500 leading-relaxed">
            Setelah disimpan, customer dinotifikasi via WA dengan total final untuk dibayar.
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
          v-if="isMenungguOngkir && isPickup && canUpdateStatus"
          class="rounded-lg border border-hairline bg-canvas p-5"
        >
          <h3 class="text-sm font-semibold text-ink-900">Konfirmasi total (pickup)</h3>
          <p class="mt-1 text-xs text-ink-500 leading-relaxed">
            Order pickup tidak butuh ongkir. Klik untuk lanjut ke menunggu pembayaran.
          </p>
          <button
            type="button"
            class="mt-4 w-full inline-flex items-center justify-center gap-2 rounded-md bg-ink-950 px-3 py-2 text-sm font-semibold text-canvas hover:bg-ink-900 transition-colors disabled:opacity-60"
            :disabled="pickupBusy"
            @click="confirmPickup"
          >
            <Loader2 v-if="pickupBusy" class="h-3.5 w-3.5 animate-spin" :stroke-width="1.75" />
            Konfirmasi total pickup
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
         atas) — cuma dipakai saat `window.print()` dipicu tombol "Cetak
         struk" di header. `hidden print:block` WAJIB dipakai (bukan
         `visibility`/`sr-only`) supaya elemen ini tidak `display: none` saat
         cetak — teknik isolasi print di ReceiptStruk.vue mengandalkan
         `visibility: visible !important` pada `#struk`, yang tidak bisa
         menembus `display: none` di elemen leluhurnya. -->
    <div v-if="order && receiptMounted" class="hidden print:block">
      <AdminReceiptStruk
        :resi="order.resi"
        :created-at="order.created_at"
        :customer-name="customer?.name ?? '—'"
        :customer-phone="customer?.phone ?? '—'"
        :product-name="order.product_name"
        :material-name="order.material_name"
        :width-cm="order.width_cm"
        :height-cm="order.height_cm"
        :quantity="order.quantity"
        :unit-price="order.unit_price"
        :subtotal="order.subtotal"
        :shipping-cost="order.shipping_cost"
        :total="order.total"
        :metode-ambil="order.metode_ambil"
        :metode-bayar="order.metode_bayar ?? '-'"
        :tracking-url="trackingUrl"
        :is-paid="orderIsPaid"
        :width-mm="receiptWidthMm"
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
