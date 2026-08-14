-- ============================================================================
-- 000003_order_schema — modul order (spec section 4-8, 11)
-- Tabel: orders (single line item + snapshot pricing), order_state_history
-- ============================================================================

-- ============================================================================
-- orders — 1 order = 1 line item (multi-item deferred; simplify MVP)
--
-- Snapshot fields (product_name_snapshot, material_name_snapshot, dll)
-- di-freeze saat order dibuat supaya harga/nama tidak berubah kalau admin
-- ubah katalog belakangan.
-- ============================================================================
CREATE TABLE orders (
    id                       UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    resi                     VARCHAR(30)  UNIQUE NOT NULL,   -- format: RJK-XXXXXXXX (section 5)
    customer_id              UUID         NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    channel                  VARCHAR(20)  NOT NULL CHECK (channel IN ('online','pos')),
    status                   VARCHAR(50)  NOT NULL,

    -- Product snapshot (denormalized untuk immutability terhadap update katalog)
    product_id               UUID         REFERENCES products(id)  ON DELETE RESTRICT,
    product_name_snapshot    VARCHAR(255) NOT NULL,
    material_id              UUID         REFERENCES materials(id) ON DELETE RESTRICT,
    material_name_snapshot   VARCHAR(255) NOT NULL,
    pricing_type_snapshot    VARCHAR(20)  NOT NULL,
    width_cm                 INT          NOT NULL CHECK (width_cm > 0),
    height_cm                INT          NOT NULL CHECK (height_cm > 0),
    quantity                 INT          NOT NULL DEFAULT 1 CHECK (quantity > 0),
    unit_price               BIGINT       NOT NULL CHECK (unit_price >= 0),
    subtotal                 BIGINT       NOT NULL CHECK (subtotal >= 0),

    -- Fulfillment
    metode_ambil             VARCHAR(20)  NOT NULL CHECK (metode_ambil IN ('pickup','kirim')),
    shipping_cost            BIGINT       CHECK (shipping_cost IS NULL OR shipping_cost >= 0),
    shipping_address         TEXT,
    shipping_recipient_name  VARCHAR(255),
    shipping_recipient_phone VARCHAR(20),
    shipping_courier         VARCHAR(100),
    shipping_tracking_number VARCHAR(100),

    -- Payment (metode_bayar diisi customer di step verifikasi, nullable saat create)
    metode_bayar             VARCHAR(20)  CHECK (metode_bayar IS NULL
                                                 OR metode_bayar IN ('transfer','qris','cash','qris_pos')),

    -- Design flow (spec section 6)
    design_source            VARCHAR(20)  NOT NULL CHECK (design_source IN ('upload','request')),
    design_approval_mode     VARCHAR(20)  CHECK (design_approval_mode IS NULL
                                                 OR design_approval_mode IN ('instant_walkin','async_notify')),
    design_brief             TEXT,        -- brief singkat kalau design_source='request'

    -- Totals snapshot
    total                    BIGINT       NOT NULL CHECK (total >= 0),  -- subtotal + shipping_cost (kalau ada)

    -- Meta
    notes                    TEXT,
    created_by               UUID         REFERENCES users(id) ON DELETE SET NULL,  -- staff kasir untuk POS
    created_at               TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    -- Shipping fields wajib terisi kalau metode_ambil = kirim
    CONSTRAINT orders_kirim_needs_address CHECK (
        metode_ambil = 'pickup'
        OR (metode_ambil = 'kirim'
            AND shipping_address         IS NOT NULL
            AND shipping_recipient_name  IS NOT NULL
            AND shipping_recipient_phone IS NOT NULL)
    ),
    -- Pickup TIDAK boleh punya shipping_cost > 0
    CONSTRAINT orders_pickup_no_shipping_cost CHECK (
        metode_ambil <> 'pickup'
        OR shipping_cost IS NULL
        OR shipping_cost = 0
    )
);

CREATE INDEX idx_orders_customer  ON orders(customer_id);
CREATE INDEX idx_orders_status    ON orders(status);
CREATE INDEX idx_orders_channel   ON orders(channel);
CREATE INDEX idx_orders_created   ON orders(created_at DESC);

-- ============================================================================
-- order_state_history — audit trail transisi status
-- Setiap perubahan status dicatat: siapa yg trigger, kapan, dari-ke, notes.
-- Berguna untuk /lacak/:resi (history status) & compliance/audit.
-- ============================================================================
CREATE TABLE order_state_history (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id     UUID         NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    from_status  VARCHAR(50),                              -- NULL untuk initial state
    to_status    VARCHAR(50)  NOT NULL,
    changed_by   UUID         REFERENCES users(id) ON DELETE SET NULL,  -- NULL untuk system-triggered
    changed_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    note         TEXT
);

CREATE INDEX idx_order_history_order ON order_state_history(order_id, changed_at DESC);
