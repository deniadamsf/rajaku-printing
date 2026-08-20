<script setup lang="ts">
/**
 * /lacak/[resi] — Public tracking page (§5).
 *
 * Anyone with resi bisa buka — data sensitif (alamat, no HP) sudah di-sensor
 * di backend (`shipping_address="Jl. Merdek**"`, `shipping_phone="0812****678"`).
 * History status ditampilkan sebagai timeline dgn label human-readable.
 */
import {
  Package,
  MapPin,
  Truck,
  Search,
  AlertTriangle,
  CheckCircle2,
  CircleDot,
  Circle,
  Loader2,
  ExternalLink,
  ShieldCheck,
  LogOut,
  Clock,
  Upload,
  FileText,
  FileWarning,
  Wallet,
  Receipt,
  XCircle,
} from '@lucide/vue'
import { ApiError } from '~/composables/useApi'
import type { PublicTracking } from '~/types/tracking'
import type { DesignFile } from '~/types/design'
import type { PaymentProofCustomer } from '~/types/payment'

// Slot QRIS dikelola admin lewat /admin/site-media. Tidak ada fallback
// statis: kalau belum diunggah, panel QRIS memang tidak ditampilkan.
const qrisUrl = computed(() => useSiteMedia().resolve('qris_code'))

// Rekening & QRIS dari modul `settings` (§7). TIDAK ADA fallback hardcode —
// lihat docblock usePaymentInfo untuk alasan keamanan. Kalau `paymentInfo`
// null, panel "transfer ke" disembunyikan di template.
const { info: paymentInfo } = usePaymentInfo()

definePageMeta({ layout: 'default' })

const route = useRoute()
const orderApi = useOrder()

const resi = computed(() => {
  const r = route.params.resi
  return typeof r === 'string' ? r.toUpperCase() : ''
})

useSeoMeta({
  title: () => `Lacak ${resi.value} — Rajaku Printing`,
  description: 'Cek status pesanan Anda di Rajaku Printing.',
  robots: 'noindex,nofollow', // tracking pages personal — jangan ke-index Google
})

// -------------------- fetch --------------------
const state = ref<'loading' | 'ok' | 'error'>('loading')
const error = ref<string | null>(null)
const data = ref<PublicTracking | null>(null)

async function load() {
  state.value = 'loading'
  error.value = null
  try {
    data.value = await orderApi.publicTracking(resi.value)
    state.value = 'ok'
  } catch (e) {
    state.value = 'error'
    error.value = e instanceof ApiError ? e.message : 'Gagal memuat data tracking'
  }
}

onMounted(load)

// -------------------- guest ownership verification (§ guest design upload) --------------------
// Guest (order tanpa akun) buktikan kepemilikan pakai resi + nomor WA, dapat
// token sesi terbatas (30 menit, sessionStorage only) untuk unggah desain —
// lihat useGuestOrderSession. Endpoint aksi desain sama dgn customer login,
// cuma header Authorization pakai token guest ini (lihat useDesign override).
const guestSession = useGuestOrderSession(() => resi.value)
const guestDesignApi = useDesign({ token: () => guestSession.token.value })
const guestPaymentApi = usePayment({ token: () => guestSession.token.value })

const guestPhone = ref('')

async function submitGuestVerify() {
  const phone = guestPhone.value.trim()
  if (!phone) return
  const ok = await guestSession.verify(phone)
  if (ok) guestPhone.value = ''
}

// Aturan gating HARUS sama persis dengan backend (design_service.go
// UploadCustomerFile), kalau tidak kartu tampil tapi upload ditolak 400:
//   design_source 'upload'  → hanya status 'dibayar'
//   design_source 'request' → 'dibayar' / 'desain_dikerjakan' / 'menunggu_approval_desain'
// Sama dengan `customerUploadKind` di /akun/pesanan/[resi].vue.
const guestUploadKind = computed<'upload' | 'request' | null>(() => {
  if (!guestSession.isVerified.value || !data.value) return null
  const d = data.value
  if (d.design_source === 'upload' && d.status === 'dibayar') return 'upload'
  if (
    d.design_source === 'request' &&
    ['dibayar', 'desain_dikerjakan', 'menunggu_approval_desain'].includes(d.status)
  ) {
    return 'request'
  }
  return null
})

const canUploadGuestDesign = computed(() => guestUploadKind.value !== null)

const guestDesignFiles = ref<DesignFile[]>([])
const loadingGuestDesignFiles = ref(false)

async function loadGuestDesignFiles() {
  if (!guestSession.isVerified.value) return
  loadingGuestDesignFiles.value = true
  try {
    const res = await guestDesignApi.listByResi(resi.value)
    guestDesignFiles.value = res.items ?? []
  } catch {
    guestDesignFiles.value = []
  } finally {
    loadingGuestDesignFiles.value = false
  }
}

watch(
  () => guestSession.isVerified.value,
  (verified) => {
    if (verified) {
      loadGuestDesignFiles()
      loadGuestProofs()
    } else {
      guestDesignFiles.value = []
      guestProofs.value = []
    }
  },
)

