/**
 * catalog-fallback.ts — isi cadangan halaman publik saat modul catalog tidak
 * bisa dijangkau.
 *
 * Kenapa ada: Postgres (dan karenanya backend) sering mati di lingkungan
 * pemilik. Sebelum ini, setiap halaman publik yang bergantung pada katalog
 * runtuh jadi kotak "belum bisa dimuat" — itulah wajah yang membuat situs
 * terasa kosong & belum jadi. Daftar di sini membuat halaman tetap berisi.
 *
 * Aturan yang tidak boleh dilanggar:
 *  - TIDAK ADA HARGA di file ini. Angka harga hanya boleh datang dari API
 *    katalog; harga cadangan yang basi lebih berbahaya daripada tidak ada.
 *  - Isinya harus benar-benar mencerminkan layanan/bahan yang ada di sistem,
 *    bukan daftar karangan untuk memenuhi ruang kosong.
 *  - Hidup di satu tempat supaya landing dan `/katalog` tidak pelan-pelan
 *    menampilkan daftar yang berbeda.
 */
import type { ArtworkVariant } from '~/utils/artwork'
import type { CatalogMaterial } from '~/types/catalog'

export interface CoreService {
  name: string
  variant: ArtworkVariant
  desc: string
}

/** Enam jenis cetak inti yang memang dilayani — dipasangkan dengan artwork. */
export const CORE_SERVICES: ReadonlyArray<CoreService> = [
  {
    name: 'Spanduk Flexi',
    variant: 'spanduk',
    desc: 'Cetak ukuran bebas untuk promo, event, dan kebutuhan outdoor harian.',
  },
  {
    name: 'X-Banner',
    variant: 'x-banner',
    desc: 'Display berdiri ukuran standar, siap pakai untuk booth & pameran.',
  },
  {
    name: 'Roll-Up Banner',
    variant: 'roll-up',
    desc: 'Praktis digulung, cocok dibawa untuk presentasi dan acara berpindah.',
  },
  {
    name: 'Baliho',
    variant: 'baliho',
    desc: 'Ukuran besar untuk promosi jarak jauh di tepi jalan.',
  },
  {
    name: 'Backdrop Panggung',
    variant: 'backdrop',
    desc: 'Latar acara dan photo booth, presisi warna untuk dokumentasi.',
  },
  {
    name: 'Stiker Vinyl',
    variant: 'stiker',
    desc: 'Cutting sticker untuk branding kendaraan, kaca, dan kemasan.',
  },
]

/**
 * Bahan bawaan sistem — salinan apa adanya dari seed
 * `backend/migrations/000002_catalog_schema.up.sql`. Kalau seed itu berubah,
 * perbarui daftar ini juga; `id` sengaja diisi kode bahan karena baris ini
 * tidak pernah dipakai untuk request (hanya untuk ditampilkan).
 */
export const FALLBACK_MATERIALS: ReadonlyArray<CatalogMaterial> = [
  {
    id: 'flexi_280',
    code: 'flexi_280',
    name: 'Flexi 280 gsm',
    description: 'Kain flexi outdoor 280 gsm — pilihan populer untuk banner harian.',
  },
  {
    id: 'flexi_340',
    code: 'flexi_340',
    name: 'Flexi 340 gsm',
    description: 'Kain flexi outdoor 340 gsm — lebih tebal, lebih awet.',
  },
  {
    id: 'vinyl_solvent',
    code: 'vinyl_solvent',
    name: 'Vinyl Solvent',
    description: 'Vinyl adhesive untuk X-banner / roll-up / stiker outdoor.',
  },
  {
    id: 'backlite',
    code: 'backlite',
    name: 'Backlite Film',
    description: 'Bahan tembus cahaya untuk lightbox / display back-lit.',
  },
]
