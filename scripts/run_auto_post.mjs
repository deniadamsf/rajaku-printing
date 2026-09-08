#!/usr/bin/env node
/**
 * CLI Runner for Auto-Posting Articles on Rajaku Printing
 *
 * TIDAK ADA TARGET SERVER BAWAAN. Dulu --api yang kosong berarti
 * "https://rajakuprinting.com", jadi satu perintah tanpa flag langsung
 * menerbitkan artikel ke web yang dilihat pelanggan. Sekarang targetnya wajib
 * disebut, dan target live wajib dikonfirmasi — sejalan dengan CLAUDE.md §29.3
 * ("lebar kertas tidak boleh ditebak"): yang tidak bisa ditarik kembali tidak
 * boleh dijalankan atas dasar tebakan.
 *
 * Usage:
 *   node scripts/run_auto_post.mjs --file=<artikel.json> --dry-run
 *   node scripts/run_auto_post.mjs --file=<artikel.json> --api=http://localhost:8080
 *   node scripts/run_auto_post.mjs --file=<artikel.json> --api=https://rajakuprinting.com
 *
 * Flags:
 *   --file=<path>       Path to article JSON payload (default: scripts/article_payload.json)
 *   --api=<url>         API base URL. WAJIB kecuali --dry-run. Bisa juga lewat env RAJAKU_API_URL
 *   --email=<email>     Staff email for authentication
 *   --password=<pwd>    Staff password for authentication
 *   --token=<jwt>       Pre-existing Bearer token
 *   --dry-run           Only validate SEO (must be 100) & brand safety without posting
 *   --yes-live          Lewati konfirmasi ketik untuk server live (untuk otomasi/CI)
 */

import fs from 'node:fs'
import path from 'node:path'
import readline from 'node:readline'
import { fileURLToPath } from 'node:url'
import { autoPostArticle, analyzeSeo, loginAndGetToken, validateBrandSafety, validateImageFile } from './auto_post_article.mjs'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

// Parse command line arguments
const args = process.argv.slice(2)
const params = {}
for (const arg of args) {
  if (arg.startsWith('--')) {
    const [k, ...v] = arg.slice(2).split('=')
    params[k] = v.length > 0 ? v.join('=') : true
  }
}

/**
 * Flag yang ditulis polos tanpa nilai (mis. `--api` saja) tersimpan sebagai
 * boolean true oleh parser di atas. Dibaca sebagai teks, itu melempar
 * TypeError mentah ke layar alih-alih pesan yang bisa ditindaklanjuti — jadi
 * flag tanpa nilai diperlakukan sama dengan flag yang tidak ditulis.
 */
function strParam(name) {
  return typeof params[name] === 'string' ? params[name] : ''
}

const fileArg = strParam('file').trim()
const filePath = fileArg
  ? path.resolve(process.cwd(), fileArg)
  : path.join(__dirname, 'article_payload.json')

if (!fs.existsSync(filePath)) {
  console.error(`❌ File payload tidak ditemukan: ${filePath}`)
  process.exit(1)
}

let article
try {
  article = JSON.parse(fs.readFileSync(filePath, 'utf8'))
} catch (err) {
  console.error(`\n❌ File artikel tidak bisa dibaca sebagai JSON: ${filePath}`)
  console.error(`   ${err.message}\n`)
  process.exit(1)
}

const isDryRun = Boolean(params['dry-run'])

/** Host lokal = server dev. Selain itu dianggap live & butuh konfirmasi. */
const DEV_HOSTS = new Set(['localhost', '127.0.0.1', '::1', '0.0.0.0'])

function resolveTarget() {
  const raw = (strParam('api') || process.env.RAJAKU_API_URL || '').trim()

  if (!raw) {
    if (isDryRun) return null // --dry-run tidak menyentuh jaringan sama sekali
    console.error('\n❌ Target server belum ditentukan — script berhenti sebelum mengirim apa pun.')
    console.error('\n   Sebutkan tujuannya secara eksplisit dengan --api:')
    console.error('     Uji lokal   : --api=http://localhost:8080')
    console.error('     Terbit live : --api=https://rajakuprinting.com')
    console.error('\n   Atau cek skor SEO saja tanpa server: tambahkan --dry-run\n')
    process.exit(1)
  }

  let url
  try {
    url = new URL(raw)
  } catch {
    console.error(`\n❌ --api bukan URL yang sah: "${raw}"`)
    console.error('   Contoh benar: http://localhost:8080 atau https://rajakuprinting.com\n')
    process.exit(1)
  }

  return {
    base: raw.replace(/\/$/, ''),
    isLive: !DEV_HOSTS.has(url.hostname) && !url.hostname.endsWith('.local'),
  }
}

