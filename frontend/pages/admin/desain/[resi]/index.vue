<script setup lang="ts">
/**
 * /admin/desain/[resi] — full design workflow untuk 1 order (§32.5 per-item).
 *
 * Sejak §32 Order Multi-Item, satu order bisa memuat banyak baris produk
 * (`order.items[]`), dan tiap file desain menempel ke SATU baris
 * (`design_files.order_item_id`) — bukan ke order secara umum. Halaman ini
 * mengelompokkan file PER ITEM supaya staf tidak salah cetak saat dua banner
 * ukurannya mirip (§32.5 doc comment `model.DesignFile.OrderItemID`).
 *
 * Aksi state-aware:
 *   - Verifikasi Upload (ORDER-WIDE, §32.6 — status desain tetap satu per
 *     order): kalau order.design_source='upload' (SEMUA item upload, bukan
 *     'mixed'), status=dibayar → tombol "Verifikasi & Lanjut Cetak" (perm
 *     design.approve). Backend menolak kalau ADA SATU SAJA item yang belum
 *     punya file customer_upload sendiri — dihitung di `missingUploadItems`
 *     supaya staf lihat item mana yang masih kurang SEBELUM menekan tombol,
 *     bukan cuma dapat galat generik sesudahnya.
 *   - Upload Draft (PER ITEM, request path): tiap baris item dengan
 *     design_source='request' punya form upload draft sendiri (perm
 *     design.work). Backend cuma mengizinkan SATU draft `pending` per ORDER
 *     (bukan per item) — kalau ada draft lain sedang menunggu respons
 *     customer, form baris lain dinonaktifkan (lihat `pendingDraftFile`).
 *   - Walk-in Approve: kalau channel=pos, status=desain_dikerjakan → tombol
 *     shortcut (perm design.approve). Field `design_approval_mode` yang
 *     dulu jadi syarat tambahan TIDAK LAGI dikirim backend di level order
 *     (lihat catatan di `types/order.ts`), jadi gating di sini dilonggarkan
 *     ke channel+status saja — backend tetap menolak final kalau order ini
 *     ternyata mode 'async_notify' (ErrWalkinOnlyForPOS).
 *   - Skip Upload: kalau channel=pos, order.design_source='upload',
 *     status=dibayar, DAN belum ada satu pun file customer_upload di order
 *     ini → tombol "Lewati Upload — Langsung Cetak" (perm design.approve).
 *
 * Pengelompokan file per item mengandalkan `order_items.id` asli
 * (`item.id`, dipetakan backend sejak 1 September 2026 — lihat doc comment
 * `OrderItem.id` di `types/order.ts`), jadi selalu akurat untuk order 1
 * maupun banyak item, tidak ada lagi jalur tebak-tebakan.
 *
 * File preview: image/pdf/webp langsung inline lewat blob URL (Bearer token
 * dikirim via fetch, bukan lewat <img src=…>). CDR/AI tidak previewable —
 * cukup download link.
 */
import { orderPrimaryProductLabel, type Order, type OrderItem } from '~/types/order'
import type { DesignFile } from '~/types/design'
import { ApiError } from '~/composables/useApi'
import {
  FileCheck2,
  Upload,
  Sparkles,
  Download,
  XCircle,
  Printer,
  AlertTriangle,
  Loader2,
} from '@lucide/vue'

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

/**
 * Error khusus dialog "Lewati Upload". Dipisah dari `errorMsg` (banner
 * halaman) karena banner ada di belakang overlay dialog — kegagalan submit
 * jadi tidak terlihat sampai dialog ditutup manual. Aksi lain di halaman ini
 * (verify, walk-in approve, upload draft, preview/download) tetap pakai
 * `errorMsg`.
 */
const modalError = ref<string | null>(null)

useSeoMeta({ title: () => `Desain ${resi.value} — Admin` })

