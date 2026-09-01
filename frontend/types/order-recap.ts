// Shape response backend `GET /admin/order-recap` (CLAUDE.md §28).

import type { OrderChannel, OrderStatus } from '~/types/order'

export interface OrderRecapSummary {
  order_count: number
  gross_subtotal: number
  total_discount: number
  total_shipping: number
  net_total: number
  average_order_value: number
}

export interface OrderRecapItem {
  created_at: string
  resi: string
  /** Backend JSON tag `omitempty` — bisa hilang sama sekali dari payload kalau kosong. */
  customer_name?: string
  /** Nama item line_no=1, sudah final untuk tampil (§32.8) — akhiran "+N lainnya" kalau jumlah_item > 1. Jangan dihitung ulang di frontend. */
  product_name: string
  /** Jumlah baris produk order ini (§32.8). */
  jumlah_item: number
  channel: OrderChannel | string
  status: OrderStatus | string
  subtotal: number
  discount_amount: number
  discount_label?: string
  shipping_cost: number
  total: number
  metode_bayar?: string
  created_by_name?: string
}

export interface OrderRecapResponse {
  summary: OrderRecapSummary
  items: OrderRecapItem[]
  total: number
  page: number
  per_page: number
}

/** Query filter — `from`/`to` wajib diisi (YYYY-MM-DD), sisanya opsional. */
export interface OrderRecapFilter {
  from: string
  to: string
  channel?: OrderChannel | ''
  status?: OrderStatus | ''
  created_by?: string
  discount_id?: string
  /**
   * Menyaring order berdiskon manual (tanpa master diskon, `discount_id`
   * NULL di tabel `orders`) — param TERPISAH dari `discount_id` supaya tidak
   * tertukar dengan "tanpa filter diskon sama sekali". Mutually exclusive
   * dengan `discount_id`: kirim salah satu saja.
   */
  discount_manual?: boolean
  only_discounted?: boolean
  page?: number
  per_page?: number
}

/** Opsi dropdown filter kasir — `GET /admin/order-recap/filters`. */
export interface OrderRecapKasirOption {
  id: string
  name: string
}

/**
 * Opsi dropdown filter diskon — `GET /admin/order-recap/filters`.
 * Entri dengan `id: null` mewakili "Diskon manual" (order berdiskon tanpa
 * master diskon) — dipetakan ke param `discount_manual`, BUKAN `discount_id`.
 */
export interface OrderRecapDiskonOption {
  id: string | null
  label: string
}

/** Response `GET /admin/order-recap/filters?from=&to=` — daftar berubah mengikuti rentang tanggal. */
export interface OrderRecapFilterOptions {
  kasir: OrderRecapKasirOption[]
  diskon: OrderRecapDiskonOption[]
}
