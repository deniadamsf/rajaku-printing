/**
 * usePayment — API wrapper untuk verifikasi bukti pembayaran (§7).
 * Semua endpoint admin membutuhkan permission payment.verify / payment.reject.
 *
 * Endpoint yg dipakai customer:
 *   GET /orders/:resi/payment-proofs — list bukti milik order (§7), bentuk
 *   response dipangkas (`PaymentProofCustomer`, tanpa `reviewed_by`) — dipanggil
 *   listProofsForOrder.
 *
 * Sama seperti useDesign, composable ini mendukung override token supaya bisa
 * dipakai sesi guest terverifikasi (bukan cuma cookie `rajaku_token`). Panggil
 * `usePayment({ token: () => guestToken.value })` untuk kasus itu; default
 * `usePayment()` tanpa argumen tetap pakai cookie seperti biasa.
 */
import type {
  PaymentProof,
  PaymentProofCustomerListResponse,
  PaymentProofListResponse,
  ProofStatus,
} from '~/types/payment'
import type { UseApiOptions } from '~/composables/useApi'

export function usePayment(options: UseApiOptions = {}) {
  const api = useApi(options)
  const config = useRuntimeConfig()
  const tokenCookie = useAuthTokenCookie()
  const getToken = options.token ?? (() => tokenCookie.value)

  // -------- Customer upload --------
  async function uploadProof(
    resi: string,
    body: { file: File; metodeBayar: 'transfer' | 'qris'; amountClaimed?: number },
  ): Promise<PaymentProof> {
    const form = new FormData()
    form.append('file', body.file)
    form.append('metode_bayar', body.metodeBayar)
    if (body.amountClaimed != null) form.append('amount_claimed', String(body.amountClaimed))
    return api.post<PaymentProof>(`/orders/${resi}/payment-proof`, form)
  }

  /**
   * listProofsForOrder — bukti pembayaran milik satu order, untuk pelanggan
   * memantau status verifikasi (bukan endpoint admin). Terurut terbaru dulu.
   */
  function listProofsForOrder(resi: string): Promise<PaymentProofCustomerListResponse> {
    return api.get<PaymentProofCustomerListResponse>(`/orders/${resi}/payment-proofs`)
  }

  function listProofs(params: {
    status?: ProofStatus | ''
    page?: number
    pageSize?: number
    orderId?: string
  }): Promise<PaymentProofListResponse> {
    const query: Record<string, string> = {}
    if (params.status) query.status = params.status
    if (params.page) query.page = String(params.page)
    if (params.pageSize) query.page_size = String(params.pageSize)
    if (params.orderId) query.order_id = params.orderId
    return api.get<PaymentProofListResponse>('/admin/payment-proofs', { query })
  }

  function approveProof(id: string): Promise<PaymentProof> {
    return api.post<PaymentProof>(`/admin/payment-proofs/${id}/verify`)
  }

  function rejectProof(id: string, reason: string): Promise<PaymentProof> {
    return api.post<PaymentProof>(`/admin/payment-proofs/${id}/reject`, { reason })
  }

  /**
   * proofFileURL — endpoint stream file bukti. Server require Authorization
   * header (Bearer token) — <img src=...> dari browser TIDAK bisa attach
   * header, jadi untuk preview di <img> pakai proofFileBlobURL() (fetch → blob).
   * Endpoint URL tetap diexpose untuk link "Download" (browser handle sendiri
   * setelah user klik + query ?download=1 memicu Content-Disposition: attachment).
   */
  function proofFileURL(id: string, opts?: { download?: boolean }): string {
    const q = opts?.download ? '?download=1' : ''
    return `${config.public.apiBase}/payment-proofs/${id}/file${q}`
  }

  /**
   * proofFilePreviewURL — fetch dgn Bearer, return object URL yg bisa dipasang
   * di <img src=...>. Caller wajib URL.revokeObjectURL saat unmount / ganti
   * proof supaya tidak leak memory.
   */
  async function proofFilePreviewURL(id: string): Promise<string> {
    const token = getToken()
    const res = await fetch(proofFileURL(id), {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    })
    if (!res.ok) throw new Error(`preview fetch gagal: ${res.status}`)
    const blob = await res.blob()
    return URL.createObjectURL(blob)
  }

  return {
    uploadProof,
    listProofs,
    listProofsForOrder,
    approveProof,
    rejectProof,
    proofFileURL,
    proofFilePreviewURL,
  }
}
