-- ============================================================================
-- 000031_discount_member_scope — diskon khusus member (§30.3 CLAUDE.md).
--
-- Kenapa bukan modul/tabel terpisah: modul discount (§28) sudah punya
-- seluruh mesin yang dibutuhkan — snapshot ke order (§28.2), validasi masa
-- berlaku/kuota/channel (§28.4), dan pola pembatasan cakupan
-- (discount_products, §28.9, migration 000028). Diskon member cuma butuh
-- SATU SUMBU PEMBATASAN BARU yang sejajar dengan channel_scope yang sudah
-- ada, bukan konsep uang yang berbeda.
--
-- 1) discounts.audience_scope — 'all' (default) | 'member'. DEFAULT 'all'
--    PENTING: semua diskon yang sudah ada tetap berperilaku PERSIS seperti
--    sebelum migration ini (berlaku untuk semua orang), tidak ada migrasi
--    data tambahan yang perlu dijalankan. Independen dari channel_scope —
--    sebuah diskon boleh sekaligus "khusus member" DAN "khusus POS".
--
-- 2) discounts.member_scope — NULL | 'all_members' | 'selected_members'.
--    Diisi HANYA kalau audience_scope='member'. BEDA PENTING dengan
--    applies_to/discount_products (§28.9) — di sana daftar kosong berarti
--    "tidak berlaku untuk produk manapun" (ditolak). Di sini member_scope
--    adalah enum EKSPLISIT, bukan disimpulkan dari isi discount_customers
--    kosong-atau-tidak:
--      - 'all_members'      -> berlaku untuk SEMUA member 'active',
--                               discount_customers tidak dipakai sama
--                               sekali (boleh kosong, itu normal).
--      - 'selected_members'  -> berlaku HANYA untuk customer yang ada
--                               barisnya di discount_customers; daftar
--                               kosong TETAP ditolak di lapisan Go (sama
--                               pola dengan applies_to='selected' kosong).
--    Alasan dibuat enum eksplisit: kalau dipakai konvensi "kosong = semua"
--    ala discount_products, kosong pada discount_customers jadi AMBIGU —
--    "semua member" atau "belum ada satupun dipilih, jangan berlaku dulu"?
--    Enum eksplisit menghapus tebak-tebakan itu dari awal.
--
--    CHECK constraint kombinasi audience_scope<->member_scope BISA (dan
--    wajib) ditegakkan murni lewat SQL di sini — beda dengan kasus
--    discount_products yang butuh JOIN lintas tabel (tidak bisa lewat CHECK
--    biasa): member_scope HARUS NULL kalau audience_scope != 'member', dan
--    HARUS diisi kalau audience_scope = 'member'. DB jadi penjaga terakhir,
--    bukan cuma validasi service (§28.1 aturan yang sama untuk kolom
--    value_percent/value_amount, migration 000027).
--
-- 3) discount_customers — daftar customer yang termasuk cakupan sebuah
--    diskon member_scope='selected_members'. Mirror PERSIS discount_products
--    (migration 000028): FK discount_id -> discounts(id) ON DELETE CASCADE
--    (baris ini cuma daftar CAKUPAN, bukan catatan transaksi, jadi aman
--    cascade). FK customer_id -> users(id) TANPA cascade khusus — status
--    membership hidup di kolom users.membership_status (migration 000030,
--    "tabel customer" secara fisik diimplementasikan sebagai users), dan
--    users TIDAK PERNAH hard-delete (soft-delete/deactivate saja, lihat
--    internal/auth/model/user.go), jadi FK ini aman.
--
-- ATURAN KERAS yang wajib dijaga di lapisan Go (TIDAK bisa sepenuhnya
-- ditegakkan lewat CHECK constraint SQL karena butuh JOIN lintas tabel):
-- member_scope='selected_members' dengan discount_customers KOSONG (belum
-- diisi admin) TIDAK BOLEH diperlakukan sebagai "berlaku untuk semua member"
-- — dicegah di discount/service, DUA kali (create/update DAN validateForUse
-- saat dipakai), sama persis pola §28.9.
-- ============================================================================

ALTER TABLE discounts
    ADD COLUMN audience_scope VARCHAR(20) NOT NULL DEFAULT 'all',
    ADD COLUMN member_scope   VARCHAR(20) NULL;

ALTER TABLE discounts
    ADD CONSTRAINT chk_discounts_audience_scope CHECK (audience_scope IN ('all', 'member'));

ALTER TABLE discounts
    ADD CONSTRAINT chk_discounts_member_scope
        CHECK (member_scope IS NULL OR member_scope IN ('all_members', 'selected_members'));

-- Konsistensi antar-kolom di baris yang sama — murni SQL, tidak butuh JOIN.
ALTER TABLE discounts
    ADD CONSTRAINT chk_discounts_member_scope_consistency
        CHECK (
            (audience_scope = 'member' AND member_scope IS NOT NULL)
            OR (audience_scope != 'member' AND member_scope IS NULL)
        );

CREATE TABLE discount_customers (
    discount_id UUID        NOT NULL REFERENCES discounts(id) ON DELETE CASCADE,
    customer_id UUID        NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- PRIMARY KEY (discount_id, customer_id) sudah menegakkan UNIQUE
    -- berpasangan yang diminta brief.
    PRIMARY KEY (discount_id, customer_id)
);

-- Index terpisah pada customer_id — PRIMARY KEY di atas hanya efisien untuk
-- query yang memfilter dari discount_id (kolom pertama komposit). Query
-- kebalikannya ("diskon mana saja yang menyertakan customer X", dipakai
-- GET /admin/discounts/applicable?customer_id=...) butuh index sendiri.
CREATE INDEX idx_discount_customers_customer_id ON discount_customers (customer_id);
