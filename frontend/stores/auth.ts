import { defineStore } from 'pinia'
import type { AuthResponse, AuthUser, MeResponse, UserType } from '~/types/auth'
import { decodeJWTPayload } from '~/utils/jwt'

interface AuthState {
  user: AuthUser | null
  roles: string[]
  permissions: string[]
  isHydrating: boolean
}

/**
 * Auth store — single source of truth untuk identitas user aktif di frontend.
 *
 * Token disimpan di cookie (via useAuthTokenCookie), sementara profile
 * (user/roles/permissions) di Pinia state — hydrated saat app boot atau setelah
 * login/register.
 *
 * Konsumsi umum:
 *   const auth = useAuthStore()
 *   if (auth.isStaff) { ... }
 *   if (auth.hasPermission('payment.verify')) { ... }
 */
export const useAuthStore = defineStore('auth', {
  state: (): AuthState => ({
    user: null,
    roles: [],
    permissions: [],
    isHydrating: false,
  }),

  getters: {
    isAuthenticated: (s): boolean => s.user !== null,
    isStaff: (s): boolean => s.user?.user_type === 'staff',
    isCustomer: (s): boolean => s.user?.user_type === 'customer',
    hasPermission: (s) => (code: string): boolean => s.permissions.includes(code),
    hasAnyPermission: (s) => (codes: string[]): boolean => codes.some((c) => s.permissions.includes(c)),
    /** Home path berdasarkan user_type — dipakai redirect pasca login. */
    homePath(): string {
      if (this.isStaff) return '/admin'
      if (this.isCustomer) return '/akun'
      return '/'
    },
  },

  actions: {
    async login(email: string, password: string): Promise<UserType> {
      const api = useApi()
      const res = await api.post<AuthResponse>('/auth/login', { email, password })
      this.applyAuthResponse(res)
      return res.user.user_type
    },

    async register(input: { email: string; phone: string; name: string; password: string }): Promise<UserType> {
      const api = useApi()
      const res = await api.post<AuthResponse>('/auth/register', input)
      this.applyAuthResponse(res)
      return res.user.user_type
    },

    async logout(): Promise<void> {
      const api = useApi()
      try {
        await api.post<null>('/auth/logout')
      } catch {
        // Endpoint stateless — abaikan error (client-side drop tetap efektif).
      }
      this.clear()
    },

    /**
     * Fetch profile lengkap dari /auth/me. Dipakai HANYA oleh plugin boot
     * (auth-hydrate) untuk restore session dari token cookie — bukan setelah
     * login/register (yang sudah punya semua data dari response + JWT claims).
     *
     * Kalau /me error (401/network), user di-clear (session dianggap invalid).
     */
    async fetchMe(): Promise<void> {
      const api = useApi()
      this.isHydrating = true
      try {
        const me = await api.get<MeResponse>('/auth/me')
        this.user = {
          id: me.user_id,
          email: me.email,
          phone: me.phone,
          phone_verified: me.phone_verified,
          name: me.name,
          user_type: me.user_type,
        }
        this.roles = me.roles ?? []
        this.permissions = me.permissions ?? []
      } catch {
        this.clear()
      } finally {
        this.isHydrating = false
      }
    },

    /**
     * Apply token + user dari login/register response ke store + cookie.
     * Roles/permissions di-decode dari JWT claims — hindari round-trip /me
     * yang punya race dgn cookie ref freshness.
     */
    applyAuthResponse(res: AuthResponse) {
      const tokenCookie = useAuthTokenCookie()
      tokenCookie.value = res.token.access_token
      this.user = res.user
      const claims = decodeJWTPayload(res.token.access_token)
      this.roles = claims?.roles ?? []
      this.permissions = claims?.perms ?? []
    },

    clear() {
      const tokenCookie = useAuthTokenCookie()
      tokenCookie.value = null
      this.user = null
      this.roles = []
      this.permissions = []
    },

    /**
     * Update nomor WA di profile setelah user menambah/verifikasi nomor
     * lewat `useAccountPhone` (halaman /akun) — hindari round-trip /me,
     * respons endpoint tambah-nomor sudah cukup buat sinkronkan state lokal.
     */
    setPhone(phone: string, verified: boolean) {
      if (!this.user) return
      this.user.phone = phone
      this.user.phone_verified = verified
    },
  },
})