const guestUploadedFiles = computed(() =>
  guestDesignFiles.value.filter((f) => f.role === 'customer_upload' || f.role === 'customer_asset'),
)

function guestLogout() {
  guestSession.logout()
  guestDesignFiles.value = []
  guestProofs.value = []
}

// -------------------- guest payment proof upload (§7) --------------------
// Aturan gating identik dengan `canUploadProof` di /akun/pesanan/[resi].vue —
// kalau backend UploadProof mengubah status yg diizinkan, sinkronkan di sini juga.
const canUploadGuestProof = computed(() => {
  if (!guestSession.isVerified.value || !data.value) return false
  const s = data.value.status
  return s === 'menunggu_pembayaran' || s === 'ditolak'
})

const guestProofs = ref<PaymentProofCustomer[]>([])
const loadingGuestProofs = ref(false)

async function loadGuestProofs() {
  if (!guestSession.isVerified.value) return
  loadingGuestProofs.value = true
  try {
    const res = await guestPaymentApi.listProofsForOrder(resi.value)
    guestProofs.value = res.items ?? []
  } catch {
    guestProofs.value = []
  } finally {
    loadingGuestProofs.value = false
  }
}

const guestProofFile = ref<File | null>(null)
const guestProofMetode = ref<'transfer' | 'qris'>('transfer')
const guestProofAmount = ref<number | null>(null)
const guestProofUploading = ref(false)
const guestProofFileInput = ref<HTMLInputElement | null>(null)
const guestProofError = ref<string | null>(null)
const guestProofSuccess = ref<string | null>(null)

function onGuestProofFilePick(ev: Event) {
  const input = ev.target as HTMLInputElement
  guestProofFile.value = input.files?.[0] ?? null
}

async function submitGuestProof() {
  guestProofError.value = null
  guestProofSuccess.value = null
  if (!guestProofFile.value) {
    guestProofError.value = 'Pilih file bukti transfer dulu.'
    return
  }
  guestProofUploading.value = true
  try {
    await guestPaymentApi.uploadProof(resi.value, {
      file: guestProofFile.value,
      metodeBayar: guestProofMetode.value,
      amountClaimed: guestProofAmount.value ?? undefined,
    })
    guestProofSuccess.value = 'Bukti transfer diupload. Tim kami akan verifikasi segera.'
    guestProofFile.value = null
    if (guestProofFileInput.value) guestProofFileInput.value.value = ''
    guestProofAmount.value = null
    // Upload bukti bisa memindahkan status order (mis. menunggu_pembayaran →
    // menunggu_verifikasi) — reload timeline + daftar bukti supaya UI sinkron.
    await Promise.all([load(), loadGuestProofs()])
  } catch (e) {
    guestProofError.value = e instanceof ApiError ? e.message : 'Gagal upload bukti transfer'
  } finally {
    guestProofUploading.value = false
  }
}

const proofStatusLabelMap: Record<string, string> = {
  pending: 'Menunggu verifikasi',
  approved: 'Terverifikasi',
  rejected: 'Ditolak',
}
function proofStatusLabel(s: string): string {
  return proofStatusLabelMap[s] ?? s
}
function proofStatusBadgeClass(s: string): string {
  if (s === 'approved') return 'bg-emerald-50 text-emerald-800 ring-emerald-200'
  if (s === 'rejected') return 'bg-brand-50 text-brand-700 ring-brand-200'
  return 'bg-amber-50 text-amber-800 ring-amber-200'
}

function fmtIDR(n?: number | null): string {
  if (n == null) return '—'
  return new Intl.NumberFormat('id-ID', {
    style: 'currency',
    currency: 'IDR',
    maximumFractionDigits: 0,
  }).format(n)
}

// -------------------- guest design upload form --------------------
// Validasi & batas identik dengan /akun/pesanan/[resi].vue (jalur customer
// login) — jangan menyimpang supaya perilaku konsisten lintas jalur.
const DESIGN_MAX_UPLOAD_MB = 25
const DESIGN_ALLOWED_EXT = ['jpg', 'jpeg', 'png', 'webp', 'pdf', 'cdr', 'ai']

const guestDesignFile = ref<File | null>(null)
const guestDesignNotes = ref('')
const guestDesignUploading = ref(false)
const guestDesignFileInput = ref<HTMLInputElement | null>(null)
const guestDesignError = ref<string | null>(null)
const guestDesignSuccess = ref<string | null>(null)

function onGuestDesignFilePick(ev: Event) {
  const input = ev.target as HTMLInputElement
  guestDesignFile.value = input.files?.[0] ?? null
}

