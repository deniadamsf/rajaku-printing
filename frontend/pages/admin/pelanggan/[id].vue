<script setup lang="ts">
/**
 * /admin/pelanggan/:id — Detail pelanggan (CLAUDE.md §3 modul `auth`, §11).
 *
 * Layout: header (nama + badge tipe/member/status akun) → kartu statistik
 * order → daftar 5 order terakhir → form ubah data (`customer.manage`) →
 * blokir/aktifkan (modal alasan wajib, pola sama `/admin/membership`) →
 * riwayat perubahan status. Permission `customer.view` untuk lihat halaman.
 */
import {
  AlertCircle,
  Ban,
  CircleAlert,
  ClipboardList,
  Crown,
  ShieldAlert,
  ShieldCheck,
  ShoppingBag,
  UserRound,
} from '@lucide/vue'
import { ApiError } from '~/composables/useApi'
import type { CustomerDetail, CustomerMembershipStatus, CustomerUpdateInput } from '~/types/customer'

definePageMeta({
  middleware: ['staff-only'],
  layout: 'admin',
})

const route = useRoute()
const customerId = computed(() => String(route.params.id))

const auth = useAuthStore()
const customerSvc = useAdminCustomer()

const canView = computed(() => auth.hasPermission('customer.view'))
const canManage = computed(() => auth.hasPermission('customer.manage'))
const canViewOrders = computed(() => auth.hasPermission('order.view'))

// -------------------- fetch detail --------------------
const loading = ref(false)
const notFound = ref(false)
const errorMsg = ref<string | null>(null)
const detail = ref<CustomerDetail | null>(null)

useSeoMeta({
  title: () => (detail.value ? `${detail.value.customer.name} — Pelanggan — Rajaku Admin` : 'Pelanggan — Rajaku Admin'),
})

async function fetchDetail() {
  if (!canView.value) return
  loading.value = true
  errorMsg.value = null
  notFound.value = false
  try {
    detail.value = await customerSvc.get(customerId.value)
    resetForm()
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) {
      notFound.value = true
    } else {
      errorMsg.value = e instanceof ApiError ? e.message : 'Gagal memuat data pelanggan'
    }
  } finally {
    loading.value = false
  }
}
onMounted(fetchDetail)

// -------------------- form ubah data --------------------
const form = reactive({ name: '', phone: '', email: '' })
const fieldErrors = reactive<{ phone?: string; email?: string }>({})
const saving = ref(false)
const saveSuccess = ref(false)
const saveError = ref<string | null>(null)

function resetForm() {
  if (!detail.value) return
  const c = detail.value.customer
  form.name = c.name
  form.phone = c.phone || ''
  form.email = c.email || ''
  fieldErrors.phone = undefined
  fieldErrors.email = undefined
}

const dirty = computed(() => {
  if (!detail.value) return false
  const c = detail.value.customer
  return form.name.trim() !== c.name || form.phone.trim() !== (c.phone || '') || form.email.trim() !== (c.email || '')
})

/**
 * Backend membalas 409 saat nomor/email sudah dipakai pelanggan lain, dengan
 * kode error PER FIELD (`PHONE_ALREADY_USED` / `EMAIL_ALREADY_USED`) — kosakata
 * yang sama dipakai jalur registrasi & phone-claim di modul auth. Dicocokkan
 * lewat kode, bukan teks pesan: pesan Indonesia bisa diperbaiki kapan saja dan
 * pencocokan kata kunci akan diam-diam berhenti menempel ke field yang benar.
 * Kode lain (mis. CONFLICT dari status yang tidak berubah) jatuh ke banner.
 */
function applyConflictError(e: ApiError) {
  switch (e.code) {
    case 'PHONE_ALREADY_USED':
      fieldErrors.phone = e.message
      break
    case 'EMAIL_ALREADY_USED':
      fieldErrors.email = e.message
      break
    default:
      saveError.value = e.message
  }
}

