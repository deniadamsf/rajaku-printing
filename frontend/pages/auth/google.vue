<script setup lang="ts">
/**
 * /auth/google — landing pasca redirect OAuth Google dari backend.
 *
 * Backend selalu kirim user ke sini dengan `?code=<one-time>` (sukses).
 * Kegagalan OAuth (state mismatch, dibatalkan, dsb) diarahkan backend ke
 * `/login?oauth_error=<slug>` — bukan ke sini — tapi halaman ini tetap punya
 * state error untuk kasus `code` hilang/kadaluarsa/exchange gagal.
 *
 * Publik — TIDAK boleh kena middleware `guest`/`auth` (user belum tentu
 * punya sesi saat mendarat di sini).
 *
 * Alur akun baru (nomor WA OPSIONAL — owner batasi kirim OTP demi jaga
 * nomor WA gateway/Baileys tidak diblokir, lihat `useGoogleAuth`):
 *  1. `details_form` — isi nama (wajib) + nomor WhatsApp (opsional, bisa
 *     "Lewati dulu"). Kalau nomor diisi → POST /auth/google/request-otp
 *     untuk cek nomor bebas/tabrakan.
 *     - Bebas (`otp_required:false`) → langsung POST /auth/google/complete
 *       TANPA `otp` — user tidak pernah melihat form OTP.
 *     - Kosong/dilewati → langsung POST /auth/google/complete tanpa `phone`.
 *  2. `phone_conflict` — nomor sudah dipakai akun lain (OTP sudah terkirim
 *     saat request-otp menjawab `otp_required:true`). Tawarkan dua jalan:
 *     pakai nomor lain (kembali ke `details_form`), atau verifikasi.
 *  3. `otp_form` — masukkan kode 6 digit → POST /auth/google/complete
 *     (menyertakan `otp`) → sesi terbentuk.
 *
 * Design: patuh CLAUDE.md §26 (Fraunces + Inter + Lucide, brand/ink/hairline).
 */
import { Loader2, AlertTriangle, ShieldCheck, ArrowLeft, Mail, KeyRound, RotateCw, SkipForward, Info, MessageCircle } from '@lucide/vue'
import { ApiError } from '~/composables/useApi'
import { isValidIndonesianPhone, maskPhoneDisplay } from '~/utils/phone'
import type { GoogleAuthSession } from '~/types/auth'

definePageMeta({
  layout: 'default',
})

useSeoMeta({
  title: 'Menyelesaikan login Google',
  robots: 'noindex,nofollow',
})

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const google = useGoogleAuth()

type Phase = 'processing' | 'details_form' | 'phone_conflict' | 'otp_form' | 'error'
const phase = ref<Phase>('processing')
const errorMessage = ref('')
/** Code envelope error dari langkah gagal terakhir — dipakai buat tombol khusus. */
const errorCode = ref<string | null>(null)

// -------------------- Data OAuth pending --------------------
const pendingCode = ref('')
const googleEmail = ref('')
const form = reactive({ name: '', phone: '' })

// -------------------- Sub-state: details_form --------------------
const formError = ref('')
const submitting = ref(false)
/** Ref ke komponen `<BaseInput>` (bukan elemen DOM langsung) — exposes `focus()`. */
const phoneInputRef = ref<{ focus: () => void } | null>(null)

const phoneError = computed(() => {
  if (!form.phone) return ''
  return isValidIndonesianPhone(form.phone) ? '' : 'Format nomor tidak valid. Contoh: 08xxxxxxxxxx.'
})

// -------------------- Sub-state: phone_conflict / otp_form --------------------
// Nomor yang sedang diverifikasi — snapshot terpisah dari `form.phone` supaya
// tidak ikut berubah kalau user balik ke details_form lalu edit lagi.
const conflictPhone = ref('')
const phoneMasked = computed(() => maskPhoneDisplay(conflictPhone.value))
const otpCode = ref('')
const otpError = ref('')
const verifyingOtp = ref(false)
const attemptsExhausted = ref(false)
const resendIn = ref(0)
const otpInputRef = ref<HTMLInputElement | null>(null)

