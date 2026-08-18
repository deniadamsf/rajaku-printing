-- ============================================================================
-- 000018_site_media — modul sitemedia: gambar landing page editable dari
-- admin panel tanpa deploy ulang (bukan artikel CMS — lihat internal/sitemedia).
--
-- Registry SLOT (label, deskripsi, dimensi sarankan) hidup di kode Go
-- (internal/sitemedia/model/slot_registry.go), BUKAN di tabel — slot baru =
-- tambah konstanta di kode, tidak perlu migration. Tabel ini hanya menyimpan
-- slot yang SUDAH diisi admin (unfilled slot = tidak ada baris).
-- ============================================================================

CREATE TABLE site_media (
    slot          VARCHAR(80)  PRIMARY KEY,
    storage_path  TEXT         NOT NULL,
    original_name VARCHAR(255) NOT NULL,
    mime_type     VARCHAR(50)  NOT NULL,
    size_bytes    BIGINT       NOT NULL,
    width_px      INTEGER      NOT NULL,
    height_px     INTEGER      NOT NULL,
    uploaded_by   UUID         REFERENCES users(id) ON DELETE SET NULL,
    uploaded_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE site_media IS
    'Gambar landing page yang diganti admin (§ modul sitemedia). Registry slot ada di kode Go, bukan di sini.';

-- ----------------------------------------------------------------------------
-- Permission baru — hanya super_admin yang boleh kelola media landing page
-- (mengikuti pola settings.manage di migration 000011).
-- ----------------------------------------------------------------------------
INSERT INTO menu_permissions (code, display_name, category, description) VALUES
    ('sitemedia.manage', 'Kelola gambar landing page', 'admin', 'Ganti/hapus gambar landing page (hero, logo, langkah proses, OG image) tanpa deploy ulang');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, menu_permissions p
WHERE r.name = 'super_admin'
  AND p.code = 'sitemedia.manage';
