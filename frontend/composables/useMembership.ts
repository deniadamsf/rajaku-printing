/**
 * useMembership — API wrapper untuk modul membership (CLAUDE.md §30).
 * Endpoint customer (`account/membership/*`) dan admin (`admin/membership/*`)
 * hidup di satu composable karena backend-nya juga satu modul kecil.
 */
import type {
  Membership,
  MembershipListParams,
  MembershipListResponse,
} from '~/types/membership'

export function useMembership() {
  const api = useApi()

  // -------------------- customer (§30.2) --------------------

  /** POST /account/membership/apply — customer mengajukan diri jadi member. Body kosong. */
  function apply(): Promise<Membership> {
    return api.post<Membership>('/account/membership/apply')
  }

  /** GET /account/membership — customer baca status membership dirinya sendiri. */
  function getOwn(): Promise<Membership> {
    return api.get<Membership>('/account/membership')
  }

  // -------------------- admin (§30.2/§30.5) --------------------

  /** GET /admin/membership — permission `membership.view`. */
  function list(params: MembershipListParams = {}): Promise<MembershipListResponse> {
    const query: Record<string, string> = {}
    if (params.status) query.status = params.status
    if (params.page) query.page = String(params.page)
    if (params.per_page) query.per_page = String(params.per_page)
    return api.get<MembershipListResponse>('/admin/membership', { query })
  }

  /** POST /admin/membership/:id/approve — permission `membership.manage`, pending -> active. */
  function approve(customerId: string): Promise<Membership> {
    return api.post<Membership>(`/admin/membership/${customerId}/approve`)
  }

  /** POST /admin/membership/:id/reject — permission `membership.manage`, pending -> rejected, alasan wajib. */
  function reject(customerId: string, reason: string): Promise<Membership> {
    return api.post<Membership>(`/admin/membership/${customerId}/reject`, { reason })
  }

  /** POST /admin/membership/:id/revoke — permission `membership.manage`, active -> revoked, alasan wajib. */
  function revoke(customerId: string, reason: string): Promise<Membership> {
    return api.post<Membership>(`/admin/membership/${customerId}/revoke`, { reason })
  }

  /** POST /admin/membership/:id/reinstate — permission `membership.manage`, revoked -> active, alasan wajib (§30.2 — tidak self-service). */
  function reinstate(customerId: string, reason: string): Promise<Membership> {
    return api.post<Membership>(`/admin/membership/${customerId}/reinstate`, { reason })
  }

  return { apply, getOwn, list, approve, reject, revoke, reinstate }
}
