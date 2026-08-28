/**
 * useAdminSettings — API wrapper untuk setting global aplikasi (§19).
 *
 * Semua endpoint di-gate permission `settings.manage` di backend (hanya
 * super_admin yang punya secara default).
 */

export interface AppSetting {
  key: string
  value: string
  display_name: string
  description?: string
  updated_at: string
  updated_by?: string
  /**
   * Daftar nilai yang sah untuk key ini (mis. `["58", "80"]` untuk lebar
   * kertas struk) — proyeksi dari aturan enum di backend
   * (`settings/model/setting_item.go`). Hanya key enum yang membawa field
   * ini; key free-form (retensi hari, payment.*) tidak punya daftar
   * tertutup, jadi field ini undefined dan frontend tahu untuk merender
   * input teks/angka biasa, bukan pilihan tertutup.
   */
  allowed_values?: string[]
}

/** Key setting yang dikenal — samakan dengan settingsapi di backend. */
export const SETTING_DESIGN_RETENTION_DAYS = 'design_retention_days'

/**
 * Key rekening & QRIS (§7 pembayaran manual). Nilainya tampil LANGSUNG ke
 * pembeli di halaman pembayaran (`GET /payment-info`, publik) — lihat
 * `composables/usePaymentInfo.ts`. `qris_*` boleh kosong (QRIS belum tentu
 * tersedia), tiga key bank wajib diisi admin.
 */
export const SETTING_PAYMENT_BANK_NAME = 'payment.bank_name'
export const SETTING_PAYMENT_ACCOUNT_NAME = 'payment.account_name'
export const SETTING_PAYMENT_ACCOUNT_NUMBER = 'payment.account_number'
export const SETTING_PAYMENT_QRIS_NOTE = 'payment.qris_note'
export const SETTING_PAYMENT_QRIS_MERCHANT_NAME = 'payment.qris_merchant_name'
export const SETTING_PAYMENT_QRIS_NMID = 'payment.qris_nmid'

/**
 * Lebar roll kertas thermal kasir (§12). Nilainya string `"58"` atau `"80"`
 * (mm), dipakai frontend POS untuk membangun `@page { size: <n>mm auto }`
 * dinamis saat cetak struk — lihat `pages/admin/pos/index.vue`.
 */
export const SETTING_POS_RECEIPT_WIDTH_MM = 'pos.receipt_width_mm'

/**
 * Saklar on/off fitur membership customer (§30.1). Nilainya string
 * `"true"`/`"false"` — backend `settings/service` (`boolRules`) yang
 * memvalidasi, frontend cukup kirim string hasil `String(boolean)`.
 */
export const SETTING_MEMBERSHIP_ENABLED = 'membership_enabled'

/** Urutan render di card "Rekening & QRIS" — lihat `pages/admin/pengaturan/index.vue`. */
export const PAYMENT_SETTING_KEYS = [
  SETTING_PAYMENT_BANK_NAME,
  SETTING_PAYMENT_ACCOUNT_NAME,
  SETTING_PAYMENT_ACCOUNT_NUMBER,
  SETTING_PAYMENT_QRIS_NOTE,
  SETTING_PAYMENT_QRIS_MERCHANT_NAME,
  SETTING_PAYMENT_QRIS_NMID,
] as const

export function useAdminSettings() {
  const api = useApi()

  function list(): Promise<{ items: AppSetting[] }> {
    return api.get<{ items: AppSetting[] }>('/admin/settings')
  }

  function update(key: string, value: string): Promise<AppSetting> {
    return api.put<AppSetting>(`/admin/settings/${key}`, { value })
  }

  return { list, update }
}