async function saveForm() {
  if (!detail.value || !canManage.value) return
  saving.value = true
  saveError.value = null
  saveSuccess.value = false
  fieldErrors.phone = undefined
  fieldErrors.email = undefined
  try {
    const c = detail.value.customer
    const payload: CustomerUpdateInput = {}
    if (form.name.trim() !== c.name) payload.name = form.name.trim()
    if (form.phone.trim() !== (c.phone || '')) payload.phone = form.phone.trim()
    if (form.email.trim() !== (c.email || '')) payload.email = form.email.trim()
    const updated = await customerSvc.update(customerId.value, payload)
    detail.value.customer = updated
    resetForm()
    saveSuccess.value = true
    setTimeout(() => (saveSuccess.value = false), 2500)
  } catch (e) {
    if (e instanceof ApiError && e.status === 409) {
      applyConflictError(e)
    } else {
      saveError.value = e instanceof ApiError ? e.message : 'Gagal menyimpan perubahan'
    }
  } finally {
    saving.value = false
  }
}

// -------------------- blokir / aktifkan --------------------
const statusModalOpen = ref(false)
const statusReason = ref('')
const statusBusy = ref(false)
const statusModalError = ref<string | null>(null)

const statusAction = computed<'deactivate' | 'activate'>(() =>
  detail.value?.customer.is_active ? 'deactivate' : 'activate',
)
const reasonCanSubmit = computed(() => statusReason.value.trim().length >= 10)

function openStatusModal() {
  statusReason.value = ''
  statusModalError.value = null
  statusModalOpen.value = true
}

async function confirmStatusChange() {
  if (!detail.value) return
  if (!reasonCanSubmit.value) {
    statusModalError.value = 'Alasan minimal 10 karakter.'
    return
  }
  statusBusy.value = true
  statusModalError.value = null
  try {
    const reason = statusReason.value.trim()
    const updated =
      statusAction.value === 'deactivate'
        ? await customerSvc.deactivate(customerId.value, reason)
        : await customerSvc.activate(customerId.value, reason)
    detail.value.customer = updated
    statusModalOpen.value = false
    await fetchDetail() // refresh admin_logs juga
  } catch (e) {
    statusModalError.value = e instanceof ApiError ? e.message : 'Gagal memproses perubahan status'
  } finally {
    statusBusy.value = false
  }
}

// -------------------- helpers --------------------
function fmtDate(s?: string | null): string {
  if (!s) return '—'
  try {
    return new Date(s).toLocaleString('id-ID', { day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' })
  } catch {
    return s
  }
}
function fmtIDR(n: number | null | undefined): string {
  if (n == null) return '—'
  return new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 }).format(n)
}
function typeLabel(t: string): string {
  return t === 'registered' ? 'Terdaftar' : 'Guest'
}
function channelLabel(c: string): string {
  return c === 'pos' ? 'POS' : 'Online'
}
function orderStatusLabel(s: string): string {
  return s.replace(/_/g, ' ')
}
function membershipLabel(s: CustomerMembershipStatus | string): string {
  const map: Record<string, string> = {
    none: 'Belum ajukan',
    pending: 'Pending',
    active: 'Member aktif',
    rejected: 'Ditolak',
    revoked: 'Dicabut',
  }
  return map[s] ?? s
}
function membershipTone(s: CustomerMembershipStatus | string): 'green' | 'amber' | 'ink' | 'rose' {
  if (s === 'active') return 'green'
  if (s === 'pending') return 'amber'
  if (s === 'rejected') return 'rose'
  return 'ink'
}
</script>

