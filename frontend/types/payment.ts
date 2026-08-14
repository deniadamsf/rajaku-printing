// Response shape backend payment module.

export type ProofStatus = 'pending' | 'approved' | 'rejected'
export type MetodeBayar = 'transfer' | 'qris' | 'cash' | 'qris_pos'

export interface PaymentProof {
  id: string
  order_id: string
  metode_bayar: MetodeBayar | string
  file_original_name: string
  file_size_bytes: number
  file_mime_type: string
  amount_claimed?: number | null
  uploaded_at: string
  status: ProofStatus | string
  reviewed_at?: string
  reviewed_by?: string
  reject_reason?: string
}

export interface PaymentProofListResponse {
  items: PaymentProof[]
  total: number
  page: number
  page_size: number
}
