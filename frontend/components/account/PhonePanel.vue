<script setup lang="ts">
/**
 * PhonePanel — kartu "Nomor WA" di /akun. Dipakai baik oleh customer yang
 * belum punya nomor sama sekali (memilih "Lewati dulu" saat registrasi
 * Google) maupun yang sudah punya nomor tapi belum terverifikasi.
 *
 * Semantik simpan identik dengan alur registrasi (lihat `/pages/auth/google.vue`
 * & `useGoogleAuth`), hanya beda endpoint (otentikasi via token, bukan `code`
 * OAuth) — lihat `useAccountPhone`:
 *   - Nomor bebas → langsung tersimpan, TIDAK ada OTP, `phone_verified:false`
 *     (belum pernah dibuktikan kepemilikannya — normal, bukan kesalahan).
 *   - Nomor tabrakan (dipakai akun lain) → tawarkan dua jalan: pakai nomor
 *     lain, atau verifikasi OTP (yang sudah terkirim) → `phone_verified:true`.
 *
 * Keputusan UX: badge "Belum terverifikasi" (§26, tone amber — bukan
 * alarmis) selalu menyertakan tombol "Verifikasi nomor ini" yang membuka
 * form yang SAMA (pre-filled dengan nomor yang sudah tersimpan) — menekan
 * submit menjalankan pengecekan yang sama seperti menambah nomor baru.
 * Tidak ada endpoint terpisah "verifikasi ulang nomor sendiri" di kontrak
 * backend saat ini.
 */
import { Loader2, ShieldCheck, ArrowLeft, KeyRound, Pencil, Plus, MessageCircle, Info, CheckCircle2, X } from '@lucide/vue'
import { ApiError } from '~/composables/useApi'
import { isValidIndonesianPhone, maskPhoneDisplay } from '~/utils/phone'

const auth = useAuthStore()
const accountPhone = useAccountPhone()

type Phase = 'view' | 'edit' | 'conflict' | 'otp' | 'blocked'
const phase = ref<Phase>('view')
// Pesan untuk phase 'blocked' — nomor yang tidak bisa direbut dengan cara apa pun.
const blockedMsg = ref('')

const currentPhone = computed(() => auth.user?.phone || '')
const isUnverified = computed(() => !!currentPhone.value && auth.user?.phone_verified === false)

// -------------------- edit --------------------
const phoneInput = ref('')
const formError = ref('')
const submitting = ref(false)
const phoneInputRef = ref<{ focus: () => void } | null>(null)

const phoneInputError = computed(() => {
  if (!phoneInput.value) return ''
  return isValidIndonesianPhone(phoneInput.value) ? '' : 'Format nomor tidak valid. Contoh: 08xxxxxxxxxx.'
})

function openAdd() {
  phoneInput.value = ''
  formError.value = ''
  phase.value = 'edit'
  nextTick(() => phoneInputRef.value?.focus())
}

function openVerifyExisting() {
  phoneInput.value = currentPhone.value
  formError.value = ''
  phase.value = 'edit'
  nextTick(() => phoneInputRef.value?.focus())
}

function cancelEdit() {
  formError.value = ''
  phase.value = 'view'
}

// -------------------- conflict / otp --------------------
const conflictPhone = ref('')
const phoneMasked = computed(() => maskPhoneDisplay(conflictPhone.value))
const otpCode = ref('')
const otpError = ref('')
const verifyingOtp = ref(false)
const attemptsExhausted = ref(false)
const resendIn = ref(0)
const otpInputRef = ref<HTMLInputElement | null>(null)
const successMsg = ref('')

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

async function onSave() {
  if (submitting.value) return
  formError.value = ''
  const phone = phoneInput.value.trim()

  if (!isValidIndonesianPhone(phone)) {
    formError.value = 'Nomor WhatsApp tidak valid.'
    return
  }

  submitting.value = true
  try {
    await proceedWithPhone(phone)
  } catch (e) {
    await handleSaveError(e, phone)
  } finally {
    submitting.value = false
  }
}

async function goToOtpEntry() {
  phase.value = 'otp'
  otpCode.value = ''
  otpError.value = ''
  attemptsExhausted.value = false
  await nextTick()
  otpInputRef.value?.focus()
}

