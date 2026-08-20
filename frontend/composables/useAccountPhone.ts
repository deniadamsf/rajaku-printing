/**
 * useAccountPhone — tambah/ganti nomor WA untuk user yang SUDAH login, dipakai
 * di halaman akun (`/akun`) oleh customer yang tadinya memilih "Lewati dulu"
 * saat registrasi Google (lihat `/auth/google` & `useGoogleAuth`).
 *
 * Alurnya dua langkah: `requestOtp()` menilai status nomor, lalu `savePhone()`
 * menyimpannya. Cabangnya ditentukan `reason` (lihat `AccountPhoneOtpReason`):
 *   - `free` / `self_verified` — tidak ada OTP dikirim, langsung `savePhone()`
 *     tanpa `otp`.
 *   - `self_verify` — nomor milik user sendiri yang belum terverifikasi. OTP
 *     sudah dikirim; tampilkan kolom kode dengan kalimat VERIFIKASI, bukan
 *     kalimat "nomor sudah dipakai" — nomornya memang miliknya sendiri.
 *   - `owned_by_other` — nomor milik akun lain tapi masih bisa diklaim. OTP
 *     sudah dikirim; tawarkan dua jalan (pakai nomor lain / verifikasi).
 *
 * Bedanya dari alur registrasi: request pakai Bearer token (sesi customer
 * yang sudah login), bukan `code` OAuth sekali-pakai.
 *
 * Kedua endpoint di bawah berada di limiter ketat `RATE_LIMIT_OTP_*` di
 * backend — jangan panggil `requestOtp()` sebagai validasi ketikan realtime.
 */
import type { AccountPhoneOtpCheckResult } from '~/types/auth'

// Path final backend (`internal/auth/handler/phone_claim_handler.go`),
// sengaja dikumpulkan di sini supaya perubahan cukup satu tempat.
const REQUEST_OTP_PATH = '/auth/phone/request-otp'
const SAVE_PATH = '/auth/phone'

export interface AccountPhoneSaveResult {
  phone: string
  phone_verified: boolean
  /**
   * Jumlah order lama dari nomor WA yang sama (mis. bekas order walk-in di
   * kasir) yang ikut dipindahkan ke akun ini saat nomor disimpan — janji
   * "satu pelanggan, satu riwayat" (CLAUDE.md §11). Selalu ada; `0` kalau
   * tidak ada order lama yang ikut pindah.
   */
  merged_orders: number
}

export function useAccountPhone() {
  const api = useApi()

  /** Nilai status nomor sebelum disimpan. Lihat `reason` untuk cabang UI-nya. */
  function requestOtp(phone: string): Promise<AccountPhoneOtpCheckResult> {
    return api.post<AccountPhoneOtpCheckResult>(REQUEST_OTP_PATH, { phone })
  }

  /**
   * Simpan nomor ke akun. `otp` hanya disertakan untuk `reason` yang memang
   * mengirim kode (`self_verify` / `owned_by_other`) dan user memilih
   * memverifikasi — bukan saat user memilih ganti ke nomor lain.
   */
  function savePhone(input: { phone: string; otp?: string }): Promise<AccountPhoneSaveResult> {
    const body: Record<string, unknown> = { phone: input.phone }
    if (input.otp) body.otp = input.otp
    return api.post<AccountPhoneSaveResult>(SAVE_PATH, body)
  }

  return { requestOtp, savePhone }
}
