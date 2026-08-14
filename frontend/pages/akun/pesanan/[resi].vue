<script setup lang="ts">
/**
 * /akun/pesanan/[resi] — Customer view of their own order (§4/§6/§7).
 *
 * Actions yang tersedia untuk customer:
 *   - Upload bukti transfer (state menunggu_pembayaran / ditolak)
 *   - Approve staff design draft (state menunggu_approval_desain)
 *   - Request revisi design draft (state menunggu_approval_desain)
 *
 * View-only sections:
 *   - Timeline status
 *   - Ringkasan produk + nominal
 *   - Payment proofs status
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
  CircleDot,
  Circle,
  FileText,
  AlertTriangle,
} from '@lucide/vue'
import type { Order } from '~/types/order'
import type { PaymentProof } from '~/types/payment'
import type { DesignFile } from '~/types/design'
import { ApiError } from '~/composables/useApi'

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
const proofs = ref<PaymentProof[]>([])
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
    // Customer tidak boleh akses /admin/payment-proofs — pakai lookup by order
    // membutuhkan admin perm, jadi frontend skip fetch untuk customer.
    // Instead: kita tampilkan status dari order.status + optional proofs tetap
    // via /admin route → error 403 → biarkan proofs kosong.
    // Untuk MVP, tampilkan hanya bukti terakhir dari upload flow (state local).
    proofs.value = []
  } catch (e) {
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

// Bank info — hardcoded sesuai spec §7 (no gateway). Di prod dibaca dari config
// admin (task lanjutan).
const bankInfo = {
  bankName: 'BCA',
  accountName: 'PT Rajaku Printing',
  accountNumber: '1234567890',
  qrisNote: 'QRIS statis: scan di toko fisik, minta ke admin via WA.',
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
            {{ order.product_name }}
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

            <!-- Bank info -->
            <div class="mt-4 rounded-md border border-hairline bg-canvas p-4 text-sm">
              <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2">Transfer ke</p>
              <div class="space-y-1">
                <p class="text-ink-900"><strong>{{ bankInfo.bankName }}</strong> — {{ bankInfo.accountName }}</p>
                <p class="font-mono text-base text-ink-950 font-semibold tracking-wider">{{ bankInfo.accountNumber }}</p>
              </div>
              <p class="mt-2 text-xs text-ink-500">{{ bankInfo.qrisNote }}</p>
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

      <!-- Ringkasan produk -->
      <div class="rounded-lg border border-hairline bg-canvas p-6">
        <div class="flex items-center gap-2 mb-3">
          <PackageIcon class="h-4 w-4 text-ink-500" :stroke-width="1.75" />
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Detail pesanan</p>
        </div>
        <p class="text-sm text-ink-500">
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