async function proceedWithPhone(phone: string) {
  const check = await accountPhone.requestOtp(phone)

  // `free` & `self_verified` sama sekali tidak menerbitkan OTP — simpan
  // langsung. `self_verified` berarti nomor ini memang sudah milik user dan
  // sudah terbukti; backend menjawab idempoten, tidak ada yang perlu dikirim.
  if (!check.otp_required) {
    const res = await accountPhone.savePhone({ phone })
    auth.setPhone(res.phone, res.phone_verified)
    successMsg.value =
      check.reason === 'self_verified' ? 'Nomor WA sudah terverifikasi.' : 'Nomor WA tersimpan.'
    phase.value = 'view'
    clearSuccessSoon()
    return
  }

  conflictPhone.value = phone
  restartResendCountdown(check.resend_after_seconds ?? 30)

  // `self_verify` = nomornya memang milik user sendiri, cuma belum terbukti.
  // Jangan lewat layar konflik — itu menuduh user merebut nomornya sendiri.
  if (check.reason === 'self_verify') {
    await goToOtpEntry()
    return
  }
  phase.value = 'conflict'
}

async function handleSaveError(e: unknown, phone: string) {
  if (e instanceof ApiError) {
    // Jalan buntu: nomor dimiliki akun terdaftar/staff. OTP pun tidak bisa
    // memindahkannya, jadi JANGAN tawarkan verifikasi — menawarkan jalan yang
    // pasti gagal lebih menjengkelkan daripada menolak sejak awal.
    if (e.code === 'PHONE_ALREADY_USED') {
      conflictPhone.value = phone
      blockedMsg.value =
        e.message || 'nomor ini terikat akun lain dan tidak bisa dipindahkan. Silakan pakai nomor lain.'
      phase.value = 'blocked'
      return
    }
    if (e.code === 'PHONE_SELF_VERIFICATION_REQUIRED' || e.code === 'PHONE_ALREADY_IN_USE') {
      // Race: OTP belum tentu sudah terkirim di jalur ini — terbitkan ulang
      // supaya kodenya benar-benar ada sebelum user diminta mengetiknya.
      try {
        const check = await accountPhone.requestOtp(phone)
        conflictPhone.value = phone
        restartResendCountdown(check.resend_after_seconds ?? 30)
        if (check.reason === 'self_verify') {
          await goToOtpEntry()
          return
        }
        phase.value = 'conflict'
      } catch (e2) {
        formError.value = e2 instanceof ApiError ? e2.message : 'Nomor sudah dipakai akun lain. Coba nomor lain.'
      }
      return
    }
    formError.value = e.message
    return
  }
  formError.value = 'Gagal menyimpan nomor. Coba lagi.'
}

let successTimer: ReturnType<typeof setTimeout> | null = null
function clearSuccessSoon() {
  if (successTimer) clearTimeout(successTimer)
  successTimer = setTimeout(() => {
    successMsg.value = ''
  }, 4000)
}
onUnmounted(() => {
  if (successTimer) clearTimeout(successTimer)
})

function onUseAnotherNumber() {
  clearAllTimers()
  phoneInput.value = conflictPhone.value
  formError.value = ''
  phase.value = 'edit'
  nextTick(() => phoneInputRef.value?.focus())
}

async function onChooseVerify() {
  phase.value = 'otp'
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
    const check = await accountPhone.requestOtp(conflictPhone.value)
    otpCode.value = ''
    otpError.value = ''
    attemptsExhausted.value = false
    restartResendCountdown(check.resend_after_seconds ?? 30)
    await nextTick()
    otpInputRef.value?.focus()
  } catch (e) {
    otpError.value = e instanceof ApiError ? e.message : 'Gagal mengirim ulang kode. Coba lagi.'
  } finally {
    submitting.value = false
  }
}

