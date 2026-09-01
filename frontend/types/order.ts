// Shape response backend order module. Sinkronkan kalau backend berubah.
//
// §32 (Order Multi-Item, 31 Agustus 2026): satu order sekarang bisa memuat
// banyak baris produk (`items[]`) — kolom produk/ukuran/harga PINDAH dari
// `orders` ke `order_items`. Field UANG (subtotal, discount_amount,
// shipping_cost, total) TETAP di level order (§32.2/§28.2 lapis 3) — jangan
// pernah menjumlahkan `items[]` untuk angka itu, selalu baca field order.

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
/** Sumber desain PER ITEM (§32.5) — cuma dua nilai, item tidak pernah 'mixed'. */
export type DesignSource = 'upload' | 'request'
/** Turunan di level order (§32.5.2): 'mixed' kalau item-itemnya campur upload+request. */
export type OrderDesignSource = DesignSource | 'mixed'
export type DesignApprovalMode = 'instant_walkin' | 'async_notify'
export type PricingType = 'per_m2' | 'paket'

/**
 * OrderItem — satu baris produk di dalam order (§32). Field uang di sini
 * (unit_price, subtotal, discount_amount) HANYA penjelas baris — jangan
 * pernah dijumlahkan untuk angka uang order, selalu baca field level `Order`
 * (§32.2, sambungan §28.2 lapis 3).
 */
export interface OrderItem {
  /**
   * order_items.id (UUID) — dibutuhkan untuk upload file desain per item
   * (§32.5, field `order_item_id` wajib di multipart upload) dan untuk
   * koreksi baris item super admin (§32.9). Dipetakan backend sejak
   * 1 September 2026 (`orderItemResponse.ID` di
   * `backend/internal/order/handler/dto.go`) — selalu terisi untuk order
   * manapun (termasuk order lama pra-§32, lihat backfill migration 000033).
   */
  id: string
  line_no: number
  product_id?: string
  product_name: string
  material_name: string
  pricing_type: PricingType | string
  width_cm: number
  height_cm: number
  quantity: number
  unit_price: number
  subtotal: number
  discount_amount: number
  design_source: DesignSource | string
  design_brief?: string
  item_notes?: string
}

export interface Order {
  id: string
  resi: string
  status: OrderStatus | string
  channel: OrderChannel | string
  /** Baris produk (§32) — 1..20 item. Order lama (pra-migrasi) tetap tampil normal sebagai array 1 item. */
  items: OrderItem[]
  /** Level ORDER, bukan penjumlahan `items[]` (§32.2) — selalu baca ini untuk uang. */
  subtotal: number
  metode_ambil: MetodeAmbil | string
  shipping_cost?: number | null
  shipping_address?: string
  shipping_recipient_name?: string
  shipping_recipient_phone?: string
  metode_bayar?: MetodeBayar | string
  /** Turunan dari kumpulan `items[].design_source` — 'mixed' kalau campur upload+request (§32.5.2). */
  design_source: OrderDesignSource | string
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

/**
 * Label produk siap-tampil untuk konteks ringkas (daftar order, kartu akun,
 * dst) — nama item pertama + "+N lainnya" kalau order punya >1 item.
 * Backend sudah menyediakan pola yang sama di kolom rekap (§32.8); helper ini
 * dipakai layar yang TIDAK lewat endpoint rekap (mis. `/admin/order`,
 * `/akun`) sehingga tidak menghitung ulang string itu berkali-kali di template.
 */
export function orderPrimaryProductLabel(o: Pick<Order, 'items'>): string {
  const items = o.items ?? []
  if (items.length === 0) return '—'
  const first = items[0].product_name
  return items.length > 1 ? `${first} +${items.length - 1} lainnya` : first
}
