<script setup lang="ts">
/**
 * /admin/diskon — Master diskon (CLAUDE.md §28, permission `discount.manage`).
 *
 * Layout: filter bar (status + pencarian) + tabel + modal create/edit +
 * modal hapus (destructive, alasan wajib). Pola sama dengan
 * `/admin/order` (AdminDataTable + AdminPagination) — bukan tabel custom
 * seperti `/admin/katalog`, karena halaman ini butuh server-side pagination.
 */
import {
  Plus,
  Pencil,
  Trash2,
  Loader2,
  TicketPercent,
} from '@lucide/vue'
import type { DataTableColumn } from '~/components/admin/DataTable.vue'
import { ApiError } from '~/composables/useApi'
import type { Discount, DiscountAppliesTo, DiscountInput, DiscountStatus, DiscountType } from '~/types/discount'
import type { AdminProduct } from '~/types/catalog-admin'

definePageMeta({
  middleware: ['staff-only'],
  layout: 'admin',
})

useSeoMeta({ title: 'Diskon — Rajaku Admin' })

const discountSvc = useDiscount()
const catalogSvc = useAdminCatalog()

// -------------------- produk (untuk cakupan §28.9) --------------------
const products = ref<AdminProduct[]>([])
const productsLoading = ref(false)
async function fetchProducts() {
  productsLoading.value = true
  try {
    const res = await catalogSvc.listProducts()
    products.value = res.products
  } catch {
    // Non-blocking — kalau gagal, pemilih produk tampil kosong dan admin
    // masih bisa memakai "Semua produk"; daftar diskon utama tetap jalan.
  } finally {
    productsLoading.value = false
  }
}
/** Nama produk untuk ringkasan cakupan di tabel — fallback ke id kalau produk sudah tidak ada di katalog yang termuat. */
function productName(id: string): string {
  return products.value.find((p) => p.id === id)?.name ?? id
}

// -------------------- list state --------------------
const items = ref<Discount[]>([])
const total = ref(0)
const loading = ref(false)
const errorMsg = ref<string | null>(null)
const successMsg = ref<string | null>(null)

const statusFilter = ref<DiscountStatus | ''>('')
const search = ref('')
const page = ref(1)
const pageSize = 10

const statusTabs: Array<{ v: DiscountStatus | ''; label: string }> = [
  { v: '', label: 'Semua' },
  { v: 'aktif', label: 'Aktif' },
  { v: 'terjadwal', label: 'Terjadwal' },
  { v: 'kuota_habis', label: 'Kuota habis' },
  { v: 'kadaluarsa', label: 'Kadaluarsa' },
  { v: 'nonaktif', label: 'Nonaktif' },
]

function showSuccess(msg: string) {
  successMsg.value = msg
  setTimeout(() => (successMsg.value = null), 2500)
}
function toApiError(e: unknown, fallback: string): string {
  return e instanceof ApiError ? e.message : fallback
}

/** Error milik modal terbuka — banner halaman ada di belakang overlay modal. */
const modalError = ref<string | null>(null)
function toModalError(e: unknown, fallback: string) {
  modalError.value = toApiError(e, fallback)
}

async function fetchList() {
  loading.value = true
  errorMsg.value = null
  try {
    const res = await discountSvc.list({
      status: statusFilter.value || undefined,
      q: search.value.trim() || undefined,
      page: page.value,
      per_page: pageSize,
    })
    items.value = res.items
    total.value = res.total
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal memuat diskon')
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchList()
  fetchProducts()
})
watch([statusFilter, page], fetchList)

let searchTimer: ReturnType<typeof setTimeout> | null = null
watch(search, () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    page.value = 1
    fetchList()
  }, 300)
})

