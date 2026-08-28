-- ============================================================================
-- 000030_membership_module — modul Membership customer (§30 CLAUDE.md).
--
-- Fitur member untuk customer, modular — bisa dimatikan total lewat satu
-- setting (`membership_enabled`, default false) tanpa mengubah data yang
-- sudah ada.
--
-- 1) Kolom status membership ditambahkan LANGSUNG di `users` (bukan tabel
--    terpisah) — status member adalah state eksplisit yang cuma berubah lewat
--    satu jalur aksi admin bertahap (§30.2), beda dari kasus kuota diskon
--    (§28.4) yang sengaja TIDAK disimpan sebagai counter karena rawan banyak
--    jalur lupa update. `users` di sini karena tabel "customer" secara fisik
--    diimplementasikan sebagai `users` (user_type='customer') — lihat
--    internal/auth/model/user.go.
--
-- 2) `membership_status_logs` — tabel log MURNI untuk audit trail, BUKAN
--    sumber kebenaran status (`users.membership_status` itu sumber
--    kebenarannya). Tidak pernah dibaca untuk keputusan bisnis, hanya untuk
--    telusur balik histori transisi.
--
-- 3) Setting `membership_enabled` (boolean, app_settings) — saklar on/off.
--    Saat false: halaman "Ajukan jadi Member" disembunyikan DAN service
--    menolak POST /account/membership/apply (§30.1 — UI-only hiding tanpa
--    validasi service dilarang keras, pola yang sama berulang di §22/§28.9).
--
-- 4) Permission `membership.manage` (approve/reject/revoke) & `membership.view`
--    (read-only) — default super_admin, toggleable lewat "Kelola Role" (§10).
-- ============================================================================

-- ----------------------------------------------------------------------------
-- 1) users — kolom status membership (§30.2)
-- ----------------------------------------------------------------------------
ALTER TABLE users
    ADD COLUMN membership_status         VARCHAR(20)  NOT NULL DEFAULT 'none',
    ADD COLUMN membership_requested_at   TIMESTAMPTZ  NULL,
    ADD COLUMN membership_decided_at     TIMESTAMPTZ  NULL,
    ADD COLUMN membership_decided_by     UUID         NULL REFERENCES users(id),
    ADD COLUMN membership_decision_note  TEXT         NULL;

ALTER TABLE users ADD CONSTRAINT chk_users_membership_status
    CHECK (membership_status IN ('none', 'pending', 'active', 'rejected', 'revoked'));

-- Partial index — dipakai GET /admin/membership (daftar member/pengajuan,
-- §30.2), yang SELALU menyaring baris membership_status <> 'none' (customer
-- yang belum pernah menyentuh fitur ini tidak relevan untuk daftar itu).
CREATE INDEX idx_users_membership_status ON users (membership_status)
    WHERE membership_status <> 'none';

-- ----------------------------------------------------------------------------
-- 2) membership_status_logs — audit trail murni (§30.2)
-- ----------------------------------------------------------------------------
CREATE TABLE membership_status_logs (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id  UUID         NOT NULL REFERENCES users(id),
    from_status  VARCHAR(20)  NOT NULL,
    to_status    VARCHAR(20)  NOT NULL,
    changed_by   UUID         NULL REFERENCES users(id), -- NULL = customer sendiri (mis. Apply)
    note         TEXT         NULL,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_membership_logs_from_status
        CHECK (from_status IN ('none', 'pending', 'active', 'rejected', 'revoked')),
    CONSTRAINT chk_membership_logs_to_status
        CHECK (to_status IN ('none', 'pending', 'active', 'rejected', 'revoked'))
);

CREATE INDEX idx_membership_status_logs_customer_id ON membership_status_logs (customer_id);

-- ----------------------------------------------------------------------------
-- 3) setting membership_enabled — saklar on/off (§30.1)
-- ----------------------------------------------------------------------------
INSERT INTO app_settings (key, value, display_name, description) VALUES
    (
        'membership_enabled',
        'false',
        'Aktifkan fitur membership',
        'Saklar on/off fitur membership customer (§30). Saat nonaktif, halaman "Ajukan jadi Member" disembunyikan dan pengajuan baru ditolak — data member yang sudah ada TIDAK ikut ter-reset.'
    )
-- ON CONFLICT DO NOTHING — sama alasannya dengan 000022: bikin migration ini
-- aman diulang tanpa menandai skema `dirty`, dan tidak menimpa nilai yang
-- mungkin sudah diubah admin.
ON CONFLICT (key) DO NOTHING;

-- ----------------------------------------------------------------------------
-- 4) permissions — membership.manage & membership.view (§30.5)
-- ----------------------------------------------------------------------------
INSERT INTO menu_permissions (code, display_name, category, description) VALUES
    ('membership.manage', 'Kelola pengajuan membership', 'membership', 'Approve/reject/revoke pengajuan membership customer'),
    ('membership.view',   'Lihat daftar membership',     'membership', 'Lihat daftar & status membership customer (read-only, mis. untuk CS)');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, menu_permissions p
WHERE r.name = 'super_admin'
  AND p.code IN ('membership.manage', 'membership.view');
