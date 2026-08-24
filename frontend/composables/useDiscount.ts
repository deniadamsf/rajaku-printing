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
   */
  async function applicable(params: { channel: Exclude<DiscountChannelScope, 'all'>; subtotal: number }): Promise<ApplicableDiscount[]> {
    const res = await api.get<{ items: ApplicableDiscount[] }>('/admin/discounts/applicable', {
      query: { channel: params.channel, subtotal: String(params.subtotal) },
    })
    return res.items
  }

  return { list, get, create, update, remove, applicable }
}
