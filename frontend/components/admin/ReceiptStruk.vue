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
import { computed } from 'vue'
import { renderSVG } from 'uqr'

import { business } from '~/utils/business'

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

// Identitas toko dibaca dari SATU sumber (utils/business.ts) yang juga
// menyuplai schema markup LocalBusiness & footer — sekali pemilik memperbarui
// alamat/telepon di sana, struk ikut berubah tanpa sentuh file ini.
// Sengaja HANYA nama, alamat jalan, kota, dan telepon: postalCode/email/geo
// di file itu masih ditandai TODO (placeholder) dan tidak boleh tercetak ke
// tangan pelanggan.
const storeAddress = computed(() => `${business.streetAddress}, ${business.addressLocality}`)

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
 * `@page { size: ... }` HARUS pakai angka literal — browser tidak bisa
 * membaca custom property (`var(--w)`) di properti `size`. Satu-satunya cara
 * membuat lebar kertas dinamis di satu-satunya `@page` yang aktif per
 * halaman adalah menyuntik stylesheet baru tiap kali `receiptWidthMm`
 * berubah, lewat `useHead` (unhead men-support ref/computed reaktif di
 * dalam array `style`, jadi tag <style> ini otomatis diperbarui).
 *
 * Dipisah dari `<style scoped>` di bawah karena Vue SFC scoped-CSS compiler
 * tidak menambahkan atribut scope ke isi blok `@page` (tidak ada selector
 * untuk di-scope) — taruh di sini sebagai stylesheet biasa (selector
 * `#struk` tetap match elemen aslinya walau lewat tag <style> terpisah,
 * karena ID selector tidak butuh atribut scope untuk match).
 */
useHead({
  style: [
    {
      key: 'pos-receipt-page-size',
      innerHTML: computed(
        () => `@media print {
  @page { size: ${receiptWidthMm.value}mm auto; margin: 0; }
  #struk { width: ${receiptWidthMm.value}mm; }
}`,
      ),
    },
  ],
})

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
</script>

<template>
  <div class="flex justify-center bg-canvas-alt p-6 print:block print:bg-transparent print:p-0">
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
        <p class="break-words">{{ storeAddress }}</p>
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
</template>

<style scoped>
/*
 * Isolasi cetak — teknik klasik: sembunyikan seluruh body, tampilkan cuma
 * #struk. Dipindah apa adanya dari pages/admin/pos/index.vue supaya kedua
 * halaman pemakai (POS & Detail Order) mendapat teknik yang sama persis.
 * Halaman pemanggil tetap disarankan menambah `print:hidden` di elemen
 * levelnya sendiri (header, sidebar dst) sebagai jaring pengaman kedua —
 * lihat pemakaian di `pages/admin/pos/index.vue` & `pages/admin/order/[resi]/index.vue`.
 *
 * `:global(...)` wajib dipakai untuk selector yang menyentuh elemen di luar
 * root komponen ini (`body`, `html`) — `<style scoped>` polos tidak bisa
 * menjangkaunya. Semua di sini dibungkus `@media print`, jadi tidak
 * berpengaruh sama sekali ke tampilan layar — termasuk saat komponen ini
 * di-mount tersembunyi di layar (mis. `hidden print:block` di Detail Order,
 * struk cuma boleh muncul saat benar-benar mencetak).
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

  :global(body *) {
    visibility: hidden;
  }

  #struk,
  #struk * {
    visibility: visible !important;
  }

  #struk {
    position: absolute;
    top: 0;
    left: 0;
    padding: 3mm;
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