const target = resolveTarget()

console.log('╔════════════════════════════════════════════════════════════════╗')
console.log('║       RAJAKU PRINTING — AUTO-POST ARTIKEL (SEO 100)            ║')
console.log('╚════════════════════════════════════════════════════════════════╝')
if (!target) {
  console.log('Target Server: — (mode --dry-run, tidak ada data dikirim)')
} else {
  console.log(`Target Server: ${target.base}${target.isLive ? '   ⚠️  LIVE — DILIHAT PELANGGAN' : '   (dev)'}`)
}
console.log(`File Artikel : ${path.basename(filePath)}`)
console.log(`Judul        : ${article.title}`)
console.log(`Focus Keyword: ${article.focus_keyword}`)

// Run SEO Analysis
console.log('\n[1/4] 🔍 Menguji Skor SEO On-Page (12 Kriteria)...')
const analysis = analyzeSeo({
  title: article.title,
  slug: article.slug,
  metaTitle: article.meta_title,
  metaDescription: article.meta_description,
  contentMd: article.content_md,
  focusKeyword: article.focus_keyword,
  secondaryKeywords: article.secondary_keywords,
  coverAltText: article.cover_alt_text,
})

console.log(`\n📊 SKOR SEO: ${analysis.score} / 100 (${analysis.passedCount}/${analysis.totalChecks} kriteria lolos)`)
console.log(`   Panjang Konten  : ${analysis.wordCount} kata (Syarat: >= 600)`)
console.log(`   Keyword Density : ${analysis.keywordDensity ? analysis.keywordDensity.toFixed(2) : 0}% (Syarat: 0.5% - 2.5%)`)
// Fallback '' supaya artikel dengan field kurang lengkap dilaporkan sebagai
// skor SEO gagal (di bawah), bukan mati sebagai TypeError di baris ringkasan.
console.log(`   Karakter Judul  : ${(article.meta_title || article.title || '').length} (Syarat: 40 - 60)`)
console.log(`   Karakter Meta   : ${(article.meta_description || '').length} (Syarat: 120 - 160)`)

console.log('\nDetail Kriteria:')
for (const c of analysis.checks) {
  console.log(`  ${c.passed ? '✅ [PASS]' : '❌ [FAIL]'} ${c.label} ${c.detail ? `(${c.detail})` : ''}`)
}

if (analysis.score !== 100) {
  console.error(`\n❌ GAGAL: Skor SEO harus tepat 100! (Saat ini: ${analysis.score}). Perbaiki kriteria yang gagal sebelum posting.`)
  process.exit(1)
}

// Check Brand Safety & Image Sizes
console.log('\n[2/4] 🔒 Memeriksa Aturan Brand Safety & Ukuran Gambar...')
try {
  validateBrandSafety(article.cover_alt_text, 'Alt Text Cover')
  validateBrandSafety(article.title, 'Judul Artikel')
  console.log('✅ Lolos: Gambar dan teks TIDAK menyebutkan brand kompetitor / pihak ketiga.')

  // Validasi ukuran cover image
  const baseDir = path.dirname(filePath)
  if (article.image_path) {
    const coverPath = path.isAbsolute(article.image_path) ? article.image_path : path.resolve(baseDir, article.image_path)
    validateImageFile(coverPath)
  }

  // Validasi ukuran gambar inline pada isi markdown
  const inlineImageRegex = /!\[([^\]]*)\]\(([^)]+)\)/g
  const matches = [...(article.content_md || '').matchAll(inlineImageRegex)]
  for (const match of matches) {
    const rawUrl = match[2]
    if (!/^https?:\/\//i.test(rawUrl) && !rawUrl.includes('/api/v1/cms/images/')) {
      const imgPath = path.isAbsolute(rawUrl) ? rawUrl : path.resolve(baseDir, rawUrl)
      validateImageFile(imgPath)
    }
  }
  console.log('✅ Lolos: Seluruh aset gambar memenuhi batas ukuran upload (< 950 KB, batas Nginx 1 MB).')
} catch (err) {
  console.error(`\n❌ ${err.message}`)
  process.exit(1)
}

