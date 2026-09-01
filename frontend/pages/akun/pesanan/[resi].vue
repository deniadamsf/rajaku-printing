<script setup lang="ts">
/**
 * /akun/pesanan/[resi] — Customer view of their own order (§4/§6/§7).
 *
 * Actions yang tersedia untuk customer:
 *   - Upload bukti transfer (state menunggu_pembayaran / ditolak)
 *   - Upload desain siap cetak, design_source 'upload' (state dibayar) — §6
 *   - Upload aset desain (logo/foto) untuk request desain, design_source
 *     'request' (state dibayar / desain_dikerjakan / menunggu_approval_desain) — §6
 *   - Approve staff design draft (state menunggu_approval_desain)
 *   - Request revisi design draft (state menunggu_approval_desain)
 *
 * View-only sections:
 *   - Timeline status
 *   - Ringkasan produk + nominal
 *   - Daftar bukti pembayaran yang sudah diupload beserta status verifikasinya
 *     (pending/approved/rejected) — GET /orders/:resi/payment-proofs
 *   - Daftar file desain yang sudah diupload customer
 *
 * Design: patuh CLAUDE.md §26.
 */
import {
  ChevronLeft,
  Package as PackageIcon,
  Wallet,
  Truck,
  Palette,
  Upload,
  Check,
  X,
  Loader2,
  CheckCircle2,
  FileText,
  FileWarning,
  AlertTriangle,
  Receipt,
  Clock,
  XCircle,
} from '@lucide/vue'
import { orderPrimaryProductLabel, type Order } from '~/types/order'
import type { PaymentProofCustomer } from '~/types/payment'
import type { DesignFile } from '~/types/design'
import { ApiError } from '~/composables/useApi'

// Slot QRIS dikelola admin lewat /admin/site-media. Tidak ada fallback
// statis: kalau belum diunggah, panel QRIS memang tidak ditampilkan.
const qrisUrl = computed(() => useSiteMedia().resolve('qris_code'))

// Rekening & QRIS dari modul `settings` (§7). TIDAK ADA fallback hardcode —
// lihat docblock usePaymentInfo untuk alasan keamanan. Kalau `paymentInfo`
// null, panel "transfer ke" disembunyikan di template.
const { info: paymentInfo } = usePaymentInfo()

definePageMeta({
  middleware: ['customer-only'],
  layout: 'default',
})

const route = useRoute()
const orderApi = useOrder()
const paymentApi = usePayment()
const designApi = useDesign()

const resi = computed(() => String(route.params.resi))

useSeoMeta({
  title: () => `Pesanan ${resi.value} — Rajaku Printing`,
  robots: 'noindex,nofollow',
})

// -------------------- state --------------------
const order = ref<Order | null>(null)
const proofs = ref<PaymentProofCustomer[]>([])
const designFiles = ref<DesignFile[]>([])
const loading = ref(true)
const errorMsg = ref<string | null>(null)
const successMsg = ref<string | null>(null)

function showSuccess(msg: string) {
  successMsg.value = msg
  setTimeout(() => (successMsg.value = null), 3500)
}

function toApiError(e: unknown, fallback: string): string {
  return e instanceof ApiError ? e.message : fallback
}

// -------------------- fetch --------------------
async function loadAll() {
  loading.value = true
  errorMsg.value = null
  try {
    order.value = await orderApi.getByResi(resi.value)
    // Fetch attachments in parallel — non-blocking.
    await Promise.all([loadProofs(), loadDesignFiles()])
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal memuat pesanan')
    order.value = null
  } finally {
    loading.value = false
  }
}

async function loadProofs() {
  if (!order.value) return
  try {
    const res = await paymentApi.listProofsForOrder(resi.value)
    proofs.value = res.items ?? []
  } catch (e) {
    console.error('Gagal load payment proofs', e)
    proofs.value = []
  }
}

async function loadDesignFiles() {
  try {
    const res = await designApi.listByResi(resi.value)
    designFiles.value = res.items ?? []
  } catch (e) {
    console.error('Gagal load design files', e)
    designFiles.value = []
  }
}

