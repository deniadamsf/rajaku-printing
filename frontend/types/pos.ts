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
  design_approval_mode: PosDesignApprovalMode
  design_brief?: string
  notes?: string
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
}
