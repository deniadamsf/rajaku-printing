/**
 * Route middleware: hanya user_type=customer yang boleh akses. Staff yang salah
 * masuk halaman customer di-redirect ke /admin.
 */
export default defineNuxtRouteMiddleware((to) => {
  const auth = useAuthStore()
  if (!auth.isAuthenticated) {
    return navigateTo({ path: '/login', query: { redirect: to.fullPath } })
  }
  if (!auth.isCustomer) {
    return navigateTo('/admin')
  }
})
