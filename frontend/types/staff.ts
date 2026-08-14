// Shape untuk admin staff/role/invite modul. Match dengan backend Go
// internal/auth/handler + service.

export interface AdminRole {
  id: string
  name: string
  display_name: string
  description?: string
  is_system: boolean
  created_at: string
  updated_at: string
  permissions?: AdminPermission[]
}

export interface AdminPermission {
  id: string
  code: string
  display_name: string
  category: string
  description?: string
  created_at: string
}

export interface AdminStaffUser {
  id: string
  email?: string
  phone: string
  name: string
  user_type: 'staff' | 'customer'
  is_active: boolean
  last_login_at?: string
  created_at: string
  updated_at: string
  roles?: AdminRole[]
}

export interface AdminStaffListResponse {
  Items: AdminStaffUser[]
  Total: number
  Page: number
  PageSize: number
}

export interface CreateStaffInput {
  name: string
  email: string
  phone: string
  role_ids: string[]
}

export interface UpdateStaffInput {
  name: string
  email: string
  phone: string
}

export interface CreateStaffResult {
  staff: AdminStaffUser
  invite_url: string
  invite_token: string
}
