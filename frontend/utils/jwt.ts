/**
 * Client-side JWT payload decoder — tidak memverifikasi signature (backend
 * yang verify, kita cuma butuh claims untuk populate state).
 *
 * Aman dipakai untuk membaca roles/permissions dari token yang backend baru
 * kembalikan — menghindari round-trip ke /auth/me pasca login/register.
 */

export interface JWTPayload {
  uid?: string
  typ?: string
  em?: string
  ph?: string
  roles?: string[]
  perms?: string[]
  exp?: number
  iat?: number
  nbf?: number
  iss?: string
  sub?: string
}

export function decodeJWTPayload(token: string): JWTPayload | null {
  try {
    const parts = token.split('.')
    if (parts.length !== 3) return null
    const b64 = parts[1].replace(/-/g, '+').replace(/_/g, '/')
    // Pad base64 to multiple of 4.
    const padded = b64 + '='.repeat((4 - (b64.length % 4)) % 4)
    // atob available in browsers; on server (Node 18+), atob is global too.
    const json = typeof atob === 'function'
      ? atob(padded)
      : Buffer.from(padded, 'base64').toString('utf-8')
    return JSON.parse(json) as JWTPayload
  } catch {
    return null
  }
}
