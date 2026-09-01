-- ============================================================================
-- 000033_order_multi_item (DOWN) — LOSSY, DIBUAT SADAR BEGITU.
--
-- Mengembalikan kolom item lama ke `orders` HANYA dari baris order_items
-- dengan line_no = 1. Order yang punya lebih dari satu item KEHILANGAN item
-- ke-2 dan seterusnya secara permanen begitu down ini dijalankan — ini bukan
-- migration yang reversibel penuh, dan sengaja tidak berpura-pura begitu
-- (§32.1). Jangan jalankan down ini di database yang sudah punya order
-- multi-item tanpa backup terpisah.
-- ============================================================================

-- 1) Kembalikan CHECK design_source ke 2 nilai lama (order yang sempat
--    'mixed' akan GAGAL constraint ini kalau masih ada — lihat catatan di
--    bawah, seharusnya sudah tidak ada order 'mixed' setelah langkah 4).
ALTER TABLE orders DROP CONSTRAINT orders_design_source_check;

-- 2) Tambahkan kembali kolom lama ke orders.
ALTER TABLE orders
    ADD COLUMN product_id               UUID         REFERENCES products(id)  ON DELETE RESTRICT,
    ADD COLUMN product_name_snapshot    VARCHAR(255),
    ADD COLUMN material_id              UUID         REFERENCES materials(id) ON DELETE RESTRICT,
    ADD COLUMN material_name_snapshot   VARCHAR(255),
    ADD COLUMN pricing_type_snapshot    VARCHAR(20),
    ADD COLUMN width_cm                 INT,
    ADD COLUMN height_cm                INT,
    ADD COLUMN quantity                 INT,
    ADD COLUMN unit_price               BIGINT,
    ADD COLUMN design_brief             TEXT;

-- 3) Salin balik dari line_no = 1 SAJA (lossy untuk item ke-2+ — lihat
--    header file ini).
UPDATE orders o
SET product_id             = oi.product_id,
    product_name_snapshot  = oi.product_name_snapshot,
    material_id            = oi.material_id,
    material_name_snapshot = oi.material_name_snapshot,
    pricing_type_snapshot  = oi.pricing_type_snapshot,
    width_cm               = oi.width_cm,
    height_cm              = oi.height_cm,
    quantity                = oi.quantity,
    unit_price             = oi.unit_price,
    design_brief           = oi.design_brief
FROM order_items oi
WHERE oi.order_id = o.id
  AND oi.line_no = 1;

-- 4) Order yang tersisa tanpa line_no=1 (seharusnya tidak ada — setiap order
--    selalu punya line_no 1) akan punya kolom NULL di sini; itu di luar
--    kontrak down ini. design_source order dipaksa balik ke 'upload' kalau
--    sempat 'mixed', supaya CHECK 2-nilai di langkah 5 tidak gagal —
--    informasi 'mixed'-nya memang hilang, konsisten dengan sifat lossy file
--    ini.
UPDATE orders SET design_source = 'upload' WHERE design_source = 'mixed';

-- 5) Kolom yang baru ditambah wajib NOT NULL sesuai skema asli (000003).
ALTER TABLE orders
    ALTER COLUMN product_name_snapshot  SET NOT NULL,
    ALTER COLUMN material_name_snapshot SET NOT NULL,
    ALTER COLUMN pricing_type_snapshot  SET NOT NULL,
    ALTER COLUMN width_cm               SET NOT NULL,
    ALTER COLUMN height_cm              SET NOT NULL,
    ALTER COLUMN quantity               SET NOT NULL,
    ALTER COLUMN unit_price             SET NOT NULL;

ALTER TABLE orders
    ALTER COLUMN quantity SET DEFAULT 1;

ALTER TABLE orders ADD CONSTRAINT orders_width_cm_check CHECK (width_cm > 0);
ALTER TABLE orders ADD CONSTRAINT orders_height_cm_check CHECK (height_cm > 0);
ALTER TABLE orders ADD CONSTRAINT orders_quantity_check CHECK (quantity > 0);
ALTER TABLE orders ADD CONSTRAINT orders_unit_price_check CHECK (unit_price >= 0);

ALTER TABLE orders ADD CONSTRAINT orders_design_source_check
    CHECK (design_source IN ('upload', 'request'));

-- 6) Drop order_items.
DROP TABLE order_items;
