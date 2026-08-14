// Shape response backend order module — public tracking (censored).

export interface PublicTrackingHistoryRow {
  status: string
  changed_at: string
}

export interface PublicTracking {
  resi: string
  status: string
  channel: string
  metode_ambil: string
  /**
   * 'upload' = pelanggan bawa desain sendiri, 'request' = minta dibuatkan tim.
   * Dipakai halaman lacak untuk menentukan aksi unggah mana yang ditawarkan
   * ke guest terverifikasi. Backend membukanya di payload publik karena bukan
   * data sensitif (lihat PublicTrackingResult di order/service/dto.go).
   */
  design_source: string
  product_name: string
  material_name: string
  shipping_recipient?: string
  shipping_phone?: string
  shipping_address?: string
  created_at: string
  history: PublicTrackingHistoryRow[]
}
