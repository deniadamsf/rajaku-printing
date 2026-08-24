/**
 * useOrderRecap — API wrapper untuk laporan rekap order (CLAUDE.md §28,
 * permission `report.view`).
 *
 * `exportCsv` SENGAJA tidak lewat `useApi()` — endpoint export membalas file
 * CSV mentah, bukan envelope `{success, data, error}` yang diasumsikan
 * wrapper itu. Fetch manual dengan Bearer token dari cookie sesi yang sama
 * (§2: base URL tetap dari `runtimeConfig`, bukan hardcode), lalu picu unduhan
 * lewat anchor sementara — pola sama dengan `proofFileURL`/`download` di
 * `usePayment.ts`.
 */
import type { OrderRecapFilter, OrderRecapFilterOptions, OrderRecapResponse } from '~/types/order-recap'

/** Bilangan bulat positif valid untuk dikirim sebagai `page`/`per_page` — backend menolak (400) nilai non-angka. */
function safeInt(n: number | undefined): string | undefined {
  if (n === undefined || !Number.isFinite(n)) return undefined
  const rounded = Math.trunc(n)
  return rounded > 0 ? String(rounded) : undefined
}

function buildQuery(f: OrderRecapFilter): Record<string, string> {
  const q: Record<string, string> = { from: f.from, to: f.to }
  if (f.channel) q.channel = f.channel
  if (f.status) q.status = f.status
  if (f.created_by) q.created_by = f.created_by
  // `discount_id` dan `discount_manual` mutually exclusive (lihat types/order-recap.ts).
  if (f.discount_manual) {
    q.discount_manual = 'true'
  } else if (f.discount_id) {
    q.discount_id = f.discount_id
  }
  if (f.only_discounted) q.only_discounted = 'true'
  const page = safeInt(f.page)
  if (page) q.page = page
  const perPage = safeInt(f.per_page)
  if (perPage) q.per_page = perPage
  return q
}

export function useOrderRecap() {
  const api = useApi()
  const config = useRuntimeConfig()
  const tokenCookie = useAuthTokenCookie()

  function list(filter: OrderRecapFilter): Promise<OrderRecapResponse> {
    return api.get<OrderRecapResponse>('/admin/order-recap', { query: buildQuery(filter) })
  }

  /**
   * Opsi dropdown kasir/diskon untuk rentang tanggal tertentu — daftar
   * berubah tiap `from`/`to` berubah (nilai distinct yang benar-benar
   * muncul di rentang itu), jadi TIDAK boleh di-cache lintas rentang.
   */
  function filters(range: { from: string; to: string }): Promise<OrderRecapFilterOptions> {
    return api.get<OrderRecapFilterOptions>('/admin/order-recap/filters', {
      query: { from: range.from, to: range.to },
    })
  }

  async function exportCsv(filter: OrderRecapFilter): Promise<void> {
    const qs = new URLSearchParams(buildQuery(filter)).toString()
    const url = `${config.public.apiBase}/admin/order-recap/export?${qs}`
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
    a.download = `rekap-order_${filter.from}_${filter.to}.csv`
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(objectUrl)
  }

  return { list, filters, exportCsv }
}
