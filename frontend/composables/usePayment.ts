/**
 * usePayment — API wrapper untuk verifikasi bukti pembayaran (§7).
 * Semua endpoint admin membutuhkan permission payment.verify / payment.reject.
 */
import type { PaymentProof, PaymentProofListResponse, ProofStatus } from '~/types/payment'

export function usePayment() {
  const api = useApi()
  const config = useRuntimeConfig()
  const tokenCookie = useAuthTokenCookie()

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
    const res = await fetch(proofFileURL(id), {
      headers: tokenCookie.value ? { Authorization: `Bearer ${tokenCookie.value}` } : {},
    })
    if (!res.ok) throw new Error(`preview fetch gagal: ${res.status}`)
    const blob = await res.blob()
    return URL.createObjectURL(blob)
  }

  return {
    uploadProof,
    listProofs,
    approveProof,
    rejectProof,
    proofFileURL,
    proofFilePreviewURL,
  }
}