const columns: DataTableColumn[] = [
  { key: 'code', label: 'Kode / Nama' },
  { key: 'value', label: 'Nilai', class: 'w-40' },
  { key: 'min_subtotal', label: 'Min. subtotal', class: 'w-32 text-right hidden md:table-cell' },
  { key: 'period', label: 'Periode', class: 'w-44 hidden lg:table-cell' },
  { key: 'usage', label: 'Pemakaian', class: 'w-28 hidden md:table-cell' },
  { key: 'channel_scope', label: 'Channel', class: 'w-24 hidden lg:table-cell' },
  { key: 'scope', label: 'Cakupan', class: 'w-32 hidden md:table-cell' },
  { key: 'status', label: 'Status', class: 'w-32' },
  { key: 'actions', label: '', class: 'w-24 text-right' },
]

// -------------------- form modal (create + edit) --------------------
const formOpen = ref(false)
const editingId = ref<string | null>(null)
const saving = ref(false)

const form = reactive<DiscountInput>({
  code: '',
  name: '',
  type: 'percent',
  value_percent: null,
  value_amount: null,
  max_discount_amount: null,
  min_subtotal: 0,
  starts_at: null,
  ends_at: null,
  quota: null,
  channel_scope: 'all',
  is_active: true,
  applies_to: 'all',
  product_ids: [],
})

/** Kode diskon: huruf besar, angka, dan `-`/`_` saja — dirapikan sambil mengetik. */
function normalizeCode(raw: string): string {
  return raw
    .toUpperCase()
    .replace(/\s+/g, '')
    .replace(/[^A-Z0-9_-]/g, '')
    .slice(0, 30)
}

/** Backend ISO datetime <-> input `datetime-local` (tanpa timezone offset). */
function isoToLocalInput(iso: string | null): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}
function localInputToIso(v: string): string | null {
  if (!v) return null
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return null
  return d.toISOString()
}

const startsAtLocal = ref('')
const endsAtLocal = ref('')

function openCreate() {
  errorMsg.value = null
  modalError.value = null
  editingId.value = null
  form.code = ''
  form.name = ''
  form.type = 'percent'
  form.value_percent = null
  form.value_amount = null
  form.max_discount_amount = null
  form.min_subtotal = 0
  form.starts_at = null
  form.ends_at = null
  form.quota = null
  form.channel_scope = 'all'
  form.is_active = true
  form.applies_to = 'all'
  form.product_ids = []
  startsAtLocal.value = ''
  endsAtLocal.value = ''
  formOpen.value = true
}

function openEdit(d: Discount) {
  errorMsg.value = null
  modalError.value = null
  editingId.value = d.id
  form.code = d.code
  form.name = d.name
  form.type = d.type
  form.value_percent = d.value_percent
  form.value_amount = d.value_amount
  form.max_discount_amount = d.max_discount_amount
  form.min_subtotal = d.min_subtotal
  form.starts_at = d.starts_at
  form.ends_at = d.ends_at
  form.quota = d.quota
  form.channel_scope = d.channel_scope
  form.is_active = d.is_active
  // Backend lama (belum di-deploy paralel) mungkin belum mengirim dua field
  // ini — jatuh ke default "semua produk" supaya form tidak error, bukan
  // diam-diam menganggap daftar kosong = tercakup semua (§28.9).
  form.applies_to = d.applies_to ?? 'all'
  form.product_ids = d.product_ids ? [...d.product_ids] : []
  startsAtLocal.value = isoToLocalInput(d.starts_at)
  endsAtLocal.value = isoToLocalInput(d.ends_at)
  formOpen.value = true
}

/** Ganti "Semua produk" → bersihkan daftar tercentang lama (jangan ikut kirim sisa pilihan §-brief). */
function onAppliesToChange(v: DiscountAppliesTo) {
  form.applies_to = v
  if (v === 'all') form.product_ids = []
}

function onTypeChange(t: DiscountType) {
  form.type = t
  if (t === 'percent') {
    form.value_amount = null
  } else {
    form.value_percent = null
    form.max_discount_amount = null
  }
}