<template>
  <section>
    <AdminPageHeader
      :title="detail?.customer.name || 'Pelanggan'"
      :subtitle="detail ? [detail.customer.phone, detail.customer.email].filter(Boolean).join(' · ') || 'Tidak ada kontak tercatat' : undefined"
      :breadcrumb="[{ label: 'Pelanggan', to: '/admin/pelanggan' }, { label: detail?.customer.name || '…' }]"
    />

    <div v-if="!canView" class="rounded-lg border border-hairline bg-canvas-alt p-6 flex items-start gap-3">
      <ShieldAlert class="h-5 w-5 text-ink-400 flex-none mt-0.5" :stroke-width="1.5" />
      <p class="text-sm text-ink-600 leading-relaxed">
        Anda tidak punya izin <span class="font-mono text-xs">customer.view</span> untuk melihat halaman ini.
        Hubungi super admin kalau ini keliru.
      </p>
    </div>

    <div v-else-if="notFound" class="rounded-lg border border-hairline bg-canvas-alt p-10 text-center">
      <UserRound class="mx-auto h-8 w-8 text-ink-300" :stroke-width="1.5" />
      <h2 class="mt-3 font-serif text-lg font-semibold text-ink-950">Pelanggan tidak ditemukan</h2>
      <p class="mt-1 text-sm text-ink-500">Data mungkin sudah dihapus atau tautannya keliru.</p>
      <NuxtLink
        to="/admin/pelanggan"
        class="mt-4 inline-flex items-center rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
      >
        Kembali ke daftar pelanggan
      </NuxtLink>
    </div>

    <template v-else>
      <AlertMessage v-if="errorMsg" variant="error" :message="errorMsg" class="mb-4" />

      <div v-if="loading && !detail" class="rounded-lg border border-hairline bg-canvas p-8 text-center text-sm text-ink-500">
        Memuat…
      </div>

      <template v-else-if="detail">
        <!-- Badges -->
        <div class="mb-6 flex flex-wrap items-center gap-2">
          <AdminStatusBadge :status="typeLabel(detail.customer.customer_type)" tone="ink" />
          <AdminStatusBadge :status="detail.customer.is_active ? 'Aktif' : 'Diblokir'" :tone="detail.customer.is_active ? 'green' : 'ink'" />
          <!-- Member aktif pakai badge emas dedicated (§26.1/§30.4, pola sama /admin/pelanggan list &
               /admin/pos) — bukan AdminStatusBadge, supaya konteks premium membership tetap konsisten
               lintas halaman. Status member lain (pending/rejected/revoked/none) tetap StatusBadge biasa. -->
          <span
            v-if="detail.customer.membership_status === 'active'"
            class="inline-flex items-center gap-1.5 rounded-full bg-gold-50 px-2.5 py-1 text-xs font-medium text-gold-900 ring-1 ring-inset ring-gold-200"
          >
            <Crown class="h-3 w-3" :stroke-width="1.75" />
            Member Aktif
          </span>
          <AdminStatusBadge
            v-else
            :status="membershipLabel(detail.customer.membership_status)"
            :tone="membershipTone(detail.customer.membership_status)"
          />
          <span
            v-if="detail.customer.phone && !detail.customer.phone_verified"
            class="inline-flex items-center gap-1 rounded-full bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-800 ring-1 ring-inset ring-amber-200"
            title="Nomor WA belum diverifikasi lewat OTP"
          >
            <CircleAlert class="h-3 w-3" :stroke-width="1.75" />
            Nomor belum terverifikasi
          </span>
          <span v-if="detail.customer.oauth_provider" class="text-xs text-ink-400">
            Login via {{ detail.customer.oauth_provider === 'google' ? 'Google' : detail.customer.oauth_provider }}
          </span>
        </div>

        <!-- Kartu statistik order -->
        <div class="mb-8 rounded-lg border border-hairline bg-canvas p-6 md:p-8">
          <h2 class="text-sm font-semibold text-ink-900">Riwayat order</h2>
          <div v-if="detail.order_stats.total_orders === 0" class="mt-4 flex items-center gap-3 rounded-md border border-dashed border-hairline bg-canvas-alt px-4 py-6 text-sm text-ink-500">
            <ShoppingBag class="h-5 w-5 text-ink-300 flex-none" :stroke-width="1.5" />
            Pelanggan ini belum pernah order, baik online maupun walk-in.
          </div>
          <div v-else class="mt-4 grid grid-cols-2 gap-4 sm:grid-cols-5">
            <div>
              <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Jumlah order</p>
              <p class="mt-1 text-lg font-semibold text-ink-950">{{ detail.order_stats.total_orders }}</p>
            </div>
            <div>
              <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Selesai</p>
              <p class="mt-1 text-lg font-semibold text-ink-950">{{ detail.order_stats.completed_orders }}</p>
            </div>
            <div>
              <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Batal</p>
              <p class="mt-1 text-lg font-semibold text-ink-950">{{ detail.order_stats.cancelled_orders }}</p>
            </div>
            <div>
              <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Total belanja</p>
              <p class="mt-1 text-lg font-semibold text-ink-950">{{ fmtIDR(detail.order_stats.total_spend) }}</p>
              <p class="mt-0.5 text-[11px] text-ink-400">Dari order yang sudah dibayar</p>
            </div>
            <div>
              <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Order terakhir</p>
              <p class="mt-1 text-sm font-medium text-ink-900">{{ fmtDate(detail.order_stats.last_order_at) }}</p>
            </div>
          </div>
        </div>

        <!-- Order terakhir -->
        <div v-if="detail.recent_orders.length > 0" class="mb-8">
          <h2 class="mb-3 text-sm font-semibold text-ink-900">5 order terakhir</h2>
          <div class="overflow-x-auto rounded-lg border border-hairline bg-canvas">
            <table class="min-w-full divide-y divide-hairline text-sm">
              <thead class="bg-canvas-alt">
                <tr>
                  <th class="px-4 py-3 text-left text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Resi</th>
                  <th class="px-4 py-3 text-left text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Tanggal</th>
                  <th class="px-4 py-3 text-left text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Channel</th>
                  <th class="px-4 py-3 text-left text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Status</th>
                  <th class="px-4 py-3 text-right text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Total</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-hairline">
                <tr v-for="o in detail.recent_orders" :key="o.resi" class="hover:bg-canvas-alt/60 transition-colors">
                  <td class="px-4 py-3 align-middle">
                    <NuxtLink
                      v-if="canViewOrders"
                      :to="`/admin/order/${o.resi}`"
                      class="font-mono text-xs text-ink-900 hover:text-brand-500 transition-colors"
                    >
                      {{ o.resi }}
                    </NuxtLink>
                    <span v-else class="font-mono text-xs text-ink-700">{{ o.resi }}</span>
                  </td>
                  <td class="px-4 py-3 align-middle text-xs text-ink-500">{{ fmtDate(o.created_at) }}</td>
                  <td class="px-4 py-3 align-middle text-xs uppercase text-ink-600">{{ channelLabel(o.channel) }}</td>
                  <td class="px-4 py-3 align-middle">
                    <AdminStatusBadge :status="orderStatusLabel(o.status)" />
                  </td>
                  <td class="px-4 py-3 align-middle text-right text-sm font-medium text-ink-900">{{ fmtIDR(o.total) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <div class="grid gap-6 lg:grid-cols-2">
          <!-- Form ubah data -->
          <div class="rounded-lg border border-hairline bg-canvas p-6 md:p-8">
            <h2 class="text-sm font-semibold text-ink-900">Ubah data</h2>
            <p v-if="!canManage" class="mt-1 text-xs text-ink-500">
              Anda tidak punya izin <span class="font-mono">customer.manage</span> — data hanya bisa dilihat.
            </p>

            <AlertMessage v-if="saveError" variant="error" :message="saveError" class="mt-4" />
            <AlertMessage v-if="saveSuccess" variant="success" message="Perubahan tersimpan." class="mt-4" />

            <form class="mt-4 space-y-4" @submit.prevent="saveForm">
              <div>
                <label for="cust-name" class="block text-sm font-medium text-ink-900">Nama</label>
                <input
                  id="cust-name"
                  v-model="form.name"
                  type="text"
                  :readonly="!canManage"
                  class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors read-only:bg-canvas-alt read-only:text-ink-500"
                >
              </div>

              <div>
                <label for="cust-phone" class="block text-sm font-medium text-ink-900">No. WA</label>
                <input
                  id="cust-phone"
                  v-model="form.phone"
                  type="text"
                  placeholder="628xxxxxxxxxx"
                  :readonly="!canManage"
                  class="mt-1 block w-full rounded-md border bg-canvas px-3 py-2 font-mono text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors read-only:bg-canvas-alt read-only:text-ink-500"
                  :class="fieldErrors.phone ? 'border-brand-400' : 'border-hairline'"
                >
                <p v-if="fieldErrors.phone" class="mt-1 text-xs text-brand-600">{{ fieldErrors.phone }}</p>
                <p class="mt-1.5 flex items-start gap-1.5 text-xs text-ink-500 leading-relaxed">
                  <AlertCircle class="h-3.5 w-3.5 flex-none mt-0.5" :stroke-width="1.75" />
                  Mengubah nomor akan menghapus status terverifikasi — pelanggan perlu verifikasi ulang lewat OTP.
                </p>
              </div>

              <div>
                <label for="cust-email" class="block text-sm font-medium text-ink-900">Email</label>
                <input
                  id="cust-email"
                  v-model="form.email"
                  type="email"
                  :readonly="!canManage"
                  class="mt-1 block w-full rounded-md border bg-canvas px-3 py-2 text-sm text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors read-only:bg-canvas-alt read-only:text-ink-500"
                  :class="fieldErrors.email ? 'border-brand-400' : 'border-hairline'"
                >
                <p v-if="fieldErrors.email" class="mt-1 text-xs text-brand-600">{{ fieldErrors.email }}</p>
              </div>

              <div v-if="canManage" class="flex justify-end gap-2 pt-1">
                <button
                  type="button"
                  class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
                  :disabled="!dirty || saving"
                  @click="resetForm"
                >
                  Batalkan
                </button>
                <button
                  type="submit"
                  class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-1.5 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-60"
                  :disabled="!dirty || saving"
                >
                  <span
                    v-if="saving"
                    class="inline-block h-3 w-3 rounded-full border-2 border-canvas/70 border-t-transparent animate-spin"
                    aria-hidden="true"
                  />
                  Simpan perubahan
                </button>
              </div>
            </form>
          </div>

          <!-- Status akun -->
          <div class="rounded-lg border border-hairline bg-canvas p-6 md:p-8">
            <h2 class="text-sm font-semibold text-ink-900">Status akun</h2>
            <p class="mt-1 text-sm text-ink-500 leading-relaxed">
              Pelanggan saat ini
              <strong class="text-ink-900">{{ detail.customer.is_active ? 'aktif' : 'diblokir' }}</strong>.
              {{ detail.customer.is_active ? 'Blokir akan mencegah pelanggan membuat order baru.' : 'Aktifkan supaya pelanggan bisa order kembali.' }}
            </p>

            <div v-if="canManage" class="mt-4">
              <button
                v-if="detail.customer.is_active"
                type="button"
                class="inline-flex items-center gap-2 rounded-md border border-brand-200 bg-canvas px-3 py-1.5 text-sm font-medium text-brand-700 hover:bg-brand-50 hover:border-brand-300 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
                @click="openStatusModal"
              >
                <Ban class="h-4 w-4" :stroke-width="1.75" />
                Blokir pelanggan
              </button>
              <button
                v-else
                type="button"
                class="inline-flex items-center gap-2 rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
                @click="openStatusModal"
              >
                <ShieldCheck class="h-4 w-4" :stroke-width="1.75" />
                Aktifkan pelanggan
              </button>
            </div>

            <!-- Riwayat perubahan (blokir/aktifkan/ubah data — §admin_logs) -->
            <div v-if="detail.admin_logs.length > 0" class="mt-6 border-t border-hairline pt-4">
              <p class="mb-2 flex items-center gap-1.5 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
                <ClipboardList class="h-3.5 w-3.5" :stroke-width="1.75" />
                Riwayat perubahan
              </p>
              <ul class="space-y-3">
                <li v-for="(log, i) in detail.admin_logs" :key="i" class="text-xs text-ink-600 leading-relaxed">
                  <template v-if="log.action === 'block'">
                    <strong class="text-ink-900">Diblokir</strong> oleh {{ log.changed_by_name || 'sistem' }} · {{ fmtDate(log.created_at) }}
                    <span v-if="log.reason"> — {{ log.reason }}</span>
                  </template>
                  <template v-else-if="log.action === 'unblock'">
                    <strong class="text-ink-900">Diaktifkan kembali</strong> oleh {{ log.changed_by_name || 'sistem' }} · {{ fmtDate(log.created_at) }}
                    <span v-if="log.reason"> — {{ log.reason }}</span>
                  </template>
                  <template v-else>
                    <strong class="text-ink-900">Data diubah</strong> oleh {{ log.changed_by_name || 'sistem' }} · {{ fmtDate(log.created_at) }}
                    <span v-if="log.changes" class="mt-1 block whitespace-pre-wrap break-words font-mono text-[11px] text-ink-500">{{ log.changes }}</span>
                  </template>
                </li>
              </ul>
            </div>
          </div>
        </div>
      </template>
    </template>

    <!-- Modal blokir/aktifkan -->
    <AdminConfirmDialog
      v-model:open="statusModalOpen"
      :title="statusAction === 'deactivate' ? 'Blokir pelanggan ini?' : 'Aktifkan kembali pelanggan ini?'"
      :variant="statusAction === 'deactivate' ? 'danger' : 'default'"
      :confirm-label="statusAction === 'deactivate' ? 'Blokir pelanggan' : 'Aktifkan pelanggan'"
      :loading="statusBusy"
      :confirm-disabled="!reasonCanSubmit"
      :error="statusModalError"
      @confirm="confirmStatusChange"
    >
      <template v-if="detail">
        <p class="mt-2 text-sm text-ink-500 leading-relaxed">
          <strong class="text-ink-900">{{ detail.customer.name }}</strong>
          <span v-if="detail.customer.phone"> ({{ detail.customer.phone }})</span>
          {{ statusAction === 'deactivate' ? 'tidak akan bisa membuat order baru sampai diaktifkan kembali.' : 'akan bisa membuat order kembali.' }}
        </p>

        <div v-if="statusAction === 'deactivate'" class="mt-3 rounded-md border border-hairline bg-canvas-alt px-3 py-3">
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Yang berlaku</p>
          <ul class="mt-2 space-y-1.5 text-xs text-ink-600 leading-relaxed">
            <li>Pelanggan tidak bisa login lagi.</li>
            <li>Order baru atas nomor WA-nya ditolak, termasuk lewat kasir POS.</li>
            <li>Sesi yang sudah berjalan tidak ikut terputus — token akses yang terlanjur terbit tetap sah sampai maksimal 24 jam, karena verifikasi token tidak menyentuh database.</li>
          </ul>
        </div>

        <div class="mt-3">
          <label for="status-reason" class="block text-sm font-medium text-ink-900">Alasan <span class="text-brand-500">*</span></label>
          <textarea
            id="status-reason"
            v-model="statusReason"
            rows="2"
            required
            placeholder="Minimal 10 karakter, mis. penyalahgunaan diskon berulang"
            class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
          />
          <p class="mt-1 text-xs text-ink-400">{{ statusReason.trim().length }}/10 karakter minimum</p>
        </div>
      </template>
    </AdminConfirmDialog>
  </section>
</template>
