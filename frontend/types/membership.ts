// Shape response backend modul membership (CLAUDE.md §30). Sinkronkan kalau
// backend berubah — lihat `backend/internal/membership/service/dto.go`
// (MembershipView) dan `backend/internal/membership/handler/routes.go`.

export type MembershipStatus = 'none' | 'pending' | 'active' | 'rejected' | 'revoked'

/**
 * Satu proyeksi dipakai semua endpoint (Apply/GetOwn/Approve/Reject/Revoke/
 * Reinstate/List item) — sama pola dengan `Discount` di modul diskon.
 */
export interface Membership {
  customer_id: string
  name: string
  phone: string
  status: MembershipStatus
  /** Snapshot setting `membership_enabled` (§30.1) saat response ini dibuat — pakai untuk sembunyikan CTA "Ajukan jadi Member" saat fitur nonaktif. */
  membership_enabled: boolean
  requested_at?: string | null
  decided_at?: string | null
  decided_by?: string | null
  /** Alasan reject/revoke terakhir. Kosong kalau belum pernah diputuskan, atau habis dikosongkan ulang saat re-apply (§30.2). */
  decision_note?: string
}

export interface MembershipListParams {
  status?: MembershipStatus | ''
  page?: number
  per_page?: number
}

export interface MembershipListResponse {
  items: Membership[]
  total: number
  page: number
  per_page: number
}