async function saveDiscount() {
  modalError.value = null
  // Validasi wajib di UI (§28.9 brief): "Produk tertentu" tanpa satu pun
  // produk tercentang HARUS ditolak sebelum request dikirim — daftar kosong
  // bukan berarti "berlaku semua", itu diskon yang mustahil dipakai.
  if (form.applies_to === 'selected' && form.product_ids.length === 0) {
    modalError.value = 'Pilih minimal satu produk, atau ubah cakupan ke "Semua produk".'
    return
  }
  saving.value = true
  try {
    const body: DiscountInput = {
      code: form.code.trim(),
      name: form.name.trim(),
      type: form.type,
      // `value_percent`/`value_amount` HARUS diomit (bukan `null`) kalau tidak
      // relevan — beda dengan max_discount_amount/starts_at/ends_at/quota di
      // bawah, backend PATCH (`parseUpdateInput`) menolak keras `null` untuk
      // dua field ini (cuma menerima "absen" atau "ada nilai"), sementara
      // empat field lain justru punya dukungan clear-via-null. `undefined` di
      // sini membuat key-nya tidak ikut ter-serialize di body JSON.
      value_percent: form.type === 'percent' ? (form.value_percent ?? undefined) : undefined,
      value_amount: form.type === 'nominal' ? (form.value_amount ?? undefined) : undefined,
      max_discount_amount: form.type === 'percent' ? (form.max_discount_amount || null) : null,
      min_subtotal: form.min_subtotal || 0,
      starts_at: localInputToIso(startsAtLocal.value),
      ends_at: localInputToIso(endsAtLocal.value),
      quota: form.quota || null,
      channel_scope: form.channel_scope,
      is_active: form.is_active,
      applies_to: form.applies_to,
      product_ids: form.applies_to === 'selected' ? [...form.product_ids] : [],
    }
    if (editingId.value) {
      await discountSvc.update(editingId.value, body)
      showSuccess('Diskon diperbarui.')
    } else {
      await discountSvc.create(body)
      showSuccess('Diskon baru dibuat.')
    }
    formOpen.value = false
    await fetchList()
  } catch (e) {
    toModalError(e, 'Gagal menyimpan diskon')
  } finally {
    saving.value = false
  }
}

// -------------------- delete modal --------------------
const deleteTarget = ref<Discount | null>(null)
const deleteReason = ref('')
const deleteBusy = ref(false)
const deleteOpen = computed({
  get: () => deleteTarget.value !== null,
  set: (v: boolean) => {
    if (!v) deleteTarget.value = null
  },
})
const deleteCanSubmit = computed(() => deleteReason.value.trim().length >= 3)

function openDelete(d: Discount) {
  deleteTarget.value = d
  deleteReason.value = ''
  modalError.value = null
}

async function confirmDelete() {
  if (!deleteTarget.value) return
  if (!deleteCanSubmit.value) {
    modalError.value = 'Alasan hapus minimal 3 karakter.'
    return
  }
  deleteBusy.value = true
  modalError.value = null
  try {
    await discountSvc.remove(deleteTarget.value.id, deleteReason.value.trim())
    showSuccess('Diskon dihapus.')
    deleteTarget.value = null
    await fetchList()
  } catch (e) {
    toModalError(e, 'Gagal menghapus diskon')
  } finally {
    deleteBusy.value = false
  }
}

