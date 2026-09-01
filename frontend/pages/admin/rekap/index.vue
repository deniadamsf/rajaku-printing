<script setup lang="ts">
/**
 * /admin/rekap — Rekap order & diskon (CLAUDE.md §28, permission `report.view`).
 *
 * Panel filter (rentang tanggal wajib + channel/status/kasir/diskon) → kartu
 * ringkasan → tabel per-order (server-side pagination, pola sama dengan
 * `/admin/order`) → unduh CSV dengan filter yang sedang aktif.
 */
import {
  CalendarRange,
  Download,
  Loader2,
  ClipboardList,
} from '@lucide/vue'
import type { DataTableColumn } from '~/components/admin/DataTable.vue'
import { ApiError } from '~/composables/useApi'
import type { OrderRecapFilter, OrderRecapFilterOptions, OrderRecapItem, OrderRecapSummary } from '~/types/order-recap'
import type { OrderChannel, OrderStatus } from '~/types/order'

definePageMeta({
  middleware: ['staff-only'],
  layout: 'admin',
})

useSeoMeta({ title: 'Rekap Order — Rajaku Admin' })

const recapSvc = useOrderRecap()

// -------------------- filter state --------------------
function isoDate(d: Date): string {
  return d.toISOString().slice(0, 10)
}
const today = new Date()
const thirtyDaysAgo = new Date(today)
thirtyDaysAgo.setDate(thirtyDaysAgo.getDate() - 30)

const dateFrom = ref(isoDate(thirtyDaysAgo))
const dateTo = ref(isoDate(today))
const channel = ref<OrderChannel | ''>('')
const status = ref<OrderStatus | ''>('')
const createdBy = ref('')
/** '' = semua diskon, `MANUAL_DISCOUNT_SENTINEL` = diskon manual, selain itu = UUID diskon. */
const discountSelection = ref('')
const onlyDiscounted = ref(false)

const MAX_RANGE_DAYS = 366
/**
 * Nilai sentinel untuk opsi "Diskon manual" (entri `{ id: null, label: … }`
 * dari `/order-recap/filters`) — dipilih karena tidak akan pernah bentrok
 * dengan UUID diskon asli. Dipetakan ke param query terpisah
 * `discount_manual=true` (BUKAN `discount_id`), lihat `types/order-recap.ts`.
 */
const MANUAL_DISCOUNT_SENTINEL = '__manual__'

const page = ref(1)
const pageSize = 50

const statusOptions: Array<{ v: OrderStatus | ''; label: string }> = [
  { v: '', label: 'Semua status' },
  { v: 'order_masuk', label: 'Order masuk' },
  { v: 'menunggu_ongkir', label: 'Menunggu ongkir' },
  { v: 'menunggu_pembayaran', label: 'Menunggu pembayaran' },
  { v: 'menunggu_verifikasi', label: 'Menunggu verifikasi' },
  { v: 'dibayar', label: 'Dibayar' },
  { v: 'proses_cetak', label: 'Proses cetak' },
  { v: 'siap_kirim', label: 'Siap kirim' },
  { v: 'siap_ambil', label: 'Siap ambil' },
  { v: 'dikirim', label: 'Dikirim' },
  { v: 'selesai', label: 'Selesai' },
  { v: 'dibatalkan', label: 'Dibatalkan' },
]

// -------------------- data state --------------------
const summary = ref<OrderRecapSummary | null>(null)
const items = ref<OrderRecapItem[]>([])
const total = ref(0)
const loading = ref(false)
const errorMsg = ref<string | null>(null)

const exporting = ref(false)
const exportError = ref<string | null>(null)

function toApiError(e: unknown, fallback: string): string {
  return e instanceof ApiError ? e.message : fallback
}

// -------------------- opsi dropdown kasir & diskon --------------------
// Diisi dari `/admin/order-recap/filters?from=&to=` — nilai distinct yang
// benar-benar muncul pada rentang tanggal aktif, jadi dimuat ulang tiap
// `dateFrom`/`dateTo` berubah (lihat watcher `scheduleDateChange` di bawah).
const filterOptions = ref<OrderRecapFilterOptions | null>(null)
const filterOptionsLoading = ref(false)
const filterOptionsError = ref<string | null>(null)

