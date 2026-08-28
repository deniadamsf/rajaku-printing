<script setup lang="ts">
/**
 * /akun/membership — Card status membership customer (CLAUDE.md §30.4).
 *
 * PENTING soal path: notifikasi WA yang dikirim backend saat status berubah
 * memakai link `APP_BASE_URL + "/akun/membership"` (§30.2) — jangan pindahkan
 * file ini ke path lain tanpa mengubah juga template WA di backend.
 *
 * Design: patuh CLAUDE.md §26. Badge aktif pakai aksen gold.500 (§26.1 —
 * membership memang konteks premium yang cocok dengan token emas).
 */
import { Crown, Sparkles, Clock, ShieldOff, Loader2, ArrowLeft, Lock } from '@lucide/vue'
import { ApiError } from '~/composables/useApi'
import type { Membership } from '~/types/membership'

definePageMeta({
  middleware: ['customer-only'],
  layout: 'default',
})

useSeoMeta({
  title: 'Membership Saya',
  robots: 'noindex,nofollow',
})

const membershipSvc = useMembership()

const membership = ref<Membership | null>(null)
const loading = ref(true)
const errorMsg = ref<string | null>(null)
const applying = ref(false)
const applyError = ref<string | null>(null)

async function fetchStatus() {
  loading.value = true
  errorMsg.value = null
  try {
    membership.value = await membershipSvc.getOwn()
  } catch (e) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal memuat status membership'
  } finally {
    loading.value = false
  }
}

onMounted(fetchStatus)

async function onApply() {
  applying.value = true
  applyError.value = null
  try {
    membership.value = await membershipSvc.apply()
  } catch (e) {
    applyError.value = e instanceof ApiError ? e.message : 'Gagal mengajukan membership'
  } finally {
    applying.value = false
  }
}

function fmtDate(s?: string | null): string {
  if (!s) return '—'
  try {
    return new Date(s).toLocaleDateString('id-ID', { day: '2-digit', month: 'long', year: 'numeric' })
  } catch {
    return s
  }
}
</script>

