<script setup lang="ts">
/**
 * /admin/pelanggan — Manajemen Pelanggan (CLAUDE.md §3 modul `auth`, §11).
 *
 * Layout: filter bar (pencarian + tipe + status akun + status member) +
 * tabel server-side pagination + ekspor CSV dengan filter aktif — pola sama
 * dengan `/admin/order` dan `/admin/rekap`. Permission `customer.view` untuk
 * lihat halaman; tombol/aksi ubah data ada di halaman detail (`customer.manage`).
 */
import { CircleAlert, Contact, Crown, Download, Loader2, ShieldAlert } from '@lucide/vue'
import type { DataTableColumn } from '~/components/admin/DataTable.vue'
import { ApiError } from '~/composables/useApi'
import type { Customer, CustomerListParams, CustomerMembershipStatus, CustomerType } from '~/types/customer'

definePageMeta({
  middleware: ['staff-only'],
  layout: 'admin',
})

useSeoMeta({ title: 'Pelanggan — Rajaku Admin' })

const auth = useAuthStore()
const customerSvc = useAdminCustomer()

const canView = computed(() => auth.hasPermission('customer.view'))

// -------------------- filter & list state --------------------
const search = ref('')
const customerType = ref<CustomerType | ''>('')
const activeFilter = ref<'' | 'true' | 'false'>('')
const membershipFilter = ref<CustomerMembershipStatus | ''>('')
const page = ref(1)
const pageSize = 20

const items = ref<Customer[]>([])
const total = ref(0)
const loading = ref(false)
const errorMsg = ref<string | null>(null)
/** true kalau permintaan terakhir memakai pencarian/filter (untuk bedakan pesan empty state). */
const hasActiveFilter = computed(
  () => !!search.value.trim() || !!customerType.value || activeFilter.value !== '' || !!membershipFilter.value,
)

function currentFilter(): CustomerListParams {
  return {
    q: search.value.trim() || undefined,
    customer_type: customerType.value || undefined,
    is_active: activeFilter.value === '' ? '' : activeFilter.value === 'true',
    membership_status: membershipFilter.value || undefined,
    page: page.value,
    per_page: pageSize,
  }
}

async function fetchList() {
  if (!canView.value) return
  loading.value = true
  errorMsg.value = null
  try {
    const res = await customerSvc.list(currentFilter())
    items.value = res.items
    total.value = res.total
  } catch (e) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal memuat daftar pelanggan'
  } finally {
    loading.value = false
  }
}

onMounted(fetchList)
// Setiap select filter dipasangi `@change="page = 1"` di template (dieksekusi
// SEBELUM watcher ini jalan), jadi satu watcher gabungan ini cukup — tidak
// perlu watcher kedua yang bisa memicu fetch dobel (page berubah -> watcher
// ini jalan lagi).
watch([customerType, activeFilter, membershipFilter, page], fetchList)

// Pencarian didebounce ~350ms (brief) — filter lain langsung fetch lewat watch di atas.
let searchTimer: ReturnType<typeof setTimeout> | null = null
watch(search, () => {
  page.value = 1
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(fetchList, 350)
})

const columns: DataTableColumn[] = [
  { key: 'name', label: 'Nama' },
  { key: 'phone', label: 'No. WA', class: 'w-40' },
  { key: 'email', label: 'Email', class: 'hidden md:table-cell' },
  { key: 'customer_type', label: 'Tipe', class: 'w-28 hidden lg:table-cell' },
  { key: 'is_active', label: 'Status akun', class: 'w-28' },
  { key: 'created_at', label: 'Terdaftar', class: 'w-32 hidden md:table-cell' },
]

// -------------------- export CSV --------------------
const exporting = ref(false)
const exportError = ref<string | null>(null)

async function downloadCsv() {
  exporting.value = true
  exportError.value = null
  try {
    await customerSvc.exportCsv(currentFilter())
  } catch (e) {
    exportError.value = e instanceof Error ? e.message : 'Gagal mengunduh CSV'
  } finally {
    exporting.value = false
  }
}

// -------------------- helpers --------------------
function fmtDate(s: string): string {
  try {
    return new Date(s).toLocaleDateString('id-ID', { day: '2-digit', month: 'short', year: 'numeric' })
  } catch {
    return s
  }
}
function typeLabel(t: string): string {
  return t === 'registered' ? 'Terdaftar' : 'Guest'
}
function goTo(row: Customer) {
  navigateTo(`/admin/pelanggan/${row.id}`)
}
</script>

