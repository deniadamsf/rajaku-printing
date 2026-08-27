// Shape response backend order module. Sinkronkan kalau backend berubah.

export type OrderStatus =
  | 'order_masuk'
  | 'menunggu_ongkir'
  | 'menunggu_pembayaran'
  | 'menunggu_verifikasi'
  | 'dibayar'
  | 'ditolak'
  | 'desain_dikerjakan'
  | 'menunggu_approval_desain'
  | 'desain_diverifikasi'
  | 'proses_cetak'
  | 'qc'
  | 'siap_kirim'
  | 'siap_ambil'
  | 'dikirim'
  | 'selesai'
  | 'dibatalkan'

export type OrderChannel = 'online' | 'pos'
export type MetodeAmbil = 'pickup' | 'kirim'
export type MetodeBayar = 'transfer' | 'qris' | 'cash' | 'qris_pos'
export type DesignSource = 'upload' | 'request'
export type DesignApprovalMode = 'instant_walkin' | 'async_notify'
export type PricingType = 'per_m2' | 'paket'

export interface Order {
  id: string
  resi: string
  status: OrderStatus | string
  channel: OrderChannel | string
  product_name: string
  material_name: string
  pricing_type: PricingType | string
  width_cm: number
  height_cm: number
  quantity: number
  unit_price: number
  subtotal: number
  metode_ambil: MetodeAmbil | string
  shipping_cost?: number | null
  shipping_address?: string
  shipping_recipient_name?: string
  shipping_recipient_phone?: string
  metode_bayar?: MetodeBayar | string
  design_source: DesignSource | string
  /** Cuma terisi kalau design_source='request' (§11 Skenario B) — Skenario A tidak pernah punya mode approval. */
  design_approval_mode?: DesignApprovalMode | string | null
  design_brief?: string
  total: number
  notes?: string
  created_at: string
  /** Diskon terpakai (CLAUDE.md §28) — snapshot pada saat order dibuat, tetap tampil walau master diskon sudah dihapus. */
  discount_amount?: number | null
  discount_name_snapshot?: string | null
  discount_code_snapshot?: string | null
  discount_note?: string | null
}

export interface OrderListResponse {
  items: Order[]
  total: number
  page: number
  page_size: number
}