const canResend = computed(() => resendIn.value <= 0)

let resendTimer: ReturnType<typeof setInterval> | null = null

function restartResendCountdown(seconds: number) {
  resendIn.value = seconds
  if (resendTimer) clearInterval(resendTimer)
  resendTimer = setInterval(() => {
    if (resendIn.value > 0) {
      resendIn.value -= 1
    } else if (resendTimer) {
      clearInterval(resendTimer)
      resendTimer = null
    }
  }, 1000)
}

function clearAllTimers() {
  if (resendTimer) {
    clearInterval(resendTimer)
    resendTimer = null
  }
}

onUnmounted(clearAllTimers)

function applySessionAndRedirect(res: GoogleAuthSession) {
  clearAllTimers()
  auth.applyAuthResponse({ token: res.token, user: res.user })
  navigateTo(res.redirect_path || auth.homePath, { replace: true })
}

function goToSessionExpired() {
  clearAllTimers()
  phase.value = 'error'
  errorCode.value = 'OAUTH_CODE_INVALID'
  errorMessage.value = 'Sesi login dengan Google sudah kedaluwarsa. Silakan ulangi login.'
}

async function runExchange() {
  const codeParam = route.query.code
  const code = typeof codeParam === 'string' ? codeParam : ''

  // Bersihkan query segera setelah dibaca — `code` sekali pakai, jangan
  // sampai ikut history/refresh (refresh akan gagal exchange ulang).
  if (Object.keys(route.query).length > 0) {
    await router.replace({ query: {} })
  }

  if (!code) {
    phase.value = 'error'
    errorMessage.value = 'Link login tidak lengkap. Silakan login lagi.'
    return
  }

  try {
    const res = await google.exchangeCode(code)
    if (res.status === 'session') {
      applySessionAndRedirect(res)
      return
    }
    // need_phone — akun baru, minta lengkapi nama (nomor opsional).
    pendingCode.value = res.code
    googleEmail.value = res.email
    form.name = res.name || ''
    phase.value = 'details_form'
  } catch (e) {
    phase.value = 'error'
    errorCode.value = e instanceof ApiError ? e.code : null
    errorMessage.value = e instanceof ApiError ? e.message : 'Gagal menyelesaikan login Google.'
  }
}

onMounted(runExchange)

/**
 * Alur inti details_form: `phoneValue` kosong = dilewati (baik lewat tombol
 * "Lewati dulu" maupun field memang dikosongkan) → langsung complete tanpa
 * nomor. `phoneValue` terisi → cek dulu (request-otp) sebelum simpan.
 */
async function trySubmit(phoneValue: string) {
  if (submitting.value) return
  formError.value = ''

  if (!form.name.trim()) {
    formError.value = 'Nama wajib diisi.'
    return
  }
  if (phoneValue && !isValidIndonesianPhone(phoneValue)) {
    formError.value = 'Format nomor WhatsApp tidak valid.'
    return
  }

  submitting.value = true
  try {
    if (!phoneValue) {
      const res = await google.completeRegistration({ code: pendingCode.value, name: form.name.trim() })
      applySessionAndRedirect(res)
      return
    }

    const check = await google.requestOtp({ code: pendingCode.value, phone: phoneValue })
    if (!check.otp_required) {
      // Nomor bebas — TIDAK ada OTP terkirim, user tidak pernah melihat form OTP.
      const res = await google.completeRegistration({
        code: pendingCode.value,
        name: form.name.trim(),
        phone: phoneValue,
      })
      applySessionAndRedirect(res)
      return
    }

    // Nomor tabrakan — OTP sudah terkirim. Tawarkan dua jalan keluar.
    conflictPhone.value = phoneValue
    restartResendCountdown(check.resend_after_seconds ?? 30)
    phase.value = 'phone_conflict'
  } catch (e) {
    await handleSubmitError(e, phoneValue)
  } finally {
    submitting.value = false
  }
}