// Permissions
const canApprove = computed(() => auth.hasPermission('design.approve'))
// Permission tersendiri (§10) — kasir boleh melewati upload untuk walk-in di
// depannya tanpa ikut dapat kuasa verifikasi desain order online.
const canSkip = computed(() => auth.hasPermission('design.skip_upload'))
const canWork = computed(() => auth.hasPermission('design.work'))

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

// --- Pengelompokan file per item (§32.5) ---
function filesForItem(item: OrderItem): DesignFile[] {
  return files.value.filter((f) => f.order_item_id === item.id)
}

function itemUploadFile(item: OrderItem): DesignFile | undefined {
  return filesForItem(item).find((f) => f.role === 'customer_upload' && !f.is_purged)
}
function itemAssets(item: OrderItem): DesignFile[] {
  return filesForItem(item).filter((f) => f.role === 'customer_asset' && !f.is_purged)
}
function itemDrafts(item: OrderItem): DesignFile[] {
  return filesForItem(item)
    .filter((f) => f.role === 'staff_draft')
    .sort((a, b) => (a.uploaded_at < b.uploaded_at ? 1 : -1))
}

// --- Derived state (order-wide) ---
const anyUploadFileExists = computed(() =>
  files.value.some((f) => f.role === 'customer_upload' && !f.is_purged),
)

interface MissingItemInfo { key: string; label: string }

/**
 * Item yang masih belum punya file customer_upload — dihitung SEBELUM staf
 * menekan "Verifikasi & Lanjut Cetak" supaya galat backend (§32.5:
 * StaffVerifyUpload menolak kalau ada satu saja item tanpa file) sudah
 * terlihat lebih dulu, bukan cuma pesan generik sesudahnya.
 */
const missingUploadItems = computed<MissingItemInfo[]>(() => {
  if (!order.value) return []
  return order.value.items
    .filter((it) => !itemUploadFile(it))
    .map((it) => ({ key: `item-${it.line_no}`, label: `Item ${it.line_no} — ${it.product_name}` }))
})

const canVerifyUpload = computed(
  () =>
    canApprove.value &&
    order.value?.design_source === 'upload' &&
    order.value?.status === 'dibayar',
)
const canWalkinApprove = computed(
  () =>
    canApprove.value &&
    order.value?.channel === 'pos' &&
    order.value?.status === 'desain_dikerjakan',
)
const canSkipUpload = computed(
  () =>
    canSkip.value &&
    order.value?.channel === 'pos' &&
    order.value?.design_source === 'upload' &&
    order.value?.status === 'dibayar' &&
    !anyUploadFileExists.value,
)

const anyItemCanUploadDraft = computed(() =>
  order.value ? order.value.items.some((it) => canUploadDraftForItem(it)) : false,
)

// --- Draft desain: aturan SATU pending draft per ORDER (bukan per item) ---
const pendingDraftFile = computed(() =>
  files.value.find((f) => f.role === 'staff_draft' && f.approval_status === 'pending'),
)
const pendingDraftItemLabel = computed(() => {
  const f = pendingDraftFile.value
  if (!f || !order.value) return null
  const it = order.value.items.find((x) => x.id === f.order_item_id)
  return it ? `Item ${it.line_no} (${it.product_name})` : 'item lain'
})

function canUploadDraftForItem(item: OrderItem): boolean {
  if (!canWork.value || !order.value) return false
  if (item.design_source !== 'request') return false
  if (!['dibayar', 'desain_dikerjakan'].includes(order.value.status)) return false
  return !pendingDraftFile.value
}

/**
 * Alasan tombol upload draft TIDAK ditawarkan untuk baris request — cuma
 * dihitung untuk item yang MEMANG relevan (request, status masih dalam
 * jendela upload, staf punya permission) supaya tidak menampilkan pesan
 * yang membingungkan untuk item yang memang tidak butuh aksi ini sekarang.
 */
function draftBlockedReason(item: OrderItem): string | null {
  if (!order.value || !canWork.value) return null
  if (item.design_source !== 'request') return null
  if (!['dibayar', 'desain_dikerjakan'].includes(order.value.status)) return null
  if (pendingDraftFile.value) {
    return `Ada draft lain (${pendingDraftItemLabel.value}) masih menunggu respons customer — selesaikan itu dulu sebelum upload draft baru.`
  }
  return null
}

