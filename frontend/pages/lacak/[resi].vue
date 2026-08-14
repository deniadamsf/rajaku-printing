<script setup lang="ts">
/**
 * /lacak/[resi] — Public tracking page (§5).
 *
 * Anyone with resi bisa buka — data sensitif (alamat, no HP) sudah di-sensor
 * di backend (`shipping_address="Jl. Merdek**"`, `shipping_phone="0812****678"`).
 * History status ditampilkan sebagai timeline dgn label human-readable.
 */
import {
  Package,
  MapPin,
  Truck,
  Search,
  AlertTriangle,
  CheckCircle2,
  CircleDot,
  Circle,
  Loader2,
  ExternalLink,
} from '@lucide/vue'
import { ApiError } from '~/composables/useApi'
import type { PublicTracking } from '~/types/tracking'

definePageMeta({ layout: 'default' })

const route = useRoute()
const orderApi = useOrder()

const resi = computed(() => {
  const r = route.params.resi
  return typeof r === 'string' ? r.toUpperCase() : ''
})

useSeoMeta({
  title: () => `Lacak ${resi.value} — Rajaku Printing`,
  description: 'Cek status pesanan Anda di Rajaku Printing.',
  robots: 'noindex,nofollow', // tracking pages personal — jangan ke-index Google
})

// -------------------- fetch --------------------
const state = ref<'loading' | 'ok' | 'error'>('loading')
const error = ref<string | null>(null)
const data = ref<PublicTracking | null>(null)

async function load() {
  state.value = 'loading'
  error.value = null
  try {
    data.value = await orderApi.publicTracking(resi.value)
    state.value = 'ok'
  } catch (e) {
    state.value = 'error'
    error.value = e instanceof ApiError ? e.message : 'Gagal memuat data tracking'
  }
}

onMounted(load)

