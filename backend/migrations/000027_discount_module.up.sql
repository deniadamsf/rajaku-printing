-- ============================================================================
-- 000027_discount_module — modul Diskon & Rekap Order (§28 CLAUDE.md).
--
-- Keputusan pemilik proyek (24 Agustus 2026): diskon dikelola & dipakai
-- INTERNAL (admin bikin master diskon, kasir/admin memilihnya saat membuat
-- order) — TIDAK ADA kolom "kode promo" di form order publik. Diskon hanya
-- memotong subtotal produk, tidak pernah ongkir.
--
-- Masalah yang harus dicegah SEJAK DB (§28.2): diskon punya masa berlaku dan
-- bisa dinonaktifkan/dihapus admin kapan saja. Kalau order cuma menyimpan FK
-- discount_id, begitu diskonnya dihapus/berubah, rekap transaksi bulan lalu
-- ikut rusak (angka kosong, atau query JOIN gagal/berubah retroaktif). Tiga
-- lapis pencegahan diimplementasikan di sini sekaligus:
--
-- 1) Tabel `discounts` — master diskon, TIDAK PERNAH di-hard-delete (soft
--    delete via deleted_at/deleted_by/delete_reason, pola sama seperti
--    `orders` di migration 000025). CHECK constraint menjaga konsistensi
--    type vs kolom nilai LANGSUNG DI DB — jangan andalkan validasi service
--    saja, DB adalah penjaga terakhir. Unique index pada `code` bersifat
--    PARTIAL (hanya baris deleted_at IS NULL) — supaya kode yang sama boleh
--    dipakai lagi setelah diskon lama dihapus, tanpa bentrok unique
--    constraint dengan baris lama yang masih tersimpan untuk histori.
--
-- 2) Kolom snapshot di `orders` — nilai diskon yang berlaku SAAT order dibuat
--    disalin ke kolom milik `orders` sendiri (bukan cuma FK). Semua
--    perhitungan uang (rekap, invoice, struk) WAJIB membaca kolom snapshot
--    ini, TIDAK PERNAH JOIN ke `discounts` untuk angka uang — itu tugas
--    lapisan service (order/repository, lihat kode Go), migration ini hanya
--    menyediakan kolomnya + FK murni untuk telusur balik ("order mana saja
--    pakai promo ini") TANPA ON DELETE CASCADE (kalau ada yang tergoda
--    menambah CASCADE nanti: JANGAN — itu jalan yang membuat rekap hilang).
--
-- 3) Permission baru (discount.manage, discount.apply, report.view) — default
--    di-grant ke super_admin untuk ketiganya, PLUS cashier untuk
--    discount.apply (kasir yang memakai diskon saat transaksi POS). Sama
--    seperti 000025: tetap permission granular biasa (§10), boleh
--    ditoggle ke role lain lewat "Kelola Role" kalau memang dibutuhkan.
--    Risiko yang perlu disadari sebelum memberi discount.apply ke role lain:
--    pemegangnya bisa memotong harga jual (termasuk lewat diskon manual
--    tanpa batas nominal) — beri hanya ke role yang memang berwenang
--    menentukan harga.
-- ============================================================================

-- ----------------------------------------------------------------------------
-- 1) discounts — master diskon
-- ----------------------------------------------------------------------------
CREATE TABLE discounts (
    id                  UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    code                VARCHAR(30)   NOT NULL,   -- handle pendek unik, dipakai kasir mencari cepat
    name                VARCHAR(150)  NOT NULL,   -- nama yang dibaca manusia, tampil di struk/invoice
    type                VARCHAR(20)   NOT NULL,   -- 'percent' | 'nominal'
    value_percent       NUMERIC(5,2)  NULL,       -- diisi hanya kalau type='percent', 0 < v <= 100
    value_amount        BIGINT        NULL,       -- diisi hanya kalau type='nominal', rupiah bulat > 0
    max_discount_amount BIGINT        NULL,       -- batas atas rupiah utk tipe persen; NULL = tanpa batas
    min_subtotal        BIGINT        NOT NULL DEFAULT 0,
    starts_at           TIMESTAMPTZ   NULL,       -- NULL = tanpa batas mulai
    ends_at             TIMESTAMPTZ   NULL,       -- NULL = tanpa batas akhir
    quota               INT           NULL,       -- maks berapa order boleh pakai; NULL = tak terbatas.
                                                   -- DIHITUNG dari orders (COUNT WHERE discount_id=...
                                                   -- AND deleted_at IS NULL) — TIDAK ADA kolom counter
                                                   -- di sini (§28.4, hindari dual-write yang bisa melenceng).
    channel_scope       VARCHAR(20)   NOT NULL DEFAULT 'all', -- 'all' | 'online' | 'pos'
    is_active           BOOLEAN       NOT NULL DEFAULT true,  -- saklar manual admin, terpisah dari masa berlaku

    -- Soft delete — TIDAK PERNAH hard delete (lihat header). Pola sama
    -- seperti orders (migration 000025): barisnya tetap ada, jadi FK
    -- orders.discount_id tidak pernah menggantung dan telusur balik tetap
    -- jalan selamanya.
    deleted_at          TIMESTAMPTZ   NULL,
    deleted_by          UUID          NULL REFERENCES users(id),
    delete_reason       TEXT          NULL,

    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_discounts_type CHECK (type IN ('percent', 'nominal')),
    CONSTRAINT chk_discounts_channel_scope CHECK (channel_scope IN ('all', 'online', 'pos')),
    CONSTRAINT chk_discounts_min_subtotal CHECK (min_subtotal >= 0),
    CONSTRAINT chk_discounts_quota CHECK (quota IS NULL OR quota > 0),
    -- Temuan review #9(b) — max_discount_amount HANYA relevan untuk
    -- type='percent' (computeAmount mengabaikannya total untuk 'nominal' —
    -- lihat calc.go); diperketat dari sekadar "> 0" jadi "hanya boleh diisi
    -- kalau type=percent", supaya diskon nominal ber-max_discount_amount
    -- (yang tidak pernah punya efek nyata) ditolak sejak DB, bukan cuma
    -- diam-diam diabaikan di kode Go.
    CONSTRAINT chk_discounts_max_discount_amount CHECK (
        (max_discount_amount IS NULL)
        OR (type = 'percent' AND max_discount_amount > 0)
    ),
    -- Temuan review #3 — ends_at harus setelah starts_at kalau KEDUANYA
    -- diisi (salah satu/keduanya NULL selalu valid, artinya "tanpa batas").
    -- Diskon dengan ends_at <= starts_at tersimpan tapi validateForUse
    -- menolaknya SELAMANYA tanpa petunjuk — dicegah sejak service (§ lihat
    -- discount_service.go validatePeriod), CHECK ini penjaga terakhir di DB.
    CONSTRAINT chk_discounts_period CHECK (
        ends_at IS NULL OR starts_at IS NULL OR ends_at > starts_at
    ),
    -- Konsistensi type <-> kolom nilai — DB sebagai penjaga terakhir (§28.1),
    -- jangan andalkan validasi service saja.
    CONSTRAINT chk_discounts_value_consistency CHECK (
        (type = 'percent' AND value_percent IS NOT NULL AND value_percent > 0 AND value_percent <= 100
            AND value_amount IS NULL)
        OR
        (type = 'nominal' AND value_amount IS NOT NULL AND value_amount > 0
            AND value_percent IS NULL)
    )
);

