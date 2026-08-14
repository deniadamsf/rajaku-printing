<script setup lang="ts">
import type { PaymentProof, ProofStatus } from '~/types/payment'
import type { DataTableColumn } from '~/components/admin/DataTable.vue'
import { ApiError } from '~/composables/useApi'

definePageMeta({
  middleware: ['staff-only'],
  layout: 'admin',
})

useSeoMeta({ title: 'Verifikasi Pembayaran — Admin' })

const auth = useAuthStore()
const pay = usePayment()

const canApprove = computed(() => auth.hasPermission('payment.verify'))
const canReject = computed(() => auth.hasPermission('payment.reject'))

const page = ref(1)
const pageSize = ref(10)
const status = ref<ProofStatus | ''>('pending')
const items = ref<PaymentProof[]>([])
const total = ref(0)
const loading = ref(false)
const errorMsg = ref<string | null>(null)
const successMsg = ref<string | null>(null)

async function fetchList() {
  loading.value = true
  errorMsg.value = null
  try {
    const res = await pay.listProofs({
      status: status.value || undefined,
      page: page.value,
      pageSize: pageSize.value,
    })
    items.value = res.items
    total.value = res.total
  } catch (e: unknown) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal memuat data'
  } finally {
    loading.value = false
  }
}

onMounted(fetchList)
watch([page, status], fetchList)

const columns: DataTableColumn[] = [
  { key: 'uploaded_at', label: 'Uploaded', class: 'w-40 hidden md:table-cell' },
  { key: 'order_id', label: 'Order' },
  { key: 'metode_bayar', label: 'Metode', class: 'w-32' },
  { key: 'amount_claimed', label: 'Nominal', class: 'w-32 text-right' },
  { key: 'status', label: 'Status', class: 'w-28' },
  { key: 'actions', label: '', class: 'w-32 text-right' },
]

function fmtDate(s: string): string {
  try {
    return new Date(s).toLocaleString('id-ID', {
      day: '2-digit', month: 'short',
      hour: '2-digit', minute: '2-digit',
    })
  } catch {
    return s
  }
}

function fmtIDR(amount?: number | null): string {
  if (amount == null) return '—'
  return new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 }).format(amount)
}

// --- Preview modal & actions ---
const previewProof = ref<PaymentProof | null>(null)
const previewURL = ref<string | null>(null)
const previewLoading = ref(false)
const rejectReason = ref('')
const rejectOpen = ref(false)
const busy = ref(false)

async function openPreview(p: PaymentProof) {
  previewProof.value = p
  previewURL.value = null
  rejectReason.value = ''
  previewLoading.value = true
  try {
    previewURL.value = await pay.proofFilePreviewURL(p.id)
  } catch (e: unknown) {
    errorMsg.value = e instanceof Error ? e.message : 'Gagal load preview'
  } finally {
    previewLoading.value = false
  }
}

function closePreview() {
  if (previewURL.value) URL.revokeObjectURL(previewURL.value)
  previewURL.value = null
  previewProof.value = null
  rejectReason.value = ''
}

async function onApprove() {
  const p = previewProof.value
  if (!p) return
  busy.value = true
  errorMsg.value = null
  try {
    await pay.approveProof(p.id)
    successMsg.value = `Bukti order ${p.order_id.slice(0, 8)}… disetujui.`
    setTimeout(() => (successMsg.value = null), 3000)
    closePreview()
    await fetchList()
  } catch (e: unknown) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal approve'
  } finally {
    busy.value = false
  }
}

function askReject() {
  rejectOpen.value = true
}

async function confirmReject() {
  const p = previewProof.value
  if (!p) return
  if (rejectReason.value.trim().length < 3) {
    errorMsg.value = 'Alasan reject minimal 3 karakter.'
    return
  }
  busy.value = true
  errorMsg.value = null
  try {
    await pay.rejectProof(p.id, rejectReason.value.trim())
    successMsg.value = `Bukti order ${p.order_id.slice(0, 8)}… ditolak.`
    setTimeout(() => (successMsg.value = null), 3000)
    rejectOpen.value = false
    closePreview()
    await fetchList()
  } catch (e: unknown) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal reject'
  } finally {
    busy.value = false
  }
}

onBeforeUnmount(() => {
  if (previewURL.value) URL.revokeObjectURL(previewURL.value)
})

