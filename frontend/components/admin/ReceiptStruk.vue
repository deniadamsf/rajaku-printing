<script setup lang="ts">
/**
 * ReceiptStruk — struk kasir thermal (58/80mm), dipakai bersama oleh POS
 * (§11, cetak langsung setelah order dibuat) & Detail Order (cetak ulang,
 * mis. kertas macet). Markup + CSS print + injeksi `@page` dinamis SEMUA
 * hidup di sini, digerakkan lewat props — supaya hasil cetak identik di
 * kedua halaman (§12: "satu struktur data invoice → dua template render",
 * prinsip yang sama berlaku ke struk).
 *
 * PENTING — kontrak pemakaian:
 *   - Root elemen memakai `id="struk"` (dipakai selector CSS print di bawah)
 *     dan `useHead` menyuntik SATU `@page` aktif per halaman (browser cuma
 *     baca satu `@page` yang aktif per halaman). Pastikan HANYA SATU
 *     instance komponen ini ter-render pada satu waktu di halaman mana pun —
 *     dua instance sekaligus akan bikin id duplikat & saling menimpa `@page`.
 *   - `useHead` di sini otomatis membersihkan entrinya sendiri saat komponen
 *     unmount (`onBeforeUnmount` bawaan @unhead/vue, terikat ke instance
 *     komponen yang memanggilnya — bukan sesuatu yang perlu di-cleanup
 *     manual), jadi toggle v-if antar halaman aman.
 *   - Mencetak WAJIB lewat method yang diekspos `printNow()` (lihat
 *     `defineExpose` di bawah), bukan `window.print()` langsung dari halaman
 *     pemanggil. `printNow()` men-teleport root komponen ke `<body>` sesaat
 *     sebelum mencetak (lihat blok komentar di atas root template) — kalau
 *     dilewati, `window.print()` polos akan mencetak seluruh halaman admin,
 *     bukan struk.
 *
 * TIPOGRAFI — kenapa seluruh struk pakai `font-mono` (§26.3 vs realita cetak):
 *   §26.3 melarang JetBrains Mono untuk body UI aplikasi (hanya untuk data
 *   non-prosa seperti ID/slug). Struk BUKAN UI aplikasi — ini artefak cetak
 *   untuk printer thermal berlebar kolom tetap (58/80mm), di mana nominal
 *   & label wajib berbaris lurus secara visual seperti struk kasir asli.
 *   Monospace di sini murni fungsional (menjamin kolom "label : nilai" dan
 *   nominal rata kanan tetap sejajar tanpa perhitungan lebar piksel manual),
 *   bukan pilihan gaya. §26 mengatur bahasa visual UI aplikasi, bukan
 *   dokumen cetak — pengecualian ini sadar & disengaja, bukan drift.
 */
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
import { renderSVG } from 'uqr'

import { business, fullAddress } from '~/utils/business'

export interface ReceiptStrukProps {
  resi: string
  createdAt: string
  customerName: string
  customerPhone: string
  productName: string
  materialName: string
  widthCm: number
  heightCm: number
  quantity: number
  unitPrice: number
  subtotal: number
  /** null/undefined = pickup atau ongkir belum di-set (§8) — baris disembunyikan kecuali metodeAmbil "kirim". */
  shippingCost?: number | null
  total: number
  metodeAmbil: string
  metodeBayar: string
  trackingUrl: string
  /**
   * Apakah uangnya SUDAH diterima. Menentukan stempel `*** LUNAS ***` vs
   * `*** BELUM LUNAS ***` di struk.
   *
   * WAJIB diisi (tanpa default) — DISENGAJA. Halaman "cetak ulang" di detail
   * order bisa dipakai pada order status apa pun, termasuk order online yang
   * masih `menunggu_pembayaran`. Kalau prop ini punya default `true`, satu
   * pemakaian yang lupa mengisinya akan mencetak kertas bertuliskan LUNAS
   * untuk pesanan yang belum dibayar — bukti pembayaran palsu yang bisa
   * difoto pelanggan. Memaksa setiap pemanggil menyatakannya secara eksplisit
   * membuat kelalaian itu jadi error TypeScript, bukan masalah di kasir.
   */
  isPaid: boolean
  /** Lebar kertas thermal aktif (mm). Nilai selain 58/80 di-guard ke 58. */
  widthMm?: number
}

