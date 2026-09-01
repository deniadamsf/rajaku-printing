-- ============================================================================
-- 000033_order_multi_item — order boleh memuat lebih dari satu produk (§32).
--
-- Sampai migration ini, satu order = satu produk: kolom item (product_id,
-- material_id, width_cm, height_cm, quantity, unit_price, subtotal,
-- design_brief) menempel langsung di `orders`. Migration ini memindahkan
-- kolom-kolom itu ke tabel baru `order_items` (satu baris per produk dalam
-- satu order, dibedakan `line_no`), supaya satu order bisa memuat banyak
-- ukuran/produk sekaligus dengan satu resi, satu ongkir, satu diskon (§32.3).
--
-- Urutan wajib: (1) buat order_items, (2) verifikasi data lama aman untuk
-- di-backfill, (3) BACKFILL dari orders yang sudah ada SEBELUM (4) kolom
-- lama di orders di-drop — supaya tidak ada order yang kehilangan datanya
-- di tengah migrasi.
--
-- orders.subtotal, orders.discount_amount, orders.shipping_cost,
-- orders.total, dan seluruh kolom snapshot diskon (§28.2) TIDAK berubah
-- artinya — tetap satu-satunya angka yang dibaca rekap/invoice/struk.
-- orders.design_source TETAP ADA tapi sekarang turunan (upload|request|
-- mixed) dari order_items.design_source, ditulis ulang oleh order service
-- setiap kali daftar item berubah (§32.1).
-- ============================================================================

-- ----------------------------------------------------------------------------
-- 1) order_items
-- ----------------------------------------------------------------------------
CREATE TABLE order_items (
    id                       UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    -- TANPA ON DELETE CASCADE — order tidak pernah di-hard-delete (migration
    -- 000025, soft delete saja), jadi item-nya juga tidak pernah perlu
    -- dihapus otomatis lewat cascade.
    order_id                 UUID         NOT NULL REFERENCES orders(id),
    line_no                  INT          NOT NULL CHECK (line_no >= 1),

    -- Product snapshot (sama pola dgn orders sebelumnya — denormalized
    -- supaya immun terhadap perubahan katalog belakangan).
    product_id               UUID         REFERENCES products(id)  ON DELETE RESTRICT,
    product_name_snapshot    VARCHAR(255) NOT NULL,
    material_id              UUID         REFERENCES materials(id) ON DELETE RESTRICT,
    material_name_snapshot   VARCHAR(255) NOT NULL,
    pricing_type_snapshot    VARCHAR(20)  NOT NULL,
    width_cm                 INT          NOT NULL CHECK (width_cm > 0),
    height_cm                INT          NOT NULL CHECK (height_cm > 0),
    quantity                 INT          NOT NULL DEFAULT 1 CHECK (quantity > 0),
    unit_price               BIGINT       NOT NULL CHECK (unit_price >= 0),
    subtotal                 BIGINT       NOT NULL CHECK (subtotal >= 0),

    -- Bagian diskon order (orders.discount_amount, §28.2) yang jatuh ke
    -- baris ini (§32.3 — metode sisa terbesar). HANYA untuk menjelaskan
    -- angka agregat per baris; layar uang (rekap/invoice/struk) tetap baca
    -- orders.discount_amount, TIDAK menjumlahkan kolom ini (§32.2).
    discount_amount          BIGINT       NOT NULL DEFAULT 0 CHECK (discount_amount >= 0 AND discount_amount <= subtotal),

    -- Desain PER ITEM (§32.5) — satu order boleh campur: item A bawa desain
    -- sendiri, item B minta dibuatkan.
    design_source            VARCHAR(20)  NOT NULL CHECK (design_source IN ('upload','request')),
    design_brief             TEXT,        -- brief singkat kalau design_source='request'
    item_notes               TEXT,        -- catatan kasir/staff khusus baris ini

    created_at               TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    UNIQUE (order_id, line_no)
);

CREATE INDEX idx_order_items_order ON order_items(order_id);

-- ----------------------------------------------------------------------------
-- 2) VERIFIKASI SEBELUM BACKFILL — order_items.discount_amount punya
--    CHECK (discount_amount <= subtotal) yang TIDAK ADA di orders (orders
--    cuma punya CHECK (discount_amount >= 0)). Backfill di bawah menyalin
--    orders.discount_amount mentah ke order_items.discount_amount dengan
--    order_items.subtotal = orders.subtotal (line_no=1) — kalau ada satu
--    baris data yang menyimpang (discount_amount > subtotal, seharusnya
--    tidak mungkin lolos §28.3 tapi jangan diasumsikan), INSERT di bawah
--    akan gagal dengan pesan constraint Postgres mentah tanpa diagnosa.
--    Gagalkan migrasi di sini dulu dengan pesan yang menyebutkan resi mana
--    saja yang bermasalah, supaya operator tahu persis apa yang harus
--    diperbaiki manual sebelum migrasi bisa jalan lagi.
-- ----------------------------------------------------------------------------
DO $$
DECLARE
    bad_count INT;
    bad_resis TEXT;
BEGIN
    SELECT COUNT(*), STRING_AGG(resi, ', ' ORDER BY resi)
      INTO bad_count, bad_resis
      FROM orders
     WHERE discount_amount > subtotal;
    IF bad_count > 0 THEN
        RAISE EXCEPTION
            '000033_order_multi_item: % order punya discount_amount > subtotal (order_items.discount_amount CHECK <= subtotal akan gagal saat backfill) — resi: %',
            bad_count, bad_resis;
    END IF;
END $$;

-- ----------------------------------------------------------------------------
-- 3) BACKFILL — setiap order lama jadi satu baris order_items (line_no=1),
--    SEBELUM kolom lama di orders dihapus. Tidak boleh ada order tanpa item.
-- ----------------------------------------------------------------------------
INSERT INTO order_items (
    order_id, line_no,
    product_id, product_name_snapshot,
    material_id, material_name_snapshot,
    pricing_type_snapshot,
    width_cm, height_cm, quantity, unit_price, subtotal,
    discount_amount,
    design_source, design_brief,
    created_at, updated_at
)
SELECT
    o.id, 1,
    o.product_id, o.product_name_snapshot,
    o.material_id, o.material_name_snapshot,
    o.pricing_type_snapshot,
    o.width_cm, o.height_cm, o.quantity, o.unit_price, o.subtotal,
    o.discount_amount,
    o.design_source, o.design_brief,
    o.created_at, o.updated_at
FROM orders o;

-- ----------------------------------------------------------------------------
-- 4) Drop kolom item lama dari orders — sudah dipindah & di-backfill di atas.
-- ----------------------------------------------------------------------------
ALTER TABLE orders
    DROP COLUMN product_id,
    DROP COLUMN product_name_snapshot,
    DROP COLUMN material_id,
    DROP COLUMN material_name_snapshot,
    DROP COLUMN pricing_type_snapshot,
    DROP COLUMN width_cm,
    DROP COLUMN height_cm,
    DROP COLUMN quantity,
    DROP COLUMN unit_price,
    DROP COLUMN design_brief;

-- ----------------------------------------------------------------------------
-- 5) orders.design_source TETAP ADA, tapi sekarang bisa 'mixed' juga —
--    ditulis ulang oleh order service dari order_items.design_source setiap
--    kali daftar item berubah (§32.1). Perluas CHECK constraint-nya.
-- ----------------------------------------------------------------------------
ALTER TABLE orders DROP CONSTRAINT orders_design_source_check;
ALTER TABLE orders ADD CONSTRAINT orders_design_source_check
    CHECK (design_source IN ('upload', 'request', 'mixed'));
