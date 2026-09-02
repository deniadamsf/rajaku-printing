/**
 * Auto-Post Article Tool for Rajaku Printing
 *
 * Skor SEO TIDAK dihitung ulang di sini. Rumusnya dibaca langsung dari
 * `frontend/composables/useSeoAnalysis.ts` — file yang sama yang dipakai
 * panel SEO di admin panel. Dulu file ini menyimpan salinan rumus itu, dan
 * salinan berarti dua sumber kebenaran: begitu kriteria di aplikasi diubah,
 * script ini tetap melaporkan "100/100" berdasarkan aturan lama tanpa ada
 * yang error. Itu persis kegagalan senyap yang dilarang CLAUDE.md §22.
 *
 * Yang tetap milik script ini (memang tidak ada di aplikasi):
 *   - validateBrandSafety() — aturan "jangan sebut brand lain"
 *   - kolom `detail` per kriteria, untuk keterbacaan output CLI
 */
import fs from 'node:fs'
import path from 'node:path'

/**
 * Rumus SEO asli milik aplikasi. Di-import dinamis supaya kegagalannya bisa
 * dijelaskan dengan kalimat yang berguna, bukan ERR_UNKNOWN_FILE_EXTENSION
 * mentah. Node >= 22.6 membaca TypeScript langsung (>= 23.6 tanpa flag).
 */
let analyzeSeoCore
try {
  ;({ analyzeSeo: analyzeSeoCore } = await import('../frontend/composables/useSeoAnalysis.ts'))
} catch (err) {
  const major = Number(process.versions.node.split('.')[0])
  console.error('\n❌ Gagal membaca rumus SEO dari frontend/composables/useSeoAnalysis.ts')
  if (major < 23) {
    console.error(`   Node yang terpasang: v${process.versions.node}. Script ini butuh Node >= 23.6`)
    console.error('   (atau Node 22.6+ dijalankan dengan flag --experimental-strip-types).')
  } else {
    console.error(`   Penyebab: ${err.message}`)
    console.error('   Pastikan file composable-nya masih ada dan tidak dipindah.')
  }
  console.error('\n   Script sengaja berhenti, bukan memakai rumus cadangan — skor SEO')
  console.error('   dari rumus yang berbeda dengan aplikasi lebih berbahaya daripada gagal.\n')
  process.exit(1)
}

// Third-party brands blacklist to strictly enforce the user's rule
const FORBIDDEN_BRANDS = [
  'canva', 'photoshop', 'coreldraw', 'illustrator', 'adobe', 'epson', 'canon', 
  'mimaki', 'roland', 'mutoh', 'hp', 'fujifilm', 'snapy', 'shopee', 'tokopedia',
  'lazada', 'tiktok', 'gramedia', 'komet', 'ronita'
]

export function validateBrandSafety(text, fieldName = 'gambar/alt text') {
  const lower = (text || '').toLowerCase()
  for (const brand of FORBIDDEN_BRANDS) {
    const regex = new RegExp(`\b${brand}\b`, 'i')
    if (regex.test(lower)) {
      throw new Error(`[ATURAN DILANGGAR] Teks ${fieldName} menyebutkan brand lain: "${brand}". Harap gunakan istilah generik atau Rajaku Printing saja.`)
    }
  }
}

/** Terima camelCase maupun snake_case, dan pastikan semuanya string. */
function normalizeInput(input) {
  const pick = (a, b) => String(input[a] ?? input[b] ?? '')
  return {
    title: pick('title', 'title').trim(),
    slug: pick('slug', 'slug').trim(),
    metaTitle: pick('metaTitle', 'meta_title').trim(),
    metaDescription: pick('metaDescription', 'meta_description').trim(),
    contentMd: pick('contentMd', 'content_md'),
    focusKeyword: pick('focusKeyword', 'focus_keyword').trim(),
    secondaryKeywords: pick('secondaryKeywords', 'secondary_keywords'),
    coverAltText: pick('coverAltText', 'cover_alt_text').trim(),
  }
}

/**
 * Membungkus analyzeSeo milik aplikasi: menormalkan input, menegakkan aturan
 * brand, lalu menambahkan `detail`/`passedCount`/`totalChecks` untuk output CLI.
 * Angka skor & daftar kriteria datang apa adanya dari aplikasi.
 */
