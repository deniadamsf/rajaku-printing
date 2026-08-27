// Shape response backend POS module (§11).

export type PosMetodeBayar = 'cash' | 'qris_pos'
export type PosMetodeAmbil = 'pickup' | 'kirim'
export type PosDesignSource = 'upload' | 'request'
export type PosDesignApprovalMode = 'instant_walkin' | 'async_notify'

export interface PosCreateOrderInput {
  customer_name: string
  customer_phone: string
  product_id: string
  material_id: string
  width_cm: number
  height_cm: number
  quantity: number
  metode_ambil: PosMetodeAmbil
  shipping_address?: string
  shipping_recipient_name?: string
  shipping_recipient_phone?: string
  shipping_cost?: number
  metode_bayar: PosMetodeBayar
  design_source: PosDesignSource
  /** Cuma relevan kalau design_source='request' — Skenario A ("bawa desain siap cetak") tidak pernah masuk loop approval (§11). */
  design_approval_mode?: PosDesignApprovalMode
  design_brief?: string
  notes?: string
  /** Diskon master (CLAUDE.md §28) yang dipilih kasir — mutually exclusive dengan `manual_discount_amount`. */
  discount_id?: string | null
  /** Diskon manual (nominal Rp) — backend WAJIB menolak kalau `discount_note` kosong. */
  manual_discount_amount?: number
  /** Alasan diskon manual. Wajib diisi kalau `manual_discount_amount` > 0. */
  discount_note?: string
}

export interface PosCreateOrderResult {
  order_id: string
  resi: string
  customer_id: string
  total: number
  metode_bayar: string
  metode_ambil: string
  created_at: string
  tracking_url: string
  invoice_url?: string
  invoice_number?: string
  /** Lebar kertas struk kasir (58 atau 80mm) — dipakai untuk `@page` saat cetak struk. */
  receipt_width_mm: number

  // Rincian pelanggan & item — dipakai `ReceiptStruk.vue` supaya struk yang
  // dicetak menyebut nama pembeli & barang yang dibeli, bukan cuma
  // resi/tanggal/total.
  customer_name: string
  customer_phone: string
  product_name: string
  material_name: string
  width_cm: number
  height_cm: number
  quantity: number
  unit_price: number
  subtotal: number
  /** undefined kalau pickup / belum di-set (§8). */
  shipping_cost?: number
  /** 0 kalau tidak ada diskon dipakai. */
  discount_amount?: number
  /** Nama diskon master ATAU keterangan diskon manual — null/undefined kalau `discount_amount` 0. */
  discount_label?: string | null
}

/** Konfigurasi lebar kertas struk aktif — GET /admin/pos/receipt-config (§12, dipakai fitur cetak ulang di Detail Order). */
export interface PosReceiptConfig {
  width_mm: number
  allowed_widths_mm: number[]
}
