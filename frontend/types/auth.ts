// Shared types untuk modul auth di frontend. Match dengan response envelope
// backend Go (internal/auth/handler + service dto).

export type UserType = 'customer' | 'staff'

export interface AuthUser {
  id: string
  email?: string
  phone: string
  name: string
  user_type: UserType
}

export interface AuthToken {
  access_token: string
  token_type: string
  expires_in: number // seconds
}

/** Response dari POST /auth/register dan POST /auth/login */
export interface AuthResponse {
  token: AuthToken
  user: AuthUser
}

/** Response dari GET /auth/me */
export interface MeResponse {
  user_id: string
  user_type: UserType
  email?: string
  phone: string
  name: string
  roles: string[]
  permissions: string[]
}

// -------------------- OAuth Google (§10) --------------------

/** Slug error dari redirect backend setelah OAuth Google gagal (`?oauth_error=`). */
export type GoogleOAuthErrorSlug =
  | 'state_mismatch'
  | 'dibatalkan'
  | 'staff_not_allowed'
  | 'account_conflict'
  | 'user_inactive'
  | 'server_error'
  | 'email_unverified'

/**
 * Hasil sukses tukar `code` (POST /auth/google/exchange atau
 * /auth/google/complete) — sesi langsung terbentuk (user sudah lengkap datanya).
 */
export interface GoogleAuthSession {
  status: 'session'
  token: AuthToken
  user: AuthUser
  redirect_path?: string
}

/**
 * Hasil tukar `code` untuk akun Google baru yang belum punya nomor WA
 * tersimpan — customer harus lengkapi nama + nomor sebelum sesi dibuat
 * (POST /auth/google/complete).
 */
export interface GoogleAuthNeedPhone {
  status: 'need_phone'
  code: string
  email: string
  name: string
  redirect_path?: string
}

/** Response dari POST /auth/google/exchange */
export type GoogleAuthExchangeResult = GoogleAuthSession | GoogleAuthNeedPhone

// -------------------- Verifikasi OTP WhatsApp (registrasi Google) --------------------

/**
 * Hasil sukses POST /auth/google/request-otp — kode OTP dikirim via WA ke
 * nomor yang diinput di form `phone_form`. Sub-state berikutnya: `otp_form`.
 */
export interface GoogleOtpRequestResult {
  status: 'otp_sent'
  /** Nomor tersensor untuk ditampilkan, mis. "0812****678". */
  phone_masked: string
  /** Detik hingga kode kedaluwarsa (dipakai countdown di UI). */
  expires_in: number
  /** Detik hingga tombol "Kirim ulang kode" boleh ditekan lagi. */
  resend_available_in: number
}