async function handleSubmitError(e: unknown, phoneValue: string) {
  if (e instanceof ApiError) {
    if (e.code === 'OAUTH_CODE_INVALID') {
      goToSessionExpired()
      return
    }
    // Jalan buntu: nomor terikat akun terdaftar/staff. OTP tidak akan bisa
    // memindahkannya, jadi jangan tawarkan verifikasi — biarkan user langsung
    // mengganti nomor di form yang masih terbuka.
    if (e.code === 'PHONE_ALREADY_USED') {
      formError.value =
        e.message || 'Nomor ini sudah terikat akun lain dan tidak bisa dipindahkan. Silakan pakai nomor lain.'
      return
    }
    // Race jarang terjadi: nomor baru saja dipakai orang lain antara
    // request-otp bebas dan complete. OTP belum tentu sudah terkirim di
    // titik ini — kirim ulang cek supaya kode benar-benar terkirim sebelum
    // menawarkan verifikasi.
    if (e.code === 'PHONE_ALREADY_IN_USE') {
      try {
        const check = await google.requestOtp({ code: pendingCode.value, phone: phoneValue })
        conflictPhone.value = phoneValue
        restartResendCountdown(check.resend_after_seconds ?? 30)
        phase.value = 'phone_conflict'
      } catch (e2) {
        formError.value = e2 instanceof ApiError ? e2.message : 'Nomor sudah dipakai akun lain. Coba nomor lain.'
      }
      return
    }
    formError.value = e.message
    return
  }
  formError.value = 'Gagal menyimpan data. Coba lagi.'
}

function onSubmitDetails() {
  trySubmit(form.phone.trim())
}

function onSkip() {
  form.phone = ''
  trySubmit('')
}

function onUseAnotherNumber() {
  clearAllTimers()
  form.phone = conflictPhone.value
  formError.value = ''
  phase.value = 'details_form'
  nextTick(() => phoneInputRef.value?.focus())
}

async function onChooseVerify() {
  phase.value = 'otp_form'
  otpCode.value = ''
  otpError.value = ''
  attemptsExhausted.value = false
  await nextTick()
  otpInputRef.value?.focus()
}

async function onResendOtp() {
  if (!canResend.value || submitting.value) return
  submitting.value = true
  try {
    const check = await google.requestOtp({ code: pendingCode.value, phone: conflictPhone.value })
    otpCode.value = ''
    otpError.value = ''
    attemptsExhausted.value = false
    restartResendCountdown(check.resend_after_seconds ?? 30)
    await nextTick()
    otpInputRef.value?.focus()
  } catch (e) {
    if (e instanceof ApiError && e.code === 'OAUTH_CODE_INVALID') {
      goToSessionExpired()
      return
    }
    otpError.value = e instanceof ApiError ? e.message : 'Gagal mengirim ulang kode. Coba lagi.'
  } finally {
    submitting.value = false
  }
}

function onChangeNumber() {
  clearAllTimers()
  form.phone = ''
  phase.value = 'details_form'
  otpCode.value = ''
  otpError.value = ''
  attemptsExhausted.value = false
}

function onOtpInput(e: Event) {
  const raw = (e.target as HTMLInputElement).value
  const digits = raw.replace(/\D/g, '').slice(0, 6)
  otpCode.value = digits
  ;(e.target as HTMLInputElement).value = digits
  if (digits.length === 6 && !verifyingOtp.value && !attemptsExhausted.value) {
    onSubmitOtp()
  }
}

