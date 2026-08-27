// Shape response backend §14. Sinkronkan kalau backend berubah.

export type ArticleStatus = 'draft' | 'published' | 'archived'

export interface Article {
  id: string
  slug: string
  title: string
  excerpt?: string | null
  content_md: string
  cover_image_id?: string | null
  meta_title?: string | null
  meta_description?: string | null
  focus_keyword?: string | null
  secondary_keywords?: string | null
  seo_score?: number | null
  status: ArticleStatus
  published_at?: string | null
  author_id: string
  created_at: string
  updated_at: string
}

export interface ArticleImage {
  id: string
  article_id?: string | null
  original_name: string
  mime_type: string
  size_bytes: number
  width_px: number
  height_px: number
  alt_text?: string | null
  uploaded_by?: string | null
  uploaded_at: string
}

export interface ArticleListResponse {
  items: Article[]
  total: number
  limit: number
  page: number
}

export interface CreateArticleBody {
  title: string
  slug?: string
  excerpt?: string
  content_md: string
  meta_title?: string
  meta_description?: string
  cover_image_id?: string | null
  focus_keyword?: string | null
  secondary_keywords?: string | null
  seo_score?: number | null
}

export interface UpdateArticleBody {
  title?: string
  slug?: string
  excerpt?: string
  content_md?: string
  meta_title?: string
  meta_description?: string
  cover_image_id?: string | null
  focus_keyword?: string | null
  secondary_keywords?: string | null
  seo_score?: number | null
}
