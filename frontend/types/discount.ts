// Shape response backend modul discount (CLAUDE.md §28). Sinkronkan kalau
// backend berubah.

export type DiscountType = 'percent' | 'nominal' | 'nominal_per_m2'
export type DiscountChannelScope = 'all' | 'online' | 'pos'
export type DiscountStatus = 'aktif' | 'terjadwal' | 'kadaluarsa' | 'nonaktif' | 'kuota_habis'
/** Cakupan produk (CLAUDE.md §28.9). `selected` TANPA `product_ids` bukan "berlaku semua" — itu diskon yang tidak bisa dipakai sama sekali. */
export type DiscountAppliesTo = 'all' | 'selected'
/** Cakupan audiens (CLAUDE.md §30.3). `member` menambah syarat customer harus berstatus `active` (lihat useMembership). */
export type DiscountAudienceScope = 'all' | 'member'
/**
 * Hanya relevan kalau `audience_scope === 'member'`. BEDA dengan `applies_to`
 * (§28.9): ini enum eksplisit, bukan "kosong = semua" — `selected_members`
 * dengan `customer_ids` kosong WAJIB ditolak backend (§30.3), jangan
 * disamakan dengan pola `applies_to==='selected'`.
 */
export type DiscountMemberScope = 'all_members' | 'selected_members' | ''

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
  /** Default `all` — sinkron dengan default backend (§28.9). */
  applies_to: DiscountAppliesTo
  /** Kosong kalau `applies_to === 'all'`. */
  product_ids: string[]
  /** Default `all` — sinkron dengan default backend (§30.3). */
  audience_scope: DiscountAudienceScope
  /** "" kalau `audience_scope === 'all'`. */
  member_scope: DiscountMemberScope
  /** Kosong kalau `member_scope !== 'selected_members'`. */
  customer_ids: string[]
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
  applies_to: DiscountAppliesTo
  /** Wajib diisi (minimal 1) kalau `applies_to === 'selected'` — backend menolak daftar kosong (§28.9). */
  product_ids: string[]
  audience_scope: DiscountAudienceScope
  /** Wajib diisi ('all_members'/'selected_members') kalau `audience_scope === 'member'`, wajib "" kalau tidak (§30.3). */
  member_scope: DiscountMemberScope
  /** Wajib diisi (minimal 1) kalau `member_scope === 'selected_members'` — backend menolak daftar kosong (§30.3). */
  customer_ids: string[]
}