function handleCompleteError(e: unknown) {
  if (!(e instanceof ApiError)) {
    otpError.value = 'Gagal memverifikasi kode. Coba lagi.'
    return
  }

  switch (e.code) {
    case 'OAUTH_CODE_INVALID':
      goToSessionExpired()
      return
    case 'OTP_INVALID': {
      const details = e.details as { attempts_left?: number } | undefined
      otpError.value =
        typeof details?.attempts_left === 'number'
          ? `Kode salah. Sisa ${details.attempts_left} percobaan.`
          : 'Kode verifikasi salah.'
      otpCode.value = ''
      nextTick(() => otpInputRef.value?.focus())
      return
    }
    case 'OTP_TOO_MANY_ATTEMPTS':
      attemptsExhausted.value = true
      otpCode.value = ''
      otpError.value = 'Percobaan verifikasi sudah habis. Minta kode baru untuk mencoba lagi.'
      return
    case 'OTP_EXPIRED':
      otpCode.value = ''
      otpError.value = 'Kode verifikasi sudah kedaluwarsa. Minta kode baru.'
      return
    case 'PHONE_ALREADY_IN_USE':
      // Sangat jarang: nomor terpakai orang lain persis di antara verifikasi.
      otpError.value = 'Nomor ini baru saja terpakai akun lain. Coba nomor lain.'
      return
    case 'PHONE_ALREADY_USED':
      // Jalan buntu: nomor terikat akun terdaftar/staff. Verifikasi tidak akan
      // pernah berhasil, jadi kunci form supaya user tidak menebak sia-sia.
      attemptsExhausted.value = true
      otpCode.value = ''
      otpError.value =
        'Nomor ini terikat akun lain dan tidak bisa dipindahkan. Kembali dan pakai nomor lain.'
      return
    default:
      otpError.value = e.message
  }
}

async function onSubmitOtp() {
  if (verifyingOtp.value || attemptsExhausted.value) return
  if (otpCode.value.length !== 6) {
    otpError.value = 'Masukkan 6 digit kode verifikasi.'
    return
  }

  otpError.value = ''
  verifyingOtp.value = true
  try {
    const res = await google.completeRegistration({
      code: pendingCode.value,
      name: form.name.trim(),
      phone: conflictPhone.value,
      otp: otpCode.value,
    })
    applySessionAndRedirect(res)
  } catch (e) {
    handleCompleteError(e)
  } finally {
    verifyingOtp.value = false
  }
}
</script>