// --- Actions: verify / walk-in / skip (order-wide, tidak berubah dari §32.6) ---
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
    errorMsg.value =
      e instanceof ApiError
        ? // Backend masih menolak tegas order mode 'async_notify' (§11) —
          // gating frontend sekarang cuma channel+status (lihat blocker
          // design_approval_mode di doc comment atas), jadi penolakan ini
          // BISA terjadi pada kondisi normal, bukan cuma bug.
          e.message
        : 'Gagal walk-in approve'
  } finally {
    walkinBusy.value = false
  }
}

const skipUploadBusy = ref(false)
const skipUploadNote = ref('')
const skipUploadDialogOpen = ref(false)

function openSkipUploadDialog() {
  modalError.value = null
  skipUploadDialogOpen.value = true
}

async function doSkipUpload() {
  if (!skipUploadNote.value.trim()) return
  skipUploadBusy.value = true
  modalError.value = null
  try {
    await designSvc.skipUpload(resi.value, skipUploadNote.value.trim())
    successMsg.value = 'Order lanjut ke desain_diverifikasi tanpa file. Catatan tersimpan di riwayat.'
    setTimeout(() => (successMsg.value = null), 3000)
    skipUploadNote.value = ''
    skipUploadDialogOpen.value = false
    await load()
  } catch (e: unknown) {
    modalError.value = e instanceof ApiError ? e.message : 'Gagal melewati upload'
  } finally {
    skipUploadBusy.value = false
  }
}

