<script setup lang="ts">
/**
 * /admin/desain/[resi] — full design workflow untuk 1 order.
 *
 * Aksi state-aware:
 *   - Verifikasi Upload: kalau design_source=upload, status=dibayar, ada
 *     file customer_upload → tombol "Verifikasi & Lanjut Cetak" (perm design.approve)
 *   - Upload Draft: kalau design_source=request, status ∈ [dibayar,
 *     desain_dikerjakan, menunggu_approval_desain] → form upload draft
 *     (perm design.work). Auto-advance backend ke desain_dikerjakan /
 *     menunggu_approval_desain.
 *   - Walk-in Approve: kalau channel=pos, status=desain_dikerjakan → tombol
 *     shortcut (perm design.approve). Backend enforce channel+mode di service.
 *
 * File preview: image/pdf/webp langsung inline lewat blob URL (Bearer token
 * dikirim via fetch, bukan lewat <img src=…>). CDR/AI tidak previewable —
 * cukup download link.
 */
import type { Order } from '~/types/order'
import type { DesignFile } from '~/types/design'
import { ApiError } from '~/composables/useApi'
import { FileCheck2, Upload, Sparkles, Download, XCircle } from '@lucide/vue'

definePageMeta({
  middleware: ['staff-only'],
  layout: 'admin',
})

const route = useRoute()
const auth = useAuthStore()
const orderSvc = useOrder()
const designSvc = useDesign()

const resi = computed(() => String(route.params.resi))

const order = ref<Order | null>(null)
const files = ref<DesignFile[]>([])
const loading = ref(true)
const errorMsg = ref<string | null>(null)
const successMsg = ref<string | null>(null)

useSeoMeta({ title: () => `Desain ${resi.value} — Admin` })

// Permissions
const canApprove = computed(() => auth.hasPermission('design.approve'))
const canWork = computed(() => auth.hasPermission('design.work'))

// --- Derived state ---
const customerUpload = computed(() =>
  files.value.find((f) => f.role === 'customer_upload' && !f.is_purged),
)
const customerAssets = computed(() =>
  files.value.filter((f) => f.role === 'customer_asset' && !f.is_purged),
)
const staffDrafts = computed(() =>
  files.value.filter((f) => f.role === 'staff_draft').sort((a, b) => (a.uploaded_at < b.uploaded_at ? 1 : -1)),
)
const latestDraft = computed<DesignFile | null>(() => staffDrafts.value[0] ?? null)

const canVerifyUpload = computed(
  () =>
    canApprove.value &&
    order.value?.design_source === 'upload' &&
    order.value?.status === 'dibayar' &&
    !!customerUpload.value,
)
const canUploadDraft = computed(
  () =>
    canWork.value &&
    order.value?.design_source === 'request' &&
    order.value !== null &&
    ['dibayar', 'desain_dikerjakan', 'menunggu_approval_desain'].includes(order.value.status),
)
const canWalkinApprove = computed(
  () =>
    canApprove.value &&
    order.value?.channel === 'pos' &&
    order.value?.status === 'desain_dikerjakan',
)

async function load() {
  loading.value = true
  errorMsg.value = null
  try {
    const [o, list] = await Promise.all([
      orderSvc.getByResi(resi.value),
      designSvc.listByResi(resi.value),
    ])
    order.value = o
    files.value = list.items ?? []
  } catch (e: unknown) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal memuat data'
  } finally {
    loading.value = false
  }
}

onMounted(load)

// --- Actions ---
const verifyBusy = ref(false)
const verifyNote = ref('')
async function doVerify() {
  verifyBusy.value = true
  errorMsg.value = null
  try {
    await designSvc.verifyUpload(resi.value, verifyNote.value || undefined)
    successMsg.value = 'Upload diverifikasi. Order lanjut ke desain_diverifikasi.'
    setTimeout(() => (successMsg.value = null), 3000)
    verifyNote.value = ''
    await load()
  } catch (e: unknown) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal verify'
  } finally {
    verifyBusy.value = false
  }
}

