// Shape response backend modul customer/admin panel "Pelanggan" (CLAUDE.md
// §3 modul `auth`, §11 penyatuan identitas walk-in/online). Sinkronkan kalau
// backend berubah — endpoint `admin/customers*`.

export type CustomerType = 'guest' | 'registered'

/** Sinkron dengan `MembershipStatus` di `types/membership.ts` (§30.2) — didup­likasi di sini supaya file ini berdiri sendiri tanpa import silang antar tipe. */
export type CustomerMembershipStatus = 'none' | 'pending' | 'active' | 'rejected' | 'revoked'

export interface Customer {
  id: string
  name: string
  phone: string | null
  phone_verified: boolean
  email: string | null
  customer_type: CustomerType
  is_active: boolean
  membership_status: CustomerMembershipStatus
  oauth_provider: 'google' | null
  last_login_at: string | null
  created_at: string
}

export interface CustomerListParams {
  q?: string
  customer_type?: CustomerType | ''
  /** Dikirim sebagai string "true"/"false" ke query — dikosongkan berarti semua. */
  is_active?: boolean | ''
  membership_status?: CustomerMembershipStatus | ''
  page?: number
  per_page?: number
}

export interface CustomerListResponse {
  items: Customer[]
  total: number
  page: number
  per_page: number
}

export interface CustomerOrderStats {
  total_orders: number
  completed_orders: number
  cancelled_orders: number
  total_spend: number
  last_order_at: string | null
}

export interface CustomerRecentOrder {
  resi: string
  status: string
  channel: 'online' | 'pos' | string
  total: number
  created_at: string
}

export type CustomerAdminLogAction = 'block' | 'unblock' | 'profile_update'

export interface CustomerAdminLog {
  action: CustomerAdminLogAction
  reason: string | null
  /** Ringkasan teks utk `profile_update`, mis. `nama: "A" → "B"; wa: "62…" → "62…"`. Null utk block/unblock. */
  changes: string | null
  /** Null berarti perubahan dilakukan sistem, bukan staff. */
  changed_by_name: string | null
  created_at: string
}

export interface CustomerDetail {
  customer: Customer
  order_stats: CustomerOrderStats
  recent_orders: CustomerRecentOrder[]
  admin_logs: CustomerAdminLog[]
}

/** Body `PATCH /admin/customers/:id` — kirim hanya field yang benar-benar diubah. String kosong berarti mengosongkan email/nomor. */
export interface CustomerUpdateInput {
  name?: string
  phone?: string
  email?: string
}
