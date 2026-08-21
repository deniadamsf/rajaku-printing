/**
 * useAuditLog — API wrapper untuk jejak audit aksi super admin (edit/override
 * status/delete pesanan, dll). Dipakai di halaman detail order admin.
 *
 * Endpoint: GET /admin/audit-log?entity_id=<id>&limit=<n> — perm `audit.view`.
 */
import type { AuditLogEntry } from '~/types/audit'

export function useAuditLog() {
  const api = useApi()

  function listByEntity(entityId: string, limit = 50): Promise<AuditLogEntry[]> {
    return api.get<AuditLogEntry[]>('/admin/audit-log', {
      query: { entity_id: entityId, limit: String(limit) },
    })
  }

  return {
    listByEntity,
  }
}
