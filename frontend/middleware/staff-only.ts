/**
 * Route middleware: hanya user_type=staff yang boleh akses. Kalau belum login
 * → ke /login. Kalau login tapi customer → ke home customer.
 */
export default defineNuxtRouteMiddleware((to) => {
  const auth = useAuthStore()
  if (!auth.isAuthenticated) {
    return navigateTo({ path: '/login', query: { redirect: to.fullPath } })
  }
  if (!auth.isStaff) {
    return navigateTo('/akun')
  }
})
