<script setup lang="ts">
/**
 * /admin/whatsapp — pairing Baileys (WA gateway) dari browser, tanpa SSH.
 *
 * Tiga keadaan (lihat docblock useAdminWhatsApp.ts untuk kontrak lengkap):
 *   1. worker_reachable:false → layanan notifikasi mati total, tidak ada QR/unlink.
 *   2. connected:true         → tertaut, tampilkan self_number (font-mono).
 *   3. connected:false        → menunggu scan, tampilkan QR + polling 10 detik.
 *
 * Polling HANYA jalan selagi halaman terbuka DAN belum tertaut — dihentikan
 * begitu tertaut atau saat unmount (lihat stopPolling/onBeforeUnmount).
 *
 * Endpoint (GET & POST) sama-sama di-gate permission `notification.manage` di
 * backend — kalau user tidak punya izin, kita tidak memanggil API sama sekali
 * (bukan cuma menyembunyikan tombol), cukup tampilkan banner penjelas — pola
 * yang sama seperti /admin/site-media untuk aksi yang tidak diizinkan.
 *
 * Design: patuh CLAUDE.md §26 — anchor pattern sama dengan pages/admin/site-media.
 */
import {
  QrCode,
  Smartphone,
  WifiOff,
  ShieldAlert,
  RefreshCw,
  Loader2,
  Unlink,
  TriangleAlert,
} from '@lucide/vue'
import { ApiError } from '~/composables/useApi'
import { useAdminWhatsApp, type WhatsAppPairingStatus } from '~/composables/useAdminWhatsApp'

definePageMeta({
  middleware: ['staff-only'],
  layout: 'admin',
})

useSeoMeta({ title: 'Pairing WhatsApp — Rajaku Admin', robots: 'noindex,nofollow' })

const auth = useAuthStore()
const wa = useAdminWhatsApp()

const canManage = computed(() => auth.hasPermission('notification.manage'))

const status = ref<WhatsAppPairingStatus | null>(null)
const loading = ref(false)
const refreshing = ref(false)
const errorMsg = ref('')
const successMsg = ref('')

let pollTimer: ReturnType<typeof setInterval> | null = null

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

function startPolling() {
  if (pollTimer) return
  pollTimer = setInterval(() => fetchStatus({ silent: true }), 10_000)
}

async function fetchStatus(opts: { silent?: boolean } = {}) {
  if (!canManage.value) return
  if (opts.silent) refreshing.value = true
  else loading.value = true
  errorMsg.value = ''
  try {
    status.value = await wa.getPairing()
    // Polling hanya relevan selama belum tertaut (§ kontrak) — mencakup baik
    // "menunggu scan" maupun "worker mati", supaya otomatis pulih begitu
    // worker hidup lagi/QR baru terbit tanpa operator harus reload manual.
    if (status.value.connected) {
      stopPolling()
    } else {
      startPolling()
    }
  } catch (e) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal memuat status pairing WhatsApp'
    // Request benar-benar gagal (bukan worker_reachable:false yang tetap 200) —
    // hentikan polling supaya tidak menghajar backend yang sedang bermasalah.
    stopPolling()
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

const viewState = computed<'unreachable' | 'connected' | 'waiting' | null>(() => {
  if (!status.value) return null
  if (!status.value.worker_reachable) return 'unreachable'
  if (status.value.connected) return 'connected'
  return 'waiting'
})

function entries(obj: Record<string, unknown> | null | undefined): Array<[string, unknown]> {
  return obj ? Object.entries(obj) : []
}

function formatValue(v: unknown): string {
  if (v === null || v === undefined || v === '') return '—'
  if (typeof v === 'object') return JSON.stringify(v)
  return String(v)
}

function humanizeKey(k: string): string {
  return k.replace(/_/g, ' ')
}

const hasTechInfo = computed(() => {
  if (!status.value) return false
  return (
    entries(status.value.quota).length > 0 ||
    entries(status.value.circuit).length > 0 ||
    entries(status.value.pacing).length > 0
  )
})

// --- Putuskan pairing (destruktif) ---
const unlinkOpen = ref(false)
const unlinking = ref(false)

function askUnlink() {
  unlinkOpen.value = true
}

async function confirmUnlink() {
  unlinking.value = true
  errorMsg.value = ''
  try {
    await wa.unlink()
    successMsg.value = 'Pairing WhatsApp diputus. Menunggu QR baru untuk dipindai.'
    setTimeout(() => (successMsg.value = ''), 4000)
    await fetchStatus()
  } catch (e) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal memutus pairing WhatsApp'
  } finally {
    unlinking.value = false
    unlinkOpen.value = false
  }
}

