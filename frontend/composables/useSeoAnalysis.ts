/**
 * useSeoAnalysis — analisis SEO on-page gaya Rank Math, murni & tanpa reaktivitas
 * (pemanggil membungkusnya dalam `computed()`). Dipakai oleh `AdminSeoPanel`.
 *
 * 12 pengecekan, masing-masing berkontribusi sama rata ke skor 0-100. Kalau
 * `focusKeyword` kosong, seluruh pengecekan berbasis kata kunci (1-7) dianggap
 * gagal — bukan dikeluarkan dari penyebut — supaya skor rendah mendorong
 * penulis mengisi kata kunci, bukan diam-diam mengecilkan skala penilaian.
 */

export interface SeoAnalysisInput {
  title: string
  slug: string
  metaTitle: string
  metaDescription: string
  contentMd: string
  focusKeyword: string
  /** Dipisah koma. */
  secondaryKeywords: string
  coverAltText: string
}

export interface SeoCheckResult {
  id: string
  label: string
  passed: boolean
}

export interface SeoAnalysisResult {
  score: number
  checks: SeoCheckResult[]
  wordCount: number
  /** Persentase; null kalau tidak ada kata kunci utama atau konten kosong. */
  keywordDensity: number | null
}

/** Buang sintaks markdown dasar supaya hitungan kata & pencarian teks bersih. */
function stripMarkdown(md: string): string {
  return md
    .replace(/```[\s\S]*?```/g, ' ')
    .replace(/`[^`]*`/g, ' ')
    .replace(/!\[[^\]]*\]\([^)]*\)/g, ' ')
    .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/^#{1,6}\s+/gm, '')
    .replace(/[*_>~`#]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
}

function slugify(s: string): string {
  return s
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

export function analyzeSeo(input: SeoAnalysisInput): SeoAnalysisResult {
  const focusKeyword = input.focusKeyword.trim()
  const kw = focusKeyword.toLowerCase()
  const hasKw = kw.length > 0

  const strippedContent = stripMarkdown(input.contentMd)
  const strippedLower = strippedContent.toLowerCase()
  const words = strippedContent.length > 0 ? strippedContent.split(/\s+/).filter(Boolean) : []
  const wordCount = words.length

  const effectiveMetaTitle = input.metaTitle.trim() || input.title.trim()

  // 1. Kata kunci di judul SEO
  const kwInTitle = hasKw && effectiveMetaTitle.toLowerCase().includes(kw)

  // 2. Kata kunci di meta description
  const kwInMetaDesc = hasKw && input.metaDescription.toLowerCase().includes(kw)

  // 3. Kata kunci di slug URL
  const kwSlugForm = slugify(kw)
  const kwInSlug = hasKw && kwSlugForm.length > 0 && input.slug.toLowerCase().includes(kwSlugForm)

  // 4. Kata kunci muncul di konten
  const kwInContent = hasKw && strippedLower.includes(kw)

  // 5. Kepadatan kata kunci ideal (0.5%–2.5%)
  let keywordDensity: number | null = null
  let kwDensityOk = false
  if (hasKw && wordCount > 0) {
    const occurrences = strippedLower.split(kw).length - 1
    keywordDensity = (occurrences / wordCount) * 100
    kwDensityOk = keywordDensity >= 0.5 && keywordDensity <= 2.5
  }

  // 6. Kata kunci muncul di 10% awal konten
  const introLen = Math.ceil(strippedContent.length * 0.1)
  const kwInIntro = hasKw && strippedLower.slice(0, introLen).includes(kw)

  // 7. Kata kunci muncul di subheading (H2/H3 markdown mentah)
  const headingLines = input.contentMd.split('\n').filter((line) => /^#{2,3}\s+/.test(line))
  const kwInHeading = hasKw && headingLines.some((line) => line.toLowerCase().includes(kw))

  // 8. Panjang konten minimal 600 kata
  const contentLengthOk = wordCount >= 600

  // 9. Panjang judul SEO 40–60 karakter
  const titleLengthOk = effectiveMetaTitle.length >= 40 && effectiveMetaTitle.length <= 60

  // 10. Panjang meta description 120–160 karakter
  const metaDescLengthOk = input.metaDescription.length >= 120 && input.metaDescription.length <= 160

  // 11. Cover image punya alt text
  const coverAltOk = input.coverAltText.trim() !== ''

  // 12. Kata kunci turunan dipakai di konten
  const secondaryList = input.secondaryKeywords
    .split(',')
    .map((s) => s.trim())
    .filter((s) => s.length > 0)
  const secondaryKwOk = secondaryList.length > 0 && secondaryList.some((s) => strippedLower.includes(s.toLowerCase()))

  const checks: SeoCheckResult[] = [
    { id: 'kw_in_title', label: 'Kata kunci ada di judul SEO', passed: kwInTitle },
    { id: 'kw_in_meta_desc', label: 'Kata kunci ada di meta description', passed: kwInMetaDesc },
    { id: 'kw_in_slug', label: 'Kata kunci ada di slug URL', passed: kwInSlug },
    { id: 'kw_in_content', label: 'Kata kunci muncul di konten', passed: kwInContent },
    { id: 'kw_density', label: 'Kepadatan kata kunci ideal (0.5%–2.5%)', passed: kwDensityOk },
    { id: 'kw_in_intro', label: 'Kata kunci muncul di 10% awal konten', passed: kwInIntro },
    { id: 'kw_in_heading', label: 'Kata kunci muncul di subheading', passed: kwInHeading },
    { id: 'content_length', label: 'Panjang konten minimal 600 kata', passed: contentLengthOk },
    { id: 'title_length', label: 'Panjang judul SEO 40–60 karakter', passed: titleLengthOk },
    { id: 'meta_desc_length', label: 'Panjang meta description 120–160 karakter', passed: metaDescLengthOk },
    { id: 'cover_alt_text', label: 'Cover image punya alt text', passed: coverAltOk },
    { id: 'secondary_kw_used', label: 'Kata kunci turunan dipakai di konten', passed: secondaryKwOk },
  ]

  const passedCount = checks.filter((c) => c.passed).length
  const score = Math.round((100 * passedCount) / checks.length)

  return { score, checks, wordCount, keywordDensity }
}
