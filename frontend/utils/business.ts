/**
 * business.ts — satu-satunya sumber data NAP (Name/Address/Phone) & jam operasional
 * Rajaku Printing (CLAUDE.md §15, §26.11 spirit "single source"). Dipakai oleh:
 *  - JSON-LD LocalBusiness di `layouts/default.vue`
 *  - Footer di semua halaman publik (`layouts/default.vue`)
 *  - Halaman `/tentang-kami` (kartu kontak + kartu lokasi/peta)
 *  - Struk kasir (`components/admin/ReceiptStruk.vue`)
 *
 * JANGAN duplikasi/hardcode data ini ke file lain — kalau butuh, import dari sini.
 */

export interface BusinessOpeningHours {
  /** Nilai schema.org `DayOfWeek` (bahasa Inggris, wajib untuk JSON-LD). */
  days: string[]
  opens: string
  closes: string
  /** Label berbahasa Indonesia untuk ditampilkan ke user. */
  label: string
}

export interface BusinessInfo {
  name: string
  legalName: string
  /**
   * Nama lama tempat ini sebelum berganti jadi Rajaku Printing. Dipakai sebagai
   * `alternateName` di JSON-LD dan sebagai patokan tertulis di halaman lokasi:
   * warga sekitar (dan listing Google Maps) masih mengenal tempat ini dengan
   * nama lamanya, jadi menyebutkannya justru menolong orang menemukan toko —
   * bukan sekadar nostalgia.
   */
  formerName?: string
  description: string
  streetAddress: string
  /** Patokan arah untuk pelanggan yang datang langsung (bukan bagian alamat pos). */
  landmark: string
  addressLocality: string
  addressRegion: string
  postalCode?: string
  addressCountry: string
  /** Format E.164 untuk `tel:` & JSON-LD. */
  telephone: string
  /** Format `62xxx` (§13) — dipakai untuk link `wa.me`. */
  whatsapp: string
  email: string
  openingHours: BusinessOpeningHours[]
  serviceArea: string[]
  /**
   * Koordinat lokasi toko. OPSIONAL dengan sengaja — koordinat yang meleset
   * menaruh pin peta di rumah orang lain dan mengirim pelanggan ke alamat
   * yang salah. Kalau `undefined`, blok `geo` dihilangkan dari JSON-LD.
   */
  geo?: { latitude: number; longitude: number }
  /** Tautan Google Maps resmi toko (dibagikan pemilik). */
  mapsUrl: string
  /** Skala schema.org priceRange, mis. "$", "$$". */
  priceRange: string
  /** Metode bayar yang diterima (§7 & §11) — untuk JSON-LD `paymentAccepted`. */
  paymentAccepted: string[]
}

// SUMBER TUNGGAL identitas usaha. Dipakai JSON-LD LocalBusiness
// (layouts/default.vue), footer, halaman tentang-kami, dan struk kasir yang
// dicetak untuk pelanggan — jadi salah di sini menyebar ke mana-mana.
//
// DIKONFIRMASI ASLI oleh pemilik (22 Agustus 2026): alamat lengkap, kode pos,
// patokan lokasi, dan koordinat — dikirim langsung bersama tautan Google Maps
// tokonya. Alamat sebelumnya ("Dobangsan, Ngantru", tanpa kode pos & koordinat)
// SALAH dan sudah diganti; jangan dikembalikan.
//
// `geo` diambil dari titik tengah tautan Maps milik pemilik, bukan tebakan —
// inilah syarat yang dulu menahan `geo` & `postalCode` tetap kosong. Kalau
// suatu saat pin dirasa meleset, perbaiki DI SINI (satu tempat), jangan
// menambal koordinat baru di komponen.
export const business: BusinessInfo = {
  name: 'Rajaku Printing',
  legalName: 'Rajaku Printing',
  formerName: 'Toko Purnama Sari',
  description:
    'Percetakan banner dan digital printing large-format untuk usaha, event, dan kebutuhan pribadi di Trenggalek, Jawa Timur.',
  streetAddress: 'Jl. Panglima Sudirman No. 88',
  landmark: 'Depan Pasar Pon Trenggalek',
  addressLocality: 'Trenggalek',
  addressRegion: 'Jawa Timur',
  postalCode: '66311',
  addressCountry: 'ID',
  telephone: '+6282146343549',
  whatsapp: '6282146343549',
  email: 'rajakuprinting@gmail.com',
  openingHours: [
    {
      days: ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'],
      opens: '08:00',
      closes: '17:00',
      label: 'Senin–Sabtu, 08.00–17.00 WIB',
    },
  ],
  serviceArea: ['Trenggalek', 'Tulungagung', 'Ponorogo', 'Pacitan'],
  geo: { latitude: -8.0550688, longitude: 111.7082517 },
  // Tautan yang dibagikan pemilik. Parameter `g_ep` (cap build Maps) sengaja
  // dibuang: nilainya kedaluwarsa dan tidak dibutuhkan supaya tautan bekerja.
  mapsUrl:
    'https://www.google.com/maps/search/Toko+Purnama+Sari/@-8.0550688,111.7082517,862m/data=!3m1!1e3?hl=id',
  priceRange: '$$',
  // §7 & §11 — transfer bank + QRIS untuk order online, tunai + QRIS di kasir.
  paymentAccepted: ['Cash', 'QRIS', 'Bank Transfer'],
}

/**
 * Alamat satu baris (jalan, kota, provinsi, kode pos) — dipakai di tempat yang
 * cuma punya satu baris muat, mis. struk kasir. Dirakit di sini supaya format
 * pemisahnya tidak ditulis ulang beda-beda di tiap komponen.
 */
export const fullAddress = [
  business.streetAddress,
  business.addressLocality,
  `${business.addressRegion} ${business.postalCode ?? ''}`.trim(),
]
  .filter(Boolean)
  .join(', ')

const geoQuery = business.geo ? `${business.geo.latitude},${business.geo.longitude}` : null

/**
 * Peta siap-embed TANPA API key (endpoint `output=embed` klasik). Sengaja tidak
 * memakai Maps Embed API: itu butuh kunci berbayar yang harus dirotasi, dan
 * halaman ini cuma perlu menampilkan satu pin statis.
 *
 * `null` kalau koordinat belum ada — komponen pemanggil WAJIB memperlakukan
 * peta sebagai pelengkap: alamat, patokan, dan tombol arah harus tetap tampil
 * sebagai teks asli walau petanya tidak muncul.
 */
export const mapsEmbedUrl = geoQuery
  ? `https://maps.google.com/maps?q=${geoQuery}&z=17&hl=id&output=embed`
  : null

/**
 * Petunjuk arah. Sengaja memakai koordinat, bukan nama tempat: nama listing di
 * Google Maps masih nama lama, dan pencarian nama bisa mendarat di tempat lain
 * yang mirip. Koordinat selalu menunjuk titik yang sama.
 */
export const mapsDirectionsUrl = geoQuery
  ? `https://www.google.com/maps/dir/?api=1&destination=${geoQuery}`
  : business.mapsUrl
