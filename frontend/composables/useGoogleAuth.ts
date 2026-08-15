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
 *     dapat sesi ATAU `need_phone` (akun baru, minta lengkapi nomor WA).
 *  4. Akun baru WAJIB verifikasi kepemilikan nomor WA lewat OTP sebelum akun
 *     dibuat (security review): `requestOtp()` mengirim kode via WA, lalu
 *     `completeRegistration()` menyertakan `otp` untuk membuat sesi.
 */
import type {
  GoogleAuthExchangeResult,
  GoogleAuthSession,
  GoogleOAuthErrorSlug,
  GoogleOtpRequestResult,
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
   * POST /auth/google/request-otp — langkah 1 verifikasi nomor WA: kirim
   * kode OTP ke nomor yang diinput. Wajib sukses sebelum `completeRegistration()`.
   */
  function requestOtp(input: { code: string; phone: string; name: string }): Promise<GoogleOtpRequestResult> {
    return api.post<GoogleOtpRequestResult>('/auth/google/request-otp', input)
  }

  /**
   * POST /auth/google/complete — langkah 2: kirim kode OTP bersama data lain
   * untuk membuat akun + sesi.
   */
  function completeRegistration(input: {
    code: string
    phone: string
    name: string
    otp: string
  }): Promise<GoogleAuthSession> {
    return api.post<GoogleAuthSession>('/auth/google/complete', input)
  }

  return { startGoogleLogin, exchangeCode, requestOtp, completeRegistration }
}
