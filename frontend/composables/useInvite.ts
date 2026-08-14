/**
 * useInvite — API wrapper untuk /invites/* endpoints (§10).
 * Public — dipakai halaman /invites/accept oleh calon staff yang belum punya
 * akun aktif, jadi jangan attach permission check di client.
 */

export interface InviteVerification {
  user_id: string
  email: string
  name: string
  expires_at: string
}

export function useInvite() {
  const api = useApi()

  function verify(token: string): Promise<InviteVerification> {
    return api.get<InviteVerification>('/invites/verify', { query: { token } })
  }

  function accept(token: string, password: string): Promise<{ ok: boolean }> {
    return api.post<{ ok: boolean }>('/invites/accept', { token, password })
  }

  return { verify, accept }
}
