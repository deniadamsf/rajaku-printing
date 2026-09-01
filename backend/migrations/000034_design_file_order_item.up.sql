-- ============================================================================
-- 000034_design_file_order_item — file desain menempel ke BARIS ITEM, bukan
-- ke order (§32.5).
--
-- Sejak migration 000033, satu order bisa punya banyak baris `order_items`
-- (mis. banner A minta desain dibuatkan, banner B bawa desain sendiri).
-- design_files.order_id saja tidak cukup lagi untuk tahu file itu punya
-- banner yang mana kalau ukurannya mirip — dan validasi
-- `ErrDesignSourceMismatch` yang tadinya membaca orders.design_source jadi
-- salah pada order campuran (orders.design_source sekarang bisa 'mixed',
-- §32.1). Migration ini menambatkan setiap file ke order_items.id.
--
-- Urutan wajib sama pola dengan 000033: (1) tambah kolom nullable dulu,
-- (2) BACKFILL, (3) verifikasi tidak ada baris yatim, BARU (4) kolom
-- dijadikan NOT NULL. Kalau ada baris yatim, migration GAGAL total (§22 —
-- tidak boleh diam-diam menyisakan NULL).
--
-- Backfill: setiap order lama (sebelum migration ini) dijamin cuma punya
-- satu order_items dengan line_no=1 untuk tiap order_id — order multi-item
-- baru bisa dibuat SETELAH migration 000033 selesai, jadi tidak ada
-- design_files lama yang perlu dipetakan ke line_no > 1.
-- ============================================================================

ALTER TABLE design_files ADD COLUMN order_item_id UUID;

-- Backfill: arahkan setiap file ke item satu-satunya (line_no=1) milik
-- order yang sama.
UPDATE design_files df
SET order_item_id = oi.id
FROM order_items oi
WHERE oi.order_id = df.order_id
  AND oi.line_no = 1;

-- Verifikasi tidak ada baris yatim SEBELUM kolom dijadikan NOT NULL — kalau
-- ada design_files yang order_id-nya tidak punya order_items line_no=1
-- (seharusnya mustahil setelah 000033, tapi jangan diasumsikan), migration
-- ini gagal total daripada diam-diam menyisakan NULL.
DO $$
DECLARE
    orphan_count INT;
BEGIN
    SELECT COUNT(*) INTO orphan_count FROM design_files WHERE order_item_id IS NULL;
    IF orphan_count > 0 THEN
        RAISE EXCEPTION
            '000034_design_file_order_item: % baris design_files tidak menemukan order_items line_no=1 yang cocok — backfill gagal, migration dibatalkan',
            orphan_count;
    END IF;
END $$;

ALTER TABLE design_files ALTER COLUMN order_item_id SET NOT NULL;

ALTER TABLE design_files
    ADD CONSTRAINT design_files_order_item_id_fkey
    FOREIGN KEY (order_item_id) REFERENCES order_items(id);

CREATE INDEX idx_design_files_order_item ON design_files(order_item_id);