<template>
  <main class="mx-auto max-w-md px-4 py-16 md:py-24">
    <!-- Memproses -->
    <div v-if="phase === 'processing'" class="rounded-lg border border-hairline bg-canvas p-8 text-center">
      <Loader2 class="mx-auto h-5 w-5 animate-spin text-ink-400" :stroke-width="1.5" />
      <p class="mt-3 text-sm text-ink-500">Menyelesaikan proses masuk…</p>
    </div>

    <!-- Sub-state 1: lengkapi nama (wajib) + nomor WA (opsional) -->
    <template v-else-if="phase === 'details_form'">
      <p class="mb-2 flex items-center gap-2 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
        <Mail class="h-3.5 w-3.5" :stroke-width="1.75" />
        Satu langkah lagi
      </p>
      <h1 class="font-serif text-2xl md:text-3xl font-semibold tracking-tight text-ink-950">
        Lengkapi data Anda
      </h1>
      <p class="mt-2 text-sm text-ink-500 leading-relaxed">
        Nomor WhatsApp dipakai untuk mengirim notifikasi status pesanan Anda. Bersifat opsional —
        boleh dilewati dan diisi kapan saja belakangan dari halaman akun.
      </p>

      <form class="mt-8 space-y-4" novalidate @submit.prevent="onSubmitDetails">
        <AlertMessage :message="formError" variant="error" />

        <div>
          <span class="block text-sm font-medium text-ink-900">Email Google</span>
          <p class="mt-1 rounded-md border border-hairline bg-canvas-alt px-3 py-2 text-xs font-mono text-ink-500">
            {{ googleEmail }}
          </p>
        </div>

        <BaseInput
          id="gname"
          v-model="form.name"
          label="Nama"
          autocomplete="name"
          required
          :disabled="submitting"
        />
        <BaseInput
          id="gphone"
          ref="phoneInputRef"
          v-model="form.phone"
          label="Nomor WhatsApp (opsional)"
          type="tel"
          autocomplete="tel"
          placeholder="08xxxxxxxxxx"
          :error="phoneError"
          helper="Otomatis dikonversi ke format 62xxx. Boleh dikosongkan."
          :disabled="submitting"
        />

        <div class="flex flex-col gap-2 pt-1">
          <button
            type="submit"
            :disabled="submitting"
            class="inline-flex w-full items-center justify-center gap-2 rounded-md bg-brand-500 px-4 py-2.5 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas disabled:cursor-not-allowed disabled:opacity-60"
          >
            <Loader2 v-if="submitting" class="h-4 w-4 animate-spin" :stroke-width="1.5" />
            <ShieldCheck v-else class="h-4 w-4" :stroke-width="1.5" />
            <span>{{ submitting ? 'Memproses…' : 'Lanjutkan' }}</span>
          </button>
          <button
            type="button"
            :disabled="submitting"
            class="inline-flex w-full items-center justify-center gap-2 rounded-md border border-hairline bg-canvas px-4 py-2.5 text-sm font-semibold text-ink-900 transition-colors hover:bg-canvas-alt focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas disabled:cursor-not-allowed disabled:opacity-60"
            @click="onSkip"
          >
            <SkipForward class="h-4 w-4" :stroke-width="1.5" />
            <span>Lewati dulu, isi nanti</span>
          </button>
        </div>
      </form>
    </template>

    <!-- Sub-state 2: nomor tabrakan — dua jalan keluar -->
    <template v-else-if="phase === 'phone_conflict'">
      <p class="mb-2 flex items-center gap-2 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
        <Info class="h-3.5 w-3.5" :stroke-width="1.75" />
        Nomor sudah terpakai
      </p>
      <h1 class="font-serif text-2xl md:text-3xl font-semibold tracking-tight text-ink-950">
        Nomor ini sudah terdaftar
      </h1>
      <p class="mt-2 text-sm text-ink-500 leading-relaxed">
        <span class="font-mono text-xs text-ink-700">{{ phoneMasked }}</span>
        sudah dipakai di akun lain. Kalau ini nomor Anda sendiri, kami sudah kirim kode verifikasi
        via WhatsApp — atau pakai nomor lain saja.
      </p>

      <div class="mt-8 space-y-3">
        <button
          type="button"
          class="inline-flex w-full items-center justify-center gap-2 rounded-md bg-brand-500 px-4 py-2.5 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
          @click="onChooseVerify"
        >
          <MessageCircle class="h-4 w-4" :stroke-width="1.5" />
          <span>Ini nomor saya, verifikasi sekarang</span>
        </button>
        <button
          type="button"
          class="inline-flex w-full items-center justify-center gap-2 rounded-md border border-hairline bg-canvas px-4 py-2.5 text-sm font-semibold text-ink-900 transition-colors hover:bg-canvas-alt focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
          @click="onUseAnotherNumber"
        >
          <ArrowLeft class="h-4 w-4" :stroke-width="1.5" />
          <span>Pakai nomor lain</span>
        </button>
      </div>
    </template>

    <!-- Sub-state 3: verifikasi OTP -->
    <template v-else-if="phase === 'otp_form'">
      <p class="mb-2 flex items-center gap-2 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
        <KeyRound class="h-3.5 w-3.5" :stroke-width="1.75" />
        Verifikasi WhatsApp
      </p>
      <h1 class="font-serif text-2xl md:text-3xl font-semibold tracking-tight text-ink-950">
        Masukkan kode verifikasi
      </h1>
      <p class="mt-2 text-sm text-ink-500 leading-relaxed">
        Kode 6 digit sudah dikirim via WhatsApp ke
        <span class="font-mono text-xs text-ink-700">{{ phoneMasked }}</span>.
      </p>

      <form class="mt-8 space-y-4" novalidate @submit.prevent="onSubmitOtp">
        <AlertMessage :message="otpError" variant="error" />

        <div>
          <label for="gotp" class="block text-sm font-medium text-ink-900">Kode verifikasi</label>
          <input
            id="gotp"
            ref="otpInputRef"
            :value="otpCode"
            type="text"
            inputmode="numeric"
            autocomplete="one-time-code"
            maxlength="6"
            placeholder="000000"
            :disabled="verifyingOtp || attemptsExhausted"
            class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2.5 text-center font-mono text-lg tracking-[0.5em] text-ink-900 placeholder-ink-300 transition-colors focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-500/20 disabled:cursor-not-allowed disabled:bg-canvas-alt disabled:text-ink-500"
            @input="onOtpInput"
          >
        </div>

        <button
          type="submit"
          :disabled="verifyingOtp || attemptsExhausted || otpCode.length !== 6"
          class="inline-flex w-full items-center justify-center gap-2 rounded-md bg-brand-500 px-4 py-2.5 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas disabled:cursor-not-allowed disabled:opacity-60"
        >
          <Loader2 v-if="verifyingOtp" class="h-4 w-4 animate-spin" :stroke-width="1.5" />
          <ShieldCheck v-else class="h-4 w-4" :stroke-width="1.5" />
          <span>{{ verifyingOtp ? 'Memverifikasi…' : 'Verifikasi & masuk' }}</span>
        </button>

        <div class="flex items-center justify-between pt-1 text-xs">
          <button
            type="button"
            :disabled="submitting"
            class="rounded-sm text-ink-500 transition-colors hover:text-ink-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas disabled:cursor-not-allowed disabled:opacity-60"
            @click="onChangeNumber"
          >
            <ArrowLeft class="mr-1 inline h-3 w-3 align-[-1px]" :stroke-width="1.75" />
            Ganti nomor
          </button>

          <button
            type="button"
            :disabled="!canResend || submitting"
            class="rounded-sm font-medium text-brand-500 transition-colors hover:text-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas disabled:cursor-not-allowed disabled:text-ink-400"
            @click="onResendOtp"
          >
            <span v-if="submitting" class="inline-flex items-center gap-1.5">
              <Loader2 class="h-3.5 w-3.5 animate-spin" :stroke-width="1.5" />
              Mengirim…
            </span>
            <span v-else-if="canResend">Kirim ulang kode</span>
            <span v-else>Kirim ulang dalam {{ resendIn }} detik</span>
          </button>
        </div>
      </form>
    </template>

    <!-- Gagal -->
    <div v-else class="rounded-lg border border-brand-200 bg-brand-50/50 p-6">
      <div class="flex items-start gap-3">
        <AlertTriangle class="mt-0.5 h-5 w-5 flex-none text-brand-700" :stroke-width="1.75" />
        <div class="flex-1">
          <h2 class="font-serif text-lg font-semibold text-ink-950">Login Google gagal</h2>
          <p class="mt-1 text-sm leading-relaxed text-ink-700">{{ errorMessage }}</p>
          <div class="mt-4 flex flex-wrap gap-2">
            <button
              v-if="errorCode === 'OAUTH_CODE_INVALID'"
              type="button"
              class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-1.5 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
              @click="google.startGoogleLogin()"
            >
              <RotateCw class="h-4 w-4" :stroke-width="1.75" />
              Ulangi login dengan Google
            </button>
            <NuxtLink
              to="/login"
              class="inline-flex items-center gap-2 rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-medium text-ink-700 transition-colors hover:border-ink-300 hover:bg-canvas-alt focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
            >
              <ArrowLeft class="h-4 w-4" :stroke-width="1.75" />
              Kembali ke halaman masuk
            </NuxtLink>
          </div>
        </div>
      </div>
    </div>
  </main>
</template>
