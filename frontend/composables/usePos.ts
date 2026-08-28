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

/**
 * Hasil pencarian pelanggan lintas channel (§11) — WA jadi matching key,
 * jadi pelanggan yang sama muncul satu kali walau pernah order online
 * maupun walk-in sebelumnya.
 */
export interface PosCustomerSearchResult {
  id: string
  name: string
  phone: string
  /**
   * Proyeksi `authapi.Identity.MembershipStatus` (§30.4) — dipakai layar
   * kasir untuk menampilkan badge "Member" saat `'active'`, dan untuk
   * mengirim `customer_id` ke `useDiscount().applicable()` supaya diskon
   * khusus member ikut tersaring (§30.3).
   */
  membership_status: 'none' | 'pending' | 'active' | 'rejected' | 'revoked'
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
   * Cari pelanggan yang sudah pernah order (online atau walk-in) berdasarkan
   * nama/no. WA, supaya kasir bisa autofill form alih-alih mengetik ulang dan
   * berisiko membuat identitas mendekati-duplikat (§11 — WA = matching key).
   */
  function searchCustomers(q: string): Promise<PosCustomerSearchResult[]> {
    return api.get<PosCustomerSearchResult[]>('/admin/pos/customers/search', {
      query: { q },
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

  return { createOrder, reconciliation, receiptConfig, searchCustomers }
}
