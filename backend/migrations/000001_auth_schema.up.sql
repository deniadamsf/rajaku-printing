-- ============================================================================
-- 000001_auth_schema — modul auth (spec section 3, 10)
-- Tabel: users (unified customer + staff), roles, menu_permissions,
--        role_permissions, user_roles
-- Seed:  default roles + core permissions
-- ============================================================================

CREATE EXTENSION IF NOT EXISTS "pgcrypto"; -- untuk gen_random_uuid()

-- ============================================================================
-- users
-- ============================================================================
-- Satu tabel untuk semua identitas (customer + staff). Diambil dari section 3
-- (unified `customer` table lintas channel) & section 10 (staff account modular).
-- Nomor WA `phone` = matching key global (section 11), format 62xxx (section 13).
CREATE TABLE users (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email              VARCHAR(255) UNIQUE,
    phone              VARCHAR(20)  UNIQUE NOT NULL,
    name               VARCHAR(255) NOT NULL,
    password_hash      VARCHAR(255),
    user_type          VARCHAR(20)  NOT NULL CHECK (user_type IN ('customer','staff')),
    customer_type      VARCHAR(20)           CHECK (customer_type IN ('guest','registered')),
    oauth_provider     VARCHAR(20),
    oauth_subject      VARCHAR(255),
    is_active          BOOLEAN      NOT NULL DEFAULT TRUE,
    last_login_at      TIMESTAMPTZ,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    -- Konsistensi antar-tipe user:
    CONSTRAINT users_customer_type_only_for_customer CHECK (
        (user_type = 'customer' AND customer_type IS NOT NULL) OR
        (user_type = 'staff'    AND customer_type IS NULL)
    ),
    -- Staff wajib punya email (login pakai email), customer registered juga wajib.
    CONSTRAINT users_email_required CHECK (
        (user_type = 'customer' AND customer_type = 'guest') OR email IS NOT NULL
    ),
    -- Password wajib untuk staff & registered customer; guest & OAuth boleh null.
    CONSTRAINT users_password_or_oauth_or_guest CHECK (
        password_hash IS NOT NULL
        OR oauth_provider IS NOT NULL
        OR (user_type = 'customer' AND customer_type = 'guest')
    )
);

CREATE INDEX idx_users_user_type ON users(user_type);
CREATE UNIQUE INDEX idx_users_oauth ON users(oauth_provider, oauth_subject)
    WHERE oauth_provider IS NOT NULL;

