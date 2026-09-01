-- ============================================================================
-- 000034_design_file_order_item — rollback.
--
-- LOSSY untuk order multi-item: menjatuhkan kolom order_item_id membuang
-- informasi "file ini milik baris item yang mana". Untuk order 1 item ini
-- tidak lossy (satu-satunya kemungkinan tetap line_no=1, persis backfill
-- migration up). Kalau migration ini di-up lagi setelah down pada database
-- yang sudah punya order multi-item, backfill-nya akan salah menaruh SEMUA
-- file order itu ke line_no=1 — bukan mengembalikan pemetaan aslinya, karena
-- pemetaan itu sudah hilang begitu kolomnya di-drop.
-- ============================================================================

DROP INDEX IF EXISTS idx_design_files_order_item;
ALTER TABLE design_files DROP CONSTRAINT IF EXISTS design_files_order_item_id_fkey;
ALTER TABLE design_files DROP COLUMN IF EXISTS order_item_id;
