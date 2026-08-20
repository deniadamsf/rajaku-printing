-- ============================================================================
-- 000020_payment_settings — rekening/QRIS pembayaran manual jadi configurable
-- (§7 — pembayaran manual, tanpa payment gateway)
--
-- Sebelumnya nilai ini hardcode di frontend/utils/payment.ts. Konsekuensinya:
-- kalau rekening berubah, mengubahnya berarti edit kode + deploy ulang — kalau
-- deploy tertunda, uang pelanggan bisa terlanjur masuk ke rekening yang salah.
--
-- Migration ini seed 6 baris ke app_settings (tabel sudah ada, dibuat migration
-- 000011) supaya super admin bisa ubah dari admin panel tanpa deploy:
--   - payment.bank_name           nama bank
--   - payment.account_name        nama pemilik rekening
--   - payment.account_number      nomor rekening (digit saja, ternormalisasi
--                                 oleh settings/service saat di-update)
--   - payment.qris_note           teks instruksi di bawah gambar QRIS
--   - payment.qris_merchant_name  nama merchant yang tampil saat QRIS dipindai
--   - payment.qris_nmid           National Merchant ID QRIS
--
-- Nilai awal = nilai produksi yang sebelumnya hardcode di payment.ts (JANGAN
-- diubah di migration lanjutan — perubahan rekening dilakukan lewat
-- PUT /admin/settings/:key, bukan migration baru).
--
-- Gambar QRIS TIDAK termasuk di sini — sudah ditangani slot `qris_code` modul
-- sitemedia (migration terpisah).
-- ============================================================================

INSERT INTO app_settings (key, value, display_name, description) VALUES
    (
        'payment.bank_name',
        'BCA',
        'Nama bank tujuan transfer',
        'Nama bank yang ditampilkan di halaman pembayaran, mis. "BCA". Wajib diisi.'
    ),
    (
        'payment.account_name',
        'CV WANSHOU NIAGA UTAMA',
        'Nama pemilik rekening',
        'Nama pemilik rekening tujuan transfer, ditampilkan supaya pembeli yakin transfer ke tempat yang benar. Wajib diisi.'
    ),
    (
        'payment.account_number',
        '3245070777',
        'Nomor rekening',
        'Nomor rekening tujuan transfer. Hanya boleh berisi digit (spasi/strip di input akan dibuang otomatis saat disimpan). Wajib diisi — ini menentukan ke mana uang pelanggan dikirim.'
    ),
    (
        'payment.qris_note',
        'Pindai QRIS dengan aplikasi apa pun berlogo QRIS, lalu unggah bukti bayarnya.',
        'Catatan/instruksi QRIS',
        'Teks instruksi yang tampil di bawah gambar QRIS di halaman pembayaran. Boleh dikosongkan kalau QRIS belum tersedia.'
    ),
    (
        'payment.qris_merchant_name',
        'EVENT WOWINFOOD',
        'Nama merchant QRIS',
        'Nama merchant yang MUNCUL di aplikasi pembayaran pembeli saat QRIS dipindai — bisa berbeda dari nama rekening bank, ditampilkan supaya pembeli tidak ragu. Boleh dikosongkan kalau QRIS belum tersedia.'
    ),
    (
        'payment.qris_nmid',
        'ID2026569993978',
        'National Merchant ID (NMID) QRIS',
        'Kode NMID QRIS resmi. Boleh dikosongkan kalau QRIS belum tersedia.'
    );
