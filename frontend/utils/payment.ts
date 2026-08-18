/**
 * payment.ts — satu-satunya sumber info rekening/QRIS Rajaku Printing
 * (§7 — pembayaran manual, tanpa payment gateway). Dipakai oleh halaman mana
 * pun yang menampilkan panel "transfer ke" untuk pembeli:
 *  - `/akun/pesanan/[resi].vue` (customer login)
 *  - `/lacak/[resi].vue` (guest terverifikasi)
 *
 * JANGAN duplikasi/hardcode nilai ini ke file lain — kalau butuh, import dari sini.
 *
 * TODO(rajaku): nilai di bawah idealnya datang dari modul `settings` admin
 * supaya bisa diubah tanpa deploy ulang, dan gambar QRIS dari slot modul
 * `sitemedia`. Belum dikerjakan — sampai itu ada, mengganti rekening berarti
 * mengedit berkas ini lalu deploy ulang.
 */

export interface BankInfo {
  bankName: string
  accountName: string
  accountNumber: string
  qrisNote: string
}

export const bankInfo: BankInfo = {
  bankName: 'BCA',
  accountName: 'CV WANSHOU NIAGA UTAMA',
  accountNumber: '3245070777',
  qrisNote: 'Pembayaran QRIS segera hadir — untuk saat ini gunakan transfer bank.',
}
