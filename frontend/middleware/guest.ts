/**
 * Route middleware: hanya untuk user yang BELUM login. Kalau sudah login,
 * redirect ke home area masing-masing (staff → /admin, customer → /akun).
 *
 * Dipakai di /login dan /register.
 */
export default defineNuxtRouteMiddleware(() => {
  const auth = useAuthStore()
  if (auth.isAuthenticated) {
    return navigateTo(auth.homePath)
  }
})