export function analyzeSeo(input) {
  const n = normalizeInput(input)
  const result = analyzeSeoCore(n)

  validateBrandSafety(n.coverAltText, 'coverAltText')
  validateBrandSafety(n.title, 'title')

  const effectiveMetaTitle = n.metaTitle || n.title
  const details = {
    kw_in_title: `Judul: "${effectiveMetaTitle}" (${effectiveMetaTitle.length} char)`,
    kw_in_meta_desc: `Meta desc (${n.metaDescription.length} char)`,
    kw_in_slug: `Slug: "${n.slug}"`,
    kw_density: `${result.keywordDensity ? result.keywordDensity.toFixed(2) : 0}%`,
    content_length: `${result.wordCount} kata`,
    title_length: `${effectiveMetaTitle.length} karakter`,
    meta_desc_length: `${n.metaDescription.length} karakter`,
    cover_alt_text: n.coverAltText,
  }

  const checks = result.checks.map((c) => ({ ...c, detail: details[c.id] }))
  const passedCount = checks.filter((c) => c.passed).length

  return {
    score: result.score,
    checks,
    wordCount: result.wordCount,
    keywordDensity: result.keywordDensity,
    passedCount,
    totalChecks: checks.length,
  }
}

export async function loginAndGetToken(apiBaseUrl, email, password) {
  console.log(`🔐 Login ke ${apiBaseUrl} sebagai ${email}...`)
  const res = await fetch(`${apiBaseUrl}/api/v1/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  })
  if (!res.ok) {
    const txt = await res.text()
    throw new Error(`Login gagal (${res.status}): ${txt}`)
  }
  const data = await res.json()
  const token = data.data?.token?.access_token || data.token?.access_token
  if (!token) throw new Error('Response tidak memiliki access_token')
  console.log('✅ Login berhasil! Token didapatkan.')
  return token
}

async function uploadImageFile(apiBaseUrl, token, filePath, altText = '') {
  if (altText) {
    validateBrandSafety(altText, `Alt Text Gambar (${path.basename(filePath)})`)
  }
  const fileBuffer = fs.readFileSync(filePath)
  const ext = path.extname(filePath).toLowerCase()
  const mimeType = ext === '.png' ? 'image/png' : ext === '.webp' ? 'image/webp' : 'image/jpeg'
  const blob = new Blob([fileBuffer], { type: mimeType })
  const formData = new FormData()
  formData.append('file', blob, path.basename(filePath))
  if (altText) {
    formData.append('alt_text', altText)
  }

  const imgRes = await fetch(`${apiBaseUrl}/api/v1/admin/articles/images`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
    body: formData,
  })

  if (!imgRes.ok) {
    const errBody = await imgRes.text()
    throw new Error(`Gagal upload gambar ${path.basename(filePath)}: ${imgRes.status} ${errBody}`)
  }

  const imgJson = await imgRes.json()
  return imgJson.data?.id || imgJson.id
}

export async function autoPostArticle({ apiBaseUrl, token, article, imagePath, baseDir }) {
  // Tanpa baris ini, pemanggil yang lupa mengisi apiBaseUrl dulu diam-diam
  // menerbitkan ke produksi lewat nilai bawaan parameter ini.
  if (!apiBaseUrl) {
    throw new Error('apiBaseUrl wajib diisi — tidak ada target server bawaan (lihat run_auto_post.mjs).')
  }

  console.log(`\n🔍 Memvalidasi Skor SEO Artikel: "${article.title}"...`)
  const analysis = analyzeSeo({
    title: article.title,
    slug: article.slug,
    metaTitle: article.meta_title || article.title,
    metaDescription: article.meta_description,
    contentMd: article.content_md,
    focusKeyword: article.focus_keyword,
    secondaryKeywords: article.secondary_keywords,
    coverAltText: article.cover_alt_text || '',
  })

  console.log(`\n📊 Hasil Analisis SEO: ${analysis.score}/100 (${analysis.passedCount}/${analysis.totalChecks} lulus)`)
  for (const c of analysis.checks) {
    console.log(`  ${c.passed ? '✅' : '❌'} ${c.label} ${c.detail ? `[${c.detail}]` : ''}`)
  }

  if (analysis.score !== 100) {
    throw new Error(`\n❌ Skor SEO belum 100! (Skor saat ini: ${analysis.score}). Perbaiki item yang bertanda silang di atas.`)
  }

  console.log('\n🔒 Aturan Keamanan Brand: Memastikan tidak ada penyebutan brand lain di gambar & teks...')
  validateBrandSafety(article.cover_alt_text, 'cover_alt_text')
  console.log('✅ Verifikasi nama brand lolos: Tidak ada brand pihak ketiga yang disebutkan.')

  // 1. Upload Cover Image
  let coverImageId = null
  if (imagePath && fs.existsSync(imagePath)) {
    console.log(`\n🖼️ [1/2] Mengunggah cover gambar utama: ${path.basename(imagePath)}...`)
    coverImageId = await uploadImageFile(apiBaseUrl, token, imagePath, article.cover_alt_text)
    console.log(`✅ Cover image berhasil diunggah & dikonversi ke WebP! ID: ${coverImageId}`)
  }

  // 2. Scan & Upload Inline Images in content_md
  let processedContentMd = article.content_md
  const dir = baseDir || (imagePath ? path.dirname(imagePath) : process.cwd())
  const inlineImageRegex = /!\[([^\]]*)\]\(([^)]+)\)/g
  const matches = [...article.content_md.matchAll(inlineImageRegex)]

  if (matches.length > 0) {
    console.log(`\n🖼️ [2/2] Memeriksa ${matches.length} gambar inline pada isi markdown...`)
    for (const match of matches) {
      const altText = match[1]
      const rawUrl = match[2]

      // Jika bukan URL online dan belum berupa URL CMS server
      if (!/^https?:\/\//i.test(rawUrl) && !rawUrl.includes('/api/v1/cms/images/')) {
        const absPath = path.isAbsolute(rawUrl) ? rawUrl : path.resolve(dir, rawUrl)
        if (fs.existsSync(absPath)) {
          console.log(`   ⬆️ Mengunggah gambar inline: ${path.basename(absPath)} (Alt: "${altText}")...`)
          const inlineId = await uploadImageFile(apiBaseUrl, token, absPath, altText)
          const serverUrl = `${apiBaseUrl}/api/v1/cms/images/${inlineId}`
          console.log(`   ✅ Berhasil diunggah! ID: ${inlineId} -> ${serverUrl}`)
          processedContentMd = processedContentMd.replaceAll(rawUrl, serverUrl)
        } else {
          console.warn(`   ⚠️ File gambar inline tidak ditemukan: ${absPath}`)
        }
      }
    }
  }

  console.log('\n📝 Mengirim payload artikel ke backend...')
  const payload = {
    title: article.title,
    slug: article.slug,
    excerpt: article.excerpt,
    content_md: processedContentMd,
    meta_title: article.meta_title,
    meta_description: article.meta_description,
    focus_keyword: article.focus_keyword,
    secondary_keywords: article.secondary_keywords,
    cover_image_id: coverImageId,
    seo_score: 100,
  }

  let articleId = null
  const postRes = await fetch(`${apiBaseUrl}/api/v1/admin/articles`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(payload),
  })

  if (postRes.status === 409) {
    console.log(`⚠️ Artikel dengan slug "${article.slug}" sudah ada di server. Melakukan update konten & gambar...`)
    // Cari ID artikel yang sudah ada
    const listRes = await fetch(`${apiBaseUrl}/api/v1/admin/articles?q=${encodeURIComponent(article.slug)}`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    if (listRes.ok) {
      const listJson = await listRes.json()
      const existing = (listJson.data?.items || listJson.items || []).find((a) => a.slug === article.slug)
      if (existing) {
        articleId = existing.id
      }
    }
    if (!articleId) {
      const pubRes = await fetch(`${apiBaseUrl}/api/v1/articles/${encodeURIComponent(article.slug)}`)
      if (pubRes.ok) {
        const pubJson = await pubRes.json()
        articleId = pubJson.data?.id || pubJson.id
      }
    }

    if (articleId) {
      const putRes = await fetch(`${apiBaseUrl}/api/v1/admin/articles/${articleId}`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify(payload),
      })
      if (!putRes.ok) {
        const putErr = await putRes.text()
        throw new Error(`Gagal update artikel: ${putRes.status} ${putErr}`)
      }
      console.log(`✅ Artikel berhasil diperbarui di database! ID: ${articleId}`)
    } else {
      throw new Error(`Slug "${article.slug}" sudah ada, tapi ID artikel tidak ditemukan untuk diupdate.`)
    }
  } else if (!postRes.ok) {
    const errBody = await postRes.text()
    throw new Error(`Gagal membuat artikel: ${postRes.status} ${errBody}`)
  } else {
    const postJson = await postRes.json()
    articleId = postJson.data?.id || postJson.id
    console.log(`✅ Artikel baru berhasil dibuat di database! ID: ${articleId}`)
  }

  console.log(`\n🚀 Mempublikasikan artikel (${articleId})...`)
  const pubRes = await fetch(`${apiBaseUrl}/api/v1/admin/articles/${articleId}/publish`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })

  if (!pubRes.ok) {
    const errBody = await pubRes.text()
    throw new Error(`Gagal publish artikel: ${pubRes.status} ${errBody}`)
  }

  console.log(`\n🎉 SUKSES! Artikel dan seluruh gambar telah berhasil diposting & published!`)
  console.log(`🔗 Link artikel: ${apiBaseUrl}/artikel/${article.slug}\n`)
  return { articleId, url: `${apiBaseUrl}/artikel/${article.slug}`, score: 100 }
}