onMounted(() => {
  if (canManage.value) fetchStatus()
})
onBeforeUnmount(stopPolling)
</script>

<template>
  <section>
    <AdminPageHeader
      title="Pairing WhatsApp"
      subtitle="Tautkan nomor WhatsApp toko untuk notifikasi Baileys langsung dari browser — tidak perlu SSH ke server lagi."
    >
      <template #actions>
        <button
          v-if="canManage"
          type="button"
          :disabled="loading || refreshing"
          class="inline-flex items-center gap-2 rounded-md border border-hairline bg-canvas px-3 py-2 text-sm font-medium text-ink-700 transition-colors hover:border-ink-300 hover:bg-canvas-alt focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas disabled:cursor-not-allowed disabled:opacity-50"
          @click="fetchStatus()"
        >
          <Loader2 v-if="refreshing" class="h-4 w-4 animate-spin" :stroke-width="1.75" />
          <RefreshCw v-else class="h-4 w-4" :stroke-width="1.75" />
          Muat ulang
        </button>
      </template>
    </AdminPageHeader>

    <AlertMessage v-if="errorMsg" variant="error" :message="errorMsg" class="mb-4" />
    <AlertMessage v-if="successMsg" variant="success" :message="successMsg" class="mb-4" />

    <div
      v-if="!canManage"
      class="mb-6 flex items-start gap-2 rounded-md border border-hairline bg-canvas-alt p-4 text-xs text-ink-500"
    >
      <TriangleAlert class="mt-0.5 h-4 w-4 shrink-0" :stroke-width="1.5" />
      <p>Anda tidak punya izin melihat atau mengelola pairing WhatsApp — hubungi staff yang punya izin <span class="font-mono">notification.manage</span>.</p>
    </div>

    <template v-else>
      <div v-if="loading" class="rounded-lg border border-hairline bg-canvas p-8 text-center">
        <Loader2 class="mx-auto h-5 w-5 animate-spin text-ink-400" :stroke-width="1.5" />
        <p class="mt-2 text-xs text-ink-500">Memuat status pairing…</p>
      </div>

      <template v-else-if="status">
        <!-- Keadaan 1: worker mati total — tidak ada QR/unlink -->
        <div v-if="viewState === 'unreachable'" class="rounded-lg border border-brand-200 bg-brand-50 p-6 md:p-8">
          <div class="flex items-start gap-3">
            <WifiOff class="mt-0.5 h-6 w-6 shrink-0 text-brand-600" :stroke-width="1.5" />
            <div>
              <h3 class="text-sm font-semibold text-ink-950">Layanan notifikasi WhatsApp tidak bisa dihubungi</h3>
              <p class="mt-1.5 text-sm leading-relaxed text-ink-700">
                WhatsApp sedang tidak berjalan sama sekali — semua notifikasi ke pelanggan (verifikasi pembayaran,
                status pesanan, resi, invoice) <strong>tidak akan terkirim</strong> sampai layanan ini pulih.
                Hubungi teknisi untuk memeriksa <span class="font-mono text-xs">notification-worker</span> di server.
              </p>
            </div>
          </div>
        </div>

        <!-- Keadaan 2: tertaut -->
        <div v-else-if="viewState === 'connected'" class="rounded-lg border border-hairline bg-canvas p-6 md:p-8">
          <div class="flex items-start gap-3">
            <span class="mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-emerald-50 ring-1 ring-inset ring-emerald-200">
              <Smartphone class="h-5 w-5 text-emerald-700" :stroke-width="1.5" />
            </span>
            <div>
              <div class="flex items-center gap-2">
                <h3 class="text-sm font-semibold text-ink-950">WhatsApp tertaut</h3>
                <span class="inline-flex items-center rounded-full bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-800 ring-1 ring-inset ring-emerald-200">
                  Aktif
                </span>
              </div>
              <p class="mt-1 text-sm text-ink-500">Nomor toko yang terhubung:</p>
              <p class="mt-0.5 font-mono text-sm text-ink-900">{{ status.self_number ?? '—' }}</p>
            </div>
          </div>

          <div class="mt-6 border-t border-hairline pt-4">
            <button
              type="button"
              :disabled="unlinking"
              class="inline-flex items-center gap-2 rounded-md border border-hairline bg-canvas px-3 py-2 text-sm font-medium text-ink-700 transition-colors hover:border-brand-300 hover:bg-brand-50 hover:text-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas disabled:cursor-not-allowed disabled:opacity-50"
              @click="askUnlink"
            >
              <Unlink class="h-4 w-4" :stroke-width="1.75" />
              Putuskan pairing
            </button>
          </div>
        </div>

        <!-- Keadaan 3: menunggu scan -->
        <div v-else class="rounded-lg border border-hairline bg-canvas p-6 md:p-8">
          <div class="flex items-center gap-2">
            <QrCode class="h-5 w-5 text-ink-600" :stroke-width="1.5" />
            <h3 class="text-sm font-semibold text-ink-950">Menunggu pemindaian QR</h3>
          </div>
          <p class="mt-1 text-xs text-ink-500">
            QR dari WhatsApp kedaluwarsa cepat — halaman ini otomatis memuat ulang tiap 10 detik sampai tertaut.
          </p>

          <div class="mt-6 flex flex-col items-center gap-6 sm:flex-row sm:items-start">
            <div class="flex h-56 w-56 shrink-0 items-center justify-center rounded-lg border border-hairline bg-canvas-alt p-4">
              <img
                v-if="status.qr_data_url"
                :src="status.qr_data_url"
                alt="QR pairing WhatsApp"
                class="h-full w-full object-contain"
              />
              <p v-else class="text-center text-xs text-ink-400">QR belum tersedia, menunggu layanan menerbitkannya…</p>
            </div>

            <ol class="list-decimal space-y-1.5 pl-4 text-sm leading-relaxed text-ink-700">
              <li>Buka WhatsApp di HP dengan nomor toko.</li>
              <li>Ketuk menu titik tiga di pojok kanan atas.</li>
              <li>Pilih <strong>Perangkat Tertaut</strong>.</li>
              <li>Ketuk <strong>Tautkan Perangkat</strong>.</li>
              <li>Arahkan kamera HP ke QR di sebelah kiri.</li>
            </ol>
          </div>

          <div class="mt-6 flex items-start gap-2 rounded-md border border-hairline bg-canvas-alt p-3 text-xs text-ink-500">
            <ShieldAlert class="mt-0.5 h-4 w-4 shrink-0 text-ink-400" :stroke-width="1.5" />
            <p>QR ini setara kredensial toko — siapa pun yang memindainya bisa mengirim pesan atas nama toko. Jangan difoto atau dibagikan selagi tampil di layar.</p>
          </div>
        </div>

        <!-- Info teknis opsional dari worker (bebas bentuknya) -->
        <div v-if="hasTechInfo" class="mt-6 grid gap-4 sm:grid-cols-3">
          <div v-if="entries(status.quota).length" class="rounded-lg border border-hairline bg-canvas-alt p-4">
            <h4 class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Kuota</h4>
            <dl class="mt-2 space-y-1">
              <div v-for="[k, v] in entries(status.quota)" :key="k" class="flex items-center justify-between gap-2 text-xs">
                <dt class="capitalize text-ink-500">{{ humanizeKey(k) }}</dt>
                <dd class="font-mono text-ink-900">{{ formatValue(v) }}</dd>
              </div>
            </dl>
          </div>
          <div v-if="entries(status.circuit).length" class="rounded-lg border border-hairline bg-canvas-alt p-4">
            <h4 class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Circuit Breaker</h4>
            <dl class="mt-2 space-y-1">
              <div v-for="[k, v] in entries(status.circuit)" :key="k" class="flex items-center justify-between gap-2 text-xs">
                <dt class="capitalize text-ink-500">{{ humanizeKey(k) }}</dt>
                <dd class="font-mono text-ink-900">{{ formatValue(v) }}</dd>
              </div>
            </dl>
          </div>
          <div v-if="entries(status.pacing).length" class="rounded-lg border border-hairline bg-canvas-alt p-4">
            <h4 class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Pacing</h4>
            <dl class="mt-2 space-y-1">
              <div v-for="[k, v] in entries(status.pacing)" :key="k" class="flex items-center justify-between gap-2 text-xs">
                <dt class="capitalize text-ink-500">{{ humanizeKey(k) }}</dt>
                <dd class="font-mono text-ink-900">{{ formatValue(v) }}</dd>
              </div>
            </dl>
          </div>
        </div>
      </template>
    </template>

    <AdminConfirmDialog
      v-model:open="unlinkOpen"
      title="Putuskan pairing WhatsApp?"
      message="Nomor toko akan diputus dari sistem. Selama belum ditautkan ulang lewat QR baru, TIDAK ADA notifikasi WhatsApp (verifikasi pembayaran, status pesanan, resi, invoice) yang akan terkirim ke pelanggan."
      confirm-label="Ya, putuskan"
      variant="danger"
      :loading="unlinking"
      @confirm="confirmUnlink"
    />
  </section>
</template>