onMounted(loadAll)

// -------------------- derived --------------------
const isPickup = computed(() => order.value?.metode_ambil === 'pickup')

const canUploadProof = computed(() => {
  const s = order.value?.status
  return s === 'menunggu_pembayaran' || s === 'ditolak'
})

// Draft staff yang butuh approval customer.
const pendingDraft = computed(() =>
  designFiles.value.find(
    (f) => f.role === 'staff_draft' && (f.approval_status === 'pending' || !f.approval_status),
  ),
)

const canReviewDraft = computed(
  () => order.value?.status === 'menunggu_approval_desain' && !!pendingDraft.value,
)

// -------------------- customer design upload (§6, per-item §32.5) --------------------
// Batas ukuran mengikuti env backend DESIGN_MAX_UPLOAD_MB (default 25MB).
const DESIGN_MAX_UPLOAD_MB = 25
const DESIGN_ALLOWED_EXT = ['jpg', 'jpeg', 'png', 'webp', 'pdf', 'cdr', 'ai']

interface CustomerUploadableItem {
  id: string
  kind: 'upload' | 'request'
  label: string
}

/**
 * Baris item yang boleh diunggahi customer sekarang — aturan gating per item
 * (bukan lagi per order, §32.5) HARUS sama persis dengan guard backend
 * (design_service.go `UploadCustomerFile`), kalau tidak kartu tampil tapi
 * upload ditolak 400:
 *   design_source 'upload'  → hanya status order 'dibayar'
 *   design_source 'request' → 'dibayar' / 'desain_dikerjakan' / 'menunggu_approval_desain'
 * Order campuran ('mixed') otomatis kebagian baris yang memenuhi syarat saja
 * dari masing-masing jenis — tidak perlu dicek terpisah.
 */
const customerUploadableItems = computed<CustomerUploadableItem[]>(() => {
  const o = order.value
  if (!o) return []
  const out: CustomerUploadableItem[] = []
  for (const it of o.items) {
    const label = `${it.product_name} · ${it.material_name} · ${it.width_cm}×${it.height_cm}cm`
    if (it.design_source === 'upload' && o.status === 'dibayar') {
      out.push({ id: it.id, kind: 'upload', label })
    } else if (
      it.design_source === 'request' &&
      ['dibayar', 'desain_dikerjakan', 'menunggu_approval_desain'].includes(o.status)
    ) {
      out.push({ id: it.id, kind: 'request', label })
    }
  }
  return out
})

/**
 * Baris terpilih untuk diupload — auto-pilih satu-satunya baris yang
 * memenuhi syarat supaya order 1-item tetap sederhana (tidak dipaksa
 * memilih dari daftar berisi 1 opsi).
 */
const customerUploadSelectedItemId = ref('')
watch(
  customerUploadableItems,
  (items) => {
    if (items.length === 1) {
      customerUploadSelectedItemId.value = items[0].id
    } else if (!items.some((it) => it.id === customerUploadSelectedItemId.value)) {
      customerUploadSelectedItemId.value = ''
    }
  },
  { immediate: true },
)
const selectedUploadItem = computed(
  () => customerUploadableItems.value.find((it) => it.id === customerUploadSelectedItemId.value) ?? null,
)

const customerUploadedFiles = computed(() =>
  designFiles.value.filter((f) => f.role === 'customer_upload' || f.role === 'customer_asset'),
)

/** Label banner pemilik sebuah file — cuma ditampilkan untuk order >1 item. */
function customerFileItemLabel(f: DesignFile): string | null {
  if (!order.value || order.value.items.length <= 1) return null
  const it = order.value.items.find((x) => x.id === f.order_item_id)
  return it ? it.product_name : null
}

// -------------------- upload proof form --------------------
const proofFile = ref<File | null>(null)
const proofMetode = ref<'transfer' | 'qris'>('transfer')
const proofAmount = ref<number | null>(null)
const proofUploading = ref(false)
const proofFileInput = ref<HTMLInputElement | null>(null)