async function loadFilterOptions() {
  if (!dateFrom.value || !dateTo.value || dateRangeError.value) {
    filterOptions.value = null
    return
  }
  filterOptionsLoading.value = true
  filterOptionsError.value = null
  try {
    const res = await recapSvc.filters({ from: dateFrom.value, to: dateTo.value })
    filterOptions.value = res
    // Rentang tanggal baru bisa jadi tidak lagi memuat kasir/diskon yang
    // sebelumnya dipilih — kosongkan supaya tidak menyaring dengan nilai
    // yang sudah tak terlihat di dropdown (hasil kosong tanpa penjelasan).
    if (createdBy.value && !res.kasir.some((k) => k.id === createdBy.value)) {
      createdBy.value = ''
    }
    if (discountSelection.value === MANUAL_DISCOUNT_SENTINEL) {
      if (!res.diskon.some((d) => d.id === null)) discountSelection.value = ''
    } else if (discountSelection.value && !res.diskon.some((d) => d.id === discountSelection.value)) {
      discountSelection.value = ''
    }
  } catch (e) {
    filterOptionsError.value = toApiError(e, 'Gagal memuat pilihan kasir/diskon')
    filterOptions.value = null
  } finally {
    filterOptionsLoading.value = false
  }
}

const kasirOptions = computed(() => filterOptions.value?.kasir ?? [])
const diskonOptions = computed(() => filterOptions.value?.diskon ?? [])
function diskonOptionValue(id: string | null): string {
  return id === null ? MANUAL_DISCOUNT_SENTINEL : id
}

/** Cegah request rentang > 366 hari dikirim ke backend sama sekali (backend membalas 400). */
const dateRangeError = computed<string | null>(() => {
  if (!dateFrom.value || !dateTo.value) return null
  const from = new Date(`${dateFrom.value}T00:00:00`)
  const to = new Date(`${dateTo.value}T00:00:00`)
  if (Number.isNaN(from.getTime()) || Number.isNaN(to.getTime())) return null
  const diffDays = Math.round((to.getTime() - from.getTime()) / 86_400_000)
  if (diffDays < 0) return 'Tanggal "sampai" tidak boleh sebelum tanggal "dari".'
  if (diffDays > MAX_RANGE_DAYS) {
    return `Rentang tanggal maksimal ${MAX_RANGE_DAYS} hari (saat ini ${diffDays} hari). Perpendek rentang lalu coba lagi.`
  }
  return null
})

function currentFilter(): OrderRecapFilter {
  return {
    from: dateFrom.value,
    to: dateTo.value,
    channel: channel.value || undefined,
    status: status.value || undefined,
    created_by: createdBy.value || undefined,
    discount_manual: discountSelection.value === MANUAL_DISCOUNT_SENTINEL || undefined,
    discount_id:
      discountSelection.value && discountSelection.value !== MANUAL_DISCOUNT_SENTINEL
        ? discountSelection.value
        : undefined,
    only_discounted: onlyDiscounted.value || undefined,
    page: page.value,
    per_page: pageSize,
  }
}

async function fetchRecap() {
  if (!dateFrom.value || !dateTo.value) return
  if (dateRangeError.value) {
    errorMsg.value = dateRangeError.value
    summary.value = null
    items.value = []
    total.value = 0
    return
  }
  loading.value = true
  errorMsg.value = null
  try {
    const res = await recapSvc.list(currentFilter())
    summary.value = res.summary
    items.value = res.items
    total.value = res.total
  } catch (e) {
    errorMsg.value = toApiError(e, 'Gagal memuat rekap order')
    summary.value = null
    items.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadFilterOptions()
  fetchRecap()
})
watch([channel, status, page], fetchRecap)

let filterTimer: ReturnType<typeof setTimeout> | null = null
function scheduleRefetch() {
  page.value = 1
  if (filterTimer) clearTimeout(filterTimer)
  filterTimer = setTimeout(fetchRecap, 350)
}
watch([createdBy, discountSelection, onlyDiscounted], scheduleRefetch)