-- ============================================================================
-- roles (modular, bukan enum — spec section 10)
-- ============================================================================
CREATE TABLE roles (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         VARCHAR(50)  UNIQUE NOT NULL,
    display_name VARCHAR(100) NOT NULL,
    description  TEXT,
    is_system    BOOLEAN      NOT NULL DEFAULT FALSE,  -- system roles tidak boleh dihapus/di-rename
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- ============================================================================
-- menu_permissions (kode permission granular — toggle-able per role)
-- ============================================================================
CREATE TABLE menu_permissions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code         VARCHAR(100) UNIQUE NOT NULL, -- ex: 'payment.verify', 'order.update_status'
    display_name VARCHAR(200) NOT NULL,
    category     VARCHAR(50)  NOT NULL,        -- ex: 'payment', 'order', 'design', 'production'
    description  TEXT,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_menu_permissions_category ON menu_permissions(category);

-- ============================================================================
-- role_permissions (M:N)
-- ============================================================================
CREATE TABLE role_permissions (
    role_id       UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES menu_permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

CREATE INDEX idx_role_permissions_permission ON role_permissions(permission_id);

-- ============================================================================
-- user_roles (M:N — 1 user bisa punya banyak role; staff punya 1+, customer 0)
-- ============================================================================
CREATE TABLE user_roles (
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id     UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    assigned_by UUID          REFERENCES users(id) ON DELETE SET NULL,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, role_id)
);

CREATE INDEX idx_user_roles_role ON user_roles(role_id);

-- ============================================================================
-- Seed: default roles (system-locked kecuali super_admin bisa non-system? Tidak — super_admin ikut is_system=true)
-- ============================================================================
INSERT INTO roles (name, display_name, description, is_system) VALUES
    ('super_admin',      'Super Admin',            'Akses penuh, kelola staff & role',                                     TRUE),
    ('payment_verifier', 'Verifikasi Pembayaran', 'Lihat bukti transfer, approve/reject pembayaran (section 10)',         TRUE),
    ('production_staff', 'Staff Produksi',        'Update status cetak/QC (section 10)',                                  TRUE),
    ('designer',         'Staff Desain',          'Kerjakan request desain, approve upload desain customer (section 10)', TRUE),
    ('article_admin',    'Admin Artikel',         'Tulis konten SEO, auto-webp (section 10, 14)',                         TRUE),
    ('cashier',          'Kasir/POS',             'Buat order walk-in, terima cash/QRIS, cetak struk (section 10, 11)',   TRUE);

-- ============================================================================
-- Seed: core menu_permissions
-- ============================================================================
INSERT INTO menu_permissions (code, display_name, category, description) VALUES
    -- order
    ('order.view',              'Lihat daftar order',           'order',     NULL),
    ('order.update_status',     'Update status order',          'order',     'Advance state machine (section 4)'),
    ('order.cancel',            'Batalkan order',               'order',     NULL),
    -- payment
    ('payment.verify',          'Verifikasi pembayaran',        'payment',   'Approve bukti transfer'),
    ('payment.reject',          'Tolak pembayaran',             'payment',   'Reject bukti, minta upload ulang'),
    -- shipping
    ('shipping.set_cost',       'Input ongkir manual',          'shipping',  NULL),
    -- design
    ('design.upload',           'Upload file desain',           'design',    'Upload file customer atau hasil edit staff'),
    ('design.approve',          'Approve desain customer',      'design',   'Verifikasi upload desain (section 6)'),
    ('design.work',             'Kerjakan request desain',      'design',   'Untuk staff desain, ambil task request desain (section 6)'),
    -- production
    ('production.update',       'Update status cetak/QC',       'production', NULL),
    -- catalog
    ('catalog.view',            'Lihat katalog produk',         'catalog',   NULL),
    ('catalog.manage',          'Kelola produk/harga',          'catalog',   'CRUD produk, bahan, pricing_rule (section 9)'),
    -- cms
    ('article.view',            'Lihat artikel',                'cms',       NULL),
    ('article.create',          'Buat artikel',                 'cms',       NULL),
    ('article.publish',         'Publikasi artikel',            'cms',       NULL),
    -- pos
    ('pos.create_order',        'Buat order walk-in (POS)',     'pos',       NULL),
    ('pos.reconcile',           'Lihat rekonsiliasi harian',    'pos',       'Rekap kasir (section 11)'),
    -- admin
    ('staff.manage',            'Kelola akun staff',            'admin',     'Invite staff, assign role (super admin only, section 10)'),
    ('role.manage',             'Kelola role & permission',     'admin',     'Toggle permission per role (super admin only)');

-- ============================================================================
-- Seed: assign permissions per role
-- ============================================================================
-- super_admin: SEMUA permission
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN menu_permissions p
WHERE r.name = 'super_admin';

-- payment_verifier
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, menu_permissions p
WHERE r.name = 'payment_verifier'
  AND p.code IN ('payment.verify','payment.reject','order.view');

-- production_staff
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, menu_permissions p
WHERE r.name = 'production_staff'
  AND p.code IN ('production.update','order.view','order.update_status');

-- designer
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, menu_permissions p
WHERE r.name = 'designer'
  AND p.code IN ('design.upload','design.approve','design.work','order.view');

-- article_admin
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, menu_permissions p
WHERE r.name = 'article_admin'
  AND p.code IN ('article.view','article.create','article.publish');

-- cashier
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, menu_permissions p
WHERE r.name = 'cashier'
  AND p.code IN ('pos.create_order','pos.reconcile','order.view','order.update_status','payment.verify','shipping.set_cost','design.upload','catalog.view');