const props = withDefaults(defineProps<ReceiptStrukProps>(), {
  shippingCost: null,
  widthMm: 58,
})

const receiptWidthMm = computed<58 | 80>(() => (props.widthMm === 80 ? 80 : 58))

// Area cetak SESUNGGUHNYA lebih sempit dari lebar kertas nominal — printer
// thermal tidak bisa mencetak sampai tepi kertas (ada margin mekanis di
// kedua sisi head cetak). Kertas 58mm → area cetak 48mm (384 titik @203dpi);
// kertas 80mm → area cetak 72mm (576 titik). Kalau isi struk dibuat selebar
// KERTAS (bukan area cetak), driver mengecilkan/memotong hasilnya dan
// operator terpaksa mengutak-atik skala manual di dialog cetak tiap kali.
const printableWidthMm = computed<48 | 72>(() => (receiptWidthMm.value === 80 ? 72 : 48))

// `printing` menggerbangi Teleport root ke `<body>` — lihat blok komentar di
// root template & `printNow()` di bawah.
//
// PRINT_ISOLATION_CLASS ditempel ke <html> hanya selama `printNow()` berjalan.
// Isolasi cetak (menyembunyikan seluruh isi halaman selain struk) TIDAK boleh
// aktif permanen: struk POS ter-mount terus di panel sukses, jadi kalau
// aturannya berlaku setiap kali media print aktif, operator yang menekan
// Ctrl+P — bukan tombol "Cetak struk" — akan mendapat kertas kosong, karena
// isi halaman disembunyikan sementara struk masih berada di dalamnya (belum
// ter-teleport). Dengan gerbang kelas ini, Ctrl+P biasa tetap mencetak
// halaman apa adanya seperti sebelum perubahan.
const printing = ref(false)
const PRINT_ISOLATION_CLASS = 'cetak-struk'

// Identitas toko dibaca dari SATU sumber (utils/business.ts) yang juga
// menyuplai schema markup LocalBusiness & footer — sekali pemilik memperbarui
// alamat/telepon di sana, struk ikut berubah tanpa sentuh file ini.
// `fullAddress` sudah termasuk kode pos (dikonfirmasi pemilik 22 Agustus 2026);
// email & koordinat sengaja tidak dicetak — kertas 58mm sempit, dan yang
// dibutuhkan pelanggan di tangan cuma "di mana" dan "telepon ke mana".

// QR menuju halaman lacak resi. renderSVG (uqr, tanpa dependensi) memberi SVG
// vektor hitam-putih murni — bukan raster — jadi printer thermal 1-bit
// mencetaknya sebagai kotak solid yang tajam, bukan dithering abu-abu yang
// gagal dipindai. ecc 'M' + border 2 modul: quiet zone wajib ada, tanpa itu
// banyak pemindai menolak membaca.
// SENGAJA defensif: renderSVG() MELEMPAR kalau argumennya bukan string
// (mis. undefined saat halaman pemanggil belum selesai memuat order). Error
// saat setup komponen membuat SELURUH struk gagal render tanpa pesan apa pun
// di layar — kasir menekan "Cetak struk" dan printer mengeluarkan kertas
// kosong. QR hilang cuma merugikan sedikit (URL teks di bawahnya tetap ada);
// struk hilang total merugikan banyak. Jadi jangan pernah biarkan QR
// menjatuhkan struk.
const qrSvg = computed(() => {
  const url = props.trackingUrl
  if (typeof url !== 'string' || url.trim() === '') return null
  try {
    return renderSVG(url, { ecc: 'M', border: 2, blackColor: '#000', whiteColor: '#fff' })
  } catch {
    return null
  }
})

