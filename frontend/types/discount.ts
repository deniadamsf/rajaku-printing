// Shape response backend modul discount (CLAUDE.md §28). Sinkronkan kalau
// backend berubah.

export type DiscountType = 'percent' | 'nominal'
export type DiscountChannelScope = 'all' | 'online' | 'pos'
export type DiscountStatus = 'aktif' | 'terjadwal' | 'kadaluarsa' | 'nonaktif' | 'kuota_habis'

export interface Discount {
  id: string
  code: string
  name: string
  type: DiscountType
  value_percent: number | null
  value_amount: number | null
  max_discount_amount: number | null
  min_subtotal: number
  starts_at: string | null
  ends_at: string | null
  quota: number | null
  usage_count: number
  channel_scope: DiscountChannelScope
  is_active: boolean
  status: DiscountStatus
  created_at: string
  updated_at: string
}

/** Hasil `GET /admin/discounts/applicable` — Discount + estimasi potongan untuk subtotal yang dikirim. */
export interface ApplicableDiscount extends Discount {
  preview_amount: number
}

export interface DiscountListResponse {
  items: Discount[]
  total: number
  page: number
  per_page: number
}

/**
 * Body create/update. Field kosong/undefined dianggap "tanpa batas" oleh
 * backend UNTUK `max_discount_amount`/`starts_at`/`ends_at`/`quota` — kirim
 * `null` eksplisit untuk mengosongkannya lewat PATCH.
 *
 * `value_percent`/`value_amount` BEDA ATURAN: PATCH backend (`parseUpdateInput`)
 * menolak keras `null` eksplisit untuk dua field ini (cuma menerima "field
 * absen" atau "field berisi angka") — caller WAJIB mengirim `undefined`
 * (bukan `null`) kalau field itu tidak relevan untuk `type` yang dipilih.
 */
export interface DiscountInput {
  code: string
  name: string
  type: DiscountType
  value_percent?: number | null
  value_amount?: number | null
  max_discount_amount?: number | null
  min_subtotal: number
  starts_at?: string | null
  ends_at?: string | null
  quota?: number | null
  channel_scope: DiscountChannelScope
  is_active: boolean
}