// -------------------- helpers --------------------
// Status labels sesuai §4 state machine.
const statusLabel: Record<string, string> = {
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

function labelOf(status: string): string {
  return statusLabel[status] ?? status
}

// Icon per status milestone di timeline.
function statusIcon(status: string, isCurrent: boolean) {
  if (status === 'selesai') return CheckCircle2
  if (isCurrent) return CircleDot
  return Circle
}

function fmtDateTime(iso?: string): string {
  if (!iso) return ''
  try {
    return new Date(iso).toLocaleString('id-ID', {
      day: '2-digit', month: 'short', year: 'numeric',
      hour: '2-digit', minute: '2-digit',
    })
  } catch {
    return iso
  }
}

// Kirim / pickup subtitle di header.
const fulfillmentLabel = computed(() => {
  if (!data.value) return ''
  return data.value.metode_ambil === 'kirim' ? 'Kirim' : 'Ambil di tempat'
})

// Sorted history — backend already ordered chronologically, but be defensive.
const sortedHistory = computed(() => {
  if (!data.value) return []
  return [...data.value.history].sort(
    (a, b) => new Date(a.changed_at).getTime() - new Date(b.changed_at).getTime(),
  )
})

// Current status = last history row's status (or data.status fallback).
const currentStatus = computed(() => data.value?.status || sortedHistory.value.at(-1)?.status || '')

// Copy resi to clipboard convenience.
const copied = ref(false)
async function copyResi() {
  if (!data.value) return
  try {
    await navigator.clipboard.writeText(data.value.resi)
    copied.value = true
    setTimeout(() => (copied.value = false), 1500)
  } catch {
    // no-op
  }
}
</script>

<template>
  <main class="mx-auto max-w-3xl px-4 py-10 md:py-16">
    <!-- Header eyebrow -->
    <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2 flex items-center gap-2">
      <Search class="h-3.5 w-3.5" :stroke-width="1.75" />
      Lacak Pesanan
    </p>

    <!-- Loading -->
    <div v-if="state === 'loading'" class="rounded-lg border border-hairline bg-canvas p-12 text-center">
      <Loader2 class="mx-auto h-5 w-5 animate-spin text-ink-500" :stroke-width="1.75" />
      <p class="mt-3 text-sm text-ink-500">Memuat data resi <span class="font-mono">{{ resi }}</span>…</p>
    </div>

    <!-- Error -->
    <div v-else-if="state === 'error'" class="rounded-lg border border-brand-200 bg-brand-50/50 p-8">
      <div class="flex items-start gap-3">
        <AlertTriangle class="h-5 w-5 text-brand-700 flex-none mt-0.5" :stroke-width="1.75" />
        <div class="flex-1">
          <h1 class="font-serif text-xl font-semibold text-ink-950">Resi tidak ditemukan</h1>
          <p class="mt-1 text-sm text-ink-700 leading-relaxed">{{ error }}</p>
          <p class="mt-2 text-xs text-ink-500 leading-relaxed">
            Kemungkinan: salah ketik, atau order belum tercatat di sistem. Format resi: <span class="font-mono">RJK-XXXXXXXX</span>.
          </p>
          <div class="mt-4 flex flex-wrap gap-2">
            <NuxtLink
              to="/lacak"
              class="inline-flex items-center gap-1 rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
            >
              <Search class="h-3.5 w-3.5" :stroke-width="1.75" />
              Coba resi lain
            </NuxtLink>
            <NuxtLink
              to="/order"
              class="inline-flex items-center gap-1 rounded-md bg-brand-500 px-3 py-1.5 text-sm font-semibold text-canvas hover:bg-brand-600 transition-colors"
            >
              Order baru
            </NuxtLink>
          </div>
        </div>
      </div>
    </div>

    <!-- OK -->
    <div v-else-if="data" class="space-y-6">
      <!-- Header card -->
      <div class="rounded-lg border border-hairline bg-canvas p-6 md:p-8">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <button
              type="button"
              class="group flex items-center gap-2 text-left"
              @click="copyResi"
            >
              <span class="font-serif text-2xl md:text-3xl font-semibold tracking-tight text-ink-950">{{ data.resi }}</span>
              <span
                :class="[
                  'text-[10px] font-medium uppercase tracking-[0.14em] px-2 py-0.5 rounded-full ring-1 ring-inset transition-colors',
                  copied ? 'text-emerald-800 bg-emerald-50 ring-emerald-200' : 'text-ink-500 bg-canvas-alt ring-hairline group-hover:text-ink-900',
                ]"
              >
                {{ copied ? 'Disalin' : 'Salin' }}
              </span>
            </button>
            <p class="mt-1 text-sm text-ink-500">
              Dibuat {{ fmtDateTime(data.created_at) }}
              <span class="text-ink-400">·</span> Channel <span class="capitalize text-ink-700">{{ data.channel }}</span>
            </p>
          </div>
          <div class="text-right">
            <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Status sekarang</p>
            <p class="mt-0.5 font-serif text-lg font-semibold text-ink-950">{{ labelOf(currentStatus) }}</p>
          </div>
        </div>

        <div class="mt-6 grid gap-4 sm:grid-cols-2 text-sm">
          <div class="flex items-start gap-2">
            <Package class="h-4 w-4 text-ink-500 mt-0.5" :stroke-width="1.75" />
            <div>
              <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Produk</p>
              <p class="mt-0.5 text-ink-900">{{ data.product_name }}</p>
              <p class="text-xs text-ink-500">Bahan: {{ data.material_name }}</p>
            </div>
          </div>
          <div class="flex items-start gap-2">
            <Truck class="h-4 w-4 text-ink-500 mt-0.5" :stroke-width="1.75" />
            <div>
              <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Pengambilan</p>
              <p class="mt-0.5 text-ink-900">{{ fulfillmentLabel }}</p>
              <p v-if="data.shipping_recipient" class="text-xs text-ink-500">
                Penerima: {{ data.shipping_recipient }}
              </p>
            </div>
          </div>
        </div>

        <!-- Shipping address (masked) — hanya kalau kirim -->
        <div v-if="data.metode_ambil === 'kirim' && (data.shipping_address || data.shipping_phone)"
             class="mt-4 rounded-md border border-hairline bg-canvas-alt p-3 text-xs text-ink-700">
          <div class="flex items-start gap-2">
            <MapPin class="h-3.5 w-3.5 text-ink-500 mt-0.5 flex-none" :stroke-width="1.75" />
            <div class="min-w-0">
              <p v-if="data.shipping_address" class="text-ink-700">{{ data.shipping_address }}</p>
              <p v-if="data.shipping_phone" class="mt-0.5 font-mono text-[11px] text-ink-500">
                {{ data.shipping_phone }}
              </p>
              <p class="mt-1 text-[10px] text-ink-400">
                Alamat & nomor disamarkan untuk privasi. Detail lengkap tersedia di akun pemesan.
              </p>
            </div>
          </div>
        </div>
      </div>

      <!-- Timeline -->
      <div class="rounded-lg border border-hairline bg-canvas p-6 md:p-8">
        <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-4">Riwayat status</p>
        <ol class="relative border-l border-hairline pl-6 space-y-5">
          <li
            v-for="(row, idx) in sortedHistory"
            :key="row.changed_at + row.status"
            class="relative"
          >
            <span
              :class="[
                'absolute -left-[27px] flex h-4 w-4 items-center justify-center rounded-full',
                idx === sortedHistory.length - 1
                  ? 'bg-brand-500 ring-4 ring-brand-500/15'
                  : 'bg-ink-950/80 ring-4 ring-canvas-alt',
              ]"
            >
              <component
                :is="statusIcon(row.status, idx === sortedHistory.length - 1)"
                class="h-3 w-3 text-canvas"
                :stroke-width="2"
              />
            </span>
            <div class="flex flex-wrap items-baseline justify-between gap-2">
              <p
                :class="[
                  'text-sm',
                  idx === sortedHistory.length - 1 ? 'font-semibold text-ink-950' : 'text-ink-700',
                ]"
              >
                {{ labelOf(row.status) }}
              </p>
              <time class="font-mono text-[11px] text-ink-500">{{ fmtDateTime(row.changed_at) }}</time>
            </div>
          </li>
        </ol>
      </div>

      <!-- Action footer -->
      <div class="flex flex-wrap gap-2">
        <NuxtLink
          to="/lacak"
          class="inline-flex items-center gap-1 rounded-md border border-hairline bg-canvas px-4 py-2 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors"
        >
          <Search class="h-4 w-4" :stroke-width="1.75" />
          Cek resi lain
        </NuxtLink>
        <NuxtLink
          to="/order"
          class="inline-flex items-center gap-1 rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 transition-colors"
        >
          <ExternalLink class="h-4 w-4" :stroke-width="1.75" />
          Order baru
        </NuxtLink>
      </div>
    </div>
  </main>
</template>