// Ongkir tampil kalau metode ambil "kirim", ATAU nilainya sudah pernah
// diisi (>0) — menutup kasus data lama yang tetap punya ongkir tercatat.
const showShipping = computed(
  () => props.metodeAmbil === 'kirim' || (props.shippingCost != null && props.shippingCost > 0),
)

/**
 * `@page { size: ... }` HARUS pakai angka literal — browser tidak bisa membaca
 * custom property (`var(--w)`) di properti `size`, dan Vue SFC scoped-CSS juga
 * tidak bisa menyentuh blok `@page` (tidak ada selector untuk di-scope). Jadi
 * aturan ini disuntik sebagai stylesheet tersendiri.
 *
 * Disuntik LANGSUNG ke DOM (bukan lewat `useHead`) dan tepat sebelum
 * `window.print()`, karena inilah bagian yang paling gampang gagal diam-diam:
 * unhead menulis perubahan head secara asinkron (ditumpuk lalu di-flush
 * belakangan), sementara alur cetak kita mount struk → `nextTick()` → cetak
 * dalam hitungan milidetik. Diukur di dev 22 Agustus 2026: saat
 * `window.print()` dipanggil, tag <style> dari useHead BELUM ada di dokumen —
 * artinya `@page` tidak berlaku dan browser memakai ukuran kertas default
 * (A4) berikut marginnya. Itu yang membuat struk tercetak tidak pas di kertas
 * meski lebar 58/80mm sudah diatur benar di admin.
 *
 * Penyuntikan manual di sini sinkron: begitu fungsi ini selesai, aturannya
 * dijamin sudah ada di dokumen sebelum browser memotret halaman.
 */
const PAGE_STYLE_ID = 'struk-page-style'

function injectPageStyle() {
  const el = document.getElementById(PAGE_STYLE_ID) ?? document.createElement('style')
  el.id = PAGE_STYLE_ID
  el.textContent = `@media print {
  @page { size: ${receiptWidthMm.value}mm auto; margin: 0; }
  #struk { width: ${printableWidthMm.value}mm; margin: 0 auto; padding: 0; }
}`
  if (!el.parentNode) document.head.appendChild(el)
}

function removePageStyle() {
  document.getElementById(PAGE_STYLE_ID)?.remove()
}

function fmtIDR(v: number | null | undefined): string {
  if (v == null) return '—'
  return new Intl.NumberFormat('id-ID', {
    style: 'currency',
    currency: 'IDR',
    maximumFractionDigits: 0,
  }).format(v)
}

function fmtDateTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString('id-ID', {
      day: '2-digit',
      month: 'short',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    })
  } catch {
    return iso
  }
}

/**
 * Satu-satunya cara resmi mencetak struk ini (lihat kontrak di kepala
 * berkas). Urutan wajib: teleport ke `<body>` dulu (`printing = true`),
 * tunggu DOM benar-benar berpindah (`await nextTick()`), BARU panggil
 * `window.print()` — kalau urutannya dibalik, browser memotret halaman
 * sebelum struk pindah dan yang tercetak adalah halaman admin biasa.
 *
 * `finally` + event `afterprint` dipasang dua-duanya sebagai jaring
 * pengaman: `window.print()` memblokir sampai dialog cetak ditutup di
 * sebagian besar browser, tapi tidak dijamin di semua browser — kalau
 * `finally` gagal (mis. browser lanjut eksekusi sebelum dialog ditutup),
 * `afterprint` tetap mengembalikan `printing` ke false.
 */
async function printNow() {
  printing.value = true
  await nextTick()
  // Keduanya dipasang SETELAH teleport selesai supaya isolasi cetak cuma hidup
  // di detik-detik struk benar-benar ada di `<body>` — lihat komentar
  // PRINT_ISOLATION_CLASS di atas.
  document.documentElement.classList.add(PRINT_ISOLATION_CLASS)
  injectPageStyle()
  try {
    window.print()
  } finally {
    endPrint()
  }
}

function endPrint() {
  printing.value = false
  if (import.meta.client) {
    document.documentElement.classList.remove(PRINT_ISOLATION_CLASS)
    removePageStyle()
  }
}

if (import.meta.client) {
  useEventListener(window, 'afterprint', endPrint)
}

