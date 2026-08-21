/**
 * artwork.ts — pemetaan produk katalog → varian artwork vektor (`<ArtProduct>`).
 *
 * Kenapa ada: katalog belum punya foto per-produk (field `image_url` kosong
 * selama pemilik belum unggah foto asli dari admin panel). Tanpa gambar, semua
 * kartu produk tampil identik (ikon generik yang sama) — itu penyebab utama
 * landing terasa kosong & seperti template.
 *
 * Aturan main:
 *  - Foto asli SELALU menang. Artwork ini murni cadangan saat `image_url` kosong.
 *  - Artwork BUKAN foto hasil kerja. Jangan pernah diberi caption yang mengklaim
 *    "hasil cetak kami" — labelnya ilustrasi/mockup produk.
 *
 * Pemetaan dibuat berlapis (slug → category → nama) supaya produk baru yang
 * dibuat admin lewat panel tetap dapat artwork yang masuk akal tanpa perlu
 * ubah kode: `banner-flexi-outdoor` tetap kena varian `spanduk` lewat kata kunci.
 */

/** Varian artwork yang tersedia di `components/art/Product.vue`. */
export type ArtworkVariant =
  | 'spanduk'
  | 'x-banner'
  | 'roll-up'
  | 'baliho'
  | 'backdrop'
  | 'stiker'
  | 'backlite'
  | 'umum'

/** Minimal shape produk yang dibutuhkan untuk resolusi varian. */
export interface ArtworkSubject {
  slug?: string
  category?: string
  name?: string
}

/**
 * Kata kunci → varian, dicek berurutan (yang lebih spesifik duluan).
 * `x-banner` harus dicek sebelum `banner`, kalau tidak x-banner ikut kena
 * varian spanduk.
 */
const KEYWORD_RULES: ReadonlyArray<readonly [RegExp, ArtworkVariant]> = [
  [/x[-\s]?banner/i, 'x-banner'],
  [/roll[-\s]?up|rollup/i, 'roll-up'],
  [/baliho|billboard|bando/i, 'baliho'],
  [/backdrop|photo\s?booth|panggung/i, 'backdrop'],
  [/stiker|sticker|vinyl|cutting/i, 'stiker'],
  [/backlite|backlit|lightbox|neon\s?box/i, 'backlite'],
  [/spanduk|banner|flexi|umbul/i, 'spanduk'],
]

/**
 * artworkVariantFor mengembalikan varian artwork paling cocok untuk sebuah
 * produk. Selalu mengembalikan varian valid — `umum` sebagai jaring pengaman,
 * jadi pemanggil tidak perlu menangani kasus "tidak ketemu".
 */
export function artworkVariantFor(subject: ArtworkSubject): ArtworkVariant {
  const haystack = [subject.slug, subject.category, subject.name].filter(Boolean).join(' ')
  if (!haystack) return 'umum'

  for (const [pattern, variant] of KEYWORD_RULES) {
    if (pattern.test(haystack)) return variant
  }
  return 'umum'
}

/** Label manusiawi per varian — dipakai untuk `alt`/`aria-label` artwork. */
export const ARTWORK_LABELS: Readonly<Record<ArtworkVariant, string>> = {
  spanduk: 'Ilustrasi spanduk banner horizontal dengan mata ayam',
  'x-banner': 'Ilustrasi X-banner berdiri dengan rangka X',
  'roll-up': 'Ilustrasi roll-up banner dengan kaset penggulung',
  baliho: 'Ilustrasi baliho besar di atas dua tiang',
  backdrop: 'Ilustrasi backdrop panggung dengan rangka',
  stiker: 'Ilustrasi lembar stiker vinyl potong',
  backlite: 'Ilustrasi panel backlite yang menyala dari belakang',
  umum: 'Ilustrasi gulungan bahan banner hasil cetak',
}
