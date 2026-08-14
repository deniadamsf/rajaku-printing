<script setup lang="ts">
/**
 * /akun — Dashboard customer (§10).
 *
 * Layout:
 *   - Header: welcome + profile info card (email, phone).
 *   - Order list: paginated, filter by status, klik row → /akun/pesanan/[resi].
 *   - Empty state: CTA order pertama.
 *
 * Design: patuh CLAUDE.md §26.
 */
import { User, Package, Plus, Loader2, ChevronRight, Search } from '@lucide/vue'
import type { Order, OrderStatus } from '~/types/order'
import { ApiError } from '~/composables/useApi'

definePageMeta({
  middleware: ['customer-only'],
  layout: 'default',
})

useSeoMeta({
  title: 'Akun Saya — Rajaku Printing',
  robots: 'noindex,nofollow',
})

const auth = useAuthStore()
const orderApi = useOrder()

// -------------------- state --------------------
const orders = ref<Order[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)
const statusFilter = ref<OrderStatus | ''>('')
const loading = ref(false)
const errorMsg = ref<string | null>(null)

// -------------------- fetch --------------------
async function fetchOrders() {
  loading.value = true
  errorMsg.value = null
  try {
    const res = await orderApi.listMine({
      status: statusFilter.value || undefined,
      page: page.value,
      pageSize: pageSize.value,
    })
    orders.value = res.items
    total.value = res.total
  } catch (e) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal memuat pesanan'
  } finally {
    loading.value = false
  }
}

onMounted(fetchOrders)
watch([page, statusFilter], () => {
  fetchOrders()
})

// -------------------- helpers --------------------
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))

