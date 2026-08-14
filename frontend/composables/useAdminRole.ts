/**
 * useAdminRole — API wrapper untuk kelola role & permission toggle (§10).
 * Semua endpoint di-gate permission role.manage di backend.
 *
 * Note: listRoles + listPermissions juga ada di useAdminStaff (dipakai di form
 * assign role). Duplikasi ringan lebih baik dari cross-module coupling.
 */
import type { AdminPermission, AdminRole } from '~/types/staff'

export interface CreateRoleInput {
  name: string
  display_name: string
  description?: string
  permission_codes?: string[]
}

export interface UpdateRoleBasicInput {
  display_name: string
  description?: string
}

export function useAdminRole() {
  const api = useApi()

  function listRoles(): Promise<{ items: AdminRole[] }> {
    return api.get<{ items: AdminRole[] }>('/admin/roles')
  }

  function getRole(id: string): Promise<AdminRole> {
    return api.get<AdminRole>(`/admin/roles/${id}`)
  }

  function listPermissions(): Promise<{ items: AdminPermission[] }> {
    return api.get<{ items: AdminPermission[] }>('/admin/permissions')
  }

  function createRole(body: CreateRoleInput): Promise<AdminRole> {
    return api.post<AdminRole>('/admin/roles', body)
  }

  function updateRoleBasic(id: string, body: UpdateRoleBasicInput): Promise<{ ok: boolean }> {
    return api.patch<{ ok: boolean }>(`/admin/roles/${id}`, body)
  }

  function deleteRole(id: string): Promise<{ ok: boolean }> {
    return api.delete<{ ok: boolean }>(`/admin/roles/${id}`)
  }

  function setPermissions(id: string, codes: string[]): Promise<{ ok: boolean }> {
    return api.put<{ ok: boolean }>(`/admin/roles/${id}/permissions`, { permission_codes: codes })
  }

  return {
    listRoles,
    getRole,
    listPermissions,
    createRole,
    updateRoleBasic,
    deleteRole,
    setPermissions,
  }
}