const walkinBusy = ref(false)
async function doWalkin() {
  walkinBusy.value = true
  errorMsg.value = null
  try {
    await designSvc.walkinApprove(resi.value)
    successMsg.value = 'Walk-in approved. Order lanjut ke desain_diverifikasi.'
    setTimeout(() => (successMsg.value = null), 3000)
    await load()
  } catch (e: unknown) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal walk-in approve'
  } finally {
    walkinBusy.value = false
  }
}

// Draft upload form
const draftFileInput = ref<HTMLInputElement | null>(null)
const draftFile = ref<File | null>(null)
const draftNotes = ref('')
const draftBusy = ref(false)
function onDraftFileChange(ev: Event) {
  const input = ev.target as HTMLInputElement
  draftFile.value = input.files?.[0] ?? null
}
async function submitDraft() {
  if (!draftFile.value) {
    errorMsg.value = 'Pilih file draft dulu.'
    return
  }
  draftBusy.value = true
  errorMsg.value = null
  try {
    await designSvc.uploadDraft(resi.value, draftFile.value, draftNotes.value || undefined)
    successMsg.value = 'Draft terupload. Customer bisa review.'
    setTimeout(() => (successMsg.value = null), 3000)
    draftFile.value = null
    draftNotes.value = ''
    if (draftFileInput.value) draftFileInput.value.value = ''
    await load()
  } catch (e: unknown) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal upload draft'
  } finally {
    draftBusy.value = false
  }
}

// --- Preview ---
interface PreviewState {
  file: DesignFile
  url: string
  mime: string
  name: string
}
const preview = ref<PreviewState | null>(null)
const previewLoading = ref(false)

async function openPreview(f: DesignFile) {
  if (f.is_purged) {
    errorMsg.value = 'File sudah dipurge dari disk (retensi 30 hari §19).'
    return
  }
  closePreview()
  previewLoading.value = true
  try {
    const { url, mime, name } = await designSvc.fetchFileBlob(f.id)
    preview.value = { file: f, url, mime, name }
  } catch (e: unknown) {
    errorMsg.value = e instanceof Error ? e.message : 'Gagal preview file'
  } finally {
    previewLoading.value = false
  }
}
function closePreview() {
  if (preview.value?.url) URL.revokeObjectURL(preview.value.url)
  preview.value = null
}
onBeforeUnmount(() => closePreview())

const previewIsImage = computed(() => preview.value?.mime.startsWith('image/'))
const previewIsPDF = computed(() => preview.value?.mime === 'application/pdf')

async function downloadFile(f: DesignFile) {
  if (f.is_purged) {
    errorMsg.value = 'File sudah dipurge.'
    return
  }
  try {
    const { url, name } = await designSvc.fetchFileBlob(f.id)
    // Trigger browser download by creating anchor.
    const a = document.createElement('a')
    a.href = url
    a.download = f.file_original_name || name
    document.body.appendChild(a)
    a.click()
    a.remove()
    // Delay revoke supaya browser sempat baca URL.
    setTimeout(() => URL.revokeObjectURL(url), 5000)
  } catch (e: unknown) {
    errorMsg.value = e instanceof Error ? e.message : 'Gagal download'
  }
}