<template>
  <section>
    <AdminPageHeader
      title="Pelanggan"
      subtitle="Data pelanggan lintas channel (online & walk-in), riwayat order, dan status akun."
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
          Ekspor CSV
        </button>
      </template>
    </AdminPageHeader>

    <div v-if="!canView" class="rounded-lg border border-hairline bg-canvas-alt p-6 flex items-start gap-3">
      <ShieldAlert class="h-5 w-5 text-ink-400 flex-none mt-0.5" :stroke-width="1.5" />
      <p class="text-sm text-ink-600 leading-relaxed">
        Anda tidak punya izin <span class="font-mono text-xs">customer.view</span> untuk melihat halaman ini.
        Hubungi super admin kalau ini keliru.
      </p>
    </div>

    <template v-else>
      <AlertMessage v-if="errorMsg" variant="error" :message="errorMsg" class="mb-4" />
      <AlertMessage v-if="exportError" variant="error" :message="exportError" class="mb-4" />

      <p class="mb-4 flex items-start gap-2 text-xs text-ink-500">
        <CircleAlert class="h-3.5 w-3.5 flex-none mt-0.5" :stroke-width="1.75" />
        Ekspor CSV mengikuti pencarian & filter yang sedang aktif di layar ini — bukan seluruh data pelanggan.
      </p>

      <!-- Filter bar -->
      <div class="mb-5 flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center">
        <input
          v-model="search"
          type="search"
          placeholder="Cari nama / no. WA / email…"
          class="min-w-[16rem] flex-1 rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors sm:flex-none"
        >

        <select
          v-model="customerType"
          class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
          @change="page = 1"
        >
          <option value="">Semua tipe</option>
          <option value="guest">Guest</option>
          <option value="registered">Terdaftar</option>
        </select>

        <select
          v-model="activeFilter"
          class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
          @change="page = 1"
        >
          <option value="">Semua status akun</option>
          <option value="true">Aktif</option>
          <option value="false">Diblokir</option>
        </select>

        <select
          v-model="membershipFilter"
          class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
          @change="page = 1"
        >
          <option value="">Semua status member</option>
          <option value="none">Belum ajukan</option>
          <option value="pending">Pending</option>
          <option value="active">Aktif</option>
          <option value="rejected">Ditolak</option>
          <option value="revoked">Dicabut</option>
        </select>
      </div>

      <AdminDataTable
        v-if="items.length > 0 || loading"
        :columns="columns"
        :items="items"
        :loading="loading"
        row-key="id"
        empty-message="Tidak ada pelanggan pada filter ini."
        row-clickable
        @row-click="(row) => goTo(row as Customer)"
      >
        <template #cell-name="{ row }">
          <div class="flex items-center gap-2">
            <span class="text-sm font-medium text-ink-900">{{ (row as Customer).name }}</span>
            <span
              v-if="(row as Customer).membership_status === 'active'"
              class="inline-flex items-center gap-1 rounded-full bg-gold-50 px-1.5 py-0.5 text-[10px] font-medium text-gold-900 ring-1 ring-inset ring-gold-200"
            >
              <Crown class="h-2.5 w-2.5" :stroke-width="1.75" />
              Member
            </span>
          </div>
        </template>
        <template #cell-phone="{ row }">
          <div class="flex items-center gap-1.5">
            <span class="font-mono text-xs text-ink-500">{{ (row as Customer).phone || '—' }}</span>
            <span v-if="(row as Customer).phone && !(row as Customer).phone_verified" title="Nomor belum terverifikasi">
              <CircleAlert class="h-3.5 w-3.5 text-amber-600" :stroke-width="1.75" />
            </span>
          </div>
        </template>
        <template #cell-email="{ row }">
          <span class="text-sm text-ink-700">{{ (row as Customer).email || '—' }}</span>
        </template>
        <template #cell-customer_type="{ row }">
          <span class="text-xs text-ink-600">{{ typeLabel((row as Customer).customer_type) }}</span>
        </template>
        <template #cell-is_active="{ row }">
          <AdminStatusBadge
            :status="(row as Customer).is_active ? 'Aktif' : 'Diblokir'"
            :tone="(row as Customer).is_active ? 'green' : 'ink'"
          />
        </template>
        <template #cell-created_at="{ row }">
          <span class="text-xs text-ink-500">{{ fmtDate((row as Customer).created_at) }}</span>
        </template>
      </AdminDataTable>

      <AdminEmptyState
        v-else
        :title="hasActiveFilter ? 'Tidak ada hasil untuk pencarian ini' : 'Belum ada pelanggan'"
        :message="hasActiveFilter ? 'Coba ubah kata kunci atau filter yang dipakai.' : 'Pelanggan akan muncul di sini setelah ada order online atau walk-in pertama.'"
        :icon="Contact"
      />

      <AdminPagination
        v-if="total > 0"
        :page="page"
        :limit="pageSize"
        :total="total"
        @update:page="(v) => (page = v)"
      />
    </template>
  </section>
</template>
