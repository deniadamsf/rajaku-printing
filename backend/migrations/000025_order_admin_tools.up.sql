-- ============================================================================
-- 000025_order_admin_tools — super admin order tools (§ super admin order
-- tools): edit data pesanan, override status ke status manapun, soft-delete
-- pesanan, dengan jejak audit terpisah dari order_state_history (yang khusus
-- mencatat transisi status normal).
--
-- 1) orders: kolom soft-delete (deleted_at/deleted_by/delete_reason) +
--    partial index supaya query "order aktif" tetap murah (index hanya
--    mencakup baris yang BELUM dihapus — mayoritas baris selamanya).
-- 2) admin_audit_log: satu baris per aksi super-admin (edit/override
--    status/hapus). `changes` JSONB berisi {"field": {"from": x, "to": y}}
--    per field yang BENAR-BENAR berubah — bukan snapshot penuh.
-- 3) 4 permission baru (order.edit, order.override_status, order.delete,
--    audit.view). Default di-grant HANYA ke super_admin di seed ini, TAPI
--    tetap permission granular biasa (§10) — boleh diberikan ke role lain
--    lewat menu "Kelola Role" di admin panel kalau memang dibutuhkan, sama
--    seperti permission lain manapun. Risiko yang perlu disadari sebelum
--    menambahkan: pemegang order.delete bisa menghapus pesanan dari
--    daftar/rekap (soft-delete — order berbayar tetap dijaga lewat guard
--    state.IsDeletable, tapi order pre-payment bisa lenyap begitu saja),
--    dan pemegang order.override_status/order.edit bisa memaksa data
--    finansial/status tanpa lewat validasi state machine normal. Beri
--    hanya ke role yang benar-benar butuh kewenangan koreksi tingkat admin.
-- ============================================================================

-- ----------------------------------------------------------------------------
-- 1) orders — soft delete
-- ----------------------------------------------------------------------------
ALTER TABLE orders
    ADD COLUMN deleted_at    TIMESTAMPTZ NULL,
    ADD COLUMN deleted_by    UUID        NULL REFERENCES users(id),
    ADD COLUMN delete_reason TEXT        NULL;

-- Partial index: hanya index baris yang masih aktif (deleted_at IS NULL) —
-- ini yang dicek di SETIAP query pembaca order (list admin, cari resi,
-- riwayat customer, rekap, tracking publik) per repository.go.
CREATE INDEX idx_orders_deleted_at ON orders (deleted_at) WHERE deleted_at IS NULL;

-- ----------------------------------------------------------------------------
-- 2) admin_audit_log
-- ----------------------------------------------------------------------------
CREATE TABLE admin_audit_log (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id UUID        NOT NULL REFERENCES users(id),
    action        VARCHAR(50) NOT NULL,   -- order.edit | order.override_status | order.delete
    entity_type   VARCHAR(50) NOT NULL,   -- 'order'
    entity_id     UUID        NOT NULL,
    entity_label  VARCHAR(50),            -- nomor resi, untuk dibaca manusia
    changes       JSONB,                  -- {"field": {"from": x, "to": y}}
    reason        TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_admin_audit_log_entity  ON admin_audit_log (entity_type, entity_id);
CREATE INDEX idx_admin_audit_log_created ON admin_audit_log (created_at DESC);

-- ----------------------------------------------------------------------------
-- 3) permissions — super_admin only
-- ----------------------------------------------------------------------------
INSERT INTO menu_permissions (code, display_name, category, description) VALUES
    ('order.edit',             'Edit data pesanan',        'order', 'Koreksi data pesanan (kontak, alamat, ongkir, total) di luar alur normal — tindakan super admin'),
    ('order.override_status',  'Override status pesanan',  'order', 'Paksa ubah status pesanan ke status manapun, melewati validasi transisi normal — tindakan super admin'),
    ('order.delete',           'Hapus pesanan',            'order', 'Soft-delete pesanan dari sistem — data tetap tersimpan untuk rekap, hanya disembunyikan dari tampilan normal'),
    ('audit.view',             'Lihat log audit admin',    'admin', 'Lihat riwayat aksi super admin (edit/override/hapus) untuk keperluan audit');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, menu_permissions p
WHERE r.name = 'super_admin'
  AND p.code IN ('order.edit', 'order.override_status', 'order.delete', 'audit.view');
