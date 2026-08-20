import type { PaymentInfo } from '~/types/payment'

/**
 * usePaymentInfo — jembatan halaman pembeli ↔ modul `settings` untuk info
 * rekening/QRIS (§7 pembayaran manual). Fetch `GET /payment-info` (publik).
 *
 * Pola fetch sama seperti `useSiteMedia`: `useAsyncData` dengan key tetap
 * supaya request ter-dedupe & ikut ter-render saat SSR (§15 SEO / no flash
 * konten setelah hydration).
 *
 * BEDA PALING PENTING dari `useSiteMedia`: TIDAK ADA fallback hardcode kalau
 * request gagal. `useSiteMedia` boleh jatuh ke gambar statis karena gambar
 * yang "salah" cuma jelek dilihat. Nomor rekening yang "salah" — yaitu
 * menampilkan angka lama yang mungkin sudah diganti admin atau nilai
 * hardcode basi — berarti mengarahkan uang pelanggan ke rekening yang bukan
 * milik toko lagi. Itu bukan cacat visual, itu potensi kerugian finansial
 * nyata. Jadi kalau fetch gagal (backend mati, network error, dsb), composable
 * ini mengembalikan `null` apa adanya — pemanggil WAJIB menyembunyikan panel
 * "transfer ke" dan menampilkan pesan bahwa info pembayaran sedang tidak bisa
 * dimuat, bukan diam-diam menampilkan nilai lama/placeholder.
 *
 * JANGAN "perbaiki" ini dengan menambahkan default/fallback value di masa
 * depan — itu justru menghidupkan kembali kelas bug yang composable ini
 * sengaja dihindari (lihat TODO lama di `utils/payment.ts`, sudah dihapus).
 */
export function usePaymentInfo() {
  const api = useApi()

  const asyncData = useAsyncData<PaymentInfo | null>(
    'payment-info',
    async () => {
      try {
        return await api.get<PaymentInfo>('/payment-info')
      } catch {
        // Sengaja return null, BUKAN nilai bawaan apa pun — lihat docblock
        // di atas. Pemanggil harus treat null sebagai "sembunyikan panel".
        return null
      }
    },
    { default: () => null },
  )

  return {
    info: asyncData.data,
    ready: Promise.resolve(asyncData).then(() => undefined),
  }
}