// Komponen bisa ter-unmount saat dialog cetak masih terbuka (mis. operator
// pindah halaman). Tanpa ini, kelas penanda tertinggal menempel di <html> dan
// setiap Ctrl+P berikutnya mencetak kertas kosong.
onBeforeUnmount(endPrint)

defineExpose({ printNow })
</script>

<template>
  <!--
    Teleport SENGAJA dinonaktifkan (`:disabled="!printing"`) di luar momen
    cetak — begitu `printing` false, wrapper ini kembali dirender di tempat
    aslinya (mis. panel sukses POS, atau tersembunyi di Detail Order), jadi
    preview di layar tidak berubah sama sekali. Saat `printNow()` dipanggil,
    `printing` jadi true dan wrapper (berikut #struk di dalamnya) benar-benar
    pindah jadi anak langsung `<body>` — di titik itulah CSS print di bawah
    (`body > *:not(#struk-print-root) { display: none }`) menyembunyikan
    SISA halaman admin dengan `display: none` (menghapusnya dari layout),
    bukan `visibility: hidden` (yang tetap memakan tinggi halaman dan
    membuat dialog cetak menghitung halaman kedua yang kosong).
  -->
  <Teleport to="body" :disabled="!printing">
    <div
      id="struk-print-root"
      class="flex justify-center bg-canvas-alt p-6 print:block print:bg-transparent print:p-0"
    >
      <div
        id="struk"
        class="shrink-0 space-y-2.5 bg-white p-3 font-mono text-[11px] leading-relaxed text-ink-950 shadow-sm ring-1 ring-black/5 print:w-auto print:shadow-none print:ring-0"
        :class="receiptWidthMm === 80 ? 'w-[80mm]' : 'w-[58mm]'"
      >
      <!--
        Kop: logo maskot Rajaku versi 1-bit khusus cetak.

        Aset ini BUKAN logo web (`/brand/logo-full.webp`) yang dipasang apa
        adanya, dan bukan pula hasil filter `grayscale` CSS. Printer thermal
        mencetak 1-bit — tiap titik hanya hitam atau kosong, tidak ada abu-abu.
        Gambar berwarna/abu-abu yang dikirim apa adanya akan di-dither oleh
        driver jadi pola titik yang melebur & kotor di kertas.

        `logo-mono-print.png` sudah diproses lebih dulu: transparansi diratakan
        ke putih (alpha 0 yang dibiarkan akan tercetak HITAM), diperkecil ke
        384 px (lebar dot printer 58 mm), lalu di-threshold pada 200 sehingga
        hanya berisi hitam & putih murni. Ambang 200 dipilih setelah dibanding
        dengan 110/128/150/175 dan dithering Floyd-Steinberg: ambang rendah
        membuat wajah maskot jadi gumpalan hitam, dithering pecah jadi bubur
        abu-abu saat dicetak kecil.

        Kalau logo web berubah, aset ini WAJIB dibuat ulang — dia tidak ikut
        berubah sendiri.
      -->
      <div class="text-center">
        <!--
          h-24 (~25 mm tercetak) disengaja: maskotnya padat detail, dan di
          bawah ~20 mm wajah/mahkota melebur jadi bercak. Di kertas 58 mm
          (lebar cetak ~52 mm) ini masih menyisakan margin lega.
        -->
        <img
          src="/brand/logo-mono-print.png"
          alt="Rajaku Printing"
          class="mx-auto h-24 w-auto"
        >
      </div>

      <!--
        Identitas toko — alamat & telepon dikonfirmasi asli oleh pemilik
        (lihat catatan di utils/business.ts). Ini yang membedakan struk resmi
        dari sekadar catatan: pelanggan harus tahu ke mana kembali kalau ada
        keluhan atau mau cetak ulang.
      -->
      <div class="text-center leading-snug text-ink-900">
        <p class="break-words">{{ fullAddress }}</p>
        <!--
          Patokan ikut tercetak: struk walk-in sering dibawa pulang lalu dipakai
          orang lain (suami/karyawan) untuk mengambil pesanan — mereka butuh
          penanda jalan, bukan cuma nomor rumah.
        -->
        <p class="break-words">{{ business.landmark }}</p>
        <p>Telp/WA {{ business.telephone }}</p>
      </div>

      <div class="border-t-2 border-ink-950" />
      <p class="text-center font-bold uppercase tracking-[0.2em]">Struk Pembelian</p>
      <div class="border-t-2 border-ink-950" />

      <!-- Data order -->
      <div class="space-y-1">
        <div class="flex gap-1">
          <span class="w-14 shrink-0">No.</span>
          <span class="shrink-0">:</span>
          <span class="min-w-0 break-all pl-1">{{ resi }}</span>
        </div>
        <div class="flex gap-1">
          <span class="w-14 shrink-0">Tgl</span>
          <span class="shrink-0">:</span>
          <span class="min-w-0 pl-1">{{ fmtDateTime(createdAt) }}</span>
        </div>
        <div class="flex gap-1">
          <span class="w-14 shrink-0">Nama</span>
          <span class="shrink-0">:</span>
          <span class="min-w-0 break-words pl-1">{{ customerName }}</span>
        </div>
        <div class="flex gap-1">
          <span class="w-14 shrink-0">No. WA</span>
          <span class="shrink-0">:</span>
          <span class="min-w-0 break-all pl-1">{{ customerPhone }}</span>
        </div>
      </div>

      <div class="border-t-2 border-ink-950" />

      <!-- Item — baris penuh per atribut, bukan pasangan label-nilai -->
      <div class="space-y-0.5">
        <p class="break-words font-semibold">{{ productName }}</p>
        <p class="break-words text-ink-700">{{ materialName }}</p>
        <p class="text-ink-700">{{ widthCm }} x {{ heightCm }} cm</p>
        <div class="flex flex-wrap justify-between gap-x-2 gap-y-0.5 pt-0.5">
          <span class="min-w-0 break-words text-ink-700">{{ quantity }} pcs x {{ fmtIDR(unitPrice) }}</span>
          <span class="shrink-0 tabular-nums">{{ fmtIDR(subtotal) }}</span>
        </div>
      </div>

      <div class="border-t border-dashed border-ink-400" />

      <!-- Nominal -->
      <div class="space-y-1">
        <div class="flex justify-between gap-2">
          <span>Subtotal</span>
          <span class="shrink-0 tabular-nums">{{ fmtIDR(subtotal) }}</span>
        </div>
        <div v-if="showShipping" class="flex justify-between gap-2">
          <span>Ongkir</span>
          <span class="shrink-0 tabular-nums">{{ fmtIDR(shippingCost) }}</span>
        </div>
      </div>

      <div class="border-t-2 border-ink-950" />
      <div class="flex justify-between gap-2 text-sm font-bold">
        <span>TOTAL</span>
        <span class="shrink-0 tabular-nums">{{ fmtIDR(total) }}</span>
      </div>
      <div class="border-t-2 border-ink-950" />

      <!-- Metode -->
      <div class="space-y-1">
        <div class="flex gap-1">
          <span class="w-14 shrink-0">Bayar</span>
          <span class="shrink-0">:</span>
          <span class="min-w-0 pl-1 uppercase">{{ metodeBayar }}</span>
        </div>
        <div class="flex gap-1">
          <span class="w-14 shrink-0">Ambil</span>
          <span class="shrink-0">:</span>
          <span class="min-w-0 pl-1 uppercase">{{ metodeAmbil }}</span>
        </div>
      </div>

      <p class="py-1 text-center font-bold tracking-[0.15em]">
        {{ isPaid ? '*** LUNAS ***' : '*** BELUM LUNAS ***' }}
      </p>

      <div class="border-t border-dashed border-ink-400" />

      <!--
        QR lacak resi. Ditaruh sebelum URL teks, bukan menggantikannya:
        pemindaian bisa gagal (kertas thermal pudar, tinta tipis, kamera
        buram) dan pelanggan tetap butuh jalan cadangan yang bisa diketik.
        `v-html` aman di sini — isinya string SVG hasil renderSVG() dari
        library, bukan input pengguna.
      -->
      <div class="text-center text-ink-700">
        <p v-if="qrSvg">Scan untuk lacak pesanan</p>
        <p v-else>Lacak pesanan Anda di</p>
        <!-- eslint-disable-next-line vue/no-v-html -- string SVG dari renderSVG() (uqr), bukan input pengguna; trackingUrl dibangun server dari APP_BASE_URL + resi -->
        <div v-if="qrSvg" class="mx-auto mt-1 w-[26mm]" v-html="qrSvg" />
        <p class="mt-1 break-all">{{ trackingUrl }}</p>
      </div>

      <div class="border-t border-dashed border-ink-400" />

      <p class="text-center text-ink-700">Terima kasih atas kepercayaan Anda</p>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
/*
 * Isolasi cetak — root ini (`#struk-print-root`, lihat `<Teleport>` di
 * template) pindah jadi anak langsung `<body>` selama `printNow()` berjalan.
 * Begitu dia pindah, satu-satunya yang perlu dilakukan CSS adalah
 * MENGHAPUS saudara-saudaranya dari layout dengan `display: none`.
 *
 * Teknik lama di sini dulu `visibility: hidden` pada `body *` + `visibility:
 * visible` pada `#struk` — SENGAJA DIGANTI karena `visibility: hidden` tidak
 * menghapus elemen dari layout, cuma menyembunyikannya secara visual. Seluruh
 * isi halaman admin (sidebar, form, dst) tetap memakan tinggi dokumen saat
 * dicetak, jadi browser menghitung halaman kedua yang kosong (dialog cetak
 * melaporkan "2 sheets of paper" walau yang terlihat cuma satu struk).
 * `display: none` benar-benar menghapus elemen dari alur layout, jadi tinggi
 * dokumen cetak jadi setinggi struk saja.
 *
 * `:global(...)` wajib dipakai untuk selector yang menyentuh elemen di luar
 * root komponen ini (`body`) — `<style scoped>` polos tidak bisa
 * menjangkaunya. Semua di sini dibungkus `@media print`, jadi tidak
 * berpengaruh sama sekali ke tampilan layar.
 */
@media print {
  :global(html) {
    /* Semua ukuran teks Tailwind (text-sm, text-lg, dst) pakai unit `rem`
       yang relatif ke root <html>, BUKAN ke parent — jadi cara paling rapi
       menurunkan semua ukuran teks struk sekaligus (tanpa override tiap
       kelas satu-satu) adalah mengecilkan root font-size ini, khusus saat
       print. Kertas 58mm cuma muat ~32 karakter monospace, jadi teks perlu
       jauh lebih kecil dari tampilan layar. */
    font-size: 11px;
  }

  :global(body) {
    background: white;
  }

  /* Sembunyikan SELURUH anak langsung body kecuali root struk yang
     ter-teleport. `:not(#struk-print-root)` — bukan cuma `body *` — supaya
     hanya level teratas yang dihapus dari layout (menghapus satu leluhur
     sudah cukup menghapus semua keturunannya, tidak perlu selector
     universal yang lebih berat).

     Digerbangi `html.cetak-struk` yang cuma menempel selama `printNow()`
     berjalan: tanpa gerbang itu, Ctrl+P biasa di halaman POS (yang strukya
     ter-mount permanen tapi belum ter-teleport) akan mengeluarkan kertas
     kosong. */
  :global(html.cetak-struk body > *:not(#struk-print-root)) {
    display: none !important;
  }

  #struk img,
  #struk svg {
    /* Dua-duanya sudah murni hitam-putih (logo = PNG 1-bit hasil threshold,
       QR = SVG vektor), jadi tidak ada gradasi yang perlu di-dither. Aturan
       ini jaring pengaman supaya driver/browser tidak menghapus area hitam
       solid saat opsi "print backgrounds" sedang mati — tanpa ini logo dan
       QR bisa hilang sama sekali dari kertas. */
    print-color-adjust: exact;
    -webkit-print-color-adjust: exact;
  }
}
</style>
