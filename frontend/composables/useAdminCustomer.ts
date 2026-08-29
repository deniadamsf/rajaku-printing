/**
 * useAdminCustomer — API wrapper untuk halaman admin "Pelanggan"
 * (CLAUDE.md §3 modul `auth`, §11). Permission `customer.view` untuk baca,
 * `customer.manage` untuk ubah data & blokir/aktifkan.
 *
 * `exportCsv` SENGAJA tidak lewat `useApi()` — endpoint export membalas file
 * CSV mentah, bukan envelope `{success, data, error}`. Pola fetch manual +
 * unduhan via anchor sementara ini identik dengan `useOrderRecap.exportCsv`
 * (tiru persis, jangan didesain ulang).
 */
import type {
  Customer,
  CustomerDetail,
  CustomerListParams,
  CustomerListResponse,
  CustomerUpdateInput,
} from '~/types/customer'

function buildListQuery(params: CustomerListParams): Record<string, string> {
  const query: Record<string, string> = {}
  if (params.q) query.q = params.q
  if (params.customer_type) query.customer_type = params.customer_type
  if (params.is_active !== '' && params.is_active !== undefined) {
    query.is_active = params.is_active ? 'true' : 'false'
  }
  if (params.membership_status) query.membership_status = params.membership_status
  if (params.page) query.page = String(params.page)
  if (params.per_page) query.per_page = String(params.per_page)
  return query
}

export function useAdminCustomer() {
  const api = useApi()
  const config = useRuntimeConfig()
  const tokenCookie = useAuthTokenCookie()

  /** GET /admin/customers — permission `customer.view`. */
  function list(params: CustomerListParams = {}): Promise<CustomerListResponse> {
    return api.get<CustomerListResponse>('/admin/customers', { query: buildListQuery(params) })
  }

  /** GET /admin/customers/:id — permission `customer.view`. */
  function get(id: string): Promise<CustomerDetail> {
    return api.get<CustomerDetail>(`/admin/customers/${id}`)
  }

  /** PATCH /admin/customers/:id — permission `customer.manage`. Kirim hanya field yang diubah. */
  function update(id: string, payload: CustomerUpdateInput): Promise<Customer> {
    return api.patch<Customer>(`/admin/customers/${id}`, payload)
  }

  /** POST /admin/customers/:id/deactivate — permission `customer.manage`, alasan wajib. */
  function deactivate(id: string, reason: string): Promise<Customer> {
    return api.post<Customer>(`/admin/customers/${id}/deactivate`, { reason })
  }

  /** POST /admin/customers/:id/activate — permission `customer.manage`, alasan wajib. */
  function activate(id: string, reason: string): Promise<Customer> {
    return api.post<Customer>(`/admin/customers/${id}/activate`, { reason })
  }

  /**
   * GET /admin/customers-export — path-nya SENGAJA `customers-export`, bukan
   * `customers/export`, sesuai kontrak backend (hindari bentrok wildcard
   * `/admin/customers/:id` seperti alasan yang sama di §28.5 untuk rekap).
   */
  async function exportCsv(filter: CustomerListParams): Promise<void> {
    const qs = new URLSearchParams(buildListQuery(filter)).toString()
    const url = `${config.public.apiBase}/admin/customers-export${qs ? `?${qs}` : ''}`
    const token = tokenCookie.value
    const res = await fetch(url, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    })
    if (!res.ok) {
      throw new Error(`Gagal mengunduh CSV (status ${res.status})`)
    }
    const blob = await res.blob()
    const objectUrl = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = objectUrl
    a.download = `pelanggan_${new Date().toISOString().slice(0, 10)}.csv`
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(objectUrl)
  }

  return { list, get, update, deactivate, activate, exportCsv }
}
