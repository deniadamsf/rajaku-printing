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
  postalCode: string
  addressCountry: string
  /** Format E.164 untuk `tel:` & JSON-LD. */
  telephone: string
  /** Format `62xxx` (§13) — dipakai untuk link `wa.me`. */
  whatsapp: string
  email: string
  openingHours: BusinessOpeningHours[]
  serviceArea: string[]
  geo: { latitude: number; longitude: number }
  /** Skala schema.org priceRange, mis. "$", "$$". */
  priceRange: string
}

// Nama, alamat jalan, kota, dan telepon/WA DIKONFIRMASI ASLI oleh pemilik
// (21 Agustus 2026) — dipakai apa adanya di struk kasir yang dicetak untuk
// pelanggan, jadi jangan diubah tanpa konfirmasi ulang.
//
// Yang MASIH placeholder ditandai TODO per baris di bawah: postalCode, email,
// dan geo. Jangan tampilkan ketiganya ke pelanggan sebelum diganti.
export const business: BusinessInfo = {
  name: 'Rajaku Printing',
  legalName: 'Rajaku Printing',
  description:
    'Percetakan banner dan digital printing large-format untuk usaha, event, dan kebutuhan pribadi di Trenggalek, Jawa Timur.',
  streetAddress: 'Jl. Panglima Sudirman No. 88, Dobangsan, Ngantru',
  addressLocality: 'Trenggalek',
  addressRegion: 'Jawa Timur',
  postalCode: '66312', // TODO(rajaku): kode pos asli
  addressCountry: 'ID',
  telephone: '+6282146343549',
  whatsapp: '6282146343549',
  email: 'halo@rajakuprinting.example', // TODO(rajaku): email asli
  openingHours: [
    {
      days: ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'],
      opens: '08:00',
      closes: '17:00',
      label: 'Senin–Sabtu, 08.00–17.00 WIB',
    },
  ],
  serviceArea: ['Trenggalek', 'Tulungagung', 'Ponorogo', 'Pacitan'],
  geo: { latitude: -8.0503, longitude: 111.7096 }, // TODO(rajaku): koordinat lokasi asli
  priceRange: '$$',
}
