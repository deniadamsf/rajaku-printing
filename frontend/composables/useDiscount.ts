/**
 * useDiscount — API wrapper untuk modul diskon (CLAUDE.md §28).
 * CRUD (`discount.manage`) + lookup diskon yang berlaku untuk subtotal
 * tertentu (`discount.apply`, dipakai form kasir POS).
 */
import type {
  ApplicableDiscount,
  Discount,
  DiscountChannelScope,
  DiscountInput,
  DiscountListResponse,
} from '~/types/discount'

/**
 * ApplicableCartItem — satu baris keranjang yang dikirim ke
 * `/admin/discounts/applicable` (§32.3). Satu order bisa banyak item, dan
 * diskon `applies_to='selected'` hanya memotong subtotal item yang cocok —
 * jadi backend butuh subtotal PER BARIS, bukan satu angka gabungan.
 */
export interface ApplicableCartItem {
  product_id: string
  subtotal: number
  pricing_type?: string
  chargeable_m2?: number
  width_cm?: number
  height_cm?: number
  quantity?: number
}

export interface DiscountListParams {
  status?: string
  q?: string
  page?: number
  per_page?: number
}

export function useDiscount() {
  const api = useApi()

  function list(params: DiscountListParams = {}): Promise<DiscountListResponse> {
    const query: Record<string, string> = {}
    if (params.status) query.status = params.status
    if (params.q) query.q = params.q
    if (params.page) query.page = String(params.page)
    if (params.per_page) query.per_page = String(params.per_page)
    return api.get<DiscountListResponse>('/admin/discounts', { query })
  }

  function get(id: string): Promise<Discount> {
    return api.get<Discount>(`/admin/discounts/${id}`)
  }

  function create(body: DiscountInput): Promise<Discount> {
    return api.post<Discount>('/admin/discounts', body)
  }

  function update(id: string, body: Partial<DiscountInput>): Promise<Discount> {
    return api.patch<Discount>(`/admin/discounts/${id}`, body)
  }

  /** Alasan wajib (backend menolak kalau kosong) — order lama TETAP mencatat diskon ini di rekap, cuma master-nya yang hilang (§28.2). */
  function remove(id: string, reason: string): Promise<{ id: string; deleted: boolean }> {
    return api.delete<{ id: string; deleted: boolean }>(`/admin/discounts/${id}`, { body: { reason } })
  }

  /**
   * Backend membungkus hasil sebagai `{ items: [...] }`, bukan array polos.
   * `channel` HANYA menerima 'online'/'pos' (backend menolak 'all' — §28.5).
   * `items` (§32.3) — satu entri per baris keranjang, masing-masing
   * `product_id` + `subtotal` baris itu. Ini yang membuat diskon
   * cakupan `selected` tidak pernah muncul kalau tidak ada satu pun baris
   * yang tercakup, dan membuat syarat `min_subtotal` diukur terhadap nilai
   * baris yang benar-benar didiskon — mencegah kasir memilih diskon yang
   * bakal ditolak backend saat submit.
   * `customer_id` opsional (§30.3) — TANPA ini, diskon `audience_scope=member`
   * TIDAK IKUT MUNCUL sama sekali (bukan bug, disengaja backend). Kirim
   * begitu kasir sudah resolve pelanggan (dapat customer_id dari hasil
   * pencarian POS), supaya promo member ikut tersaring sejak awal.
   */
  async function applicable(params: { channel: Exclude<DiscountChannelScope, 'all'>; items: ApplicableCartItem[]; customer_id?: string }): Promise<ApplicableDiscount[]> {
    // §32.3 — `subtotal` tunggal DIGANTI pasangan berulang
    // `product_id` + `item_subtotal`, satu pasang per baris keranjang, dan
    // backend MEWAJIBKAN keduanya sama panjang & sejajar posisinya. Ini
    // bukan kerapian: diskon `applies_to='selected'` harus tahu subtotal
    // baris mana saja yang tercakup untuk menghitung `eligible_subtotal`
    // — dengan satu angka gabungan, promo satu produk akan tampil di layar
    // kasir lalu ditolak saat "Buat Pesanan".
    const query: [string, string][] = [['channel', params.channel]]
    for (const it of params.items) {
      query.push(['product_id', it.product_id], ['item_subtotal', String(it.subtotal)])
      query.push(['item_pricing_type', it.pricing_type ?? ''])
      query.push(['item_chargeable_m2', it.chargeable_m2 != null ? String(it.chargeable_m2) : ''])
      query.push(['item_width_cm', it.width_cm != null ? String(it.width_cm) : ''])
      query.push(['item_height_cm', it.height_cm != null ? String(it.height_cm) : ''])
      query.push(['item_quantity', it.quantity != null ? String(it.quantity) : ''])
    }
    if (params.customer_id) query.push(['customer_id', params.customer_id])
    const res = await api.get<{ items: ApplicableDiscount[] }>(
      `/admin/discounts/applicable?${new URLSearchParams(query).toString()}`,
    )
    return res.items
  }

  return { list, get, create, update, remove, applicable }
}
