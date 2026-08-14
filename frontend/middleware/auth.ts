/**
 * Route middleware: require authenticated user. Redirect ke /login dengan
 * `?redirect=<current-path>` supaya post-login bisa balik ke halaman asal.
 *
 * Pakai di halaman: `definePageMeta({ middleware: ['auth'] })`
 */
export default defineNuxtRouteMiddleware((to) => {
  const auth = useAuthStore()
  if (!auth.isAuthenticated) {
    return navigateTo({
      path: '/login',
      query: { redirect: to.fullPath },
    })
  }
})
