/**
 * useGoogleAuth — integrasi "Lanjut dengan Google" (OAuth diproses backend,
 * bukan SDK client-side). Alur (kontrak final):
 *
 *  1. `startGoogleLogin()` — navigasi browser PENUH ke
 *     `${apiBase}/auth/google/start?redirect=...` (bukan $fetch — backend
 *     yang redirect ke consent screen Google, lalu balik lagi ke frontend).
 *  2. Backend redirect balik ke `/auth/google?code=...` (sukses) atau
 *     `/login?oauth_error=<slug>` (gagal) — ditangani halaman masing-masing.
 *  3. Halaman `/auth/google` tukar `code` lewat `exchangeCode()` → langsung
 *     dapat sesi ATAU `need_phone` (akun baru, minta lengkapi nama — nomor
 *     WA OPSIONAL, boleh dilewati).
 *  4. Nomor WA **tidak wajib** & OTP HANYA diterbitkan kalau nomor yang
 *     diisi ternyata sudah dipakai akun lain — owner sengaja batasi kirim
 *     OTP demi jaga nomor WA gateway (Baileys) tidak diblokir karena kirim
 *     ke orang asing yang belum tentu butuh verifikasi:
 *       - `requestOtp()` — cek nomor. `otp_required:false` = bebas, TIDAK
 *         ada OTP terkirim, langsung `completeRegistration()` tanpa `otp`.
 *       - `otp_required:true` = nomor tabrakan, OTP terkirim; tawarkan user
 *         dua jalan: pakai nomor lain, atau verifikasi (isi `otp` di
 *         `completeRegistration()`).
 *       - `completeRegistration()` tanpa `phone` = akun jadi tanpa nomor
 *         (dilewati) — bisa ditambahkan belakangan lewat `useAccountPhone`.
 */
import type {
  GoogleAuthExchangeResult,
  GoogleAuthSession,
  GoogleOAuthErrorSlug,
  GoogleOtpCheckResult,
} from '~/types/auth'

const OAUTH_ERROR_MESSAGES: Record<GoogleOAuthErrorSlug, string> = {
  state_mismatch: 'Sesi login tidak valid (kadaluarsa atau dibuka di tab lain). Silakan coba lagi.',
  dibatalkan: 'Login dengan Google dibatalkan.',
  staff_not_allowed: 'Akun staff tidak bisa masuk lewat Google. Gunakan email dan kata sandi.',
  account_conflict: 'Email ini sudah terdaftar dengan metode login lain. Gunakan email dan kata sandi.',
  user_inactive: 'Akun Anda tidak aktif. Hubungi admin untuk bantuan.',
  server_error: 'Terjadi gangguan di server saat memproses login Google. Coba lagi beberapa saat lagi.',
  email_unverified:
    'Email akun Google Anda belum terverifikasi oleh Google, jadi belum bisa dipakai mendaftar. Verifikasi email Google Anda lalu coba lagi.',
}

/** Pesan Bahasa Indonesia yang manusiawi untuk tiap slug `?oauth_error=`. */
export function googleErrorMessage(slug: string | null | undefined): string {
  if (!slug) return 'Login dengan Google gagal. Coba lagi.'
  return OAUTH_ERROR_MESSAGES[slug as GoogleOAuthErrorSlug] ?? 'Login dengan Google gagal. Coba lagi.'
}

export function useGoogleAuth() {
  const api = useApi()

  /**
   * Mulai alur OAuth. Sengaja bukan $fetch — backend butuh full page redirect
   * ke consent screen Google (§2: base URL dari runtimeConfig, jangan hardcode).
   */
  function startGoogleLogin(redirect: string = '/akun') {
    const config = useRuntimeConfig()
    const target = `${config.public.apiBase}/auth/google/start?redirect=${encodeURIComponent(redirect)}`
    window.location.href = target
  }

  /** POST /auth/google/exchange — tukar one-time code hasil callback backend. */
  function exchangeCode(code: string): Promise<GoogleAuthExchangeResult> {
    return api.post<GoogleAuthExchangeResult>('/auth/google/exchange', { code })
  }

  /**
   * POST /auth/google/request-otp — cek apakah nomor yang diisi bebas atau
   * sudah dipakai akun lain. TIDAK mengirim OTP kalau nomor bebas.
   */
  function requestOtp(input: { code: string; phone: string }): Promise<GoogleOtpCheckResult> {
    return api.post<GoogleOtpCheckResult>('/auth/google/request-otp', input)
  }

  /**
   * POST /auth/google/complete — selesaikan registrasi & buat sesi.
   * `phone` opsional (kosong = dilewati). `otp` hanya disertakan kalau
   * `requestOtp()` sebelumnya menjawab `otp_required:true` dan user memilih
   * memverifikasi nomor tsb (bukan ganti ke nomor lain).
   */
  function completeRegistration(input: {
    code: string
    name: string
    phone?: string
    otp?: string
  }): Promise<GoogleAuthSession> {
    const body: Record<string, unknown> = { code: input.code, name: input.name }
    if (input.phone) body.phone = input.phone
    if (input.otp) body.otp = input.otp
    return api.post<GoogleAuthSession>('/auth/google/complete', body)
  }

  return { startGoogleLogin, exchangeCode, requestOtp, completeRegistration }
}
