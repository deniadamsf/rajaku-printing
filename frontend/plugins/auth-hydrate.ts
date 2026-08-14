/**
 * Nuxt plugin — hydrate auth store dari token cookie saat app boot (SSR + client).
 * Menempatkan file di `plugins/` tanpa suffix `.client`/`.server` berarti jalan
 * di kedua sisi — SSR bisa render halaman authenticated dengan benar (no flash
 * of unauthenticated state).
 */
export default defineNuxtPlugin(async () => {
  const tokenCookie = useAuthTokenCookie()
  if (!tokenCookie.value) return

  const auth = useAuthStore()
  if (auth.isAuthenticated) return

  await auth.fetchMe()
})
