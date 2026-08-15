/**
 * payment.ts — satu-satunya sumber info rekening/QRIS statis Rajaku Printing
 * (§7 — pembayaran manual, tanpa payment gateway). Dipakai oleh halaman mana
 * pun yang menampilkan panel "transfer ke" untuk pembeli:
 *  - `/akun/pesanan/[resi].vue` (customer login)
 *  - `/lacak/[resi].vue` (guest terverifikasi)
 *
 * JANGAN duplikasi/hardcode nilai ini ke file lain — kalau butuh, import dari sini.
 *
 * TODO(rajaku): nilai di bawah ini idealnya datang dari modul `settings` admin
 * (sudah ada di backend) supaya bisa diubah tanpa deploy ulang. Hardcode ini
 * utang teknis sementara — belum diimplementasikan di task ini.
 */

export interface BankInfo {
  bankName: string
  accountName: string
  accountNumber: string
  qrisNote: string
}

export const bankInfo: BankInfo = {
  bankName: 'BCA',
  accountName: 'PT Rajaku Printing',
  accountNumber: '1234567890',
  qrisNote: 'QRIS statis: scan di toko fisik, minta ke admin via WA.',
}
