/**
 * business.ts — satu-satunya sumber data NAP (Name/Address/Phone) & jam operasional
 * Rajaku Printing (CLAUDE.md §15, §26.11 spirit "single source"). Dipakai oleh:
 *  - JSON-LD LocalBusiness di `layouts/default.vue`
 *  - Section kontak di landing page (`components/landing/ClosingCta.vue`)
 *
 * JANGAN duplikasi/hardcode data ini ke file lain — kalau butuh, import dari sini.
 *
 * TODO(rajaku): data NAP asli belum diberikan pemilik — seluruh field di bawah ini
 * masih placeholder dan WAJIB diganti dengan data asli sebelum go-live produksi.
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
  description: string
  streetAddress: string
  addressLocality: string
  addressRegion: string
  /**
   * Kode pos. OPSIONAL dengan sengaja: lebih baik tidak dikirim ke Google
   * daripada dikirim salah. NAP (Name-Address-Phone) yang tidak konsisten
   * dengan sumber lain justru menurunkan kepercayaan hasil pencarian lokal.
   * Kalau `undefined`, field ini dihilangkan dari JSON-LD (lihat
   * layouts/default.vue), bukan dikirim kosong.
   */
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
  /** Skala schema.org priceRange, mis. "$", "$$". */
  priceRange: string
  /** Metode bayar yang diterima (§7 & §11) — untuk JSON-LD `paymentAccepted`. */
  paymentAccepted: string[]
}

// SUMBER TUNGGAL identitas usaha. Dipakai JSON-LD LocalBusiness
// (layouts/default.vue), footer, halaman tentang-kami, dan struk kasir yang
// dicetak untuk pelanggan — jadi salah di sini menyebar ke mana-mana.
//
// DIKONFIRMASI ASLI oleh pemilik (21 Agustus 2026): nama, alamat jalan, kota,
// telepon/WA, dan email. Jangan diubah tanpa konfirmasi ulang.
//
// `postalCode` dan `geo` SENGAJA dibiarkan kosong, bukan terlupakan. Nilai
// sebelumnya (66312 dan -8.0503/111.7096) adalah tebakan pengisi yang tidak
// pernah diverifikasi. Mengirim NAP yang salah ke Google lebih merugikan
// daripada tidak mengirimnya sama sekali: kode pos yang bentrok dengan sumber
// lain menurunkan kepercayaan data, dan koordinat meleset menaruh pin peta di
// alamat orang lain. Keduanya dihilangkan dari JSON-LD selama masih kosong.
export const business: BusinessInfo = {
  name: 'Rajaku Printing',
  legalName: 'Rajaku Printing',
  description:
    'Percetakan banner dan digital printing large-format untuk usaha, event, dan kebutuhan pribadi di Trenggalek, Jawa Timur.',
  streetAddress: 'Jl. Panglima Sudirman No. 88, Dobangsan, Ngantru',
  addressLocality: 'Trenggalek',
  addressRegion: 'Jawa Timur',
  // TODO(rajaku): isi kode pos asli kelurahan Ngantru, Trenggalek. Lihat
  // catatan di atas — biarkan kosong sampai terverifikasi, jangan ditebak.
  postalCode: undefined,
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
  // TODO(rajaku): isi koordinat asli toko. Cara tercepat: buka Google Maps,
  // klik kanan tepat di lokasi toko, angka paling atas di menu itu adalah
  // latitude, longitude — salin apa adanya ke sini.
  geo: undefined,
  priceRange: '$$',
  // §7 & §11 — transfer bank + QRIS untuk order online, tunai + QRIS di kasir.
  paymentAccepted: ['Cash', 'QRIS', 'Bank Transfer'],
}