// --- Helpers ---
function fmtDate(s?: string | null): string {
  if (!s) return '—'
  try {
    return new Date(s).toLocaleString('id-ID', {
      day: '2-digit', month: 'short', year: 'numeric',
      hour: '2-digit', minute: '2-digit',
    })
  } catch {
    return s
  }
}
function fmtBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(2)} MB`
}
function statusLabel(s: string): string {
  return s.replace(/_/g, ' ')
}
</script>

<template>
  <section>
    <AdminPageHeader
      :title="loading ? 'Memuat…' : order ? `Desain ${order.resi}` : 'Order tidak ditemukan'"
      :breadcrumb="[{ label: 'Desain', to: '/admin/desain' }, { label: resi }]"
    >
      <template #actions>
        <AdminStatusBadge v-if="order" :status="statusLabel(order.status)" />
        <NuxtLink
          v-if="order"
          :to="`/admin/order/${order.resi}`"
          class="inline-flex items-center gap-2 rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-semibold text-ink-700 hover:bg-canvas-alt transition-colors"
        >
          Lihat order lengkap
        </NuxtLink>
      </template>
    </AdminPageHeader>

    <AlertMessage v-if="errorMsg" variant="error" :message="errorMsg" class="mb-4" />
    <AlertMessage v-if="successMsg" variant="success" :message="successMsg" class="mb-4" />

    <div v-if="loading" class="rounded-lg border border-hairline bg-canvas p-6 text-sm text-ink-500">
      Memuat…
    </div>
    <div v-else-if="!order" class="rounded-lg border border-hairline bg-canvas p-6 text-sm text-ink-500">
      Order tidak ditemukan.
    </div>

    <div v-else class="grid gap-6 lg:grid-cols-3">
      <!-- Left column: files & brief -->
      <div class="lg:col-span-2 space-y-4">
        <!-- Order summary card -->
        <div class="rounded-lg border border-hairline bg-canvas p-5">
          <h2 class="text-xs font-medium uppercase tracking-[0.14em] text-ink-500">Order</h2>
          <p class="mt-2 font-serif text-lg font-semibold tracking-tight text-ink-950">{{ order.product_name }}</p>
          <p class="mt-0.5 text-sm text-ink-600">
            {{ order.material_name }} · {{ order.width_cm }}×{{ order.height_cm }}cm · {{ order.quantity }} pcs
          </p>
          <div class="mt-3 flex flex-wrap gap-3 text-xs text-ink-500">
            <span><strong class="text-ink-900 uppercase">{{ order.design_source }}</strong> path</span>
            <span>·</span>
            <span class="uppercase">Channel {{ order.channel }}</span>
            <span>·</span>
            <span class="uppercase">{{ order.metode_ambil }}</span>
          </div>
        </div>

        <!-- Brief (request path) -->
        <div v-if="order.design_source === 'request' && (order.design_brief || customerAssets.length)"
             class="rounded-lg border border-hairline bg-canvas p-5">
          <h2 class="text-xs font-medium uppercase tracking-[0.14em] text-ink-500">Brief</h2>
          <p v-if="order.design_brief" class="mt-3 text-sm text-ink-800 leading-relaxed border-l-2 border-gold-400 pl-3 whitespace-pre-line">
            {{ order.design_brief }}
          </p>
          <p v-else class="mt-3 text-xs text-ink-500 italic">Tidak ada brief tekstual.</p>

          <div v-if="customerAssets.length" class="mt-4">
            <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2">
              Aset dari customer ({{ customerAssets.length }})
            </p>
            <ul class="space-y-2">
              <li v-for="f in customerAssets" :key="f.id" class="flex items-center gap-3 text-sm">
                <button
                  type="button"
                  class="text-ink-900 hover:text-brand-500 underline decoration-hairline underline-offset-2 truncate flex-1 text-left transition-colors"
                  @click="openPreview(f)"
                >
                  {{ f.file_original_name }}
                </button>
                <span class="text-xs text-ink-500 shrink-0">{{ fmtBytes(f.file_size_bytes) }}</span>
                <button
                  type="button"
                  class="rounded p-1 text-ink-500 hover:text-ink-950 transition-colors"
                  aria-label="Download"
                  @click="downloadFile(f)"
                >
                  <Download class="h-4 w-4" :stroke-width="1.75" />
                </button>
              </li>
            </ul>
          </div>
        </div>

        <!-- Customer upload file (upload path) -->
        <div v-if="customerUpload" class="rounded-lg border border-hairline bg-canvas p-5">
          <h2 class="text-xs font-medium uppercase tracking-[0.14em] text-ink-500">File siap cetak (customer upload)</h2>
          <div class="mt-3 flex items-center gap-3 text-sm">
            <button
              type="button"
              class="text-ink-900 hover:text-brand-500 underline decoration-hairline underline-offset-2 truncate flex-1 text-left transition-colors"
              @click="openPreview(customerUpload)"
            >
              {{ customerUpload.file_original_name }}
            </button>
            <span class="text-xs text-ink-500 shrink-0">
              {{ fmtBytes(customerUpload.file_size_bytes) }} · {{ customerUpload.file_mime_type }}
            </span>
            <button
              type="button"
              class="rounded p-1 text-ink-500 hover:text-ink-950 transition-colors"
              aria-label="Download"
              @click="downloadFile(customerUpload)"
            >
              <Download class="h-4 w-4" :stroke-width="1.75" />
            </button>
          </div>
          <p v-if="!customerUpload.is_previewable" class="mt-2 text-xs text-ink-500">
            <XCircle class="inline h-3 w-3 mr-1 text-ink-400" :stroke-width="2" />
            Format {{ customerUpload.file_mime_type }} tidak preview di browser — download & buka manual (CorelDRAW/Illustrator).
          </p>
        </div>

        <!-- Staff drafts history -->
        <div v-if="staffDrafts.length" class="rounded-lg border border-hairline bg-canvas p-5">
          <h2 class="text-xs font-medium uppercase tracking-[0.14em] text-ink-500">
            Draft desain ({{ staffDrafts.length }})
          </h2>
          <ul class="mt-3 space-y-3">
            <li
              v-for="(f, i) in staffDrafts"
              :key="f.id"
              class="rounded-md border border-hairline bg-canvas-alt/50 p-3"
            >
              <div class="flex items-center gap-3">
                <span class="text-[10px] font-mono text-ink-500 shrink-0">v{{ staffDrafts.length - i }}</span>
                <button
                  type="button"
                  class="text-sm text-ink-900 hover:text-brand-500 underline decoration-hairline underline-offset-2 truncate flex-1 text-left transition-colors"
                  @click="openPreview(f)"
                >
                  {{ f.file_original_name }}
                </button>
                <AdminStatusBadge v-if="f.approval_status" :status="String(f.approval_status).replace(/_/g, ' ')" />
                <button
                  type="button"
                  class="rounded p-1 text-ink-500 hover:text-ink-950 transition-colors"
                  aria-label="Download"
                  @click="downloadFile(f)"
                >
                  <Download class="h-4 w-4" :stroke-width="1.75" />
                </button>
              </div>
              <p class="mt-1 text-xs text-ink-500">
                {{ fmtDate(f.uploaded_at) }} · {{ fmtBytes(f.file_size_bytes) }}
              </p>
              <p v-if="f.notes" class="mt-2 text-xs text-ink-700 border-l-2 border-hairline pl-2">
                {{ f.notes }}
              </p>
              <p v-if="f.approval_status === 'revision_requested' && f.revision_notes" class="mt-2 rounded bg-brand-50 border border-brand-200 p-2 text-xs text-brand-800">
                <strong>Revisi diminta:</strong> {{ f.revision_notes }}
              </p>
            </li>
          </ul>
        </div>

        <div v-if="!files.length && !loading" class="rounded-lg border border-dashed border-hairline bg-canvas p-6 text-center text-sm text-ink-500">
          Belum ada file. Untuk request path, tunggu customer upload aset/brief.
        </div>
      </div>

      <!-- Right column: actions -->
      <aside class="space-y-4">
        <!-- Verify upload -->
        <div v-if="canVerifyUpload" class="rounded-lg border border-hairline bg-canvas p-5">
          <h3 class="flex items-center gap-2 text-sm font-semibold text-ink-900">
            <FileCheck2 class="h-4 w-4 text-brand-500" :stroke-width="1.75" />
            Verifikasi Upload
          </h3>
          <p class="mt-1 text-xs text-ink-500 leading-relaxed">
            Cek file siap cetak di kolom kiri. Kalau OK, klik verify untuk lanjut ke <strong>desain_diverifikasi</strong> → proses cetak.
          </p>
          <input
            v-model="verifyNote"
            type="text"
            maxlength="500"
            placeholder="Catatan (opsional)"
            class="mt-3 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
          />
          <button
            type="button"
            class="mt-3 w-full inline-flex items-center justify-center gap-2 rounded-md bg-brand-500 px-3 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 disabled:opacity-60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors"
            :disabled="verifyBusy"
            @click="doVerify"
          >
            <span
              v-if="verifyBusy"
              class="inline-block h-3 w-3 rounded-full border-2 border-canvas/70 border-t-transparent animate-spin"
              aria-hidden="true"
            />
            Verifikasi &amp; Lanjut Cetak
          </button>
        </div>

        <!-- Upload draft (request path) -->
        <div v-if="canUploadDraft" class="rounded-lg border border-hairline bg-canvas p-5">
          <h3 class="flex items-center gap-2 text-sm font-semibold text-ink-900">
            <Upload class="h-4 w-4 text-brand-500" :stroke-width="1.75" />
            Upload Draft
          </h3>
          <p class="mt-1 text-xs text-ink-500 leading-relaxed">
            Upload hasil kerja desain. Setelah simpan, order otomatis lanjut ke <strong>menunggu_approval_desain</strong> untuk direview customer.
          </p>
          <label class="mt-3 flex items-center justify-center rounded-md border border-hairline bg-canvas-alt/40 px-3 py-2 text-sm text-ink-700 cursor-pointer hover:bg-canvas-alt transition-colors">
            <span class="truncate">{{ draftFile ? draftFile.name : 'Pilih file draft…' }}</span>
            <input
              ref="draftFileInput"
              type="file"
              accept="image/jpeg,image/png,image/webp,application/pdf,.cdr,.ai"
              class="hidden"
              @change="onDraftFileChange"
            />
          </label>
          <p class="mt-1 text-[10px] text-ink-500">
            Format: JPG/PNG/WebP/PDF/CDR/AI. Max {{ Math.round(25) }} MB.
          </p>
          <input
            v-model="draftNotes"
            type="text"
            maxlength="500"
            placeholder="Catatan draft (opsional)"
            class="mt-3 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
          />
          <button
            type="button"
            class="mt-3 w-full inline-flex items-center justify-center gap-2 rounded-md bg-brand-500 px-3 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 disabled:opacity-60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors"
            :disabled="draftBusy || !draftFile"
            @click="submitDraft"
          >
            <span
              v-if="draftBusy"
              class="inline-block h-3 w-3 rounded-full border-2 border-canvas/70 border-t-transparent animate-spin"
              aria-hidden="true"
            />
            Kirim Draft ke Customer
          </button>
        </div>

        <!-- Walk-in instant approve (POS §11) -->
        <div v-if="canWalkinApprove" class="rounded-lg border border-gold-200 bg-gold-50 p-5">
          <h3 class="flex items-center gap-2 text-sm font-semibold text-gold-900">
            <Sparkles class="h-4 w-4" :stroke-width="1.75" />
            Walk-in Instant Approve
          </h3>
          <p class="mt-1 text-xs text-gold-800 leading-relaxed">
            Order POS dengan mode instant_walkin — customer approve verbal di tempat, skip loop notif WA. Klik untuk lanjut ke <strong>desain_diverifikasi</strong>.
          </p>
          <button
            type="button"
            class="mt-3 w-full inline-flex items-center justify-center gap-2 rounded-md bg-gold-500 px-3 py-2 text-sm font-semibold text-canvas hover:bg-gold-600 disabled:opacity-60 transition-colors"
            :disabled="walkinBusy"
            @click="doWalkin"
          >
            <span
              v-if="walkinBusy"
              class="inline-block h-3 w-3 rounded-full border-2 border-canvas/70 border-t-transparent animate-spin"
              aria-hidden="true"
            />
            Approve Walk-in
          </button>
        </div>

        <!-- Fallback: no actions available -->
        <div
          v-if="!canVerifyUpload && !canUploadDraft && !canWalkinApprove"
          class="rounded-lg border border-dashed border-hairline bg-canvas p-5 text-xs text-ink-500 leading-relaxed"
        >
          <p class="font-medium text-ink-700 mb-1">Tidak ada aksi tersedia</p>
          <p>
            Order pada status <strong class="font-mono">{{ order?.status }}</strong> tidak butuh aksi desain saat ini.
            Cek modul lain (pembayaran / produksi) atau tunggu customer approval.
          </p>
        </div>

        <!-- Meta -->
        <div class="rounded-lg border border-hairline bg-canvas p-5 space-y-2 text-xs text-ink-600">
          <div class="flex justify-between">
            <span class="text-ink-500">Design source</span>
            <span class="uppercase font-medium text-ink-900">{{ order.design_source }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-ink-500">Status order</span>
            <span class="uppercase font-medium text-ink-900">{{ order.status }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-ink-500">Total files</span>
            <span class="font-medium text-ink-900">{{ files.length }}</span>
          </div>
        </div>
      </aside>
    </div>

    <!-- Preview modal -->
    <Teleport to="body">
      <Transition name="fade">
        <div v-if="preview || previewLoading" class="fixed inset-0 z-50 flex items-center justify-center p-4">
          <div class="absolute inset-0 bg-ink-950/60" aria-hidden="true" @click="closePreview" />
          <div
            role="dialog"
            aria-modal="true"
            class="relative w-full max-w-3xl rounded-lg bg-canvas shadow-xl ring-1 ring-black/5 flex flex-col max-h-[90vh]"
          >
            <div class="flex items-start justify-between border-b border-hairline p-4">
              <div class="text-sm min-w-0 flex-1">
                <p class="font-semibold text-ink-950 truncate">{{ preview?.file.file_original_name || 'Memuat…' }}</p>
                <p v-if="preview" class="mt-0.5 text-xs text-ink-500">
                  {{ preview.mime }} · {{ preview.file ? fmtBytes(preview.file.file_size_bytes) : '' }}
                </p>
              </div>
              <button
                type="button"
                class="rounded-md p-1 text-ink-500 hover:bg-canvas-alt transition-colors"
                aria-label="Tutup"
                @click="closePreview"
              >
                <XCircle class="h-5 w-5" :stroke-width="1.75" />
              </button>
            </div>
            <div class="flex-1 overflow-auto p-4 bg-canvas-alt/40">
              <div v-if="previewLoading" class="text-center text-sm text-ink-500 py-12">
                Memuat preview…
              </div>
              <img
                v-else-if="previewIsImage && preview"
                :src="preview.url"
                :alt="preview.file.file_original_name"
                class="mx-auto max-h-[65vh] rounded border border-hairline"
              />
              <object
                v-else-if="previewIsPDF && preview"
                :data="preview.url"
                type="application/pdf"
                class="w-full h-[65vh] rounded"
              >
                <p class="text-sm text-ink-600 p-4">Browser tidak bisa preview PDF ini. Silakan download.</p>
              </object>
              <div v-else-if="preview" class="text-center text-sm text-ink-600 py-12">
                Format <code class="font-mono">{{ preview.mime }}</code> tidak bisa preview inline.
                Klik download untuk buka di aplikasi lokal (CorelDRAW/Illustrator).
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </section>
</template>

<style scoped>
.fade-enter-active, .fade-leave-active { transition: opacity 0.15s ease; }
.fade-enter-from, .fade-leave-to { opacity: 0; }
</style>
