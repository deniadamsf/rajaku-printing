/**
 * /sitemap.xml — auto-generate dari backend list published articles + halaman
 * publik statis (spec §15). Di-cache runtime 10 menit supaya crawler tidak
 * hammer backend.
 *
 * Tidak pakai module tambahan (mis. @nuxtjs/sitemap) — cukup nitro server route
 * karena dependency ke DB via API backend jauh lebih kecil dari re-scan file-based.
 */
import type { ArticleListResponse } from '~/types/cms'

interface ArticleEnvelope {
  success: boolean
  data?: ArticleListResponse
  error?: { code: string; message: string }
}

const CACHE_TTL_MS = 10 * 60 * 1000 // 10 menit
let cache: { xml: string; expires: number } | null = null

export default defineEventHandler(async (event) => {
  const now = Date.now()
  if (cache && cache.expires > now) {
    setHeader(event, 'Content-Type', 'application/xml; charset=utf-8')
    setHeader(event, 'Cache-Control', 'public, max-age=600')
    return cache.xml
  }

  const config = useRuntimeConfig()
  const base = config.public.appBaseUrl.replace(/\/$/, '')
  const apiBase = config.public.apiBase.replace(/\/$/, '')

  // Fetch semua published articles (limit 1000 = safe upper bound MVP; kalau
  // konten lebih banyak, paginate di sini nanti).
  let articles: ArticleListResponse['items'] = []
  try {
    const res = await $fetch<ArticleEnvelope>(`${apiBase}/articles`, {
      query: { limit: 1000 },
    })
    if (res.success && res.data) articles = res.data.items
  } catch (err) {
    console.error('[sitemap] gagal fetch artikel:', err)
  }

  const staticURLs = [
    { loc: `${base}/`, changefreq: 'weekly', priority: '1.0' },
    { loc: `${base}/artikel`, changefreq: 'weekly', priority: '0.8' },
    { loc: `${base}/order`, changefreq: 'monthly', priority: '0.9' },
    { loc: `${base}/lacak`, changefreq: 'monthly', priority: '0.5' },
  ]

  const articleURLs = articles.map((a) => ({
    loc: `${base}/artikel/${a.slug}`,
    lastmod: a.updated_at,
    changefreq: 'monthly',
    priority: '0.7',
  }))

  const xml =
    `<?xml version="1.0" encoding="UTF-8"?>\n` +
    `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n` +
    [...staticURLs, ...articleURLs]
      .map((u) => {
        const lastmod = 'lastmod' in u && u.lastmod ? `    <lastmod>${escapeXML(u.lastmod)}</lastmod>\n` : ''
        return (
          `  <url>\n` +
          `    <loc>${escapeXML(u.loc)}</loc>\n` +
          lastmod +
          `    <changefreq>${u.changefreq}</changefreq>\n` +
          `    <priority>${u.priority}</priority>\n` +
          `  </url>`
        )
      })
      .join('\n') +
    `\n</urlset>\n`

  cache = { xml, expires: now + CACHE_TTL_MS }

  setHeader(event, 'Content-Type', 'application/xml; charset=utf-8')
  setHeader(event, 'Cache-Control', 'public, max-age=600')
  return xml
})

function escapeXML(v: string): string {
  return v
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;')
}
