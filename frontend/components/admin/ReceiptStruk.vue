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
 *   - Logo diambil reaktif dari `useSiteMedia()` TANPA `await ready` di
 *     top-level: komponen ini dipakai sebagai child biasa (bukan route
 *     page), top-level await akan membuatnya jadi async component yang
 *     butuh `<Suspense>` di semua tempat pemakaian. `resolve()` tetap
 *     reaktif (computed re-evaluate begitu data site-media termuat) dan
 *     langsung fallback ke path statis selagi menunggu — cukup untuk
 *     konteks struk (bukan hero SSR-critical).
 */
import { computed } from 'vue'

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
  /** Lebar kertas thermal aktif (mm). Nilai selain 58/80 di-guard ke 58. */
  widthMm?: number
}

const props = withDefaults(defineProps<ReceiptStrukProps>(), {
  shippingCost: null,
  widthMm: 58,
})

const { resolve: resolveMedia } = useSiteMedia()
const brandLogo = computed(() => resolveMedia('brand_logo_full'))

const receiptWidthMm = computed<58 | 80>(() => (props.widthMm === 80 ? 80 : 58))

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
  <div id="struk" class="rounded-lg border border-hairline bg-canvas p-6 max-w-md print:border-0 print:shadow-none print:p-0">
    <div class="text-center">
      <img
        :src="brandLogo"
        alt="Logo Rajaku Printing"
        class="mx-auto h-10 w-auto object-contain print:grayscale"
      >
      <p class="mt-2 font-serif text-lg font-semibold text-ink-950">Rajaku Printing</p>
      <p class="mt-0.5 text-xs text-ink-500">Struk Order</p>
    </div>

    <!-- Pelanggan -->
    <div class="mt-4 border-t border-dashed border-hairline pt-4 space-y-1.5 text-sm">
      <div class="flex justify-between gap-2">
        <span class="text-ink-500 shrink-0">Resi</span>
        <span class="font-mono font-semibold text-ink-950 text-right break-all">{{ resi }}</span>
      </div>
      <div class="flex justify-between gap-2">
        <span class="text-ink-500 shrink-0">Tanggal</span>
        <span class="text-ink-900 text-right">{{ fmtDateTime(createdAt) }}</span>
      </div>
      <div class="flex justify-between gap-2">
        <span class="text-ink-500 shrink-0">Pelanggan</span>
        <span class="text-ink-900 text-right break-words">{{ customerName }}</span>
      </div>
      <div class="flex justify-between gap-2">
        <span class="text-ink-500 shrink-0">No. WA</span>
        <span class="font-mono text-ink-900 text-right break-all">{{ customerPhone }}</span>
      </div>
    </div>

    <!-- Item -->
    <div class="mt-3 border-t border-dashed border-hairline pt-3 space-y-1.5 text-sm">
      <div class="flex justify-between gap-2">
        <span class="text-ink-500 shrink-0">Produk</span>
        <span class="text-ink-900 text-right break-words">{{ productName }}</span>
      </div>
      <div class="flex justify-between gap-2">
        <span class="text-ink-500 shrink-0">Bahan</span>
        <span class="text-ink-900 text-right break-words">{{ materialName }}</span>
      </div>
      <div class="flex justify-between gap-2">
        <span class="text-ink-500 shrink-0">Ukuran</span>
        <span class="text-ink-900 text-right">{{ widthCm }} × {{ heightCm }} cm</span>
      </div>
      <div class="flex justify-between gap-2">
        <span class="text-ink-500 shrink-0">Kuantitas</span>
        <span class="text-ink-900 text-right">{{ quantity }} pcs</span>
      </div>
      <div class="flex justify-between gap-2">
        <span class="text-ink-500 shrink-0">Harga satuan</span>
        <span class="text-ink-900 text-right">{{ fmtIDR(unitPrice) }}</span>
      </div>
    </div>

    <!-- Nominal -->
    <div class="mt-3 border-t border-dashed border-hairline pt-3 space-y-1.5 text-sm">
      <div class="flex justify-between gap-2">
        <span class="text-ink-500 shrink-0">Subtotal</span>
        <span class="text-ink-900 text-right">{{ fmtIDR(subtotal) }}</span>
      </div>
      <div v-if="showShipping" class="flex justify-between gap-2">
        <span class="text-ink-500 shrink-0">Ongkir</span>
        <span class="text-ink-900 text-right">{{ fmtIDR(shippingCost) }}</span>
      </div>
      <div class="flex justify-between gap-2">
        <span class="text-ink-500 shrink-0">Ambil</span>
        <span class="text-ink-900 text-right capitalize">{{ metodeAmbil }}</span>
      </div>
      <div class="flex justify-between gap-2">
        <span class="text-ink-500 shrink-0">Bayar</span>
        <span class="text-ink-900 text-right uppercase">{{ metodeBayar }}</span>
      </div>
      <div class="flex justify-between gap-2 border-t border-dashed border-hairline pt-2 mt-2">
        <span class="font-semibold text-ink-950 shrink-0">Total</span>
        <span class="font-semibold text-ink-950 text-right">{{ fmtIDR(total) }}</span>
      </div>
    </div>

    <div class="mt-4 border-t border-dashed border-hairline pt-4 text-center">
      <p class="text-xs text-ink-500">Lacak pesanan Anda:</p>
      <p class="mt-1 font-mono text-xs text-ink-700 break-all">{{ trackingUrl }}</p>
    </div>
    <p class="mt-4 text-center text-xs text-ink-500">Terima kasih atas pesanan Anda.</p>
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

  #struk img {
    /* Thermal printer cetak 1-bit/grayscale — cegah browser mencoba
       dithering warna logo jadi kotor saat cetak. */
    print-color-adjust: exact;
    -webkit-print-color-adjust: exact;
  }
}
</style>
