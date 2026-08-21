// Shape response backend audit log (super admin — edit/override status/delete
// pesanan, §26 admin override). Sinkronkan kalau backend berubah.

export type AuditAction = 'order.edit' | 'order.override_status' | 'order.delete' | string

export interface AuditLogEntry {
  id: string
  actor_user_id: string
  actor_name?: string
  action: AuditAction
  entity_label: string
  /** Ringkasan perubahan, bentuk bebas dari backend — dirender defensif di UI. */
  changes?: Record<string, unknown> | null
  reason: string
  created_at: string
}
