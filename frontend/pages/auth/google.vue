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
 * Alur akun baru (security review — WAJIB verifikasi kepemilikan nomor WA
 * lewat OTP sebelum akun dibuat):
 *  1. `phone_form` — isi nama + nomor WhatsApp → POST /auth/google/request-otp.
 *  2. `otp_form` — masukkan kode 6 digit yang dikirim via WA → POST
 *     /auth/google/complete (menyertakan `otp`) → sesi terbentuk.
 *
 * Design: patuh CLAUDE.md §26 (Fraunces + Inter + Lucide, brand/ink/hairline).
 */
import { Loader2, AlertTriangle, ShieldCheck, ArrowLeft, Mail, KeyRound, RotateCw } from '@lucide/vue'
import { ApiError } from '~/composables/useApi'
import { isValidIndonesianPhone } from '~/utils/phone'
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

type Phase = 'processing' | 'phone_form' | 'otp_form' | 'error'
const phase = ref<Phase>('processing')
const errorMessage = ref('')
/** Code envelope error dari langkah gagal terakhir — dipakai buat tombol khusus. */
const errorCode = ref<string | null>(null)

// -------------------- Data OAuth pending --------------------
const pendingCode = ref('')
const googleEmail = ref('')
const form = reactive({ name: '', phone: '' })

// -------------------- Sub-state: phone_form --------------------
const phoneFormError = ref('')
const requestingOtp = ref(false)

const phoneError = computed(() => {
  if (!form.phone) return ''
  return isValidIndonesianPhone(form.phone) ? '' : 'Format nomor tidak valid. Contoh: 08xxxxxxxxxx.'
})

// -------------------- Sub-state: otp_form --------------------
const phoneMasked = ref('')
const otpCode = ref('')
const otpError = ref('')
const verifyingOtp = ref(false)
const attemptsExhausted = ref(false)
const identityConflict = ref(false)
const expiresIn = ref(0)
const resendIn = ref(0)
const otpInputRef = ref<HTMLInputElement | null>(null)

const otpExpired = computed(() => expiresIn.value <= 0)
const canResend = computed(() => resendIn.value <= 0)
const expiresLabel = computed(() => {
  const total = Math.max(0, expiresIn.value)
  const m = Math.floor(total / 60)
  const s = total % 60
  return `${m}:${s.toString().padStart(2, '0')}`
})

let expiresTimer: ReturnType<typeof setInterval> | null = null
let resendTimer: ReturnType<typeof setInterval> | null = null

function restartExpiresCountdown(seconds: number) {
  expiresIn.value = seconds
  if (expiresTimer) clearInterval(expiresTimer)
  expiresTimer = setInterval(() => {
    if (expiresIn.value > 0) {
      expiresIn.value -= 1
    } else if (expiresTimer) {
      clearInterval(expiresTimer)
      expiresTimer = null
    }
  }, 1000)
}

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
  if (expiresTimer) {
    clearInterval(expiresTimer)
    expiresTimer = null
  }
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
    // need_phone — akun baru, minta lengkapi nama + nomor WA lalu verifikasi OTP.
    pendingCode.value = res.code
    googleEmail.value = res.email
    form.name = res.name || ''
    phase.value = 'phone_form'
  } catch (e) {
    phase.value = 'error'
    errorCode.value = e instanceof ApiError ? e.code : null
    errorMessage.value = e instanceof ApiError ? e.message : 'Gagal menyelesaikan login Google.'
  }
}

onMounted(runExchange)

/** Tangani error POST /auth/google/request-otp — dipakai dari phone_form & tombol kirim ulang. */
function handleRequestOtpError(e: unknown, target: 'phone_form' | 'otp_form') {
  const setMessage = target === 'phone_form' ? (v: string) => (phoneFormError.value = v) : (v: string) => (otpError.value = v)

  if (e instanceof ApiError) {
    if (e.code === 'OAUTH_CODE_INVALID') {
      goToSessionExpired()
      return
    }
    if (e.code === 'OTP_COOLDOWN') {
      const details = e.details as { resend_available_in?: number } | undefined
      const secs = typeof details?.resend_available_in === 'number' ? details.resend_available_in : resendIn.value
      if (target === 'otp_form') {
        restartResendCountdown(secs)
      }
      setMessage(`Kode verifikasi masih berlaku. Kirim ulang dalam ${secs} detik.`)
      return
    }
    setMessage(e.message)
    return
  }
  setMessage('Gagal mengirim kode verifikasi. Coba lagi.')
}

async function onSubmitPhone() {
  if (requestingOtp.value) return
  phoneFormError.value = ''

  if (!form.name.trim()) {
    phoneFormError.value = 'Nama wajib diisi.'
    return
  }
  if (!isValidIndonesianPhone(form.phone)) {
    phoneFormError.value = 'Nomor WhatsApp tidak valid.'
    return
  }

  requestingOtp.value = true
  try {
    const res = await google.requestOtp({
      code: pendingCode.value,
      phone: form.phone.trim(),
      name: form.name.trim(),
    })
    phoneMasked.value = res.phone_masked
    otpCode.value = ''
    otpError.value = ''
    attemptsExhausted.value = false
    identityConflict.value = false
    restartExpiresCountdown(res.expires_in)
    restartResendCountdown(res.resend_available_in)
    phase.value = 'otp_form'
    await nextTick()
    otpInputRef.value?.focus()
  } catch (e) {
    handleRequestOtpError(e, 'phone_form')
  } finally {
    requestingOtp.value = false
  }
}