const statusLabelMap: Record<string, string> = {
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
function statusLabel(s: string): string {
  return statusLabelMap[s] ?? s
}

function fmtIDR(v?: number | null): string {
  if (v == null) return '—'
  return new Intl.NumberFormat('id-ID', {
    style: 'currency',
    currency: 'IDR',
    maximumFractionDigits: 0,
  }).format(v)
}

function fmtDate(s: string): string {
  try {
    return new Date(s).toLocaleDateString('id-ID', {
      day: '2-digit',
      month: 'short',
      year: 'numeric',
    })
  } catch {
    return s
  }
}

// Status yang butuh aksi customer (untuk highlighting di list).
const actionRequired = new Set<string>([
  'menunggu_pembayaran',
  'ditolak',
  'menunggu_approval_desain',
])
function needsAction(status: string): boolean {
  return actionRequired.has(status)
}
</script>

<template>
  <main class="mx-auto max-w-5xl px-4 py-10 md:py-16">
    <!-- Header + welcome -->
    <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2 flex items-center gap-2">
      <User class="h-3.5 w-3.5" :stroke-width="1.75" />
      Akun Saya
    </p>
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <h1 class="font-serif text-3xl md:text-4xl font-semibold tracking-tight text-ink-950">
          Halo, {{ auth.user?.name }}
        </h1>
        <p class="mt-2 text-sm text-ink-500 leading-relaxed">
          Semua pesanan Anda, terkumpul di sini. Klik salah satu untuk lihat detail atau ambil tindakan.
        </p>
      </div>
      <NuxtLink
        to="/order"
        class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors"
      >
        <Plus class="h-4 w-4" :stroke-width="1.75" />
        Order banner baru
      </NuxtLink>
    </div>

    <!-- Profile card -->
    <div class="mt-6 grid gap-4 sm:grid-cols-2">
      <div class="rounded-lg border border-hairline bg-canvas p-4">
        <dt class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Email</dt>
        <dd class="mt-1 text-sm text-ink-900">{{ auth.user?.email || '—' }}</dd>
      </div>
      <div class="rounded-lg border border-hairline bg-canvas p-4">
        <dt class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Nomor WA</dt>
        <dd class="mt-1 text-sm font-mono text-ink-900">{{ auth.user?.phone }}</dd>
      </div>
    </div>

    <!-- Orders section -->
    <div class="mt-10">
      <div class="flex flex-wrap items-center justify-between gap-3 mb-4">
        <h2 class="font-serif text-xl font-semibold tracking-tight text-ink-950">Pesanan saya</h2>
        <div class="inline-flex rounded-md border border-hairline overflow-hidden text-sm">
          <button
            v-for="opt in [
              { v: '', label: 'Semua' },
              { v: 'menunggu_pembayaran', label: 'Perlu bayar' },
              { v: 'proses_cetak', label: 'Dicetak' },
              { v: 'selesai', label: 'Selesai' },
            ]"
            :key="opt.v"
            type="button"
            :class="[
              'px-3 py-1.5 border-r border-hairline last:border-r-0 transition-colors',
              statusFilter === opt.v ? 'bg-ink-950 text-canvas' : 'bg-canvas text-ink-700 hover:bg-canvas-alt',
            ]"
            @click="statusFilter = opt.v as OrderStatus | ''; page = 1"
          >
            {{ opt.label }}
          </button>
        </div>
      </div>

      <AlertMessage v-if="errorMsg" variant="error" :message="errorMsg" class="mb-4" />

      <div v-if="loading" class="rounded-lg border border-hairline bg-canvas p-12 text-center">
        <Loader2 class="mx-auto h-5 w-5 animate-spin text-ink-500" :stroke-width="1.75" />
        <p class="mt-3 text-sm text-ink-500">Memuat pesanan…</p>
      </div>

      <div v-else-if="orders.length === 0" class="rounded-lg border border-hairline bg-canvas p-12 text-center">
        <Package class="mx-auto h-6 w-6 text-ink-400" :stroke-width="1.5" />
        <p class="mt-3 font-serif text-lg font-semibold text-ink-950">Belum ada pesanan</p>
        <p class="mt-1 text-sm text-ink-500 leading-relaxed max-w-md mx-auto">
          {{ statusFilter ? 'Tidak ada pesanan di kategori ini.' : 'Mulai dengan order banner pertama Anda.' }}
        </p>
        <NuxtLink
          v-if="!statusFilter"
          to="/order"
          class="mt-4 inline-flex items-center gap-2 rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 transition-colors"
        >
          <Plus class="h-4 w-4" :stroke-width="1.75" />
          Order banner sekarang
        </NuxtLink>
      </div>

      <ul v-else class="space-y-2">
        <li v-for="o in orders" :key="o.id">
          <NuxtLink
            :to="`/akun/pesanan/${o.resi}`"
            class="group flex items-start gap-3 rounded-lg border border-hairline bg-canvas p-4 hover:border-ink-300 transition-colors"
          >
            <div class="flex-1 min-w-0">
              <div class="flex flex-wrap items-start justify-between gap-2">
                <div>
                  <p class="font-mono text-xs text-ink-500">{{ o.resi }}</p>
                  <p class="mt-1 text-sm font-medium text-ink-900 group-hover:text-brand-500 transition-colors">
                    {{ o.product_name }}
                  </p>
                </div>
                <div class="text-right">
                  <span
                    :class="[
                      'inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium ring-1 ring-inset',
                      needsAction(o.status)
                        ? 'bg-brand-50 text-brand-700 ring-brand-200'
                        : 'bg-canvas-alt text-ink-700 ring-hairline',
                    ]"
                  >
                    {{ statusLabel(o.status) }}
                  </span>
                </div>
              </div>
              <div class="mt-2 flex flex-wrap items-center gap-3 text-xs text-ink-500">
                <span>{{ o.material_name }}</span>
                <span class="text-ink-400">·</span>
                <span class="font-mono">{{ o.width_cm }}×{{ o.height_cm }}cm</span>
                <span class="text-ink-400">·</span>
                <span>{{ o.quantity }} pcs</span>
                <span class="text-ink-400">·</span>
                <span>{{ fmtDate(o.created_at) }}</span>
              </div>
              <div class="mt-1 flex items-baseline gap-2">
                <span class="text-xs text-ink-500">Total:</span>
                <span class="font-serif text-base font-semibold text-ink-950">{{ fmtIDR(o.total) }}</span>
              </div>
            </div>
            <ChevronRight class="h-4 w-4 text-ink-400 flex-none mt-1 group-hover:text-brand-500 transition-colors" :stroke-width="1.75" />
          </NuxtLink>
        </li>
      </ul>

      <!-- Pagination -->
      <div v-if="total > pageSize" class="mt-4 flex items-center justify-between text-xs text-ink-500">
        <p>Halaman <strong class="text-ink-900">{{ page }}</strong> dari <strong class="text-ink-900">{{ totalPages }}</strong> · total <strong class="text-ink-900">{{ total }}</strong></p>
        <div class="flex gap-1">
          <button
            type="button"
            :disabled="page <= 1"
            class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            @click="page--"
          >
            ← Sebelumnya
          </button>
          <button
            type="button"
            :disabled="page >= totalPages"
            class="rounded-md border border-hairline bg-canvas px-3 py-1.5 text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            @click="page++"
          >
            Selanjutnya →
          </button>
        </div>
      </div>
    </div>

    <!-- Track any resi helper -->
    <div class="mt-10 rounded-lg border border-hairline bg-canvas p-5 flex items-start gap-3">
      <Search class="h-4 w-4 text-ink-500 mt-0.5 flex-none" :stroke-width="1.75" />
      <div class="flex-1 text-sm">
        <p class="text-ink-700">Ingin cek resi milik teman/kerabat?</p>
        <p class="mt-0.5 text-xs text-ink-500 leading-relaxed">
          Halaman <NuxtLink to="/lacak" class="text-brand-700 hover:underline">Lacak resi</NuxtLink>
          publik bisa dipakai untuk cek status pesanan siapa saja (data sensitif disamarkan).
        </p>
      </div>
    </div>
  </main>
</template>