const previewIsImage = computed(() => {
  const mime = previewProof.value?.file_mime_type || ''
  return mime.startsWith('image/')
})
const previewIsPDF = computed(() => previewProof.value?.file_mime_type === 'application/pdf')
</script>

<template>
  <section>
    <AdminPageHeader
      title="Verifikasi Pembayaran"
      subtitle="Approve atau tolak bukti transfer dari customer. Setelah approve, order lanjut ke jalur produksi."
    />

    <AlertMessage v-if="errorMsg" variant="error" :message="errorMsg" class="mb-4" />
    <AlertMessage v-if="successMsg" variant="success" :message="successMsg" class="mb-4" />

    <div class="mb-4 flex flex-wrap items-center gap-2">
      <div class="inline-flex rounded-md border border-hairline overflow-hidden text-sm">
        <button
          v-for="opt in [
            { v: 'pending', label: 'Pending' },
            { v: 'approved', label: 'Approved' },
            { v: 'rejected', label: 'Rejected' },
            { v: '', label: 'Semua' },
          ]"
          :key="opt.v"
          type="button"
          :class="[
            'px-3 py-1.5 border-r border-hairline last:border-r-0 transition-colors',
            status === opt.v ? 'bg-ink-950 text-canvas' : 'bg-canvas text-ink-700 hover:bg-canvas-alt',
          ]"
          @click="status = opt.v as ProofStatus | ''; page = 1"
        >
          {{ opt.label }}
        </button>
      </div>
    </div>

    <AdminDataTable
      :columns="columns"
      :items="items"
      :loading="loading"
      row-key="id"
      empty-message="Tidak ada bukti pembayaran pada filter ini."
    >
      <template #cell-uploaded_at="{ row }">
        <span class="text-xs text-ink-500">{{ fmtDate((row as PaymentProof).uploaded_at) }}</span>
      </template>
      <template #cell-order_id="{ row }">
        <span class="font-mono text-xs text-ink-700">{{ (row as PaymentProof).order_id.slice(0, 8) }}…</span>
      </template>
      <template #cell-metode_bayar="{ row }">
        <span class="text-xs uppercase text-ink-700">{{ (row as PaymentProof).metode_bayar }}</span>
      </template>
      <template #cell-amount_claimed="{ row }">
        <span class="text-ink-900">{{ fmtIDR((row as PaymentProof).amount_claimed) }}</span>
      </template>
      <template #cell-status="{ row }">
        <AdminStatusBadge :status="(row as PaymentProof).status" />
      </template>
      <template #cell-actions="{ row }">
        <button
          type="button"
          class="inline-flex items-center rounded-md border border-hairline bg-canvas px-2 py-1 text-xs text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
          @click="openPreview(row as PaymentProof)"
        >
          Preview
        </button>
      </template>
    </AdminDataTable>

    <AdminPagination
      v-if="total > 0"
      :page="page"
      :limit="pageSize"
      :total="total"
      @update:page="(v) => (page = v)"
    />

    <!-- Preview modal (custom, bukan ConfirmDialog karena butuh custom content). -->
    <Teleport to="body">
      <Transition name="fade">
        <div v-if="previewProof" class="fixed inset-0 z-50 flex items-center justify-center p-4">
          <div class="absolute inset-0 bg-ink-950/60" aria-hidden="true" @click="closePreview" />
          <div
            role="dialog"
            aria-modal="true"
            class="relative w-full max-w-3xl rounded-lg bg-canvas shadow-xl ring-1 ring-black/5 flex flex-col max-h-[90vh]"
          >
            <div class="flex items-start justify-between border-b border-hairline p-4">
              <div class="text-sm">
                <p class="font-serif text-lg font-semibold text-ink-950">Bukti Pembayaran</p>
                <p class="mt-0.5 text-xs text-ink-500 font-mono">
                  Order {{ previewProof.order_id }}
                </p>
                <p class="mt-0.5 text-xs text-ink-500">
                  {{ previewProof.file_original_name }} · {{ (previewProof.file_size_bytes / 1024).toFixed(1) }} KB · {{ previewProof.file_mime_type }}
                </p>
                <p class="mt-0.5 text-xs text-ink-500">
                  Metode: <strong class="uppercase text-ink-900">{{ previewProof.metode_bayar }}</strong> ·
                  Nominal: <strong class="text-ink-900">{{ fmtIDR(previewProof.amount_claimed) }}</strong>
                </p>
              </div>
              <button
                type="button"
                class="rounded-md p-1 text-ink-500 hover:bg-canvas-alt hover:text-ink-900 transition-colors"
                aria-label="Tutup"
                @click="closePreview"
              >
                <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
                  <path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd" />
                </svg>
              </button>
            </div>

            <div class="flex-1 overflow-auto p-4 bg-canvas-alt">
              <div v-if="previewLoading" class="text-center text-sm text-ink-500 py-12">
                Memuat preview…
              </div>
              <div v-else-if="!previewURL" class="text-center text-sm text-brand-700 py-12">
                Gagal memuat file preview. Coba download langsung.
              </div>
              <img
                v-else-if="previewIsImage"
                :src="previewURL"
                alt="Bukti"
                class="mx-auto max-h-[60vh] rounded shadow"
              />
              <object
                v-else-if="previewIsPDF"
                :data="previewURL"
                type="application/pdf"
                class="w-full h-[60vh] rounded"
              >
                <p class="text-sm text-ink-700 p-4">Browser tidak bisa preview PDF ini. Silakan download.</p>
              </object>
              <div v-else class="text-center text-sm text-ink-700 py-12">
                Format tidak bisa preview inline ({{ previewProof.file_mime_type }}).
              </div>

              <div class="mt-3 text-center">
                <a
                  :href="pay.proofFileURL(previewProof.id, { download: true })"
                  target="_blank"
                  rel="noopener"
                  class="text-xs text-ink-500 hover:text-ink-900 transition-colors"
                >
                  Download original ↓
                </a>
              </div>
            </div>

            <div class="border-t border-hairline p-4 flex flex-col sm:flex-row justify-end gap-2">
              <button
                type="button"
                class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-50"
                :disabled="busy"
                @click="closePreview"
              >
                Tutup
              </button>
              <button
                v-if="canReject && previewProof.status === 'pending'"
                type="button"
                class="rounded-md border border-brand-200 bg-canvas px-3 py-1.5 text-sm font-semibold text-brand-700 hover:bg-brand-50 hover:border-brand-300 transition-colors disabled:opacity-60"
                :disabled="busy"
                @click="askReject"
              >
                Tolak
              </button>
              <button
                v-if="canApprove && previewProof.status === 'pending'"
                type="button"
                class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-1.5 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-60"
                :disabled="busy"
                @click="onApprove"
              >
                <span
                  v-if="busy"
                  class="inline-block h-3 w-3 rounded-full border-2 border-canvas/70 border-t-transparent animate-spin"
                  aria-hidden="true"
                />
                Approve
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- Reject reason dialog -->
    <Teleport to="body">
      <Transition name="fade">
        <div v-if="rejectOpen" class="fixed inset-0 z-[60] flex items-center justify-center p-4">
          <div class="absolute inset-0 bg-ink-950/60" aria-hidden="true" @click="!busy && (rejectOpen = false)" />
          <div role="dialog" aria-modal="true" class="relative w-full max-w-md rounded-lg bg-canvas shadow-xl ring-1 ring-black/5 p-6">
            <h3 class="font-serif text-lg font-semibold text-ink-950">Alasan penolakan</h3>
            <p class="mt-1 text-xs text-ink-500 leading-relaxed">
              Alasan ini dikirim ke customer via WA supaya mereka tahu apa yang perlu diperbaiki.
            </p>
            <textarea
              v-model="rejectReason"
              rows="3"
              maxlength="500"
              placeholder="Contoh: Nominal transfer tidak sesuai; bukti tidak menampilkan tanggal…"
              class="mt-3 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
            />
            <p class="mt-1 text-xs text-ink-400 text-right">{{ rejectReason.length }} / 500</p>
            <div class="mt-4 flex justify-end gap-2">
              <button
                type="button"
                class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-50"
                :disabled="busy"
                @click="rejectOpen = false"
              >
                Batal
              </button>
              <button
                type="button"
                class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-1.5 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-60"
                :disabled="busy || rejectReason.trim().length < 3"
                @click="confirmReject"
              >
                <span
                  v-if="busy"
                  class="inline-block h-3 w-3 rounded-full border-2 border-canvas/70 border-t-transparent animate-spin"
                  aria-hidden="true"
                />
                Kirim Reject
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </section>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.15s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