async function onResendOtp() {
  if (!canResend.value || requestingOtp.value) return
  requestingOtp.value = true
  try {
    const res = await google.requestOtp({
      code: pendingCode.value,
      phone: form.phone.trim(),
      name: form.name.trim(),
    })
    phoneMasked.value = res.phone_masked
    otpCode.value = ''
    otpError.value = ''
    attemptsExhausted.value = false
    restartExpiresCountdown(res.expires_in)
    restartResendCountdown(res.resend_available_in)
    await nextTick()
    otpInputRef.value?.focus()
  } catch (e) {
    handleRequestOtpError(e, 'otp_form')
  } finally {
    requestingOtp.value = false
  }
}

function onChangeNumber() {
  clearAllTimers()
  phase.value = 'phone_form'
  otpCode.value = ''
  otpError.value = ''
  attemptsExhausted.value = false
  identityConflict.value = false
}

function onOtpInput(e: Event) {
  const raw = (e.target as HTMLInputElement).value
  const digits = raw.replace(/\D/g, '').slice(0, 6)
  otpCode.value = digits
  ;(e.target as HTMLInputElement).value = digits
  if (digits.length === 6 && !verifyingOtp.value && !attemptsExhausted.value && !otpExpired.value) {
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
      if (expiresTimer) {
        clearInterval(expiresTimer)
        expiresTimer = null
      }
      expiresIn.value = 0
      otpCode.value = ''
      otpError.value = 'Kode verifikasi sudah kedaluwarsa. Minta kode baru.'
      return
    case 'PHONE_ALREADY_USED':
      identityConflict.value = true
      otpError.value = 'Nomor ini sudah terdaftar pada akun lain. Ganti nomor, atau masuk dengan email & kata sandi.'
      return
    case 'EMAIL_ALREADY_USED':
      identityConflict.value = true
      otpError.value = 'Email Google ini sudah terdaftar dengan metode login lain. Masuk dengan email & kata sandi.'
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
      phone: form.phone.trim(),
      name: form.name.trim(),
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

    <!-- Sub-state 1: lengkapi nama + nomor WA -->
    <template v-else-if="phase === 'phone_form'">
      <p class="mb-2 flex items-center gap-2 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
        <Mail class="h-3.5 w-3.5" :stroke-width="1.75" />
        Satu langkah lagi
      </p>
      <h1 class="font-serif text-2xl md:text-3xl font-semibold tracking-tight text-ink-950">
        Lengkapi data Anda
      </h1>
      <p class="mt-2 text-sm text-ink-500 leading-relaxed">
        Nomor WhatsApp dipakai untuk mengirim status pesanan dan menyatukan riwayat order Anda.
        Kode verifikasi akan dikirim via WhatsApp ke nomor tersebut.
      </p>

      <form class="mt-8 space-y-4" novalidate @submit.prevent="onSubmitPhone">
        <AlertMessage :message="phoneFormError" variant="error" />

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
          :disabled="requestingOtp"
        />
        <BaseInput
          id="gphone"
          v-model="form.phone"
          label="Nomor WhatsApp"
          type="tel"
          autocomplete="tel"
          placeholder="08xxxxxxxxxx"
          :error="phoneError"
          helper="Otomatis dikonversi ke format 62xxx."
          required
          :disabled="requestingOtp"
        />

        <button
          type="submit"
          :disabled="requestingOtp"
          class="inline-flex w-full items-center justify-center gap-2 rounded-md bg-brand-500 px-4 py-2.5 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas disabled:cursor-not-allowed disabled:opacity-60"
        >
          <Loader2 v-if="requestingOtp" class="h-4 w-4 animate-spin" :stroke-width="1.5" />
          <ShieldCheck v-else class="h-4 w-4" :stroke-width="1.5" />
          <span>{{ requestingOtp ? 'Mengirim kode…' : 'Kirim kode verifikasi' }}</span>
        </button>
      </form>
    </template>

    <!-- Sub-state 2: verifikasi OTP -->
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
          <p class="mt-1.5 text-xs" :class="otpExpired ? 'text-brand-700' : 'text-ink-500'">
            <span v-if="!otpExpired">Kode berlaku {{ expiresLabel }}</span>
            <span v-else>Kode sudah kedaluwarsa. Minta kode baru.</span>
          </p>
        </div>

        <p v-if="identityConflict" class="text-xs text-ink-500">
          <NuxtLink
            to="/login"
            class="rounded-sm font-medium text-brand-500 transition-colors hover:text-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
          >
            Masuk dengan email & kata sandi →
          </NuxtLink>
        </p>

        <button
          type="submit"
          :disabled="verifyingOtp || attemptsExhausted || otpCode.length !== 6 || otpExpired"
          class="inline-flex w-full items-center justify-center gap-2 rounded-md bg-brand-500 px-4 py-2.5 text-sm font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas disabled:cursor-not-allowed disabled:opacity-60"
        >
          <Loader2 v-if="verifyingOtp" class="h-4 w-4 animate-spin" :stroke-width="1.5" />
          <ShieldCheck v-else class="h-4 w-4" :stroke-width="1.5" />
          <span>{{ verifyingOtp ? 'Memverifikasi…' : 'Verifikasi & masuk' }}</span>
        </button>

        <div class="flex items-center justify-between pt-1 text-xs">
          <button
            type="button"
            :disabled="requestingOtp"
            class="rounded-sm text-ink-500 transition-colors hover:text-ink-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas disabled:cursor-not-allowed disabled:opacity-60"
            @click="onChangeNumber"
          >
            <ArrowLeft class="mr-1 inline h-3 w-3 align-[-1px]" :stroke-width="1.75" />
            Ganti nomor
          </button>

          <button
            type="button"
            :disabled="!canResend || requestingOtp"
            class="rounded-sm font-medium text-brand-500 transition-colors hover:text-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas disabled:cursor-not-allowed disabled:text-ink-400"
            @click="onResendOtp"
          >
            <span v-if="requestingOtp" class="inline-flex items-center gap-1.5">
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
