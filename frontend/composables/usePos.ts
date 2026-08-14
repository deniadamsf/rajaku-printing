/**
 * usePos — API wrapper untuk modul POS/walk-in (§11).
 * Semua endpoint admin membutuhkan permission pos.create_order / pos.reconcile.
 */
import type { PosCreateOrderInput, PosCreateOrderResult } from '~/types/pos'

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

  return { createOrder, reconciliation }
}
