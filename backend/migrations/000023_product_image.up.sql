-- ============================================================================
-- 000023_product_image — tambah kolom image_path ke tabel products (§9)
--
-- Landing page menampilkan kartu produk katalog tanpa gambar sama sekali
-- (semua produk tampil dengan ikon generik yang sama). Kolom ini menyimpan
-- PATH PENYIMPANAN RELATIF (bukan URL absolut, §2 — base URL apa pun wajib
-- dihitung dari APP_BASE_URL saat baca, tidak boleh disimpan mentah di DB)
-- gambar produk yang diunggah admin lewat
-- POST /api/v1/admin/catalog/products/:id/image (modul catalog, lihat
-- internal/catalog/service/product_image.go). File fisik disimpan di disk
-- lokal VPS via internal/pkg/filestore (§19). Path fisik memakai UUID acak
-- per unggahan (pola sama dgn internal/sitemedia) — bukan deterministik dari
-- product_id — supaya file lama bisa tetap disajikan sampai unggahan baru
-- benar-benar tersimpan & baris DB ini diperbarui (hindari kondisi blob lama
-- tertimpa sebelum update DB sukses).
--
-- URL publik gambar (GET /api/v1/catalog/product-images/:id) DIHITUNG saat
-- serialisasi response dari APP_BASE_URL + product_id, bukan disimpan di
-- kolom ini.
--
-- Nullable — produk boleh belum punya gambar (frontend fallback ke ikon
-- generik seperti sebelumnya).
-- ============================================================================

ALTER TABLE products ADD COLUMN image_path VARCHAR(500);

COMMENT ON COLUMN products.image_path IS
    'Path penyimpanan relatif (filestore, §19) gambar produk. NULL = belum diunggah admin. URL publik dihitung saat baca dari APP_BASE_URL, tidak disimpan di sini.';
