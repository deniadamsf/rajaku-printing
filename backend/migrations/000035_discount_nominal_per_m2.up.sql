-- ============================================================================
-- 000035_discount_nominal_per_m2 — diskon nominal per meter persegi (m²).
--
-- Menambahkan tipe diskon 'nominal_per_m2' untuk produk dengan pricing_type='per_m2'
-- (seperti banner custom), di mana nominal diskon dihitung per m² luas pesanan,
-- bukan flat nominal per order.
-- ============================================================================

ALTER TABLE discounts DROP CONSTRAINT IF EXISTS chk_discounts_type;
ALTER TABLE discounts ADD CONSTRAINT chk_discounts_type
    CHECK (type IN ('percent', 'nominal', 'nominal_per_m2'));

ALTER TABLE discounts DROP CONSTRAINT IF EXISTS chk_discounts_value_consistency;
ALTER TABLE discounts ADD CONSTRAINT chk_discounts_value_consistency CHECK (
    (type = 'percent' AND value_percent IS NOT NULL AND value_percent > 0 AND value_percent <= 100
        AND value_amount IS NULL)
    OR
    (type IN ('nominal', 'nominal_per_m2') AND value_amount IS NOT NULL AND value_amount > 0
        AND value_percent IS NULL)
);

ALTER TABLE discounts DROP CONSTRAINT IF EXISTS chk_discounts_max_discount_amount;
ALTER TABLE discounts ADD CONSTRAINT chk_discounts_max_discount_amount CHECK (
    (max_discount_amount IS NULL)
    OR (type IN ('percent', 'nominal_per_m2') AND max_discount_amount > 0)
);

ALTER TABLE orders DROP CONSTRAINT IF EXISTS chk_orders_discount_type_snapshot;
ALTER TABLE orders ADD CONSTRAINT chk_orders_discount_type_snapshot
    CHECK (discount_type_snapshot IS NULL OR discount_type_snapshot IN ('percent', 'nominal', 'manual', 'nominal_per_m2'));
