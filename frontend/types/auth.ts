// Shared types untuk modul auth di frontend. Match dengan response envelope
// backend Go (internal/auth/handler + service dto).

export type UserType = 'customer' | 'staff'

export interface AuthUser {
  id: string
  email?: string
  phone: string
  name: string
  user_type: UserType
}

export interface AuthToken {
  access_token: string
  token_type: string
  expires_in: number // seconds
}

/** Response dari POST /auth/register dan POST /auth/login */
export interface AuthResponse {
  token: AuthToken
  user: AuthUser
}

/** Response dari GET /auth/me */
export interface MeResponse {
  user_id: string
  user_type: UserType
  email?: string
  phone: string
  name: string
  roles: string[]
  permissions: string[]
}