// -------------------- helpers --------------------
function fmtIDR(v?: number | null): string {
  if (v == null) return '—'
  return new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 }).format(v)
}
function fmtDate(s: string | null): string {
  if (!s) return '—'
  try {
    return new Date(s).toLocaleString('id-ID', { day: '2-digit', month: 'short', year: '2-digit', hour: '2-digit', minute: '2-digit' })
  } catch {
    return s
  }
}
function valueLabel(d: Discount): string {
  if (d.type === 'percent') {
    const base = `${d.value_percent ?? 0}%`
    return d.max_discount_amount ? `${base} (maks ${fmtIDR(d.max_discount_amount)})` : base
  }
  return fmtIDR(d.value_amount)
}
function periodLabel(d: Discount): string {
  if (!d.starts_at && !d.ends_at) return 'Tanpa batas waktu'
  return `${fmtDate(d.starts_at)} – ${d.ends_at ? fmtDate(d.ends_at) : '∞'}`
}
function usageLabel(d: Discount): string {
  return d.quota != null ? `${d.usage_count} / ${d.quota}` : String(d.usage_count)
}
/** Ringkasan cakupan produk (§28.9) untuk kolom tabel — supaya admin bisa membedakan sekilas tanpa membuka form. */
function scopeLabel(d: Discount): string {
  if (d.applies_to !== 'selected') return 'Semua produk'
  const n = d.product_ids?.length ?? 0
  return n === 1 ? '1 produk' : `${n} produk`
}
function scopeTooltip(d: Discount): string | undefined {
  if (d.applies_to !== 'selected' || !d.product_ids?.length) return undefined
  return d.product_ids.map((id) => productName(id)).join(', ')
}
function statusLabel(s: DiscountStatus | string): string {
  const map: Record<string, string> = {
    aktif: 'Aktif',
    terjadwal: 'Terjadwal',
    kadaluarsa: 'Kadaluarsa',
    nonaktif: 'Nonaktif',
    kuota_habis: 'Kuota habis',
  }
  return map[s] ?? s
}
/**
 * Tone eksplisit per status diskon — TIDAK mengandalkan pencocokan kata
 * kunci bawaan `<AdminStatusBadge>` (yang dirancang untuk status order/
 * artikel), karena tidak ada satu pun kata kunci di sana yang cocok dengan
 * kosakata diskon ('aktif', 'terjadwal', dst).
 */
function statusTone(s: DiscountStatus | string): 'green' | 'amber' | 'ink' | 'rose' {
  if (s === 'aktif') return 'green'
  if (s === 'terjadwal') return 'amber'
  if (s === 'kuota_habis') return 'rose'
  return 'ink'
}
</script>

