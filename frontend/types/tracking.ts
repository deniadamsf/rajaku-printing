// Shape response backend order module — public tracking (censored).

export interface PublicTrackingHistoryRow {
  status: string
  changed_at: string
}

/**
 * Satu baris produk (§32 Order Multi-Item) yang boleh dilihat pelanggan yang
 * melacak resinya — endpoint TANPA login (§5). Sengaja TIDAK memuat
 * unit_price/subtotal/diskon apa pun (lihat `PublicTrackingResultItem` di
 * `backend/internal/order/service/dto.go`) — halaman ini tidak pernah
 * mengekspos uang, dan itu tidak berubah sekarang order boleh multi-item.
 *
 * `id` + `design_source` ditambahkan backend (1 September 2026) SEMATA
 * supaya guest terverifikasi (token scope `guest_order`) bisa mengunggah
 * desain untuk BARIS yang benar — `POST /orders/:resi/design-files`
 * mewajibkan `order_item_id` (§32.5), dan endpoint order penuh
 * (`GET /orders/:resi`) tidak menerima token guest. Aman diekspos tanpa
 * login: `id` cuma berguna lewat endpoint unggah yang tetap memeriksa
 * kepemilikan, dan tidak ada nominal uang yang ikut.
 */
export interface PublicTrackingItem {
  id: string
  product_name: string
  material_name: string
  width_cm: number
  height_cm: number
  quantity: number
  /** 'upload' | 'request' per baris (§32.5) — dipakai memilih item mana yang boleh ditawari unggah mandiri. */
  design_source: string
}

export interface PublicTracking {
  resi: string
  status: string
  channel: string
  metode_ambil: string
  /**
   * Turunan dari kumpulan `items[].design_source` — 'mixed' kalau campur
   * upload+request (§32.5.2). Sejak `items[].design_source` per baris juga
   * dikirim, field order-level ini dipakai sekadar ringkasan tampilan, BUKAN
   * lagi untuk menggerbangi aksi unggah (itu sekarang per item, lihat
   * halaman lacak).
   */
  design_source: string
  /** Semua baris produk order ini, terurut line_no ASC — GANTI product_name/material_name datar yang lama (§32). */
  items: PublicTrackingItem[]
  shipping_recipient?: string
  shipping_phone?: string
  shipping_address?: string
  created_at: string
  history: PublicTrackingHistoryRow[]
}