function validateGuestDesignFile(file: File): string | null {
  if (file.size === 0) return 'File kosong, pilih file yang valid.'
  const ext = file.name.split('.').pop()?.toLowerCase() ?? ''
  if (!DESIGN_ALLOWED_EXT.includes(ext)) {
    return `Format .${ext || '?'} tidak didukung. Format yang diterima: ${DESIGN_ALLOWED_EXT.map((e) => `.${e}`).join(', ')}.`
  }
  const maxBytes = DESIGN_MAX_UPLOAD_MB * 1024 * 1024
  if (file.size > maxBytes) {
    return `Ukuran file ${formatBytes(file.size)} melebihi batas ${DESIGN_MAX_UPLOAD_MB} MB.`
  }
  return null
}

async function submitGuestDesign() {
  guestDesignError.value = null
  guestDesignSuccess.value = null
  if (!guestDesignFile.value) {
    guestDesignError.value = 'Pilih file desain dulu.'
    return
  }
  const validationError = validateGuestDesignFile(guestDesignFile.value)
  if (validationError) {
    guestDesignError.value = validationError
    return
  }
  guestDesignUploading.value = true
  try {
    await guestDesignApi.uploadCustomerFile(
      resi.value,
      guestDesignFile.value,
      guestDesignNotes.value.trim() || undefined,
    )
    guestDesignSuccess.value = 'File desain terupload. Tim kami akan memeriksanya.'
    guestDesignFile.value = null
    guestDesignNotes.value = ''
    if (guestDesignFileInput.value) guestDesignFileInput.value.value = ''
    // Upload aset pertama bisa meng-advance status order di backend — reload
    // timeline + daftar file supaya UI sinkron.
    await Promise.all([load(), loadGuestDesignFiles()])
  } catch (e) {
    guestDesignError.value = e instanceof ApiError ? e.message : 'Gagal upload file desain'
  } finally {
    guestDesignUploading.value = false
  }
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

function fmtMinutesLeft(ms: number): string {
  return String(Math.max(0, Math.ceil(ms / 60000)))
}

// -------------------- helpers --------------------
// Status labels sesuai §4 state machine.
const statusLabel: Record<string, string> = {
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

function labelOf(status: string): string {
  return statusLabel[status] ?? status
}

// Icon per status milestone di timeline.
function statusIcon(status: string, isCurrent: boolean) {
  if (status === 'selesai') return CheckCircle2
  if (isCurrent) return CircleDot
  return Circle
}

function fmtDateTime(iso?: string): string {
  if (!iso) return ''
  try {
    return new Date(iso).toLocaleString('id-ID', {
      day: '2-digit', month: 'short', year: 'numeric',
      hour: '2-digit', minute: '2-digit',
    })
  } catch {
    return iso
  }
}

// Kirim / pickup subtitle di header.
const fulfillmentLabel = computed(() => {
  if (!data.value) return ''
  return data.value.metode_ambil === 'kirim' ? 'Kirim' : 'Ambil di tempat'
})

// Sorted history — backend already ordered chronologically, but be defensive.
const sortedHistory = computed(() => {
  if (!data.value) return []
  return [...data.value.history].sort(
    (a, b) => new Date(a.changed_at).getTime() - new Date(b.changed_at).getTime(),
  )
})

// Current status = last history row's status (or data.status fallback).
const currentStatus = computed(() => data.value?.status || sortedHistory.value.at(-1)?.status || '')

// Copy resi to clipboard convenience.
const copied = ref(false)
async function copyResi() {
  if (!data.value) return
  try {
    await navigator.clipboard.writeText(data.value.resi)
    copied.value = true
    setTimeout(() => (copied.value = false), 1500)
  } catch {
    // no-op
  }
}
</script>

<template>
  <main class="mx-auto max-w-3xl px-4 py-10 md:py-16">
    <!-- Header eyebrow -->
    <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2 flex items-center gap-2">
      <Search class="h-3.5 w-3.5" :stroke-width="1.75" />
      Lacak Pesanan
    </p>

    <!-- Loading -->
    <div v-if="state === 'loading'" class="rounded-lg border border-hairline bg-canvas p-12 text-center">
      <Loader2 class="mx-auto h-5 w-5 animate-spin text-ink-500" :stroke-width="1.75" />
      <p class="mt-3 text-sm text-ink-500">Memuat data resi <span class="font-mono">{{ resi }}</span>…</p>
    </div>

    <!-- Error -->
    <div v-else-if="state === 'error'" class="rounded-lg border border-brand-200 bg-brand-50/50 p-8">
      <div class="flex items-start gap-3">
        <AlertTriangle class="h-5 w-5 text-brand-700 flex-none mt-0.5" :stroke-width="1.75" />
        <div class="flex-1">
          <h1 class="font-serif text-xl font-semibold text-ink-950">Resi tidak ditemukan</h1>
          <p class="mt-1 text-sm text-ink-700 leading-relaxed">{{ error }}</p>
          <p class="mt-2 text-xs text-ink-500 leading-relaxed">
            Kemungkinan: salah ketik, atau order belum tercatat di sistem. Format resi: <span class="font-mono">RJK-XXXXXXXX</span>.
          </p>
          <div class="mt-4 flex flex-wrap gap-2">
            <NuxtLink
              to="/lacak"
              class="inline-flex items-center gap-1 rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
            >
              <Search class="h-3.5 w-3.5" :stroke-width="1.75" />
              Coba resi lain
            </NuxtLink>
            <NuxtLink
              to="/order"
              class="inline-flex items-center gap-1 rounded-md bg-brand-500 px-3 py-1.5 text-sm font-semibold text-canvas hover:bg-brand-600 transition-colors"
            >
              Order baru
            </NuxtLink>
          </div>
        </div>
      </div>
    </div>

    <!-- OK -->
    <div v-else-if="data" class="space-y-6">
      <!-- Header card -->
      <div class="rounded-lg border border-hairline bg-canvas p-6 md:p-8">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <button
              type="button"
              class="group flex items-center gap-2 text-left"
              @click="copyResi"
            >
              <span class="font-serif text-2xl md:text-3xl font-semibold tracking-tight text-ink-950">{{ data.resi }}</span>
              <span
                :class="[
                  'text-[10px] font-medium uppercase tracking-[0.14em] px-2 py-0.5 rounded-full ring-1 ring-inset transition-colors',
                  copied ? 'text-emerald-800 bg-emerald-50 ring-emerald-200' : 'text-ink-500 bg-canvas-alt ring-hairline group-hover:text-ink-900',
                ]"
              >
                {{ copied ? 'Disalin' : 'Salin' }}
              </span>
            </button>
            <p class="mt-1 text-sm text-ink-500">
              Dibuat {{ fmtDateTime(data.created_at) }}
              <span class="text-ink-400">·</span> Channel <span class="capitalize text-ink-700">{{ data.channel }}</span>
            </p>
          </div>
          <div class="text-right">
            <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Status sekarang</p>
            <p class="mt-0.5 font-serif text-lg font-semibold text-ink-950">{{ labelOf(currentStatus) }}</p>
          </div>
        </div>

        <div class="mt-6 grid gap-4 sm:grid-cols-2 text-sm">
          <div class="flex items-start gap-2">
            <Package class="h-4 w-4 text-ink-500 mt-0.5" :stroke-width="1.75" />
            <div>
              <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Produk</p>
              <p class="mt-0.5 text-ink-900">{{ data.product_name }}</p>
              <p class="text-xs text-ink-500">Bahan: {{ data.material_name }}</p>
            </div>
          </div>
          <div class="flex items-start gap-2">
            <Truck class="h-4 w-4 text-ink-500 mt-0.5" :stroke-width="1.75" />
            <div>
              <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Pengambilan</p>
              <p class="mt-0.5 text-ink-900">{{ fulfillmentLabel }}</p>
              <p v-if="data.shipping_recipient" class="text-xs text-ink-500">
                Penerima: {{ data.shipping_recipient }}
              </p>
            </div>
          </div>
        </div>

        <!-- Shipping address (masked) — hanya kalau kirim -->
        <div
v-if="data.metode_ambil === 'kirim' && (data.shipping_address || data.shipping_phone)"
             class="mt-4 rounded-md border border-hairline bg-canvas-alt p-3 text-xs text-ink-700">
          <div class="flex items-start gap-2">
            <MapPin class="h-3.5 w-3.5 text-ink-500 mt-0.5 flex-none" :stroke-width="1.75" />
            <div class="min-w-0">
              <p v-if="data.shipping_address" class="text-ink-700">{{ data.shipping_address }}</p>
              <p v-if="data.shipping_phone" class="mt-0.5 font-mono text-[11px] text-ink-500">
                {{ data.shipping_phone }}
              </p>
              <p class="mt-1 text-[10px] text-ink-400">
                Alamat & nomor disamarkan untuk privasi. Detail lengkap tersedia di akun pemesan.
              </p>
            </div>
          </div>
        </div>
      </div>

      <!-- Timeline -->
      <div class="rounded-lg border border-hairline bg-canvas p-6 md:p-8">
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-4">Riwayat status</p>
        <ol class="relative border-l border-hairline pl-6 space-y-5">
          <li
            v-for="(row, idx) in sortedHistory"
            :key="row.changed_at + row.status"
            class="relative"
          >
            <span
              :class="[
                'absolute -left-[27px] flex h-4 w-4 items-center justify-center rounded-full',
                idx === sortedHistory.length - 1
                  ? 'bg-brand-500 ring-4 ring-brand-500/15'
                  : 'bg-ink-950/80 ring-4 ring-canvas-alt',
              ]"
            >
              <component
                :is="statusIcon(row.status, idx === sortedHistory.length - 1)"
                class="h-3 w-3 text-canvas"
                :stroke-width="2"
              />
            </span>
            <div class="flex flex-wrap items-baseline justify-between gap-2">
              <p
                :class="[
                  'text-sm',
                  idx === sortedHistory.length - 1 ? 'font-semibold text-ink-950' : 'text-ink-700',
                ]"
              >
                {{ labelOf(row.status) }}
              </p>
              <time class="font-mono text-[11px] text-ink-500">{{ fmtDateTime(row.changed_at) }}</time>
            </div>
          </li>
        </ol>
      </div>

      <!-- ============ Guest ownership verification ============ -->
      <div
        v-if="!guestSession.isVerified.value"
        class="rounded-lg border-2 border-brand-500 bg-brand-50/40 p-6"
      >
        <div class="flex items-start gap-3">
          <ShieldCheck class="h-5 w-5 text-brand-700 flex-none mt-0.5" :stroke-width="1.75" />
          <div class="flex-1">
            <h2 class="font-serif text-lg font-semibold text-ink-950">Ini pesanan saya</h2>
            <p class="mt-1 text-sm text-ink-700 leading-relaxed">
              Kalau ini pesanan Anda, masukkan nomor WhatsApp yang dipakai saat order untuk
              membuka akses upload file desain. Nomor ini juga jadi cara kami memverifikasi
              kepemilikan tanpa perlu Anda mendaftar akun.
            </p>

            <form class="mt-4 flex flex-wrap items-end gap-3" @submit.prevent="submitGuestVerify">
              <div class="min-w-0 flex-1">
                <label for="guest-phone" class="text-sm font-medium text-ink-900">Nomor WhatsApp</label>
                <input
                  id="guest-phone"
                  v-model="guestPhone"
                  type="tel"
                  inputmode="tel"
                  placeholder="0812xxxxxxx"
                  class="mt-1 block w-full max-w-xs rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                >
              </div>
              <button
                type="button"
                :disabled="guestSession.verifying.value || !guestPhone.trim()"
                class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                @click="submitGuestVerify"
              >
                <Loader2 v-if="guestSession.verifying.value" class="h-4 w-4 animate-spin" :stroke-width="1.75" />
                <ShieldCheck v-else class="h-4 w-4" :stroke-width="1.75" />
                {{ guestSession.verifying.value ? 'Memeriksa…' : 'Verifikasi' }}
              </button>
            </form>

            <AlertMessage v-if="guestSession.errorMsg.value" variant="error" :message="guestSession.errorMsg.value" class="mt-3" />
            <!-- Petunjuk ini sengaja tampil pada SEMUA kegagalan, bukan hanya
                 kasus akun terdaftar. Verifikasi di sini khusus pesanan tanpa
                 akun; pemilik akun harus lewat login. Kalau pesannya dibedakan
                 per penyebab, respons itu sendiri membocorkan resi mana yang
                 dimiliki akun terdaftar — jadi bentuknya harus seragam. -->
            <p v-if="guestSession.errorMsg.value" class="mt-2 text-xs text-ink-500 leading-relaxed">
              Punya akun Rajaku? Pesanan yang dibuat sambil login dikelola dari
              <NuxtLink
                to="/login"
                class="font-medium text-brand-500 underline rounded-sm hover:text-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors"
              >halaman akun</NuxtLink>, bukan dari sini.
            </p>
          </div>
        </div>
      </div>

      <!-- ============ Guest verified: upload desain + logout ============ -->
      <template v-else>
        <div class="rounded-lg border border-hairline bg-canvas-alt/50 p-4 flex flex-wrap items-center justify-between gap-3">
          <div class="flex items-center gap-2 text-sm text-ink-700">
            <ShieldCheck class="h-4 w-4 text-emerald-700" :stroke-width="1.75" />
            <span>Kepemilikan pesanan terverifikasi.</span>
            <span class="hidden sm:inline-flex items-center gap-1 text-xs text-ink-500">
              <Clock class="h-3.5 w-3.5" :stroke-width="1.75" />
              Sesi berlaku 30 menit — sisa {{ fmtMinutesLeft(guestSession.expiresInMs.value) }} menit.
            </span>
          </div>
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-md border border-hairline bg-canvas px-3 py-1.5 text-xs font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors"
            @click="guestLogout"
          >
            <LogOut class="h-3.5 w-3.5" :stroke-width="1.75" />
            Keluar dari sesi ini
          </button>
        </div>

        <!-- Upload bukti pembayaran -->
        <div
          v-if="canUploadGuestProof"
          class="rounded-lg border-2 border-brand-500 bg-brand-50/40 p-6"
        >
          <div class="flex items-start gap-3">
            <Wallet class="h-5 w-5 text-brand-700 flex-none mt-0.5" :stroke-width="1.75" />
            <div class="flex-1">
              <h2 class="font-serif text-lg font-semibold text-ink-950">
                {{ currentStatus === 'ditolak' ? 'Upload ulang bukti transfer' : 'Silakan upload bukti transfer' }}
              </h2>
              <p class="mt-1 text-sm text-ink-700 leading-relaxed">
                Nominal tagihan sudah kami konfirmasi via WhatsApp. Transfer sesuai nominal tersebut, lalu
                upload buktinya di bawah ini.
              </p>

              <!-- Bank info — TIDAK ADA fallback hardcode (lihat usePaymentInfo).
                   Kalau gagal dimuat, panel ini diganti pesan tenang di bawah
                   supaya pembeli tidak salah transfer ke rekening basi. -->
              <div v-if="paymentInfo" class="mt-4 rounded-md border border-hairline bg-canvas p-4 text-sm">
                <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2">Transfer ke</p>
                <div class="space-y-1">
                  <p class="text-ink-900"><strong>{{ paymentInfo.bank_name }}</strong> — {{ paymentInfo.account_name }}</p>
                  <p class="font-mono text-base text-ink-950 font-semibold tracking-wider">{{ paymentInfo.account_number }}</p>
                </div>
                <p v-if="paymentInfo.qris_note" class="mt-2 text-xs text-ink-500">{{ paymentInfo.qris_note }}</p>

                <!-- QRIS hanya tampil kalau admin sudah mengunggahnya ke slot
                     `qris_code`; tidak ada gambar bawaan, jadi tanpa unggahan
                     blok ini tidak dirender sama sekali. -->
                <div v-if="qrisUrl" class="mt-4 border-t border-hairline pt-4">
                  <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2">
                    Atau pindai QRIS
                  </p>
                  <img
                    :src="qrisUrl"
                    alt="Kode QRIS untuk pembayaran"
                    class="w-full max-w-[260px] rounded-md border border-hairline bg-canvas"
                    loading="lazy"
                  >
                  <p v-if="paymentInfo.qris_merchant_name || paymentInfo.qris_nmid" class="mt-2 text-xs text-ink-500">
                    <template v-if="paymentInfo.qris_merchant_name">
                      Nama merchant yang muncul:
                      <span class="font-medium text-ink-700">{{ paymentInfo.qris_merchant_name }}</span>
                    </template>
                    <template v-if="paymentInfo.qris_nmid">
                      <template v-if="paymentInfo.qris_merchant_name">&middot;</template>
                      NMID <span class="font-mono">{{ paymentInfo.qris_nmid }}</span>
                    </template>
                  </p>
                </div>
              </div>

              <!-- Info pembayaran gagal dimuat — sengaja TIDAK menampilkan
                   rekening lama/placeholder apa pun (lihat usePaymentInfo). -->
              <div v-else class="mt-4 flex items-start gap-2 rounded-md border border-hairline bg-canvas-alt/60 p-4 text-sm">
                <AlertTriangle class="h-4 w-4 text-ink-500 flex-none mt-0.5" :stroke-width="1.75" />
                <p class="text-ink-700 leading-relaxed">
                  Informasi rekening tujuan sedang tidak bisa dimuat. Untuk keamanan, kami tidak menampilkan
                  nomor rekening lama. Mohon hubungi kami dulu via WhatsApp untuk memastikan tujuan transfer
                  yang benar sebelum mengirim dana.
                </p>
              </div>

              <!-- Upload form -->
              <form class="mt-4 space-y-3" @submit.prevent="submitGuestProof">
                <div>
                  <label class="text-sm font-medium text-ink-900">Metode bayar</label>
                  <div class="mt-1 inline-flex rounded-md border border-hairline overflow-hidden text-sm">
                    <button
                      type="button"
                      :class="[
                        'px-3 py-1.5 border-r border-hairline transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-inset',
                        guestProofMetode === 'transfer' ? 'bg-ink-950 text-canvas' : 'bg-canvas text-ink-700 hover:bg-canvas-alt',
                      ]"
                      @click="guestProofMetode = 'transfer'"
                    >
                      Transfer bank
                    </button>
                    <button
                      type="button"
                      :class="[
                        'px-3 py-1.5 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-inset',
                        guestProofMetode === 'qris' ? 'bg-ink-950 text-canvas' : 'bg-canvas text-ink-700 hover:bg-canvas-alt',
                      ]"
                      @click="guestProofMetode = 'qris'"
                    >
                      QRIS
                    </button>
                  </div>
                </div>

                <div>
                  <label class="text-sm font-medium text-ink-900">Nominal yang Anda transfer (opsional)</label>
                  <input
                    v-model.number="guestProofAmount"
                    type="number"
                    min="0"
                    step="1000"
                    placeholder="Contoh: 250000"
                    class="mt-1 block w-full max-w-xs rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  >
                  <p class="mt-1 text-xs text-ink-500">Bantu tim verifikasi lebih cepat.</p>
                </div>

                <div>
                  <label class="text-sm font-medium text-ink-900">File bukti <span class="text-brand-500">*</span></label>
                  <input
                    ref="guestProofFileInput"
                    type="file"
                    accept="image/jpeg,image/png,image/webp,application/pdf"
                    required
                    class="mt-1 block w-full text-sm text-ink-700 file:mr-3 file:rounded-md file:border-0 file:bg-ink-950 file:px-3 file:py-1.5 file:text-canvas file:font-medium hover:file:bg-ink-900 file:transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
                    @change="onGuestProofFilePick"
                  >
                  <p class="mt-1 text-xs text-ink-500">Format: JPG / PNG / WebP / PDF. Max 5 MB.</p>
                </div>

                <button
                  type="button"
                  :disabled="guestProofUploading || !guestProofFile"
                  class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                  @click="submitGuestProof"
                >
                  <Loader2 v-if="guestProofUploading" class="h-4 w-4 animate-spin" :stroke-width="1.75" />
                  <Upload v-else class="h-4 w-4" :stroke-width="1.75" />
                  {{ guestProofUploading ? 'Mengupload…' : 'Upload bukti' }}
                </button>
              </form>

              <AlertMessage v-if="guestProofError" variant="error" :message="guestProofError" class="mt-3" />
              <AlertMessage v-if="guestProofSuccess" variant="success" :message="guestProofSuccess" class="mt-3" />
            </div>
          </div>
        </div>

        <!-- Daftar bukti pembayaran -->
        <div v-if="loadingGuestProofs" class="rounded-lg border border-hairline bg-canvas p-6 text-center">
          <Loader2 class="mx-auto h-4 w-4 animate-spin text-ink-500" :stroke-width="1.75" />
        </div>
        <div v-else-if="guestProofs.length" class="rounded-lg border border-hairline bg-canvas p-6">
          <div class="flex items-center gap-2 mb-3">
            <Receipt class="h-4 w-4 text-ink-500" :stroke-width="1.75" />
            <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Bukti pembayaran</p>
          </div>
          <ul class="space-y-3">
            <li
              v-for="p in guestProofs"
              :key="p.id"
              class="rounded-md border border-hairline bg-canvas-alt/40 p-3"
            >
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0 flex-1">
                  <div class="flex items-center gap-2">
                    <FileText class="h-3.5 w-3.5 text-ink-500 flex-none" :stroke-width="1.75" />
                    <p class="text-xs text-ink-900 truncate font-mono">{{ p.file_original_name }}</p>
                  </div>
                  <p class="mt-1 text-[10px] text-ink-500">
                    {{ formatBytes(p.file_size_bytes) }} · {{ p.metode_bayar }} · diupload {{ fmtDateTime(p.uploaded_at) }}
                  </p>
                  <p v-if="p.amount_claimed != null" class="mt-1 text-xs text-ink-700">
                    Nominal diklaim: <span class="font-medium text-ink-900">{{ fmtIDR(p.amount_claimed) }}</span>
                  </p>
                </div>
                <span
                  :class="[
                    'inline-flex flex-none items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset',
                    proofStatusBadgeClass(p.status),
                  ]"
                >
                  <Clock v-if="p.status === 'pending'" class="h-3 w-3" :stroke-width="1.75" />
                  <CheckCircle2 v-else-if="p.status === 'approved'" class="h-3 w-3" :stroke-width="1.75" />
                  <XCircle v-else-if="p.status === 'rejected'" class="h-3 w-3" :stroke-width="1.75" />
                  {{ proofStatusLabel(p.status) }}
                </span>
              </div>

              <!-- Rejected: alasan penolakan + ajakan upload ulang, wajib menonjol -->
              <div
                v-if="p.status === 'rejected'"
                class="mt-3 rounded-md border border-brand-200 bg-brand-50/60 p-3"
              >
                <div class="flex items-start gap-2">
                  <AlertTriangle class="h-4 w-4 text-brand-700 flex-none mt-0.5" :stroke-width="1.75" />
                  <div>
                    <p class="text-xs font-semibold text-brand-700">Alasan ditolak</p>
                    <p class="mt-0.5 text-xs text-ink-700 leading-relaxed">
                      {{ p.reject_reason || 'Bukti transfer tidak dapat diverifikasi. Silakan hubungi kami jika perlu penjelasan lebih lanjut.' }}
                    </p>
                    <p class="mt-1.5 text-[10px] text-ink-500">
                      Silakan upload ulang bukti transfer yang valid menggunakan formulir di atas.
                    </p>
                  </div>
                </div>
              </div>
            </li>
          </ul>
        </div>

        <!-- Upload desain -->
        <div
          v-if="canUploadGuestDesign"
          class="rounded-lg border-2 border-gold-400 bg-gold-50/50 p-6"
        >
          <div class="flex items-start gap-3">
            <Upload class="h-5 w-5 text-gold-700 flex-none mt-0.5" :stroke-width="1.75" />
            <div class="flex-1">
              <h2 class="font-serif text-lg font-semibold text-ink-950">
                {{ guestUploadKind === 'upload' ? 'Upload desain siap cetak' : 'Upload aset desain (logo / foto)' }}
              </h2>
              <p class="mt-1 text-sm text-ink-700 leading-relaxed">
                <template v-if="guestUploadKind === 'upload'">
                  Upload file desain final Anda. Format yang diterima: JPG, PNG, WebP, PDF, CDR, AI — maksimal
                  {{ DESIGN_MAX_UPLOAD_MB }} MB. Tim kami akan memverifikasi file sebelum masuk proses cetak.
                </template>
                <template v-else>
                  Upload logo/foto yang ingin dipakai desainer untuk mengerjakan draft Anda. Boleh upload lebih
                  dari satu file — cukup ulangi proses ini satu per satu. Maksimal {{ DESIGN_MAX_UPLOAD_MB }} MB per file.
                </template>
              </p>

              <form class="mt-4 space-y-3" @submit.prevent="submitGuestDesign">
                <div>
                  <label class="text-sm font-medium text-ink-900">File desain <span class="text-brand-500">*</span></label>
                  <input
                    ref="guestDesignFileInput"
                    type="file"
                    accept=".jpg,.jpeg,.png,.webp,.pdf,.cdr,.ai"
                    required
                    class="mt-1 block w-full rounded-md text-sm text-ink-700 file:mr-3 file:rounded-md file:border-0 file:bg-ink-950 file:px-3 file:py-1.5 file:text-canvas file:font-medium hover:file:bg-ink-900 file:transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
                    @change="onGuestDesignFilePick"
                  >
                  <p class="mt-1 text-xs text-ink-500">Format: JPG / PNG / WebP / PDF / CDR / AI. Max {{ DESIGN_MAX_UPLOAD_MB }} MB.</p>
                </div>

                <div>
                  <label class="text-sm font-medium text-ink-900">Catatan untuk tim (opsional)</label>
                  <textarea
                    v-model="guestDesignNotes"
                    rows="2"
                    maxlength="500"
                    placeholder="Contoh: ini logo terbaru, tolong pakai versi ini."
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  />
                </div>

                <button
                  type="button"
                  :disabled="guestDesignUploading || !guestDesignFile"
                  class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                  @click="submitGuestDesign"
                >
                  <Loader2 v-if="guestDesignUploading" class="h-4 w-4 animate-spin" :stroke-width="1.75" />
                  <Upload v-else class="h-4 w-4" :stroke-width="1.75" />
                  {{ guestDesignUploading ? 'Mengupload…' : 'Upload file' }}
                </button>
              </form>

              <AlertMessage v-if="guestDesignError" variant="error" :message="guestDesignError" class="mt-3" />
              <AlertMessage v-if="guestDesignSuccess" variant="success" :message="guestDesignSuccess" class="mt-3" />
            </div>
          </div>
        </div>
        <div v-else class="rounded-lg border border-hairline bg-canvas p-6">
          <p class="text-sm text-ink-700 leading-relaxed">
            Upload file desain belum tersedia untuk status pesanan saat ini
            (<strong class="text-ink-950">{{ labelOf(currentStatus) }}</strong>). Kartu upload akan muncul
            begitu pesanan siap menerima file desain.
          </p>
        </div>

        <!-- Daftar file yang sudah diupload -->
        <div v-if="loadingGuestDesignFiles" class="rounded-lg border border-hairline bg-canvas p-6 text-center">
          <Loader2 class="mx-auto h-4 w-4 animate-spin text-ink-500" :stroke-width="1.75" />
        </div>
        <div v-else-if="guestUploadedFiles.length" class="rounded-lg border border-hairline bg-canvas p-6">
          <div class="flex items-center gap-2 mb-3">
            <FileText class="h-4 w-4 text-ink-500" :stroke-width="1.75" />
            <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">File desain yang Anda upload</p>
          </div>
          <ul class="space-y-3">
            <li
              v-for="f in guestUploadedFiles"
              :key="f.id"
              class="rounded-md border border-hairline bg-canvas-alt/40 p-3"
            >
              <div class="flex items-start gap-2">
                <FileWarning v-if="f.is_purged" class="h-3.5 w-3.5 text-ink-400 flex-none mt-0.5" :stroke-width="1.75" />
                <FileText v-else class="h-3.5 w-3.5 text-ink-500 flex-none mt-0.5" :stroke-width="1.75" />
                <div class="min-w-0 flex-1">
                  <p class="text-xs text-ink-900 truncate font-mono">{{ f.file_original_name }}</p>
                  <p class="mt-1 text-[10px] text-ink-500">
                    {{ formatBytes(f.file_size_bytes) }} · {{ fmtDateTime(f.uploaded_at) }}
                  </p>
                  <p v-if="f.notes" class="mt-1.5 text-xs text-ink-700 italic border-l-2 border-gold-300 pl-2">
                    "{{ f.notes }}"
                  </p>
                  <p v-if="f.is_purged" class="mt-1.5 text-[10px] text-ink-500">
                    File sudah dihapus dari server sesuai kebijakan retensi 30 hari dan tidak bisa didownload lagi.
                  </p>
                </div>
              </div>
            </li>
          </ul>
        </div>
      </template>

      <!-- Action footer -->
      <div class="flex flex-wrap gap-2">
        <NuxtLink
          to="/lacak"
          class="inline-flex items-center gap-1 rounded-md border border-hairline bg-canvas px-4 py-2 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
        >
          <Search class="h-4 w-4" :stroke-width="1.75" />
          Cek resi lain
        </NuxtLink>
        <NuxtLink
          to="/order"
          class="inline-flex items-center gap-1 rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 transition-colors"
        >
          <ExternalLink class="h-4 w-4" :stroke-width="1.75" />
          Order baru
        </NuxtLink>
      </div>
    </div>
  </main>
</template>
