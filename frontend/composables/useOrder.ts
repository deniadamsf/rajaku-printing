/**
 * useOrder — API wrapper untuk modul order (§4).
 * Admin endpoints (/admin/orders/*) + fetch detail by resi (auth req) + public
 * create/tracking endpoints.
 */
import type { Order, OrderListResponse, OrderStatus, OrderChannel, MetodeAmbil, DesignSource } from '~/types/order'
import type { PublicTracking } from '~/types/tracking'

// Admin order detail response — merged order + customer + history.
export interface AdminOrderCustomer {
  id: string
  name: string
  phone: string
  email?: string
  type?: 'guest' | 'registered' | string
}

export interface AdminOrderHistoryRow {
  id: string
  from_status?: string
  to_status: string
  changed_by?: string
  changed_at: string
  note?: string
}

export interface AdminOrderDetailResponse {
  order: Order
  customer?: AdminOrderCustomer
  history: AdminOrderHistoryRow[]
}

// -------- Admin: edit / override status / delete (super admin only, §26) --------
/**
 * `customer_name`/`customer_phone` SENGAJA TIDAK ADA di sini — itu identitas
 * global pelanggan (dipakai lintas order), mengubahnya lewat endpoint ini
 * dulu diam-diam menyetel ulang nomor WA pelanggan untuk SEMUA pesanannya dan
 * melewati verifikasi OTP. Kontak pengiriman per-order pakai
 * `shipping_recipient_name`/`shipping_recipient_phone` di bawah.
 *
 * `total` juga TIDAK ADA — backend selalu menghitungnya sebagai
 * `subtotal + shipping_cost`, tidak lagi menerima nilai total dari client.
 */
export interface AdminOrderEditInput {
  shipping_recipient_name?: string
  shipping_recipient_phone?: string
  shipping_address?: string
  notes?: string
  subtotal?: number
  shipping_cost?: number
  /** Wajib (min 10 karakter) kalau subtotal/shipping_cost diubah setelah order `dibayar`. */
  reason?: string
}

export interface AdminOverrideStatusInput {
  to_status: OrderStatus | string
  /** Wajib, min 10 karakter. */
  reason: string
}

export interface CreateOnlineOrderInput {
  guest_name?: string
  guest_phone?: string
  product_id: string
  material_id: string
  width_cm: number
  height_cm: number
  quantity: number
  metode_ambil: MetodeAmbil
  shipping_address?: string
  shipping_recipient_name?: string
  shipping_recipient_phone?: string
  design_source: DesignSource
  design_brief?: string
  notes?: string
}

export function useOrder() {
  const api = useApi()

  // -------- Public --------
  function createOnline(body: CreateOnlineOrderInput): Promise<Order> {
    return api.post<Order>('/orders', body)
  }

  function publicTracking(resi: string): Promise<PublicTracking> {
    return api.get<PublicTracking>(`/lacak/${resi}`)
  }

  // -------- Auth (customer + staff) --------
  function getByResi(resi: string): Promise<Order> {
    return api.get<Order>(`/orders/${resi}`)
  }

  // -------- Customer own list --------
  function listMine(params: {
    status?: OrderStatus | ''
    page?: number
    pageSize?: number
  }): Promise<OrderListResponse> {
    const query: Record<string, string> = {}
    if (params.status) query.status = params.status
    if (params.page) query.page = String(params.page)
    if (params.pageSize) query.page_size = String(params.pageSize)
    return api.get<OrderListResponse>('/orders', { query })
  }

  // -------- Admin detail (order + customer + history) --------
  function adminGetDetail(resi: string): Promise<AdminOrderDetailResponse> {
    return api.get<AdminOrderDetailResponse>(`/admin/orders/${resi}`)
  }

  // -------- Admin --------
  function listAdmin(params: {
    status?: OrderStatus | ''
    channel?: OrderChannel | ''
    page?: number
    pageSize?: number
  }): Promise<OrderListResponse> {
    const query: Record<string, string> = {}
    if (params.status) query.status = params.status
    if (params.channel) query.channel = params.channel
    if (params.page) query.page = String(params.page)
    if (params.pageSize) query.page_size = String(params.pageSize)
    return api.get<OrderListResponse>('/admin/orders', { query })
  }

  function setShippingCost(resi: string, body: { shipping_cost: number; note?: string }): Promise<Order> {
    return api.post<Order>(`/admin/orders/${resi}/shipping-cost`, body)
  }

  function confirmPickup(resi: string, note?: string): Promise<Order> {
    return api.post<Order>(`/admin/orders/${resi}/confirm-pickup`, note ? { note } : undefined)
  }

  function cancelOrder(resi: string, reason: string): Promise<Order> {
    return api.post<Order>(`/admin/orders/${resi}/cancel`, { reason })
  }

  /**
   * Edit data pesanan (super admin, `order.edit`). Field yang boleh dikirim
   * tergantung status order — lihat `AdminOrderEditInput`. Backend validasi
   * ulang (FIELD_NOT_EDITABLE / REASON_REQUIRED), frontend cuma pre-check UX.
   */
  function adminEditOrder(resi: string, body: AdminOrderEditInput): Promise<Order> {
    return api.patch<Order>(`/admin/orders/${resi}`, body)
  }

  /**
   * Override status ke status APAPUN di state machine §4 (super admin,
   * `order.override_status`) — bukan cuma transisi berikutnya yang valid.
   * Tidak mengirim notifikasi WA ke pelanggan.
   */
  function adminOverrideStatus(resi: string, body: AdminOverrideStatusInput): Promise<Order> {
    return api.post<Order>(`/admin/orders/${resi}/override-status`, body)
  }

  /**
   * Soft delete pesanan (super admin, `order.delete`). Resi tetap bisa
   * dilacak publik setelah dihapus — hanya hilang dari daftar/rekap admin.
   */
  function adminDeleteOrder(resi: string, reason: string): Promise<{ ok: boolean }> {
    return api.delete<{ ok: boolean }>(`/admin/orders/${resi}`, { body: { reason } })
  }

  return {
    createOnline,
    publicTracking,
    getByResi,
    listMine,
    adminGetDetail,
    listAdmin,
    setShippingCost,
    confirmPickup,
    cancelOrder,
    adminEditOrder,
    adminOverrideStatus,
    adminDeleteOrder,
  }
}
