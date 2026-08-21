/**
 * usePos — API wrapper untuk modul POS/walk-in (§11).
 * Semua endpoint admin membutuhkan permission pos.create_order / pos.reconcile.
 */
import type { PosCreateOrderInput, PosCreateOrderResult, PosReceiptConfig } from '~/types/pos'

export interface PosReconciliationReport {
  date: string
  total_orders: number
  total_revenue: number
  by_metode_bayar: Record<string, { count: number; revenue: number }>
  by_kasir: Array<{ kasir_id: string; count: number; revenue: number }>
}

export function usePos() {
  const api = useApi()

  function createOrder(body: PosCreateOrderInput): Promise<PosCreateOrderResult> {
    return api.post<PosCreateOrderResult>('/admin/pos/orders', body)
  }

  function reconciliation(dateYYYYMMDD: string): Promise<PosReconciliationReport> {
    return api.get<PosReconciliationReport>('/admin/pos/reconciliation', {
      query: { date: dateYYYYMMDD },
    })
  }

  /**
   * Lebar kertas struk aktif — dipakai fitur "cetak ulang struk" di halaman
   * Detail Order (lintas role, bukan cuma kasir; lihat komentar route
   * backend `pos/handler/routes.go`). Caller WAJIB menangani rejection
   * sendiri dengan fallback 58mm — endpoint ini tidak boleh menggagalkan
   * render halaman pemanggil kalau request gagal.
   */
  function receiptConfig(): Promise<PosReceiptConfig> {
    return api.get<PosReceiptConfig>('/admin/pos/receipt-config')
  }

  return { createOrder, reconciliation, receiptConfig }
}