// `from`/`to` berubah → opsi dropdown kasir/diskon ikut dimuat ulang
// (dilempar SETELAH debounce yang sama supaya tidak spam request tiap
// keystroke saat user mengetik tanggal manual).
let dateTimer: ReturnType<typeof setTimeout> | null = null
function scheduleDateChange() {
  page.value = 1
  if (dateTimer) clearTimeout(dateTimer)
  dateTimer = setTimeout(() => {
    loadFilterOptions()
    fetchRecap()
  }, 350)
}
watch([dateFrom, dateTo], scheduleDateChange)

async function downloadCsv() {
  if (dateRangeError.value) {
    exportError.value = dateRangeError.value
    return
  }
  exporting.value = true
  exportError.value = null
  try {
    await recapSvc.exportCsv(currentFilter())
  } catch (e) {
    exportError.value = e instanceof Error ? e.message : 'Gagal mengunduh CSV'
  } finally {
    exporting.value = false
  }
}

const columns: DataTableColumn[] = [
  { key: 'created_at', label: 'Tanggal', class: 'w-32' },
  { key: 'resi', label: 'Resi', class: 'w-36' },
  { key: 'customer_name', label: 'Pelanggan' },
  { key: 'product_name', label: 'Produk' },
  { key: 'jumlah_item', label: 'Item', class: 'w-16 text-right hidden sm:table-cell' },
  { key: 'channel', label: 'Channel', class: 'w-20 hidden md:table-cell' },
  { key: 'status', label: 'Status', class: 'w-36 hidden lg:table-cell' },
  { key: 'subtotal', label: 'Subtotal', class: 'w-28 text-right' },
  { key: 'discount', label: 'Diskon', class: 'w-40 text-right' },
  { key: 'shipping_cost', label: 'Ongkir', class: 'w-24 text-right hidden md:table-cell' },
  { key: 'total', label: 'Total', class: 'w-28 text-right' },
  { key: 'metode_bayar', label: 'Bayar', class: 'w-20 hidden xl:table-cell' },
  { key: 'created_by_name', label: 'Kasir/Admin', class: 'w-32 hidden lg:table-cell' },
]

// -------------------- helpers --------------------
function fmtIDR(v?: number | null): string {
  if (v == null) return '—'
  return new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 }).format(v)
}
function fmtDate(s: string): string {
  try {
    return new Date(s).toLocaleString('id-ID', { day: '2-digit', month: 'short', year: '2-digit', hour: '2-digit', minute: '2-digit' })
  } catch {
    return s
  }
}
function statusLabel(s: string): string {
  return s.replace(/_/g, ' ')
}
</script>