<template>
  <section>
    <AdminPageHeader
      title="Diskon"
      subtitle="Kelola kode promo & potongan harga untuk order online dan POS."
    >
      <template #actions>
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas-alt transition-colors"
          @click="openCreate"
        >
          <Plus class="h-4 w-4" :stroke-width="1.75" />
          Diskon baru
        </button>
      </template>
    </AdminPageHeader>

    <AlertMessage v-if="errorMsg" variant="error" :message="errorMsg" class="mb-4" />
    <AlertMessage v-if="successMsg" variant="success" :message="successMsg" class="mb-4" />

    <!-- Filter bar -->
    <div class="mb-5 flex flex-col sm:flex-row sm:flex-wrap sm:items-center gap-3">
      <div class="flex flex-wrap rounded-md border border-hairline bg-canvas overflow-hidden text-sm">
        <button
          v-for="opt in statusTabs"
          :key="opt.v"
          type="button"
          :class="[
            'px-3 py-1.5 border-r border-hairline last:border-r-0 transition-colors',
            statusFilter === opt.v ? 'bg-canvas-alt font-medium text-ink-950' : 'text-ink-600 hover:bg-canvas-alt/60',
          ]"
          @click="statusFilter = opt.v; page = 1"
        >
          {{ opt.label }}
        </button>
      </div>

      <input
        v-model="search"
        type="search"
        placeholder="Cari kode / nama…"
        class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
      >
    </div>

    <AdminDataTable
      :columns="columns"
      :items="items"
      :loading="loading"
      row-key="id"
      empty-message="Belum ada diskon pada filter ini."
    >
      <template #cell-code="{ row }">
        <p class="font-mono text-xs text-ink-700">{{ (row as Discount).code }}</p>
        <p class="mt-0.5 text-sm text-ink-900">{{ (row as Discount).name }}</p>
      </template>
      <template #cell-value="{ row }">
        <span class="text-sm text-ink-900">{{ valueLabel(row as Discount) }}</span>
      </template>
      <template #cell-min_subtotal="{ row }">
        <span class="text-xs text-ink-500">{{ (row as Discount).min_subtotal > 0 ? fmtIDR((row as Discount).min_subtotal) : '—' }}</span>
      </template>
      <template #cell-period="{ row }">
        <span class="text-xs text-ink-500">{{ periodLabel(row as Discount) }}</span>
      </template>
      <template #cell-usage="{ row }">
        <span class="font-mono text-xs text-ink-700">{{ usageLabel(row as Discount) }}</span>
      </template>
      <template #cell-channel_scope="{ row }">
        <span class="text-xs uppercase text-ink-600">{{ (row as Discount).channel_scope }}</span>
      </template>
      <template #cell-scope="{ row }">
        <span class="text-xs text-ink-600" :title="scopeTooltip(row as Discount)">{{ scopeLabel(row as Discount) }}</span>
      </template>
      <template #cell-status="{ row }">
        <AdminStatusBadge :status="statusLabel((row as Discount).status)" :tone="statusTone((row as Discount).status)" />
      </template>
      <template #cell-actions="{ row }">
        <div class="flex justify-end gap-1">
          <button
            type="button"
            class="inline-flex items-center rounded-md border border-hairline bg-canvas p-1.5 text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
            :aria-label="'Edit ' + (row as Discount).name"
            @click="openEdit(row as Discount)"
          >
            <Pencil class="h-3.5 w-3.5" :stroke-width="1.75" />
          </button>
          <button
            type="button"
            class="inline-flex items-center rounded-md border border-brand-200 bg-canvas p-1.5 text-brand-700 hover:bg-brand-50 hover:border-brand-300 transition-colors"
            :aria-label="'Hapus ' + (row as Discount).name"
            @click="openDelete(row as Discount)"
          >
            <Trash2 class="h-3.5 w-3.5" :stroke-width="1.75" />
          </button>
        </div>
      </template>
    </AdminDataTable>

    <AdminPagination
      v-if="total > 0"
      :page="page"
      :limit="pageSize"
      :total="total"
      @update:page="(v) => (page = v)"
    />

    <!-- ============================ Form modal ============================ -->
    <Teleport to="body">
      <Transition name="fade">
        <div v-if="formOpen" class="fixed inset-0 z-50 flex items-center justify-center p-4">
          <div class="absolute inset-0 bg-ink-950/60" aria-hidden="true" @click="!saving && (formOpen = false)" />
          <div role="dialog" aria-modal="true" class="relative w-full max-w-xl rounded-lg bg-canvas shadow-xl ring-1 ring-black/5 max-h-[90vh] overflow-y-auto">
            <form class="p-6" @submit.prevent="saveDiscount">
              <div class="flex items-center gap-2">
                <TicketPercent class="h-5 w-5 text-brand-500" :stroke-width="1.5" />
                <h3 class="font-serif text-lg font-semibold text-ink-950">
                  {{ editingId ? 'Edit diskon' : 'Diskon baru' }}
                </h3>
              </div>

              <AlertMessage v-if="modalError" variant="error" :message="modalError" class="mt-4" />

              <div class="mt-4 grid gap-4 sm:grid-cols-2">
                <div>
                  <label for="disc-code" class="block text-sm font-medium text-ink-900">Kode <span class="text-brand-500">*</span></label>
                  <input
                    id="disc-code"
                    v-model="form.code"
                    type="text"
                    required
                    maxlength="30"
                    placeholder="LEBARAN25"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm font-mono placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                    @input="form.code = normalizeCode(form.code)"
                  >
                  <p class="mt-1 text-xs text-ink-500">Huruf besar otomatis. Dipakai pelanggan saat checkout.</p>
                </div>
                <div>
                  <label for="disc-name" class="block text-sm font-medium text-ink-900">Nama <span class="text-brand-500">*</span></label>
                  <input
                    id="disc-name"
                    v-model="form.name"
                    type="text"
                    required
                    maxlength="150"
                    placeholder="Promo Lebaran"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  >
                </div>

                <div class="sm:col-span-2">
                  <label class="block text-sm font-medium text-ink-900">Tipe <span class="text-brand-500">*</span></label>
                  <div class="mt-1 flex gap-2">
                    <label
                      :class="[
                        'flex-1 flex cursor-pointer items-center gap-2 rounded-md border px-3 py-2 text-sm transition-colors',
                        form.type === 'percent' ? 'border-brand-500 bg-brand-50/50 text-ink-950' : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300',
                      ]"
                    >
                      <input type="radio" value="percent" :checked="form.type === 'percent'" class="accent-brand-500" @change="onTypeChange('percent')">
                      <span class="font-semibold">Persen (%)</span>
                    </label>
                    <label
                      :class="[
                        'flex-1 flex cursor-pointer items-center gap-2 rounded-md border px-3 py-2 text-sm transition-colors',
                        form.type === 'nominal' ? 'border-brand-500 bg-brand-50/50 text-ink-950' : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300',
                      ]"
                    >
                      <input type="radio" value="nominal" :checked="form.type === 'nominal'" class="accent-brand-500" @change="onTypeChange('nominal')">
                      <span class="font-semibold">Nominal (Rp)</span>
                    </label>
                  </div>
                </div>

                <template v-if="form.type === 'percent'">
                  <div>
                    <label for="disc-percent" class="block text-sm font-medium text-ink-900">Nilai (%) <span class="text-brand-500">*</span></label>
                    <input
                      id="disc-percent"
                      v-model.number="form.value_percent"
                      type="number"
                      min="1"
                      max="100"
                      step="0.01"
                      required
                      placeholder="10"
                      class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                    >
                  </div>
                  <div>
                    <label for="disc-max" class="block text-sm font-medium text-ink-900">Maks. potongan (Rp)</label>
                    <input
                      id="disc-max"
                      v-model.number="form.max_discount_amount"
                      type="number"
                      min="0"
                      placeholder="Kosongkan = tidak dibatasi"
                      class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                    >
                  </div>
                </template>
                <template v-else>
                  <div class="sm:col-span-2">
                    <label for="disc-amount" class="block text-sm font-medium text-ink-900">Nilai potongan (Rp) <span class="text-brand-500">*</span></label>
                    <input
                      id="disc-amount"
                      v-model.number="form.value_amount"
                      type="number"
                      min="1"
                      required
                      placeholder="20000"
                      class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                    >
                  </div>
                </template>

                <div>
                  <label for="disc-min-subtotal" class="block text-sm font-medium text-ink-900">Min. subtotal (Rp)</label>
                  <input
                    id="disc-min-subtotal"
                    v-model.number="form.min_subtotal"
                    type="number"
                    min="0"
                    placeholder="0"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  >
                  <p class="mt-1 text-xs text-ink-500">0 = berlaku untuk subtotal berapa pun.</p>
                </div>
                <div>
                  <label for="disc-quota" class="block text-sm font-medium text-ink-900">Kuota pemakaian</label>
                  <input
                    id="disc-quota"
                    v-model.number="form.quota"
                    type="number"
                    min="1"
                    placeholder="Kosongkan = tak terbatas"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  >
                </div>

                <div>
                  <label for="disc-starts" class="block text-sm font-medium text-ink-900">Mulai berlaku</label>
                  <input
                    id="disc-starts"
                    v-model="startsAtLocal"
                    type="datetime-local"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  >
                  <p class="mt-1 text-xs text-ink-500">Kosongkan = langsung berlaku sejak disimpan.</p>
                </div>
                <div>
                  <label for="disc-ends" class="block text-sm font-medium text-ink-900">Berakhir</label>
                  <input
                    id="disc-ends"
                    v-model="endsAtLocal"
                    type="datetime-local"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  >
                  <p class="mt-1 text-xs text-ink-500">Kosongkan = tanpa batas waktu.</p>
                </div>

                <div>
                  <label for="disc-channel" class="block text-sm font-medium text-ink-900">Channel</label>
                  <select
                    id="disc-channel"
                    v-model="form.channel_scope"
                    class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
                  >
                    <option value="all">Semua channel</option>
                    <option value="online">Online saja</option>
                    <option value="pos">POS saja</option>
                  </select>
                </div>
                <div class="flex items-end pb-2">
                  <label class="inline-flex cursor-pointer items-center gap-2 text-sm font-medium text-ink-900">
                    <input v-model="form.is_active" type="checkbox" class="h-4 w-4 rounded border-hairline accent-brand-500">
                    Aktifkan diskon ini
                  </label>
                </div>

                <div class="sm:col-span-2">
                  <label class="block text-sm font-medium text-ink-900">Berlaku untuk</label>
                  <div class="mt-1 flex gap-2">
                    <label
                      :class="[
                        'flex-1 flex cursor-pointer items-center gap-2 rounded-md border px-3 py-2 text-sm transition-colors',
                        form.applies_to === 'all' ? 'border-brand-500 bg-brand-50/50 text-ink-950' : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300',
                      ]"
                    >
                      <input type="radio" value="all" :checked="form.applies_to === 'all'" class="accent-brand-500" @change="onAppliesToChange('all')">
                      <span class="font-semibold">Semua produk</span>
                    </label>
                    <label
                      :class="[
                        'flex-1 flex cursor-pointer items-center gap-2 rounded-md border px-3 py-2 text-sm transition-colors',
                        form.applies_to === 'selected' ? 'border-brand-500 bg-brand-50/50 text-ink-950' : 'border-hairline bg-canvas text-ink-700 hover:border-ink-300',
                      ]"
                    >
                      <input type="radio" value="selected" :checked="form.applies_to === 'selected'" class="accent-brand-500" @change="onAppliesToChange('selected')">
                      <span class="font-semibold">Produk tertentu</span>
                    </label>
                  </div>

                  <div v-if="form.applies_to === 'selected'" class="mt-2">
                    <AdminProductMultiSelect
                      v-model="form.product_ids"
                      :products="products"
                      :loading="productsLoading"
                    />
                    <p class="mt-1 text-xs text-ink-500">
                      Diskon ini hanya bisa dipakai kalau order-nya untuk salah satu produk tercentang. Kosongkan
                      pilihan bukan cara untuk "berlaku semua" — daftar kosong membuat diskon tidak bisa dipakai
                      sama sekali.
                    </p>
                  </div>
                </div>
              </div>

              <div class="mt-6 flex justify-end gap-2">
                <button
                  type="button"
                  class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-50"
                  :disabled="saving"
                  @click="formOpen = false"
                >
                  Batal
                </button>
                <button
                  type="submit"
                  :disabled="saving"
                  class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-1.5 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-60"
                >
                  <Loader2 v-if="saving" class="h-4 w-4 animate-spin" :stroke-width="1.75" />
                  Simpan
                </button>
              </div>
            </form>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- ============================ Delete modal ============================ -->
    <AdminConfirmDialog
      v-model:open="deleteOpen"
      title="Hapus diskon ini?"
      variant="danger"
      confirm-label="Hapus diskon"
      :loading="deleteBusy"
      :error="modalError"
      @confirm="confirmDelete"
    >
      <template v-if="deleteTarget">
        <p class="mt-2 text-sm text-ink-500 leading-relaxed">
          <strong class="font-mono text-ink-900">{{ deleteTarget.code }}</strong> — {{ deleteTarget.name }} akan
          hilang dari daftar diskon aktif dan tidak bisa dipakai pelanggan/kasir lagi.
        </p>
        <p class="mt-2 rounded-md border border-gold-200 bg-gold-50 p-3 text-xs leading-relaxed text-gold-900">
          Order yang sudah memakai diskon ini <strong>tidak berubah</strong> — nama & nominal potongannya tetap
          tercatat apa adanya di rekap order (§28.2), jadi menghapus diskon aman untuk kebutuhan pembukuan.
        </p>
        <div class="mt-3">
          <label for="disc-delete-reason" class="block text-sm font-medium text-ink-900">Alasan hapus <span class="text-brand-500">*</span></label>
          <textarea
            id="disc-delete-reason"
            v-model="deleteReason"
            rows="2"
            required
            placeholder="Contoh: promo sudah tidak berlaku, dibuat karena salah input"
            class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
          />
        </div>
      </template>
    </AdminConfirmDialog>
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
