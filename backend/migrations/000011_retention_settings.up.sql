-- ============================================================================
-- 000011_retention_settings — job retensi file desain (spec §19)
--
-- Melengkapi 000006 yang sudah menyiapkan kolom is_purged/purged_at tapi belum
-- punya mekanisme eksekusi. Migration ini menambah 3 hal:
--
--   1. app_settings — key/value setting global yang bisa diubah super admin
--      dari admin panel TANPA deploy ulang. Seed pertama: retensi 30 hari
--      (§19 "sebaiknya dibuat configurable, bukan hardcode").
--
--   2. design_files.retention_reminder_at — penanda reminder H-3 sudah dikirim
--      ke staff, supaya sweep berikutnya tidak spam WA internal (Baileys mudah
--      kena banned kalau kirim berulang, §13).
--
--   3. Permission `settings.manage` + grant ke super_admin.
--
-- Catatan: retensi yang dipakai runtime = nilai di app_settings; env var
-- RETENTION_DEFAULT_DAYS hanya fallback kalau row-nya hilang.
-- ============================================================================

-- ----------------------------------------------------------------------------
-- app_settings — setting global editable dari admin panel.
--
-- Sengaja key/value TEXT (bukan kolom per-setting): setting baru cukup INSERT
-- row, tidak perlu migration + deploy. Validasi tipe/range ada di service layer
-- (settings/service) karena tiap key punya aturan sendiri.
-- ----------------------------------------------------------------------------
CREATE TABLE app_settings (
    key          VARCHAR(80)  PRIMARY KEY,
    value        TEXT         NOT NULL,
    display_name VARCHAR(120) NOT NULL,
    description  TEXT,
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_by   UUID         REFERENCES users(id) ON DELETE SET NULL
);

INSERT INTO app_settings (key, value, display_name, description) VALUES
    (
        'design_retention_days',
        '30',
        'Retensi file desain (hari)',
        'Blob file desain dihapus otomatis dari disk setelah N hari sejak upload. Record DB (nama file, ukuran, tanggal, order) TETAP disimpan untuk rekap — hanya file fisiknya yang hilang (§19).'
    );

-- ----------------------------------------------------------------------------
-- Reminder H-3 (§19) — "kirim notif ke staff H-3 sebelum file dihapus jika
-- order terkait belum berstatus selesai, supaya staff sempat download manual
-- kalau butuh reprint."
-- ----------------------------------------------------------------------------
ALTER TABLE design_files ADD COLUMN retention_reminder_at TIMESTAMPTZ;

COMMENT ON COLUMN design_files.retention_reminder_at IS
    'Kapan reminder H-3 dikirim ke staff (§19). NULL = belum pernah dikirim.';

-- Kandidat reminder: belum purged & belum pernah diingatkan. Partial index
-- supaya scan-nya tetap kecil walau tabel membesar.
CREATE INDEX idx_design_files_retention_reminder
    ON design_files(uploaded_at)
    WHERE is_purged = false AND retention_reminder_at IS NULL;

-- ----------------------------------------------------------------------------
-- Permission baru — hanya super_admin yang boleh ubah setting global.
-- ----------------------------------------------------------------------------
INSERT INTO menu_permissions (code, display_name, category, description) VALUES
    ('settings.manage', 'Kelola pengaturan aplikasi', 'admin', 'Ubah setting global, mis. retensi file desain (section 19)');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, menu_permissions p
WHERE r.name = 'super_admin'
  AND p.code = 'settings.manage';
