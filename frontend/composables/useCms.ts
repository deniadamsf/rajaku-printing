/**
 * useCms — API wrapper untuk modul CMS artikel (§14).
 * Admin endpoints (/admin/articles/*) + upload gambar.
 */
import type {
  Article,
  ArticleImage,
  ArticleListResponse,
  ArticleStatus,
  CreateArticleBody,
  UpdateArticleBody,
} from '~/types/cms'

export function useCms() {
  const api = useApi()
  const config = useRuntimeConfig()

  async function listAdmin(params: {
    page?: number
    limit?: number
    status?: ArticleStatus | ''
    q?: string
  }): Promise<ArticleListResponse> {
    const query: Record<string, string> = {}
    if (params.page) query.page = String(params.page)
    if (params.limit) query.limit = String(params.limit)
    if (params.status) query.status = params.status
    if (params.q) query.q = params.q
    return api.get<ArticleListResponse>('/admin/articles', { query })
  }

  function getAdmin(id: string): Promise<Article> {
    return api.get<Article>(`/admin/articles/${id}`)
  }

  function create(body: CreateArticleBody): Promise<Article> {
    return api.post<Article>('/admin/articles', body)
  }

  function update(id: string, body: UpdateArticleBody): Promise<Article> {
    return api.put<Article>(`/admin/articles/${id}`, body)
  }

  function publish(id: string): Promise<Article> {
    return api.post<Article>(`/admin/articles/${id}/publish`)
  }

  function archive(id: string): Promise<Article> {
    return api.post<Article>(`/admin/articles/${id}/archive`)
  }

  function remove(id: string): Promise<{ ok: boolean }> {
    return api.delete<{ ok: boolean }>(`/admin/articles/${id}`)
  }

  /**
   * uploadImage — multipart POST /admin/articles/images. Return row image
   * (id → URL public /cms/images/:id yg langsung siap embed di markdown).
   */
  async function uploadImage(file: File, opts?: { articleId?: string; altText?: string }): Promise<ArticleImage> {
    const form = new FormData()
    form.append('file', file)
    if (opts?.articleId) form.append('article_id', opts.articleId)
    if (opts?.altText) form.append('alt_text', opts.altText)
    return api.post<ArticleImage>('/admin/articles/images', form)
  }

  /** URL public untuk render <img>; base dari apiBase yg sudah termasuk /api/v1. */
  function imageUrl(imageId: string): string {
    return `${config.public.apiBase}/cms/images/${imageId}`
  }

  /** Ubah alt text sebuah gambar (cover atau gambar inline di konten). */
  function updateImageAlt(imageId: string, altText: string): Promise<ArticleImage> {
    return api.patch<ArticleImage>(`/admin/articles/images/${imageId}`, { alt_text: altText })
  }

  /** Ambil metadata gambar (bukan file-nya) — dipakai untuk baca alt text saat ini. */
  function getImageMeta(imageId: string): Promise<ArticleImage> {
    return api.get<ArticleImage>(`/admin/articles/images/${imageId}`)
  }

  // ---- Public endpoints (no auth) — dipakai halaman /artikel & /artikel/[slug] ----

  function listPublic(params: { page?: number; limit?: number; q?: string }): Promise<ArticleListResponse> {
    const query: Record<string, string> = {}
    if (params.page) query.page = String(params.page)
    if (params.limit) query.limit = String(params.limit)
    if (params.q) query.q = params.q
    return api.get<ArticleListResponse>('/articles', { query })
  }

  function getBySlug(slug: string): Promise<Article> {
    return api.get<Article>(`/articles/${slug}`)
  }

  return {
    listAdmin,
    getAdmin,
    create,
    update,
    publish,
    archive,
    remove,
    uploadImage,
    imageUrl,
    updateImageAlt,
    getImageMeta,
    listPublic,
    getBySlug,
  }
}
