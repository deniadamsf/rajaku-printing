-- ============================================================================
-- 000032_customer_management — modul "Manajemen Pelanggan" (admin panel).
--
-- Pelanggan secara FISIK adalah baris `users` dengan user_type='customer'
-- (internal/auth/model/user.go) — fitur ini TIDAK punya tabel identitas
-- sendiri, jadi migration ini hanya menambah:
--
-- 1) Dua permission baru (`customer.view`, `customer.manage`) — pola sama
--    persis 000030 (membership.view/manage): default super_admin, toggleable
--    lewat "Kelola Role" (§10).
--
-- 2) Index parsial untuk listing admin (`GET /admin/customers`, sorted
--    created_at DESC, disaring user_type='customer') — tanpa index ini,
--    listing pelanggan melakukan seq scan di seluruh tabel `users` (staff +
--    customer campur) tiap kali admin membuka halamannya.
--
-- 3) `customer_admin_logs` — audit trail MURNI untuk SEMUA aksi admin di
--    fitur ini: blokir/aktifkan (action='block'/'unblock', pola sama
--    membership_status_logs, §30.2) DAN ubah data (action='profile_update',
--    temuan code review #9 — sebelumnya UpdateCustomer, yang bisa mengubah
--    nomor WA/email (matching key identitas lintas channel, §11), tidak
--    meninggalkan jejak sama sekali). Sumber kebenaran TETAP `users` sendiri
--    (is_active, name, email, phone) — tabel ini hanya jejak "siapa, kapan,
--    apa yang berubah, kenapa". TIDAK ada hard delete pelanggan di fitur ini
--    (§ aturan keamanan #5) — memblokir = is_active=false, itu saja.
-- ============================================================================

-- ----------------------------------------------------------------------------
-- 1) permissions — customer.view & customer.manage
-- ----------------------------------------------------------------------------
INSERT INTO menu_permissions (code, display_name, category, description) VALUES
    ('customer.view',   'Lihat daftar & detail pelanggan', 'customer', 'Lihat daftar, detail, dan riwayat order pelanggan'),
    ('customer.manage', 'Ubah data & blokir/aktifkan pelanggan', 'customer', 'Ubah nama/email/nomor WA pelanggan, serta blokir/aktifkan akunnya');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, menu_permissions p
WHERE r.name = 'super_admin'
  AND p.code IN ('customer.view', 'customer.manage');

-- ----------------------------------------------------------------------------
-- 2) index parsial untuk listing pelanggan
-- ----------------------------------------------------------------------------
CREATE INDEX idx_users_customer_list ON users (created_at DESC) WHERE user_type = 'customer';

-- ----------------------------------------------------------------------------
-- 3) customer_admin_logs — audit trail murni, SEMUA aksi admin di fitur ini
-- ----------------------------------------------------------------------------
CREATE TABLE customer_admin_logs (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id  UUID         NOT NULL REFERENCES users(id),
    action       VARCHAR(30)  NOT NULL CHECK (action IN ('block', 'unblock', 'profile_update')),
    -- reason — wajib diisi SERVICE (bukan DB) untuk action='block'/'unblock'
    -- (minimal 10 karakter, customer_admin_service.go). NULL untuk
    -- profile_update.
    reason       TEXT         NULL,
    -- changes — ringkasan LAMA -> BARU field yang benar-benar berubah, hanya
    -- diisi untuk action='profile_update' (mis. `nama: "Budi" -> "Budi
    -- Santoso"; wa: "62812..." -> "62813..."`). NULL untuk block/unblock.
    changes      TEXT         NULL,
    changed_by   UUID         NULL REFERENCES users(id),
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_customer_admin_logs_customer_id ON customer_admin_logs (customer_id, created_at DESC);
