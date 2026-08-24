-- ============================================================================
-- 000028_discount_product_scope — cakupan diskon per produk (§28.9 CLAUDE.md).
--
-- Awalnya diskon berlaku global — disaring hanya oleh channel_scope,
-- min_subtotal, dan masa berlaku (migration 000027). Ditambahkan kemampuan
-- membatasi sebuah diskon hanya untuk produk tertentu.
--
-- Yang membuat ini murah: SATU ORDER = SATU PRODUK di sistem ini (`orders`
-- menyimpan satu `product_id`, tidak ada tabel item baris) — jadi "diskon per
-- produk" tidak menuntut pembongkaran struktur order, cukup penyaringan saat
-- diskon dipakai.
--
-- 1) discounts.applies_to — 'all' (default) | 'selected'. DEFAULT 'all'
--    PENTING: semua diskon yang sudah ada di produksi harus tetap
--    berperilaku PERSIS seperti sebelum migration ini (berlaku untuk semua
--    produk), tidak ada migrasi data tambahan yang perlu dijalankan.
--
-- 2) discount_products — daftar produk yang termasuk cakupan sebuah diskon
--    applies_to='selected'. Kalau applies_to='all', tabel ini tidak relevan
--    untuk diskon tersebut (baris boleh kosong atau tidak ada sama sekali).
--
--    FK discount_id -> discounts(id) ON DELETE CASCADE — INI BOLEH CASCADE,
--    BEDA dengan orders.discount_id yang SENGAJA TANPA CASCADE (migration
--    000027): baris di sini cuma daftar CAKUPAN, bukan catatan transaksi,
--    jadi tidak ada rekap/histori yang bergantung padanya. Menghapus baris
--    discounts (kalaupun suatu saat terjadi) wajar ikut membersihkan
--    cakupannya.
--
--    FK product_id -> products(id) TANPA cascade khusus — modul catalog
--    TIDAK PERNAH hard-delete produk (hanya is_active=false, lihat
--    internal/catalog/model/product.go), jadi FK ini aman: produk yang
--    dirujuk selalu ada barisnya walau sudah dinonaktifkan.
--
-- ATURAN KERAS yang wajib dijaga di lapisan Go (TIDAK bisa sepenuhnya
-- ditegakkan lewat CHECK constraint SQL karena butuh JOIN lintas tabel):
-- applies_to='selected' dengan discount_products KOSONG (belum diisi admin,
-- ATAU seluruh produknya sudah tidak lagi tercakup) TIDAK BOLEH diperlakukan
-- sebagai "berlaku untuk semua" — daftar kosong = tidak ada order yang cocok
-- = diskon tidak bisa dipakai sama sekali. Dicegah di discount/service, DUA
-- kali (create/update DAN validateForUse saat dipakai) — lihat discount
-- service Go, bukan cuma di titik pembuatan.
-- ============================================================================

ALTER TABLE discounts
    ADD COLUMN applies_to VARCHAR(20) NOT NULL DEFAULT 'all';

ALTER TABLE discounts
    ADD CONSTRAINT chk_discounts_applies_to CHECK (applies_to IN ('all', 'selected'));

CREATE TABLE discount_products (
    discount_id UUID        NOT NULL REFERENCES discounts(id) ON DELETE CASCADE,
    product_id  UUID        NOT NULL REFERENCES products(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- PRIMARY KEY (discount_id, product_id) sudah menegakkan UNIQUE
    -- (discount_id, product_id) yang diminta brief.
    PRIMARY KEY (discount_id, product_id)
);

-- Index terpisah pada product_id — PRIMARY KEY di atas hanya efisien untuk
-- query yang memfilter dari discount_id (kolom pertama komposit). Query
-- kebalikannya ("diskon mana saja yang menyertakan produk X", dipakai
-- GET /admin/discounts/applicable?product_id=...) butuh index sendiri.
CREATE INDEX idx_discount_products_product_id ON discount_products (product_id);
