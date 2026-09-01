// Shape response backend POS module (§11, §32 Order Multi-Item).

export type PosMetodeBayar = 'cash' | 'qris_pos'
export type PosMetodeAmbil = 'pickup' | 'kirim'
export type PosDesignSource = 'upload' | 'request'
export type PosDesignApprovalMode = 'instant_walkin' | 'async_notify'

/**
 * PosCreateOrderItemInput — satu baris produk untuk `PosCreateOrderInput`
 * (§32). `design_source`/`design_brief`/`item_notes` PER ITEM (§32.5) — satu
 * order walk-in boleh campur: banner A bawa desain sendiri, banner B minta
 * dibuatkan. `design_approval_mode` TETAP per order, lihat
 * `PosCreateOrderInput` — bukan field di sini.
 */
export interface PosCreateOrderItemInput {
  product_id: string
  material_id: string
  width_cm: number
  height_cm: number
  quantity: number
  design_source: PosDesignSource
  design_brief?: string
  item_notes?: string
}

export interface PosCreateOrderInput {
  customer_name: string
  customer_phone: string
  /** 1..20 baris (§32.4) — backend menolak kosong atau lebih dari 20. */
  items: PosCreateOrderItemInput[]
  metode_ambil: PosMetodeAmbil
  shipping_address?: string
  shipping_recipient_name?: string
  shipping_recipient_phone?: string
  shipping_cost?: number
  metode_bayar: PosMetodeBayar
  /** Tetap PER ORDER (§11, §32.5) walau item-nya banyak — relevan kalau minimal satu item `design_source='request'`. */
  design_approval_mode?: PosDesignApprovalMode
  notes?: string
  /** Diskon master (CLAUDE.md §28) yang dipilih kasir — mutually exclusive dengan `manual_discount_amount`. */
  discount_id?: string | null
  /** Diskon manual (nominal Rp) — backend WAJIB menolak kalau `discount_note` kosong. */
  manual_discount_amount?: number
  /** Alasan diskon manual. Wajib diisi kalau `manual_discount_amount` > 0. */
  discount_note?: string
}

/**
 * PosReceiptItem — satu baris produk untuk struk/tampilan kasir (§32.7),
 * sejajar `backend/internal/pos/service/dto.go` `ReceiptItem`. Terurut sama
 * seperti `order_items` (line_no ASC) — backend TIDAK mengirim `line_no`
 * eksplisit di sini, urutan array-nya sendiri sudah final.
 */
export interface PosReceiptItem {
  product_name: string
  material_name: string
  width_cm: number
  height_cm: number
  quantity: number
  unit_price: number
  subtotal: number
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
  /** Baris item (§32.7) — 1..20 baris, terurut line_no ASC. */
  items: PosReceiptItem[]
  /** Agregat order (Σ item.subtotal, §32.2), BUKAN item pertama — layar uang selalu baca ini. */
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