function onProofFilePick(ev: Event) {
  const input = ev.target as HTMLInputElement
  proofFile.value = input.files?.[0] ?? null
}

async function submitProof() {
  if (!proofFile.value) {
    errorMsg.value = 'Pilih file bukti transfer dulu.'
    return
  }
  proofUploading.value = true
  errorMsg.value = null
  try {
    await paymentApi.uploadProof(resi.value, {
      file: proofFile.value,
      metodeBayar: proofMetode.value,
      amountClaimed: proofAmount.value ?? undefined,
    })
    showSuccess('Bukti transfer diupload. Tim kami akan verifikasi segera.')
    proofFile.value = null
    if (proofFileInput.value) proofFileInput.value.value = ''
    proofAmount.value = null
    await loadAll()
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal upload bukti transfer')
  } finally {
    proofUploading.value = false
  }
}

// -------------------- customer design upload form --------------------
const customerDesignFile = ref<File | null>(null)
const customerDesignNotes = ref('')
const customerDesignUploading = ref(false)
const customerDesignFileInput = ref<HTMLInputElement | null>(null)

function onCustomerDesignFilePick(ev: Event) {
  const input = ev.target as HTMLInputElement
  customerDesignFile.value = input.files?.[0] ?? null
}

function validateCustomerDesignFile(file: File): string | null {
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

async function submitCustomerDesign() {
  if (!customerUploadSelectedItemId.value) {
    errorMsg.value = 'Pilih banner yang mau diupload filenya dulu.'
    return
  }
  if (!customerDesignFile.value) {
    errorMsg.value = 'Pilih file desain dulu.'
    return
  }
  const validationError = validateCustomerDesignFile(customerDesignFile.value)
  if (validationError) {
    errorMsg.value = validationError
    return
  }
  customerDesignUploading.value = true
  errorMsg.value = null
  try {
    await designApi.uploadCustomerFile(
      resi.value,
      customerUploadSelectedItemId.value,
      customerDesignFile.value,
      customerDesignNotes.value.trim() || undefined,
    )
    showSuccess('File desain terupload. Tim kami akan memeriksanya.')
    customerDesignFile.value = null
    customerDesignNotes.value = ''
    if (customerDesignFileInput.value) customerDesignFileInput.value.value = ''
    // Upload aset pertama (design_source 'request') bisa meng-advance status
    // order jadi 'desain_dikerjakan' di backend — reload supaya UI sinkron.
    await loadAll()
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal upload file desain')
  } finally {
    customerDesignUploading.value = false
  }
}

// -------------------- design approve / revision --------------------
const revisionOpen = ref(false)
const revisionNotes = ref('')
const designBusy = ref(false)

async function approveDesign() {
  if (!pendingDraft.value) return
  designBusy.value = true
  errorMsg.value = null
  try {
    await designApi.approveDraft(pendingDraft.value.id)
    showSuccess('Desain disetujui. Order akan lanjut ke proses cetak.')
    await loadAll()
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal approve desain')
  } finally {
    designBusy.value = false
  }
}

async function submitRevision() {
  if (!pendingDraft.value) return
  if (revisionNotes.value.trim().length < 3) {
    errorMsg.value = 'Catatan revisi minimal 3 karakter.'
    return
  }
  designBusy.value = true
  errorMsg.value = null
  try {
    await designApi.requestRevision(pendingDraft.value.id, revisionNotes.value.trim())
    showSuccess('Permintaan revisi terkirim ke tim desainer.')
    revisionOpen.value = false
    revisionNotes.value = ''
    await loadAll()
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal request revisi')
  } finally {
    designBusy.value = false
  }
}

// -------------------- helpers --------------------
function fmtIDR(n?: number | null): string {
  if (n == null) return '—'
  return new Intl.NumberFormat('id-ID', {
    style: 'currency',
    currency: 'IDR',
    maximumFractionDigits: 0,
  }).format(n)
}

function fmtDate(s?: string): string {
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

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

const statusLabelMap: Record<string, string> = {
  order_masuk: 'Order masuk',
  menunggu_ongkir: 'Menunggu ongkir',
  menunggu_pembayaran: 'Menunggu pembayaran',
  menunggu_verifikasi: 'Menunggu verifikasi bukti',
  dibayar: 'Pembayaran diverifikasi',
  ditolak: 'Bukti transfer ditolak',
  desain_dikerjakan: 'Desain dikerjakan',
  menunggu_approval_desain: 'Menunggu approval desain Anda',
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
  return statusLabelMap[s] ?? s
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

</script>

<template>
  <main class="mx-auto max-w-4xl px-4 py-10 md:py-16">
    <!-- Back -->
    <NuxtLink
      to="/akun"
      class="inline-flex items-center gap-1 text-sm text-ink-500 hover:text-ink-900 transition-colors"
    >
      <ChevronLeft class="h-4 w-4" :stroke-width="1.75" />
      Kembali ke daftar pesanan
    </NuxtLink>

    <!-- Loading -->
    <div v-if="loading" class="mt-6 rounded-lg border border-hairline bg-canvas p-12 text-center">
      <Loader2 class="mx-auto h-5 w-5 animate-spin text-ink-500" :stroke-width="1.75" />
      <p class="mt-3 text-sm text-ink-500">Memuat pesanan…</p>
    </div>

    <!-- Not found -->
    <div v-else-if="!order" class="mt-6 rounded-lg border border-brand-200 bg-brand-50/50 p-8">
      <div class="flex items-start gap-3">
        <AlertTriangle class="h-5 w-5 text-brand-700 flex-none mt-0.5" :stroke-width="1.75" />
        <div>
          <h1 class="font-serif text-xl font-semibold text-ink-950">Pesanan tidak ditemukan</h1>
          <p class="mt-1 text-sm text-ink-700 leading-relaxed">{{ errorMsg || 'Order tidak ada atau bukan milik Anda.' }}</p>
        </div>
      </div>
    </div>

    <!-- Detail -->
    <div v-else class="mt-6 space-y-6">
      <!-- Header -->
      <div>
        <p class="font-mono text-xs text-ink-500">{{ order.resi }}</p>
        <div class="mt-1 flex flex-wrap items-start justify-between gap-3">
          <h1 class="font-serif text-2xl md:text-3xl font-semibold tracking-tight text-ink-950">
            {{ orderPrimaryProductLabel(order) }}
          </h1>
          <span
            :class="[
              'inline-flex items-center rounded-full px-3 py-1 text-xs font-medium ring-1 ring-inset',
              canUploadProof
                ? 'bg-brand-50 text-brand-700 ring-brand-200'
                : order.status === 'selesai'
                  ? 'bg-emerald-50 text-emerald-800 ring-emerald-200'
                  : 'bg-canvas-alt text-ink-700 ring-hairline',
            ]"
          >
            {{ statusLabel(order.status) }}
          </span>
        </div>
      </div>

      <AlertMessage v-if="errorMsg" variant="error" :message="errorMsg" />
      <AlertMessage v-if="successMsg" variant="success" :message="successMsg" />

      <!-- ============ ACTION AREA: Upload proof ============ -->
      <div
        v-if="canUploadProof"
        class="rounded-lg border-2 border-brand-500 bg-brand-50/40 p-6"
      >
        <div class="flex items-start gap-3">
          <Upload class="h-5 w-5 text-brand-700 flex-none mt-0.5" :stroke-width="1.75" />
          <div class="flex-1">
            <h2 class="font-serif text-lg font-semibold text-ink-950">
              {{ order.status === 'ditolak' ? 'Upload ulang bukti transfer' : 'Silakan upload bukti transfer' }}
            </h2>
            <p class="mt-1 text-sm text-ink-700 leading-relaxed">
              Total yang harus dibayar: <strong class="font-serif text-lg text-ink-950">{{ fmtIDR(order.total) }}</strong>
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
            <form class="mt-4 space-y-3" @submit.prevent="submitProof">
              <div>
                <label class="text-sm font-medium text-ink-900">Metode bayar</label>
                <div class="mt-1 inline-flex rounded-md border border-hairline overflow-hidden text-sm">
                  <button
                    type="button"
                    :class="[
                      'px-3 py-1.5 border-r border-hairline transition-colors',
                      proofMetode === 'transfer' ? 'bg-ink-950 text-canvas' : 'bg-canvas text-ink-700 hover:bg-canvas-alt',
                    ]"
                    @click="proofMetode = 'transfer'"
                  >
                    Transfer bank
                  </button>
                  <button
                    type="button"
                    :class="[
                      'px-3 py-1.5 transition-colors',
                      proofMetode === 'qris' ? 'bg-ink-950 text-canvas' : 'bg-canvas text-ink-700 hover:bg-canvas-alt',
                    ]"
                    @click="proofMetode = 'qris'"
                  >
                    QRIS
                  </button>
                </div>
              </div>

              <div>
                <label class="text-sm font-medium text-ink-900">Nominal yang Anda transfer (opsional)</label>
                <input
                  v-model.number="proofAmount"
                  type="number"
                  min="0"
                  step="1000"
                  :placeholder="String(order.total)"
                  class="mt-1 block w-full max-w-xs rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                >
                <p class="mt-1 text-xs text-ink-500">Bantu tim verifikasi lebih cepat.</p>
              </div>

              <div>
                <label class="text-sm font-medium text-ink-900">File bukti <span class="text-brand-500">*</span></label>
                <input
                  ref="proofFileInput"
                  type="file"
                  accept="image/jpeg,image/png,image/webp,application/pdf"
                  required
                  class="mt-1 block w-full text-sm text-ink-700 file:mr-3 file:rounded-md file:border-0 file:bg-ink-950 file:px-3 file:py-1.5 file:text-canvas file:font-medium hover:file:bg-ink-900 file:transition-colors"
                  @change="onProofFilePick"
                >
                <p class="mt-1 text-xs text-ink-500">Format: JPG / PNG / WebP / PDF. Max 5 MB.</p>
              </div>

              <!--
                Pakai type="button" + @click (bukan type="submit") untuk hindari
                edge case di sinkronisasi hydration Vue dimana form @submit.prevent
                kadang tidak terpicu via .click() sintetis / test tooling.
                Form tetap ada + @submit.prevent supaya Enter key di input tetap
                jalan (submitProof dipanggil satu kali via keyboard maupun click).
              -->
              <button
                type="button"
                :disabled="proofUploading || !proofFile"
                class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                @click="submitProof"
              >
                <Loader2 v-if="proofUploading" class="h-4 w-4 animate-spin" :stroke-width="1.75" />
                <Upload v-else class="h-4 w-4" :stroke-width="1.75" />
                {{ proofUploading ? 'Mengupload…' : 'Upload bukti' }}
              </button>
            </form>
          </div>
        </div>
      </div>

      <!-- ============ VIEW: Payment proofs status ============ -->
      <div v-if="proofs.length" class="rounded-lg border border-hairline bg-canvas p-6">
        <div class="flex items-center gap-2 mb-3">
          <Receipt class="h-4 w-4 text-ink-500" :stroke-width="1.75" />
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Bukti pembayaran</p>
        </div>
        <ul class="space-y-3">
          <li
            v-for="p in proofs"
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
                  {{ formatBytes(p.file_size_bytes) }} · {{ p.metode_bayar }} · diupload {{ fmtDate(p.uploaded_at) }}
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

      <!-- ============ ACTION AREA: Approve design ============ -->
      <div
        v-if="canReviewDraft && pendingDraft"
        class="rounded-lg border-2 border-gold-400 bg-gold-50/50 p-6"
      >
        <div class="flex items-start gap-3">
          <Palette class="h-5 w-5 text-gold-700 flex-none mt-0.5" :stroke-width="1.75" />
          <div class="flex-1">
            <h2 class="font-serif text-lg font-semibold text-ink-950">Draft desain siap review</h2>
            <p class="mt-1 text-sm text-ink-700 leading-relaxed">
              Tim desainer telah mengupload draft untuk Anda. Silakan review lalu setujui,
              atau minta revisi dengan catatan.
            </p>
            <div class="mt-4 rounded-md border border-hairline bg-canvas p-3">
              <div class="flex items-center gap-2">
                <FileText class="h-3.5 w-3.5 text-ink-500" :stroke-width="1.75" />
                <p class="text-xs text-ink-900 truncate">{{ pendingDraft.file_original_name }}</p>
              </div>
              <p class="mt-1 text-[10px] text-ink-500">
                {{ formatBytes(pendingDraft.file_size_bytes) }} · {{ pendingDraft.file_mime_type }} · {{ fmtDate(pendingDraft.uploaded_at) }}
              </p>
              <p v-if="pendingDraft.notes" class="mt-2 text-xs text-ink-700 italic border-l-2 border-gold-300 pl-2">
                "{{ pendingDraft.notes }}"
              </p>
            </div>
            <div class="mt-4 flex flex-wrap gap-2">
              <button
                type="button"
                :disabled="designBusy"
                class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 transition-colors disabled:opacity-60"
                @click="approveDesign"
              >
                <Loader2 v-if="designBusy" class="h-4 w-4 animate-spin" :stroke-width="1.75" />
                <Check v-else class="h-4 w-4" :stroke-width="1.75" />
                Setujui desain
              </button>
              <button
                type="button"
                :disabled="designBusy"
                class="inline-flex items-center gap-2 rounded-md border border-hairline bg-canvas px-4 py-2 text-sm font-semibold text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
                @click="revisionOpen = true"
              >
                <X class="h-4 w-4" :stroke-width="1.75" />
                Minta revisi
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- ============ ACTION AREA: Upload customer design (per item, §32.5) ============ -->
      <div
        v-if="customerUploadableItems.length"
        class="rounded-lg border-2 border-gold-400 bg-gold-50/50 p-6"
      >
        <div class="flex items-start gap-3">
          <Upload class="h-5 w-5 text-gold-700 flex-none mt-0.5" :stroke-width="1.75" />
          <div class="flex-1">
            <h2 class="font-serif text-lg font-semibold text-ink-950">
              {{ selectedUploadItem?.kind === 'request' ? 'Upload aset desain (logo / foto)' : 'Upload desain siap cetak' }}
            </h2>
            <p class="mt-1 text-sm text-ink-700 leading-relaxed">
              <template v-if="selectedUploadItem?.kind === 'request'">
                Upload logo/foto yang ingin dipakai desainer untuk mengerjakan draft Anda. Boleh upload lebih
                dari satu file — cukup ulangi proses ini satu per satu. Maksimal {{ DESIGN_MAX_UPLOAD_MB }} MB per file.
              </template>
              <template v-else>
                Upload file desain final Anda. Format yang diterima: JPG, PNG, WebP, PDF, CDR, AI — maksimal
                {{ DESIGN_MAX_UPLOAD_MB }} MB. Tim kami akan memverifikasi file sebelum masuk proses cetak.
              </template>
            </p>

            <!-- Pemilih baris — HANYA muncul kalau ada >1 item yang boleh
                 diupload. Order 1 item tetap sederhana: langsung ke form. -->
            <div v-if="customerUploadableItems.length > 1" class="mt-4">
              <p class="text-sm font-medium text-ink-900">Pilih banner yang mau diupload filenya</p>
              <div class="mt-1.5 space-y-1.5">
                <label
                  v-for="it in customerUploadableItems"
                  :key="it.id"
                  :class="[
                    'flex cursor-pointer items-center gap-2 rounded-md border p-2.5 text-sm transition-colors',
                    customerUploadSelectedItemId === it.id
                      ? 'border-brand-500 bg-brand-50/50 text-ink-950'
                      : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300',
                  ]"
                >
                  <input v-model="customerUploadSelectedItemId" type="radio" :value="it.id" class="accent-brand-500">
                  <span>{{ it.label }} <span class="text-xs uppercase text-ink-500">· {{ it.kind }}</span></span>
                </label>
              </div>
            </div>
            <p v-else-if="customerUploadableItems.length === 1" class="mt-2 text-xs text-ink-500">
              Untuk: <strong class="text-ink-900">{{ customerUploadableItems[0].label }}</strong>
            </p>

            <form class="mt-4 space-y-3" @submit.prevent="submitCustomerDesign">
              <div>
                <label class="text-sm font-medium text-ink-900">File desain <span class="text-brand-500">*</span></label>
                <input
                  ref="customerDesignFileInput"
                  type="file"
                  accept=".jpg,.jpeg,.png,.webp,.pdf,.cdr,.ai"
                  required
                  class="mt-1 block w-full rounded-md text-sm text-ink-700 file:mr-3 file:rounded-md file:border-0 file:bg-ink-950 file:px-3 file:py-1.5 file:text-canvas file:font-medium hover:file:bg-ink-900 file:transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
                  @change="onCustomerDesignFilePick"
                >
                <p class="mt-1 text-xs text-ink-500">Format: JPG / PNG / WebP / PDF / CDR / AI. Max {{ DESIGN_MAX_UPLOAD_MB }} MB.</p>
              </div>

              <div>
                <label class="text-sm font-medium text-ink-900">Catatan untuk tim (opsional)</label>
                <textarea
                  v-model="customerDesignNotes"
                  rows="2"
                  maxlength="500"
                  placeholder="Contoh: ini logo terbaru, tolong pakai versi ini."
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                />
              </div>

              <button
                type="button"
                :disabled="customerDesignUploading || !customerDesignFile || !customerUploadSelectedItemId"
                class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                @click="submitCustomerDesign"
              >
                <Loader2 v-if="customerDesignUploading" class="h-4 w-4 animate-spin" :stroke-width="1.75" />
                <Upload v-else class="h-4 w-4" :stroke-width="1.75" />
                {{ customerDesignUploading ? 'Mengupload…' : 'Upload file' }}
              </button>
            </form>
          </div>
        </div>
      </div>

      <!-- ============ VIEW: Customer's uploaded design files ============ -->
      <div v-if="customerUploadedFiles.length" class="rounded-lg border border-hairline bg-canvas p-6">
        <div class="flex items-center gap-2 mb-3">
          <FileText class="h-4 w-4 text-ink-500" :stroke-width="1.75" />
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">File desain yang Anda upload</p>
        </div>
        <ul class="space-y-3">
          <li
            v-for="f in customerUploadedFiles"
            :key="f.id"
            class="rounded-md border border-hairline bg-canvas-alt/40 p-3"
          >
            <div class="flex items-start gap-2">
              <FileWarning v-if="f.is_purged" class="h-3.5 w-3.5 text-ink-400 flex-none mt-0.5" :stroke-width="1.75" />
              <FileText v-else class="h-3.5 w-3.5 text-ink-500 flex-none mt-0.5" :stroke-width="1.75" />
              <div class="min-w-0 flex-1">
                <p v-if="customerFileItemLabel(f)" class="text-[10px] font-medium uppercase tracking-[0.1em] text-ink-500">
                  {{ customerFileItemLabel(f) }}
                </p>
                <p class="text-xs text-ink-900 truncate font-mono">{{ f.file_original_name }}</p>
                <p class="mt-1 text-[10px] text-ink-500">
                  {{ formatBytes(f.file_size_bytes) }} · {{ fmtDate(f.uploaded_at) }}
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

      <!-- Ringkasan produk (§32 — tabel multi-item) -->
      <div class="rounded-lg border border-hairline bg-canvas p-6">
        <div class="flex items-center justify-between gap-2 mb-3">
          <div class="flex items-center gap-2">
            <PackageIcon class="h-4 w-4 text-ink-500" :stroke-width="1.75" />
            <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Detail pesanan</p>
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
          <div class="flex justify-between">
            <dt class="text-ink-500">Ongkir</dt>
            <dd class="text-ink-900">
              <template v-if="isPickup"><span class="text-ink-500">(pickup, gratis)</span></template>
              <template v-else-if="order.shipping_cost == null"><span class="text-ink-500">menunggu admin</span></template>
              <template v-else>{{ fmtIDR(order.shipping_cost) }}</template>
            </dd>
          </div>
          <div class="flex justify-between border-t border-hairline pt-3 mt-1">
            <dt class="font-semibold text-ink-950">Total</dt>
            <dd class="font-serif text-lg font-semibold text-ink-950">{{ fmtIDR(order.total) }}</dd>
          </div>
        </dl>
      </div>

      <!-- Pengambilan -->
      <div class="rounded-lg border border-hairline bg-canvas p-6">
        <div class="flex items-center gap-2 mb-3">
          <Truck class="h-4 w-4 text-ink-500" :stroke-width="1.75" />
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
            {{ isPickup ? 'Pickup di toko' : 'Alamat pengiriman' }}
          </p>
        </div>
        <div v-if="isPickup" class="text-sm text-ink-700">
          Anda akan mengambil langsung di toko. Tidak ada ongkir.
        </div>
        <div v-else class="space-y-1 text-sm">
          <p class="text-ink-900 font-medium">{{ order.shipping_recipient_name || '—' }}</p>
          <p class="font-mono text-xs text-ink-500">{{ order.shipping_recipient_phone || '—' }}</p>
          <p class="text-ink-700 leading-relaxed mt-2">{{ order.shipping_address || 'Alamat belum diinput.' }}</p>
        </div>
      </div>

      <!-- Lacak resi CTA -->
      <div class="rounded-lg border border-hairline bg-canvas p-6 flex flex-wrap items-center justify-between gap-3">
        <div>
          <p class="text-sm font-medium text-ink-900">Lacak progres pesanan</p>
          <p class="mt-0.5 text-xs text-ink-500">Timeline status realtime di halaman publik.</p>
        </div>
        <NuxtLink
          :to="`/lacak/${order.resi}`"
          class="inline-flex items-center gap-1 rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
        >
          Lacak resi →
        </NuxtLink>
      </div>
    </div>

    <!-- ============ Revision modal ============ -->
    <Teleport to="body">
      <Transition name="fade">
        <div v-if="revisionOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4">
          <div class="absolute inset-0 bg-ink-950/60" aria-hidden="true" @click="!designBusy && (revisionOpen = false)" />
          <div role="dialog" aria-modal="true" class="relative w-full max-w-md rounded-lg bg-canvas shadow-xl ring-1 ring-black/5 p-6">
            <h3 class="font-serif text-lg font-semibold text-ink-950">Minta revisi desain</h3>
            <p class="mt-1 text-xs text-ink-500 leading-relaxed">
              Tulis catatan spesifik supaya desainer tahu apa yang perlu diubah — contoh warna, teks, layout, dll.
            </p>
            <textarea
              v-model="revisionNotes"
              rows="4"
              maxlength="500"
              placeholder="Contoh: Warna latar terlalu terang, tolong ganti biru dongker. Font judul lebih tebal."
              class="mt-3 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
            />
            <p class="mt-1 text-xs text-ink-400 text-right">{{ revisionNotes.length }} / 500</p>
            <div class="mt-4 flex justify-end gap-2">
              <button
                type="button"
                class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-medium text-ink-700 hover:bg-canvas-alt transition-colors disabled:opacity-50"
                :disabled="designBusy"
                @click="revisionOpen = false"
              >
                Batal
              </button>
              <button
                type="button"
                class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-1.5 text-sm font-semibold text-canvas hover:bg-brand-600 transition-colors disabled:opacity-60"
                :disabled="designBusy || revisionNotes.trim().length < 3"
                @click="submitRevision"
              >
                <Loader2 v-if="designBusy" class="h-3.5 w-3.5 animate-spin" :stroke-width="1.75" />
                Kirim revisi
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </main>
</template>

<style scoped>
.fade-enter-active, .fade-leave-active {
  transition: opacity 0.15s ease;
}
.fade-enter-from, .fade-leave-to {
  opacity: 0;
}
</style>