// --- Draft upload form (PER ITEM, keyed by line_no — selalu ada, tidak
// bergantung pada `item.id` yang bisa undefined) ---
interface DraftFormState {
  file: File | null
  notes: string
  busy: boolean
}
const draftForms = reactive<Record<number, DraftFormState>>({})
function draftForm(lineNo: number): DraftFormState {
  if (!draftForms[lineNo]) draftForms[lineNo] = { file: null, notes: '', busy: false }
  return draftForms[lineNo]
}
function onDraftFileChange(lineNo: number, ev: Event) {
  const input = ev.target as HTMLInputElement
  draftForm(lineNo).file = input.files?.[0] ?? null
}
async function submitItemDraft(item: OrderItem) {
  const state = draftForm(item.line_no)
  if (!state.file || !item.id) return
  state.busy = true
  errorMsg.value = null
  try {
    await designSvc.uploadDraft(resi.value, item.id, state.file, state.notes.trim() || undefined)
    successMsg.value = `Draft untuk item ${item.line_no} terupload. Customer bisa review.`
    setTimeout(() => (successMsg.value = null), 3000)
    state.file = null
    state.notes = ''
    await load()
  } catch (e: unknown) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal upload draft'
  } finally {
    state.busy = false
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
function designSourceLabel(o: Order): string {
  if (o.design_source !== 'mixed') return o.design_source
  const uploadCount = o.items.filter((it) => it.design_source === 'upload').length
  const requestCount = o.items.filter((it) => it.design_source === 'request').length
  return `campuran — ${uploadCount} upload, ${requestCount} request`
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
      <!-- Left column: order summary + per-item files & brief -->
      <div class="lg:col-span-2 space-y-4">
        <!-- Order summary card -->
        <div class="rounded-lg border border-hairline bg-canvas p-5">
          <h2 class="text-xs font-medium uppercase tracking-[0.14em] text-ink-500">Order</h2>
          <p class="mt-2 font-serif text-lg font-semibold tracking-tight text-ink-950">{{ orderPrimaryProductLabel(order) }}</p>
          <p class="mt-0.5 text-sm text-ink-600">
            {{ order.items.length }} {{ order.items.length > 1 ? 'item' : 'item' }}
          </p>
          <div class="mt-3 flex flex-wrap gap-3 text-xs text-ink-500">
            <span><strong class="text-ink-900 uppercase">{{ designSourceLabel(order) }}</strong></span>
            <span>·</span>
            <span class="uppercase">Channel {{ order.channel }}</span>
            <span>·</span>
            <span class="uppercase">{{ order.metode_ambil }}</span>
          </div>
        </div>

        <!-- Per-item cards -->
        <div
          v-for="item in order.items"
          :key="item.line_no"
          class="rounded-lg border border-hairline bg-canvas p-5"
        >
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0">
              <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Item {{ item.line_no }}</p>
              <p class="mt-0.5 font-serif text-base font-semibold text-ink-950">{{ item.product_name }}</p>
              <p class="mt-0.5 text-xs text-ink-500">
                {{ item.material_name }} · {{ item.width_cm }}×{{ item.height_cm }}cm · {{ item.quantity }} pcs
              </p>
            </div>
            <span
              class="inline-flex flex-none items-center rounded-full px-2 py-0.5 text-[10px] font-medium uppercase tracking-[0.1em] ring-1 ring-inset ring-hairline bg-canvas-alt text-ink-700"
            >
              {{ item.design_source }}
            </span>
          </div>

          <!-- Upload path: file siap cetak dari customer -->
          <template v-if="item.design_source === 'upload'">
            <div v-if="itemUploadFile(item)" class="mt-4 flex items-center gap-3 rounded-md border border-hairline bg-canvas-alt/40 p-3 text-sm">
              <button
                type="button"
                class="text-ink-900 hover:text-brand-500 underline decoration-hairline underline-offset-2 truncate flex-1 text-left transition-colors"
                @click="openPreview(itemUploadFile(item)!)"
              >
                {{ itemUploadFile(item)!.file_original_name }}
              </button>
              <span class="text-xs text-ink-500 shrink-0">
                {{ fmtBytes(itemUploadFile(item)!.file_size_bytes) }}
              </span>
              <button
                type="button"
                class="rounded p-1 text-ink-500 hover:text-ink-950 transition-colors"
                aria-label="Download"
                @click="downloadFile(itemUploadFile(item)!)"
              >
                <Download class="h-4 w-4" :stroke-width="1.75" />
              </button>
            </div>
            <p v-else class="mt-3 flex items-center gap-2 text-xs text-ink-500">
              <AlertTriangle class="h-3.5 w-3.5 text-ink-400" :stroke-width="1.75" />
              Belum ada file dari customer untuk item ini.
            </p>
          </template>

          <!-- Request path: brief + aset + draft history + form upload draft -->
          <template v-else>
            <p v-if="item.design_brief" class="mt-3 text-sm text-ink-800 leading-relaxed border-l-2 border-gold-400 pl-3 whitespace-pre-line">
              {{ item.design_brief }}
            </p>
            <p v-else class="mt-3 text-xs text-ink-500 italic">Tidak ada brief tekstual.</p>

            <div v-if="itemAssets(item).length" class="mt-3">
              <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2">
                Aset dari customer ({{ itemAssets(item).length }})
              </p>
              <ul class="space-y-2">
                <li v-for="f in itemAssets(item)" :key="f.id" class="flex items-center gap-3 text-sm">
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

            <div v-if="itemDrafts(item).length" class="mt-4">
              <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2">
                Draft desain ({{ itemDrafts(item).length }})
              </p>
              <ul class="space-y-3">
                <li
                  v-for="(f, i) in itemDrafts(item)"
                  :key="f.id"
                  class="rounded-md border border-hairline bg-canvas-alt/50 p-3"
                >
                  <div class="flex items-center gap-3">
                    <span class="text-[10px] font-mono text-ink-500 shrink-0">v{{ itemDrafts(item).length - i }}</span>
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

            <!-- Upload draft form (per item) -->
            <div v-if="canUploadDraftForItem(item)" class="mt-4 rounded-md border border-hairline bg-canvas-alt/40 p-4">
              <h3 class="flex items-center gap-2 text-sm font-semibold text-ink-900">
                <Upload class="h-4 w-4 text-brand-500" :stroke-width="1.75" />
                Upload Draft — Item {{ item.line_no }}
              </h3>
              <p class="mt-1 text-xs text-ink-500 leading-relaxed">
                Setelah simpan, order lanjut ke <strong>menunggu_approval_desain</strong> untuk direview customer.
              </p>
              <label class="mt-3 flex items-center justify-center rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-700 cursor-pointer hover:bg-canvas-alt transition-colors">
                <span class="truncate">{{ draftForm(item.line_no).file ? draftForm(item.line_no).file!.name : 'Pilih file draft…' }}</span>
                <input
                  type="file"
                  accept="image/jpeg,image/png,image/webp,application/pdf,.cdr,.ai"
                  class="hidden"
                  @change="onDraftFileChange(item.line_no, $event)"
                >
              </label>
              <p class="mt-1 text-[10px] text-ink-500">Format: JPG/PNG/WebP/PDF/CDR/AI. Max 25 MB.</p>
              <input
                v-model="draftForm(item.line_no).notes"
                type="text"
                maxlength="500"
                placeholder="Catatan draft (opsional)"
                class="mt-3 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
              >
              <button
                type="button"
                class="mt-3 w-full inline-flex items-center justify-center gap-2 rounded-md bg-brand-500 px-3 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 disabled:opacity-60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors"
                :disabled="draftForm(item.line_no).busy || !draftForm(item.line_no).file"
                @click="submitItemDraft(item)"
              >
                <Loader2 v-if="draftForm(item.line_no).busy" class="h-3.5 w-3.5 animate-spin" :stroke-width="1.75" />
                Kirim Draft ke Customer
              </button>
            </div>
            <p
              v-else-if="draftBlockedReason(item)"
              class="mt-4 flex items-start gap-2 rounded-md border border-hairline bg-canvas-alt/40 p-3 text-xs text-ink-500 leading-relaxed"
            >
              <AlertTriangle class="h-3.5 w-3.5 flex-none mt-0.5 text-ink-400" :stroke-width="1.75" />
              {{ draftBlockedReason(item) }}
            </p>
          </template>
        </div>

        <div v-if="!files.length && !loading" class="rounded-lg border border-dashed border-hairline bg-canvas p-6 text-center text-sm text-ink-500">
          Belum ada file. Untuk request path, tunggu customer upload aset/brief.
        </div>
      </div>

      <!-- Right column: order-wide actions -->
      <aside class="space-y-4">
        <!-- Verify upload -->
        <div v-if="canVerifyUpload" class="rounded-lg border border-hairline bg-canvas p-5">
          <h3 class="flex items-center gap-2 text-sm font-semibold text-ink-900">
            <FileCheck2 class="h-4 w-4 text-brand-500" :stroke-width="1.75" />
            Verifikasi Upload
          </h3>
          <p class="mt-1 text-xs text-ink-500 leading-relaxed">
            Cek file siap cetak di kolom kiri. Kalau semua item sudah oke, klik verify untuk lanjut ke
            <strong>desain_diverifikasi</strong> → proses cetak.
          </p>

          <div v-if="missingUploadItems.length" class="mt-3 rounded-md border border-brand-200 bg-brand-50/60 p-3">
            <p class="flex items-start gap-2 text-xs text-brand-800 leading-relaxed">
              <AlertTriangle class="h-3.5 w-3.5 flex-none mt-0.5" :stroke-width="1.75" />
              Belum bisa diverifikasi — item berikut belum punya file:
            </p>
            <ul class="mt-1.5 ml-5 list-disc text-xs text-brand-800 space-y-0.5">
              <li v-for="m in missingUploadItems" :key="m.key">{{ m.label }}</li>
            </ul>
          </div>

          <input
            v-model="verifyNote"
            type="text"
            maxlength="500"
            placeholder="Catatan (opsional)"
            class="mt-3 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
          >
          <button
            type="button"
            class="mt-3 w-full inline-flex items-center justify-center gap-2 rounded-md bg-brand-500 px-3 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 disabled:opacity-60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors"
            :disabled="verifyBusy || missingUploadItems.length > 0"
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

        <!-- Walk-in instant approve (POS §11) -->
        <div v-if="canWalkinApprove" class="rounded-lg border border-gold-200 bg-gold-50 p-5">
          <h3 class="flex items-center gap-2 text-sm font-semibold text-gold-900">
            <Sparkles class="h-4 w-4" :stroke-width="1.75" />
            Walk-in Instant Approve
          </h3>
          <p class="mt-1 text-xs text-gold-800 leading-relaxed">
            Order POS di tahap desain dikerjakan — customer approve verbal di tempat, skip loop notif WA. Klik untuk
            lanjut ke <strong>desain_diverifikasi</strong>. Backend menolak kalau order ini ternyata mode
            follow-up-via-WA (bukan instant) — pakai alur approval standar untuk kasus itu.
          </p>
          <button
            type="button"
            class="mt-3 w-full inline-flex items-center justify-center gap-2 rounded-md bg-gold-500 px-3 py-2 text-sm font-semibold text-canvas transition-colors hover:bg-gold-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas disabled:opacity-60"
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

        <!-- Skip upload (POS: file ada di komputer desainer, tidak masuk sistem) -->
        <div v-if="canSkipUpload" class="rounded-lg border border-hairline bg-canvas p-6">
          <h3 class="flex items-center gap-2 text-sm font-semibold text-ink-900">
            <Printer class="h-4 w-4 text-ink-500" :stroke-width="1.75" />
            Lewati Upload
          </h3>
          <p class="mt-1 text-xs text-ink-500 leading-relaxed">
            Untuk walk-in yang bawa desain siap cetak tapi filenya cuma ada di komputer desainer, bukan diupload ke
            sistem.
          </p>
          <label class="mt-3 block text-xs font-medium text-ink-700">Lokasi file desain</label>
          <textarea
            v-model="skipUploadNote"
            rows="2"
            maxlength="500"
            placeholder="PC desain — folder Agustus/spanduk-warung-bu-sri"
            class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
          />
          <p class="mt-1 text-xs text-ink-500">
            Wajib diisi. Dicatat di riwayat order sebagai jejak, karena filenya tidak masuk sistem.
          </p>
          <button
            type="button"
            class="mt-3 w-full inline-flex items-center justify-center gap-2 rounded-md border border-hairline bg-canvas px-3 py-2 text-sm font-semibold text-ink-900 hover:bg-canvas-alt disabled:opacity-60 disabled:cursor-not-allowed focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors"
            :disabled="!skipUploadNote.trim()"
            @click="openSkipUploadDialog"
          >
            <Printer class="h-4 w-4" :stroke-width="1.75" />
            Lewati Upload — Langsung Cetak
          </button>
        </div>

        <!-- Fallback: no actions available -->
        <div
          v-if="!canVerifyUpload && !canWalkinApprove && !canSkipUpload && !anyItemCanUploadDraft"
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

    <!-- Konfirmasi lewati upload (§26.7 — irreversible, wajib confirm dialog) -->
    <AdminConfirmDialog
      v-model:open="skipUploadDialogOpen"
      title="Lewati upload dan langsung cetak?"
      message="Order akan lanjut ke tahap cetak tanpa file desain tersimpan di sistem. Tindakan ini tidak bisa dibatalkan."
      confirm-label="Ya, lanjut cetak"
      variant="danger"
      :loading="skipUploadBusy"
      :error="modalError"
      @confirm="doSkipUpload"
    >
      <div class="mt-4 rounded-md bg-canvas-alt p-3">
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Lokasi file desain</p>
        <p class="mt-1 text-sm text-ink-700 whitespace-pre-line">{{ skipUploadNote }}</p>
      </div>
    </AdminConfirmDialog>

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
              >
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