if (isDryRun) {
  console.log('\n✨ Mode --dry-run aktif. Validasi SEO 100, Brand Safety, & Ukuran Gambar BERHASIL! (Tidak ada data yang dikirim ke server).')
  process.exit(0)
}

// Authentication & Auto-Post
async function askInput(promptText) {
  const rl = readline.createInterface({ input: process.stdin, output: process.stdout })
  return new Promise((resolve) => {
    rl.question(promptText, (ans) => {
      rl.close()
      resolve(ans.trim())
    })
  })
}

/** Menerbitkan ke web publik tidak bisa dibatalkan — minta persetujuan sadar. */
async function confirmLiveTarget() {
  if (!target.isLive || params['yes-live']) return

  console.log('\n⚠️  KONFIRMASI PENERBITAN LIVE')
  console.log(`   Artikel "${article.title}"`)
  console.log(`   akan TERBIT dan langsung terlihat pengunjung di ${target.base}/artikel/${article.slug}`)
  console.log('   Kalau slug ini sudah ada di server, artikel lama akan DITIMPA.')

  if (!process.stdin.isTTY) {
    console.error('\n❌ Terminal ini tidak bisa menerima ketikan, jadi konfirmasi tidak mungkin diminta.')
    console.error('   Kalau ini memang dijalankan otomatis dan Anda yakin, tambahkan flag --yes-live.\n')
    process.exit(1)
  }

  const answer = await askInput('\n   Ketik LIVE (huruf besar) untuk melanjutkan, atau Enter untuk batal: ')
  if (answer !== 'LIVE') {
    console.log('\n🛑 Dibatalkan. Tidak ada data yang dikirim ke server.\n')
    process.exit(0)
  }
}

async function main() {
  await confirmLiveTarget()

  let token = strParam('token') || process.env.RAJAKU_TOKEN

  if (!token) {
    // Password sengaja tidak di-trim: spasi bisa jadi bagian sah dari password.
    let email = strParam('email') || process.env.RAJAKU_EMAIL
    let password = strParam('password') || process.env.RAJAKU_PASSWORD

    if (!email || !password) {
      console.log('\n[3/4] 🔐 Autentikasi Admin Diperlukan:')
      console.log('Masukkan kredensial akun staff Rajaku Printing untuk auto-post:')
      if (!email) email = await askInput('Email Admin    : ')
      if (!password) password = await askInput('Password Admin : ')
    }

    if (!email || !password) {
      console.error('\n❌ Email dan Password wajib diisi untuk melakukan posting.')
      process.exit(1)
    }

    token = await loginAndGetToken(target.base, email, password)
  }

  // Resolve Image Path
  let imagePath = null
  if (article.image_path) {
    imagePath = path.isAbsolute(article.image_path)
      ? article.image_path
      : path.join(path.dirname(filePath), article.image_path)
    if (!fs.existsSync(imagePath)) {
      console.warn(`⚠️ File gambar ${imagePath} tidak ditemukan, posting tanpa cover image.`)
      imagePath = null
    }
  }

  console.log('\n[4/4] 🚀 Menjalankan Auto-Post ke Rajaku Printing...')
  const result = await autoPostArticle({
    apiBaseUrl: target.base,
    token,
    article,
    imagePath,
    baseDir: path.dirname(filePath),
  })

  console.log('════════════════════════════════════════════════════════════════')
  console.log(`🎯 Postingan berhasil dipublikasikan!`)
  console.log(`🔗 URL: ${result.url}`)
  console.log(`⭐ Skor SEO: ${result.score}/100`)
  console.log('════════════════════════════════════════════════════════════════\n')
}

main().catch((err) => {
  console.error('\n❌ Terjadi kesalahan saat auto-post:', err.message)
  process.exit(1)
})