-- Partial unique index: HANYA baris aktif (belum dihapus) yang wajib punya
-- code unik — kode boleh dipakai ulang setelah diskon lama di-soft-delete.
CREATE UNIQUE INDEX idx_discounts_code_active ON discounts (code) WHERE deleted_at IS NULL;

-- Partial index deleted_at — pola sama seperti idx_orders_deleted_at
-- (migration 000025): setiap query pembaca diskon aktif filter
-- deleted_at IS NULL, index ini yang dipakai.
CREATE INDEX idx_discounts_deleted_at ON discounts (deleted_at) WHERE deleted_at IS NULL;

-- Index tambahan untuk query "/admin/discounts/applicable" (kasir mencari
-- diskon yang boleh dipakai untuk channel tertentu) — filter is_active +
-- channel_scope dipakai di HAMPIR setiap panggilan endpoint itu.
CREATE INDEX idx_discounts_active_channel ON discounts (is_active, channel_scope) WHERE deleted_at IS NULL;

-- ----------------------------------------------------------------------------
-- 2) orders — kolom snapshot diskon (§28.2 lapis 1)
-- ----------------------------------------------------------------------------
ALTER TABLE orders
    ADD COLUMN discount_id              UUID          NULL REFERENCES discounts(id), -- TANPA ON DELETE CASCADE (§28.2)
    ADD COLUMN discount_code_snapshot   VARCHAR(30)   NULL,
    ADD COLUMN discount_name_snapshot   VARCHAR(150)  NULL,
    ADD COLUMN discount_type_snapshot   VARCHAR(20)   NULL,  -- 'percent' | 'nominal' | 'manual'
    ADD COLUMN discount_value_snapshot  NUMERIC(12,2) NULL,  -- 25.00 (persen) atau 50000 (nominal); NULL utk manual
    ADD COLUMN discount_amount          BIGINT        NOT NULL DEFAULT 0, -- rupiah yg BENAR-BENAR dipotong; SEMUA
                                                                           -- query uang (rekap/invoice/struk) baca
                                                                           -- kolom ini, TIDAK PERNAH JOIN discounts.
    ADD COLUMN discount_note            TEXT          NULL;  -- wajib diisi utk diskon manual

ALTER TABLE orders ADD CONSTRAINT chk_orders_discount_amount CHECK (discount_amount >= 0);

-- Temuan review #9(a) — discount_type_snapshot cuma dibatasi lewat komentar
-- kolom di atas, bukan constraint nyata. Kosakata 'percent' | 'nominal' |
-- 'manual' ditegakkan di DB juga (NULL tetap boleh, untuk order tanpa diskon).
ALTER TABLE orders ADD CONSTRAINT chk_orders_discount_type_snapshot
    CHECK (discount_type_snapshot IS NULL OR discount_type_snapshot IN ('percent', 'nominal', 'manual'));

CREATE INDEX idx_orders_discount_id ON orders (discount_id);

-- ----------------------------------------------------------------------------
-- 3) permissions — discount.manage & report.view (super_admin only),
--    discount.apply (super_admin + cashier, §28.6)
-- ----------------------------------------------------------------------------
INSERT INTO menu_permissions (code, display_name, category, description) VALUES
    ('discount.manage', 'Kelola master diskon',  'discount', 'Buat/ubah/nonaktifkan/hapus master diskon — tindakan admin'),
    ('discount.apply',  'Pakai diskon di order',  'discount', 'Memakai diskon (master atau manual) saat membuat order — bisa memotong harga jual'),
    ('report.view',     'Lihat rekap order',      'order',    'Buka halaman Rekap Order & ekspor CSV');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, menu_permissions p
WHERE r.name = 'super_admin'
  AND p.code IN ('discount.manage', 'discount.apply', 'report.view');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, menu_permissions p
WHERE r.name = 'cashier'
  AND p.code = 'discount.apply';
