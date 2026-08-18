/**
 * Menandai bahwa hydration benar-benar selesai, dibaca oleh watchdog di skrip
 * inline `<head>` (lihat `app.head.script` di nuxt.config.ts).
 *
 * Tanpa penanda ini watchdog tidak punya cara membedakan "JS jalan normal"
 * dari "skrip head sempat jalan tapi bundle aplikasi gagal termuat". Kasus
 * kedua akan meninggalkan konten tersembunyi selamanya kalau kelas
 * `motion-ready` dibiarkan terpasang.
 */
export default defineNuxtPlugin((nuxtApp) => {
  nuxtApp.hook('app:mounted', () => {
    ;(window as unknown as { __rjkHydrated?: boolean }).__rjkHydrated = true
  })
})