<template>
  <main class="mx-auto max-w-2xl px-4 py-10 md:py-16">
    <NuxtLink to="/akun" class="inline-flex items-center gap-1.5 text-xs font-medium text-ink-500 hover:text-ink-900 transition-colors">
      <ArrowLeft class="h-3.5 w-3.5" :stroke-width="1.75" />
      Kembali ke akun
    </NuxtLink>

    <p class="mt-4 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-2 flex items-center gap-2">
      <Crown class="h-3.5 w-3.5" :stroke-width="1.75" />
      Membership
    </p>
    <h1 class="font-serif text-2xl md:text-3xl font-semibold tracking-tight text-ink-950">
      Status Membership
    </h1>
    <p class="mt-2 text-sm text-ink-500 leading-relaxed">
      Member Rajaku Printing dapat akses diskon khusus yang tidak ditawarkan ke pelanggan umum.
    </p>

    <AlertMessage v-if="errorMsg" variant="error" :message="errorMsg" class="mt-6" />

    <div v-if="loading" class="mt-6 rounded-lg border border-hairline bg-canvas p-10 text-center">
      <Loader2 class="mx-auto h-5 w-5 animate-spin text-ink-500" :stroke-width="1.75" />
      <p class="mt-3 text-sm text-ink-500">Memuat status…</p>
    </div>

    <div v-else-if="membership" class="mt-6 rounded-lg border border-hairline bg-canvas p-6 md:p-8">
      <!-- none / rejected, tapi fitur sedang dimatikan admin (§30.1) — jangan
           tawarkan tombol yang pasti ditolak backend (ErrMembershipDisabled). -->
      <template v-if="(membership.status === 'none' || membership.status === 'rejected') && !membership.membership_enabled">
        <div class="flex items-start gap-3">
          <Lock class="h-6 w-6 text-ink-400 flex-none" :stroke-width="1.5" />
          <div class="flex-1">
            <h2 class="font-serif text-lg font-semibold text-ink-950">Program membership belum tersedia</h2>
            <p class="mt-1 text-sm text-ink-500 leading-relaxed">
              Pendaftaran member sedang tidak dibuka untuk sementara. Coba lagi lain waktu.
            </p>
          </div>
        </div>
      </template>

      <!-- none / rejected → bisa ajukan -->
      <template v-else-if="membership.status === 'none' || membership.status === 'rejected'">
        <div class="flex items-start gap-3">
          <Sparkles class="h-6 w-6 text-brand-500 flex-none" :stroke-width="1.5" />
          <div class="flex-1">
            <h2 class="font-serif text-lg font-semibold text-ink-950">
              {{ membership.status === 'rejected' ? 'Pengajuan sebelumnya belum disetujui' : 'Belum jadi member' }}
            </h2>
            <p class="mt-1 text-sm text-ink-500 leading-relaxed">
              Ajukan diri jadi member — pengajuan Anda akan ditinjau oleh tim kami.
            </p>
          </div>
        </div>

        <div v-if="membership.status === 'rejected' && membership.decision_note" class="mt-4 rounded-md border border-hairline bg-canvas-alt p-3">
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Catatan penolakan sebelumnya</p>
          <p class="mt-1 text-sm text-ink-700 leading-relaxed">{{ membership.decision_note }}</p>
        </div>

        <AlertMessage v-if="applyError" variant="error" :message="applyError" class="mt-4" />

        <button
          type="button"
          :disabled="applying"
          class="mt-5 inline-flex items-center gap-2 rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-60"
          @click="onApply"
        >
          <Loader2 v-if="applying" class="h-4 w-4 animate-spin" :stroke-width="1.75" />
          <Sparkles v-else class="h-4 w-4" :stroke-width="1.75" />
          {{ membership.status === 'rejected' ? 'Ajukan ulang' : 'Ajukan jadi Member' }}
        </button>
      </template>

      <!-- pending -->
      <template v-else-if="membership.status === 'pending'">
        <div class="flex items-start gap-3">
          <Clock class="h-6 w-6 text-amber-600 flex-none" :stroke-width="1.5" />
          <div class="flex-1">
            <h2 class="font-serif text-lg font-semibold text-ink-950">Pengajuan sedang ditinjau</h2>
            <p class="mt-1 text-sm text-ink-500 leading-relaxed">
              Diajukan pada {{ fmtDate(membership.requested_at) }}. Kami akan mengirim WA begitu ada keputusan.
            </p>
          </div>
        </div>
        <span class="mt-4 inline-flex items-center rounded-full bg-amber-50 px-2.5 py-1 text-xs font-medium text-amber-800 ring-1 ring-inset ring-amber-200">
          Menunggu Persetujuan
        </span>
      </template>

      <!-- active — aksen gold, konteks premium (§26.1) -->
      <template v-else-if="membership.status === 'active'">
        <div class="flex items-start gap-3">
          <Crown class="h-6 w-6 text-gold-500 flex-none" :stroke-width="1.5" />
          <div class="flex-1">
            <h2 class="font-serif text-lg font-semibold text-ink-950">Anda member Rajaku Printing</h2>
            <p class="mt-1 text-sm text-ink-500 leading-relaxed">
              Member sejak {{ fmtDate(membership.decided_at) }}. Diskon khusus member otomatis muncul saat checkout.
            </p>
          </div>
        </div>
        <span class="mt-4 inline-flex items-center gap-1.5 rounded-full bg-gold-50 px-2.5 py-1 text-xs font-medium text-gold-900 ring-1 ring-inset ring-gold-200">
          <Crown class="h-3 w-3" :stroke-width="1.75" />
          Member Aktif
        </span>
      </template>

      <!-- revoked — netral, tidak self-service -->
      <template v-else-if="membership.status === 'revoked'">
        <div class="flex items-start gap-3">
          <ShieldOff class="h-6 w-6 text-ink-400 flex-none" :stroke-width="1.5" />
          <div class="flex-1">
            <h2 class="font-serif text-lg font-semibold text-ink-950">Membership dicabut</h2>
            <p class="mt-1 text-sm text-ink-500 leading-relaxed">
              Status membership Anda saat ini tidak aktif. Hubungi admin kami kalau ingin meninjau ulang keputusan ini —
              pemulihan hanya bisa dilakukan lewat admin, tidak bisa diajukan ulang sendiri.
            </p>
          </div>
        </div>
        <div v-if="membership.decision_note" class="mt-4 rounded-md border border-hairline bg-canvas-alt p-3">
          <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Catatan</p>
          <p class="mt-1 text-sm text-ink-700 leading-relaxed">{{ membership.decision_note }}</p>
        </div>
        <span class="mt-4 inline-flex items-center rounded-full bg-ink-100 px-2.5 py-1 text-xs font-medium text-ink-700 ring-1 ring-inset ring-ink-200">
          Dicabut
        </span>
      </template>
    </div>
  </main>
</template>
