/**
 * useAdminStaff — API wrapper untuk admin staff & role listing (§10).
 * Semua endpoint di-gate permission staff.manage / role.manage di backend.
 */
import type {
  AdminStaffListResponse,
  AdminStaffUser,
  AdminRole,
  AdminPermission,
  CreateStaffInput,
  CreateStaffResult,
  UpdateStaffInput,
} from '~/types/staff'

export interface ListStaffParams {
  q?: string
  active?: boolean | ''
  page?: number
  pageSize?: number
}

export function useAdminStaff() {
  const api = useApi()

  // -------- Staff --------
  function listStaff(params: ListStaffParams): Promise<AdminStaffListResponse> {
    const query: Record<string, string> = {}
    if (params.q) query.q = params.q
    if (params.active !== '' && params.active !== undefined) query.active = params.active ? '1' : '0'
    if (params.page) query.page = String(params.page)
    if (params.pageSize) query.page_size = String(params.pageSize)
    return api.get<AdminStaffListResponse>('/admin/staff', { query })
  }

  function getStaff(id: string): Promise<AdminStaffUser> {
    return api.get<AdminStaffUser>(`/admin/staff/${id}`)
  }

  function createStaff(body: CreateStaffInput): Promise<CreateStaffResult> {
    return api.post<CreateStaffResult>('/admin/staff', body)
  }

  function updateStaff(id: string, body: UpdateStaffInput): Promise<{ ok: boolean }> {
    return api.patch<{ ok: boolean }>(`/admin/staff/${id}`, body)
  }

  function activateStaff(id: string): Promise<{ ok: boolean }> {
    return api.post<{ ok: boolean }>(`/admin/staff/${id}/activate`)
  }

  function deactivateStaff(id: string): Promise<{ ok: boolean }> {
    return api.post<{ ok: boolean }>(`/admin/staff/${id}/deactivate`)
  }

  function assignRoles(id: string, roleIds: string[]): Promise<{ ok: boolean }> {
    return api.post<{ ok: boolean }>(`/admin/staff/${id}/roles`, { role_ids: roleIds })
  }

  // -------- Roles / Permissions (read-only helpers dipakai halaman staff) --------
  function listRoles(): Promise<{ items: AdminRole[] }> {
    return api.get<{ items: AdminRole[] }>('/admin/roles')
  }

  function listPermissions(): Promise<{ items: AdminPermission[] }> {
    return api.get<{ items: AdminPermission[] }>('/admin/permissions')
  }

  return {
    listStaff,
    getStaff,
    createStaff,
    updateStaff,
    activateStaff,
    deactivateStaff,
    assignRoles,
    listRoles,
    listPermissions,
  }
}
