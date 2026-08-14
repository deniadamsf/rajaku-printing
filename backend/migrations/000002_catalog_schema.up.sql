-- ============================================================================
-- 000002_catalog_schema — modul catalog (spec section 9)
-- Tabel: materials, products, product_pricings (fleksibel per_m2 / paket)
-- Seed:  4 materials umum + 2 sample products (banner_flexi per_m2, x_banner paket)
-- ============================================================================

-- ============================================================================
-- materials — shared library, dipakai lintas product
-- ============================================================================
CREATE TABLE materials (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code        VARCHAR(50)  UNIQUE NOT NULL,
    name        VARCHAR(150) NOT NULL,
    description TEXT,
    is_active   BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_materials_is_active ON materials(is_active);

-- ============================================================================
-- products — banner varieties dengan pricing_type dipilih per produk
-- ============================================================================
CREATE TABLE products (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    slug          VARCHAR(150) UNIQUE NOT NULL,
    name          VARCHAR(200) NOT NULL,
    description   TEXT,
    category      VARCHAR(50)  NOT NULL,           -- 'banner' | 'x-banner' | 'roll-up' | 'flag' | dll
    pricing_type  VARCHAR(20)  NOT NULL CHECK (pricing_type IN ('per_m2', 'paket')),
    min_width_cm  INT,
    min_height_cm INT,
    max_width_cm  INT,
    max_height_cm INT,
    is_active     BOOLEAN      NOT NULL DEFAULT TRUE,
    display_order INT          NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    CONSTRAINT products_size_range_sane CHECK (
        (min_width_cm  IS NULL OR max_width_cm  IS NULL OR min_width_cm  <= max_width_cm) AND
        (min_height_cm IS NULL OR max_height_cm IS NULL OR min_height_cm <= max_height_cm)
    )
);

CREATE INDEX idx_products_category    ON products(category);
CREATE INDEX idx_products_is_active   ON products(is_active);
CREATE INDEX idx_products_display     ON products(display_order);

-- ============================================================================
-- product_pricings — 1 baris per (product, material)-atau-(product, material, size)
--
-- Schema fleksibel:
--   - Untuk product dgn pricing_type='per_m2': isi price_per_m2 + min_charge_m2 (opsional).
--     Kolom paket (width_cm, height_cm, price_total) HARUS NULL.
--   - Untuk product dgn pricing_type='paket': isi width_cm, height_cm, price_total,
--     opsional package_label. Kolom price_per_m2 HARUS NULL.
--
-- Validasi konsistensi antara pricing_type parent product & baris ini dilakukan
-- di service layer (CHECK di sini enforce salah satu set kolom terisi).
-- Harga disimpan sebagai BIGINT (IDR integer — Rupiah tidak pakai desimal di praktik).
-- ============================================================================
CREATE TABLE product_pricings (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id    UUID         NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    material_id   UUID         NOT NULL REFERENCES materials(id) ON DELETE RESTRICT,

    -- Untuk pricing_type='per_m2':
    price_per_m2  BIGINT,                              -- IDR / m²
    min_charge_m2 NUMERIC(10,4),                       -- charge minimum area (mis. 1.0 = min 1 m²)

    -- Untuk pricing_type='paket':
    width_cm      INT,
    height_cm     INT,
    package_label VARCHAR(100),                        -- 'Standard 60x160'
    price_total   BIGINT,

    is_active     BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    CONSTRAINT pricing_shape_exclusive CHECK (
        (price_per_m2 IS NOT NULL
            AND width_cm IS NULL AND height_cm IS NULL AND price_total IS NULL)
        OR
        (price_per_m2 IS NULL
            AND width_cm IS NOT NULL AND height_cm IS NOT NULL AND price_total IS NOT NULL)
    ),
    CONSTRAINT pricing_positive_prices CHECK (
        (price_per_m2 IS NULL OR price_per_m2 > 0) AND
        (price_total  IS NULL OR price_total  > 0)
    )
);

CREATE INDEX idx_product_pricings_product  ON product_pricings(product_id);
CREATE INDEX idx_product_pricings_material ON product_pricings(material_id);

-- Uniqueness: 1 baris per_m2 per (product, material), atau 1 baris paket per (product, material, size).
CREATE UNIQUE INDEX idx_pricing_per_m2_uniq
    ON product_pricings(product_id, material_id)
    WHERE price_per_m2 IS NOT NULL;

CREATE UNIQUE INDEX idx_pricing_paket_uniq
    ON product_pricings(product_id, material_id, width_cm, height_cm)
    WHERE width_cm IS NOT NULL;

-- ============================================================================
-- Seed materials
-- ============================================================================
INSERT INTO materials (code, name, description) VALUES
    ('flexi_280',   'Flexi 280 gsm',   'Kain flexi outdoor 280 gsm — pilihan populer untuk banner harian.'),
    ('flexi_340',   'Flexi 340 gsm',   'Kain flexi outdoor 340 gsm — lebih tebal, lebih awet.'),
    ('vinyl_solvent','Vinyl Solvent',  'Vinyl adhesive untuk X-banner / roll-up / stiker outdoor.'),
    ('backlite',    'Backlite Film',   'Bahan tembus cahaya untuk lightbox / display back-lit.');

-- ============================================================================
-- Seed sample products + pricings
-- ============================================================================
-- Banner Flexi — per_m2, custom size
WITH p AS (
    INSERT INTO products (slug, name, description, category, pricing_type,
                          min_width_cm, min_height_cm, max_width_cm, max_height_cm,
                          display_order)
    VALUES (
        'banner-flexi',
        'Banner Flexi (Custom Size)',
        'Cetak banner ukuran bebas menggunakan bahan flexi. Cocok untuk promo, spanduk event, backdrop.',
        'banner',
        'per_m2',
        30, 30, 300, 1000,
        10
    )
    RETURNING id
),
mflexi280 AS (SELECT id FROM materials WHERE code = 'flexi_280'),
mflexi340 AS (SELECT id FROM materials WHERE code = 'flexi_340')
INSERT INTO product_pricings (product_id, material_id, price_per_m2, min_charge_m2)
SELECT p.id, mflexi280.id, 25000, 1.0000 FROM p, mflexi280
UNION ALL
SELECT p.id, mflexi340.id, 32000, 1.0000 FROM p, mflexi340;

-- X-Banner — paket (ukuran fix)
WITH p AS (
    INSERT INTO products (slug, name, description, category, pricing_type, display_order)
    VALUES (
        'x-banner',
        'X-Banner (Paket)',
        'X-banner ukuran standar, sudah termasuk stand X-banner. Cocok untuk display di booth / event.',
        'x-banner',
        'paket',
        20
    )
    RETURNING id
),
mvinyl AS (SELECT id FROM materials WHERE code = 'vinyl_solvent')
INSERT INTO product_pricings
    (product_id, material_id, width_cm, height_cm, package_label, price_total)
SELECT p.id, mvinyl.id,  60, 160, 'Standard 60x160', 85000  FROM p, mvinyl
UNION ALL
SELECT p.id, mvinyl.id,  80, 180, 'Large 80x180',    120000 FROM p, mvinyl;
