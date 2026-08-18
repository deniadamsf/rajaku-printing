// Shared types untuk modul auth di frontend. Match dengan response envelope
// backend Go (internal/auth/handler + service dto).

export type UserType = 'customer' | 'staff'

export interface AuthUser {
  id: string
  email?: string
  /** Opsional — nomor WA sekarang boleh dilewati saat registrasi Google (lihat §OTP di bawah). */
  phone?: string
  /**
   * `true` kalau kepemilikan nomor sudah dibuktikan lewat OTP WA. `false` =
   * nomor tersimpan tapi belum diverifikasi (jalur bebas/tidak tabrakan saat
   * request-otp). `undefined` = akun lama sebelum field ini ada / tidak
   * relevan (mis. belum ada nomor sama sekali) — jangan tampilkan badge.
   */
  phone_verified?: boolean
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
  phone?: string
  /** Selalu dikirim backend (bukan lagi optional) — tidak ada field timestamp terpisah. */
  phone_verified: boolean
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
 * Hasil tukar `code` untuk akun Google baru — customer masih perlu lengkapi
 * nama (nomor WA OPSIONAL, boleh dilewati — lihat halaman `/auth/google`)
 * lewat POST /auth/google/complete.
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

// -------------------- Nomor WA opsional + verifikasi OTP kalau tabrakan --------------------
//
// OTP HANYA diterbitkan kalau nomor yang diisi user sudah dipakai akun lain —
// bukan langkah wajib di setiap registrasi (owner membatasi kirim OTP demi
// jaga nomor WA gateway/Baileys tidak diblokir karena kirim ke orang asing
// yang belum tentu perlu verifikasi).
//
// PENTING: dua pasang endpoint di bawah TIDAK berbagi bentuk response persis
// sama meski semantiknya mirip — jangan disatukan ke satu tipe:
//   - POST /auth/google/request-otp (saat registrasi, lihat `useGoogleAuth`)
//     → `GoogleOtpCheckResult` — hanya `{otp_required, resend_after_seconds?}`,
//     tidak ada `reason` (belum ada identitas "self" karena akun belum dibuat).
//   - POST /auth/phone/request-otp (endpoint terautentikasi tambah/ganti nomor
//     pasca-registrasi, lihat `useAccountPhone`, dipakai user yang tadi
//     memilih "Lewati dulu") → `AccountPhoneOtpCheckResult` — punya `reason`
//     tambahan karena user sudah punya identitas untuk dibandingkan (nomor
//     ini bisa jadi milik sendiri yang belum diverifikasi).

/** Hasil cek nomor sebelum disimpan — khusus alur registrasi Google. */
export interface GoogleOtpCheckResult {
  /**
   * `false` — nomor bebas, TIDAK ada OTP dikirim, boleh langsung lanjut ke
   * langkah simpan (tanpa pernah menampilkan form OTP).
   * `true` — nomor sudah dipakai akun lain, OTP terkirim ke WA nomor
   * tersebut untuk membuktikan kepemilikan sebelum dipakai paksa.
   */
  otp_required: boolean
  /** Detik hingga "Kirim ulang kode" boleh ditekan lagi. Hanya ada saat `otp_required` true. */
  resend_after_seconds?: number
}

/**
 * Kenapa nomor butuh/tidak butuh OTP — khusus `POST /auth/phone/request-otp`
 * (endpoint terautentikasi, lihat `useAccountPhone`):
 *   - `free` — nomor bebas, tidak ada OTP dikirim. Langsung simpan.
 *   - `self_verify` — nomor milik user sendiri tapi belum terverifikasi. OTP
 *     sudah dikirim. Tampilkan kolom OTP dengan kalimat verifikasi — BUKAN
 *     kalimat konflik/"sudah dipakai orang lain".
 *   - `self_verified` — nomor milik user sendiri dan sudah terverifikasi.
 *     Tidak ada OTP dikirim. Idempoten — tampilkan sebagai sudah beres,
 *     jangan tampilkan kolom OTP maupun error.
 *   - `owned_by_other` — nomor dipakai akun lain, masih bisa diklaim. OTP
 *     sudah dikirim. Tawarkan dua jalan: pakai nomor lain, atau verifikasi.
 */
export type AccountPhoneOtpReason = 'free' | 'self_verify' | 'self_verified' | 'owned_by_other'

/** Hasil cek nomor sebelum disimpan — khusus endpoint akun (`useAccountPhone`). */
export interface AccountPhoneOtpCheckResult {
  /** `true` untuk `self_verify` & `owned_by_other`; `false` untuk `free` & `self_verified`. */
  otp_required: boolean
  reason: AccountPhoneOtpReason
  /** Nomor tersensor dari server (mis. `0812****678`). Opsional — UI boleh mask sendiri di client. */
  phone_masked?: string
  /** Detik hingga kode OTP kedaluwarsa. Hanya ada saat OTP terkirim. */
  expires_in?: number
  /** Detik hingga "Kirim ulang kode" boleh ditekan lagi. Hanya ada saat OTP terkirim. */
  resend_after_seconds?: number
}