function onChangeNumber() {
  clearAllTimers()
  phoneInput.value = ''
  phase.value = 'edit'
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

function handleOtpError(e: unknown) {
  if (!(e instanceof ApiError)) {
    otpError.value = 'Gagal memverifikasi kode. Coba lagi.'
    return
  }
  switch (e.code) {
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
      otpError.value = 'Nomor ini baru saja terpakai akun lain. Coba nomor lain.'
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
    const res = await accountPhone.savePhone({ phone: conflictPhone.value, otp: otpCode.value })
    auth.setPhone(res.phone, res.phone_verified)
    successMsg.value = 'Nomor WA terverifikasi.'
    clearAllTimers()
    phase.value = 'view'
    clearSuccessSoon()
  } catch (e) {
    handleOtpError(e)
  } finally {
    verifyingOtp.value = false
  }
}
</script>

<template>
  <div class="rounded-lg border border-hairline bg-canvas p-4">
    <!-- View: belum ada nomor -->
    <template v-if="phase === 'view' && !currentPhone">
      <dt class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Nomor WA</dt>
      <dd class="mt-1 text-sm text-ink-500">Belum ada nomor tersimpan.</dd>
      <p v-if="successMsg" class="mt-2 flex items-center gap-1.5 text-xs text-emerald-700">
        <CheckCircle2 class="h-3.5 w-3.5" :stroke-width="1.75" />
        {{ successMsg }}
      </p>
      <button
        type="button"
        class="mt-2 inline-flex items-center gap-1.5 rounded-md border border-hairline bg-canvas px-2.5 py-1.5 text-xs font-semibold text-ink-900 transition-colors hover:bg-canvas-alt focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
        @click="openAdd"
      >
        <Plus class="h-3.5 w-3.5" :stroke-width="1.75" />
        Tambah nomor WA
      </button>
      <p class="mt-1.5 text-xs text-ink-500 leading-relaxed">
        Dipakai untuk notifikasi status pesanan lewat WhatsApp.
      </p>
    </template>

    <!-- View: sudah ada nomor -->
    <template v-else-if="phase === 'view'">
      <div class="flex items-start justify-between gap-2">
        <div>
          <dt class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">Nomor WA</dt>
          <dd class="mt-1 flex items-center gap-2 text-sm font-mono text-ink-900">
            {{ currentPhone }}
            <AdminStatusBadge v-if="isUnverified" status="Belum terverifikasi" tone="amber" />
          </dd>
        </div>
        <button
          type="button"
          class="inline-flex flex-none items-center gap-1 rounded-sm text-xs font-medium text-ink-500 transition-colors hover:text-ink-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
          @click="openAdd"
        >
          <Pencil class="h-3 w-3" :stroke-width="1.75" />
          Ubah
        </button>
      </div>
      <p v-if="successMsg" class="mt-2 flex items-center gap-1.5 text-xs text-emerald-700">
        <CheckCircle2 class="h-3.5 w-3.5" :stroke-width="1.75" />
        {{ successMsg }}
      </p>
      <p v-else-if="isUnverified" class="mt-2 text-xs text-ink-500 leading-relaxed">
        Nomor ini belum dibuktikan kepemilikannya — normal untuk nomor yang disimpan tanpa verifikasi.
        <button
          type="button"
          class="ml-1 rounded-sm font-medium text-brand-500 transition-colors hover:text-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
          @click="openVerifyExisting"
        >
          Verifikasi sekarang
        </button>
      </p>
    </template>

    <!-- Edit: isi/ganti nomor -->
    <template v-else-if="phase === 'edit'">
      <dt class="flex items-center gap-1.5 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
        <Info class="h-3 w-3" :stroke-width="1.75" />
        {{ currentPhone ? 'Ubah nomor WA' : 'Tambah nomor WA' }}
      </dt>
      <AlertMessage :message="formError" variant="error" class="mt-2" />
      <div class="mt-2">
        <BaseInput
          id="account-phone"
          ref="phoneInputRef"
          v-model="phoneInput"
          label="Nomor WhatsApp"
          type="tel"
          autocomplete="tel"
          placeholder="08xxxxxxxxxx"
          :error="phoneInputError"
          helper="Otomatis dikonversi ke format 62xxx."
          :disabled="submitting"
        />
      </div>
      <div class="mt-3 flex gap-2">
        <button
          type="button"
          :disabled="submitting"
          class="inline-flex flex-1 items-center justify-center gap-1.5 rounded-md bg-brand-500 px-3 py-1.5 text-xs font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas disabled:cursor-not-allowed disabled:opacity-60"
          @click="onSave"
        >
          <Loader2 v-if="submitting" class="h-3.5 w-3.5 animate-spin" :stroke-width="1.5" />
          <ShieldCheck v-else class="h-3.5 w-3.5" :stroke-width="1.5" />
          {{ submitting ? 'Menyimpan…' : 'Simpan' }}
        </button>
        <button
          type="button"
          :disabled="submitting"
          class="inline-flex items-center justify-center gap-1.5 rounded-md border border-hairline bg-canvas px-3 py-1.5 text-xs font-semibold text-ink-900 transition-colors hover:bg-canvas-alt focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas disabled:cursor-not-allowed disabled:opacity-60"
          @click="cancelEdit"
        >
          <X class="h-3.5 w-3.5" :stroke-width="1.5" />
          Batal
        </button>
      </div>
    </template>

    <!-- Conflict: nomor tabrakan, dua jalan keluar -->
    <template v-else-if="phase === 'conflict'">
      <dt class="flex items-center gap-1.5 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
        <Info class="h-3 w-3" :stroke-width="1.75" />
        Nomor sudah terpakai
      </dt>
      <p class="mt-2 text-xs text-ink-700 leading-relaxed">
        <span class="font-mono text-ink-900">{{ phoneMasked }}</span>
        sudah dipakai akun lain. Kalau ini nomor Anda sendiri, kode verifikasi sudah kami kirim
        via WhatsApp — atau pakai nomor lain.
      </p>
      <div class="mt-3 flex flex-col gap-2">
        <button
          type="button"
          class="inline-flex items-center justify-center gap-1.5 rounded-md bg-brand-500 px-3 py-1.5 text-xs font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
          @click="onChooseVerify"
        >
          <MessageCircle class="h-3.5 w-3.5" :stroke-width="1.5" />
          Ini nomor saya, verifikasi
        </button>
        <button
          type="button"
          class="inline-flex items-center justify-center gap-1.5 rounded-md border border-hairline bg-canvas px-3 py-1.5 text-xs font-semibold text-ink-900 transition-colors hover:bg-canvas-alt focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
          @click="onUseAnotherNumber"
        >
          <ArrowLeft class="h-3.5 w-3.5" :stroke-width="1.5" />
          Pakai nomor lain
        </button>
      </div>
    </template>

    <!-- Jalan buntu: nomor terikat akun terdaftar/staff, OTP tidak menolong -->
    <template v-else-if="phase === 'blocked'">
      <dt class="flex items-center gap-1.5 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
        <Info class="h-3 w-3" :stroke-width="1.75" />
        Nomor tidak bisa dipakai
      </dt>
      <p class="mt-2 text-xs text-ink-700 leading-relaxed">
        <span class="font-mono text-ink-900">{{ phoneMasked }}</span> — {{ blockedMsg }}
      </p>
      <div class="mt-3">
        <button
          type="button"
          class="inline-flex items-center justify-center gap-1.5 rounded-md bg-brand-500 px-3 py-1.5 text-xs font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
          @click="onUseAnotherNumber"
        >
          <ArrowLeft class="h-3.5 w-3.5" :stroke-width="1.5" />
          Pakai nomor lain
        </button>
      </div>
    </template>

    <!-- OTP: verifikasi kode -->
    <template v-else-if="phase === 'otp'">
      <dt class="flex items-center gap-1.5 text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500">
        <KeyRound class="h-3 w-3" :stroke-width="1.75" />
        Verifikasi WhatsApp
      </dt>
      <p class="mt-2 text-xs text-ink-500 leading-relaxed">
        Kode 6 digit dikirim ke <span class="font-mono text-ink-700">{{ phoneMasked }}</span>.
      </p>
      <AlertMessage :message="otpError" variant="error" class="mt-2" />
      <input
        ref="otpInputRef"
        :value="otpCode"
        type="text"
        inputmode="numeric"
        autocomplete="one-time-code"
        maxlength="6"
        placeholder="000000"
        :disabled="verifyingOtp || attemptsExhausted"
        class="mt-2 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-center font-mono text-base tracking-[0.4em] text-ink-900 placeholder-ink-300 transition-colors focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-500/20 disabled:cursor-not-allowed disabled:bg-canvas-alt disabled:text-ink-500"
        @input="onOtpInput"
      >
      <div class="mt-3 flex gap-2">
        <button
          type="button"
          :disabled="verifyingOtp || attemptsExhausted || otpCode.length !== 6"
          class="inline-flex flex-1 items-center justify-center gap-1.5 rounded-md bg-brand-500 px-3 py-1.5 text-xs font-semibold text-canvas transition-colors hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas disabled:cursor-not-allowed disabled:opacity-60"
          @click="onSubmitOtp"
        >
          <Loader2 v-if="verifyingOtp" class="h-3.5 w-3.5 animate-spin" :stroke-width="1.5" />
          <ShieldCheck v-else class="h-3.5 w-3.5" :stroke-width="1.5" />
          {{ verifyingOtp ? 'Memverifikasi…' : 'Verifikasi' }}
        </button>
      </div>
      <div class="mt-2 flex items-center justify-between text-xs">
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
          <span v-if="canResend">Kirim ulang kode</span>
          <span v-else>Kirim ulang dalam {{ resendIn }}d</span>
        </button>
      </div>
    </template>
  </div>
</template>