<template>
  <section>
    <AdminPageHeader
      title="Rekap Order"
      subtitle="Laporan penjualan, diskon, dan ongkir untuk rentang tanggal tertentu."
    >
      <template #actions>
        <button
          type="button"
          :disabled="exporting || loading"
          class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas-alt transition-colors disabled:opacity-60"
          @click="downloadCsv"
        >
          <Loader2 v-if="exporting" class="h-4 w-4 animate-spin" :stroke-width="1.75" />
          <Download v-else class="h-4 w-4" :stroke-width="1.75" />
          Unduh CSV
        </button>
      </template>
    </AdminPageHeader>

    <AlertMessage v-if="errorMsg" variant="error" :message="errorMsg" class="mb-4" />
    <AlertMessage v-if="exportError" variant="error" :message="exportError" class="mb-4" />

    <!-- ============================ Filter panel ============================ -->
    <div class="mb-6 rounded-lg border border-hairline bg-canvas p-6">
      <div class="flex items-center gap-2 mb-4">
        <CalendarRange class="h-4 w-4 text-ink-500" :stroke-width="1.75" />
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Filter</p>
      </div>
      <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <div>
          <label for="rekap-from" class="block text-sm font-medium text-ink-900">Dari tanggal <span class="text-brand-500">*</span></label>
          <input
            id="rekap-from"
            v-model="dateFrom"
            type="date"
            required
            :max="dateTo"
            class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
          >
        </div>
        <div>
          <label for="rekap-to" class="block text-sm font-medium text-ink-900">Sampai tanggal <span class="text-brand-500">*</span></label>
          <input
            id="rekap-to"
            v-model="dateTo"
            type="date"
            required
            :min="dateFrom"
            class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
          >
          <p v-if="dateRangeError" class="mt-1 text-xs text-brand-600">{{ dateRangeError }}</p>
        </div>
        <div>
          <label for="rekap-channel" class="block text-sm font-medium text-ink-900">Channel</label>
          <select
            id="rekap-channel"
            v-model="channel"
            class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
          >
            <option value="">Semua channel</option>
            <option value="online">Online</option>
            <option value="pos">POS</option>
          </select>
        </div>
        <div>
          <label for="rekap-status" class="block text-sm font-medium text-ink-900">Status order</label>
          <select
            id="rekap-status"
            v-model="status"
            class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
          >
            <option v-for="opt in statusOptions" :key="opt.v" :value="opt.v">{{ opt.label }}</option>
          </select>
        </div>
        <div>
          <label for="rekap-created-by" class="block text-sm font-medium text-ink-900">Kasir / Admin</label>
          <select
            id="rekap-created-by"
            v-model="createdBy"
            :disabled="filterOptionsLoading"
            class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:opacity-60"
          >
            <option value="">Semua kasir / admin</option>
            <option v-for="opt in kasirOptions" :key="opt.id" :value="opt.id">{{ opt.name }}</option>
          </select>
          <p class="mt-1 text-xs text-ink-500">
            {{ filterOptionsLoading ? 'Memuat pilihan…' : 'Hanya menampilkan kasir/admin yang punya order di rentang ini.' }}
          </p>
        </div>
        <div>
          <label for="rekap-discount" class="block text-sm font-medium text-ink-900">Diskon</label>
          <select
            id="rekap-discount"
            v-model="discountSelection"
            :disabled="filterOptionsLoading"
            class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors disabled:opacity-60"
          >
            <option value="">Semua diskon</option>
            <option v-for="opt in diskonOptions" :key="diskonOptionValue(opt.id)" :value="diskonOptionValue(opt.id)">{{ opt.label }}</option>
          </select>
          <p class="mt-1 text-xs text-ink-500">
            {{ filterOptionsLoading ? 'Memuat pilihan…' : 'Hanya menampilkan diskon yang dipakai pada rentang ini.' }}
          </p>
        </div>
      </div>
      <AlertMessage v-if="filterOptionsError" variant="error" :message="filterOptionsError" class="mt-4" />
      <label class="mt-4 inline-flex cursor-pointer items-center gap-2 text-sm font-medium text-ink-900">
        <input v-model="onlyDiscounted" type="checkbox" class="h-4 w-4 rounded border-hairline accent-brand-500">
        Hanya order berdiskon
      </label>
    </div>

    <!-- ============================ Ringkasan ============================ -->
    <div class="mb-6 grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
      <div class="rounded-lg border border-hairline bg-canvas p-5">
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Jumlah order</p>
        <p class="mt-2 font-serif text-2xl font-semibold text-ink-950">{{ summary?.order_count ?? '—' }}</p>
      </div>
      <div class="rounded-lg border border-hairline bg-canvas p-5">
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Omzet kotor</p>
        <p class="mt-2 font-serif text-2xl font-semibold text-ink-950">{{ fmtIDR(summary?.gross_subtotal) }}</p>
      </div>
      <div class="rounded-lg border border-hairline bg-canvas p-5">
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Total diskon</p>
        <p class="mt-2 font-serif text-2xl font-semibold text-brand-600">
          {{ summary && summary.total_discount > 0 ? `-${fmtIDR(summary.total_discount)}` : '—' }}
        </p>
      </div>
      <div class="rounded-lg border border-hairline bg-canvas p-5">
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Total ongkir</p>
        <p class="mt-2 font-serif text-2xl font-semibold text-ink-950">{{ fmtIDR(summary?.total_shipping) }}</p>
      </div>
      <div class="rounded-lg border border-hairline bg-canvas p-5">
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Omzet bersih</p>
        <p class="mt-2 font-serif text-2xl font-semibold text-ink-950">{{ fmtIDR(summary?.net_total) }}</p>
      </div>
      <div class="rounded-lg border border-hairline bg-canvas p-5">
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Rata-rata / order</p>
        <p class="mt-2 font-serif text-2xl font-semibold text-ink-950">{{ fmtIDR(summary?.average_order_value) }}</p>
      </div>
    </div>

    <!-- ============================ Tabel ============================ -->
    <AdminDataTable
      v-if="loading || items.length > 0"
      :columns="columns"
      :items="items"
      :loading="loading"
      row-key="resi"
      empty-message="Tidak ada order pada filter ini."
    >
      <template #cell-created_at="{ row }">
        <span class="text-xs text-ink-500">{{ fmtDate((row as OrderRecapItem).created_at) }}</span>
      </template>
      <template #cell-resi="{ row }">
        <NuxtLink
          :to="`/admin/order/${(row as OrderRecapItem).resi}`"
          class="font-mono text-xs text-ink-900 hover:text-brand-500 transition-colors"
        >
          {{ (row as OrderRecapItem).resi }}
        </NuxtLink>
      </template>
      <template #cell-customer_name="{ row }">
        <span class="text-sm text-ink-900">{{ (row as OrderRecapItem).customer_name || '—' }}</span>
      </template>
      <template #cell-product_name="{ row }">
        <span class="text-sm text-ink-900">{{ (row as OrderRecapItem).product_name }}</span>
      </template>
      <template #cell-jumlah_item="{ row }">
        <span class="text-sm text-ink-700">{{ (row as OrderRecapItem).jumlah_item }}</span>
      </template>
      <template #cell-channel="{ row }">
        <span class="text-xs uppercase text-ink-600">{{ (row as OrderRecapItem).channel }}</span>
      </template>
      <template #cell-status="{ row }">
        <AdminStatusBadge :status="statusLabel((row as OrderRecapItem).status)" />
      </template>
      <template #cell-subtotal="{ row }">
        <span class="text-sm text-ink-900">{{ fmtIDR((row as OrderRecapItem).subtotal) }}</span>
      </template>
      <template #cell-discount="{ row }">
        <template v-if="(row as OrderRecapItem).discount_amount > 0">
          <p class="text-sm font-medium text-brand-600">-{{ fmtIDR((row as OrderRecapItem).discount_amount) }}</p>
          <p v-if="(row as OrderRecapItem).discount_label" class="text-[11px] text-ink-500">{{ (row as OrderRecapItem).discount_label }}</p>
        </template>
        <span v-else class="text-sm text-ink-400">—</span>
      </template>
      <template #cell-shipping_cost="{ row }">
        <span class="text-sm text-ink-900">{{ fmtIDR((row as OrderRecapItem).shipping_cost) }}</span>
      </template>
      <template #cell-total="{ row }">
        <span class="text-sm font-semibold text-ink-950">{{ fmtIDR((row as OrderRecapItem).total) }}</span>
      </template>
      <template #cell-metode_bayar="{ row }">
        <span class="text-xs uppercase text-ink-600">{{ (row as OrderRecapItem).metode_bayar || '—' }}</span>
      </template>
      <template #cell-created_by_name="{ row }">
        <span class="text-xs text-ink-600">{{ (row as OrderRecapItem).created_by_name || '—' }}</span>
      </template>
    </AdminDataTable>

    <AdminEmptyState
      v-else
      title="Belum ada order pada rentang ini"
      message="Ubah rentang tanggal atau filter lain untuk melihat data."
      :icon="ClipboardList"
    />

    <AdminPagination
      v-if="total > 0"
      :page="page"
      :limit="pageSize"
      :total="total"
      @update:page="(v) => (page = v)"
    />
  </section>
</template>
