/**
 * /robots.txt — allow all crawlers, point ke sitemap.xml. Admin path
 * di-disallow supaya crawler tidak index halaman admin (walaupun middleware
 * staff-only akan redirect ke /login, tetap lebih rapi tidak sampai di-crawl).
 */
export default defineEventHandler((event) => {
  const config = useRuntimeConfig()
  const base = config.public.appBaseUrl.replace(/\/$/, '')

  setHeader(event, 'Content-Type', 'text/plain; charset=utf-8')
  setHeader(event, 'Cache-Control', 'public, max-age=3600')

  return (
    `User-agent: *\n` +
    `Allow: /\n` +
    `Disallow: /admin/\n` +
    `Disallow: /akun/\n` +
    `Disallow: /login\n` +
    `Disallow: /register\n` +
    `\n` +
    `Sitemap: ${base}/sitemap.xml\n`
  )
})
