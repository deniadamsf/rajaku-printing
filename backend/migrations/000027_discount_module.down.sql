-- 000027_discount_module — reverse

-- 3) permissions
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM menu_permissions
    WHERE code IN ('discount.manage', 'discount.apply', 'report.view')
);
DELETE FROM menu_permissions
WHERE code IN ('discount.manage', 'discount.apply', 'report.view');

-- 2) orders — drop discount snapshot columns (also drops the FK + CHECK + index)
DROP INDEX IF EXISTS idx_orders_discount_id;
ALTER TABLE orders DROP CONSTRAINT IF EXISTS chk_orders_discount_type_snapshot;
ALTER TABLE orders DROP CONSTRAINT IF EXISTS chk_orders_discount_amount;
ALTER TABLE orders
    DROP COLUMN IF EXISTS discount_note,
    DROP COLUMN IF EXISTS discount_amount,
    DROP COLUMN IF EXISTS discount_value_snapshot,
    DROP COLUMN IF EXISTS discount_type_snapshot,
    DROP COLUMN IF EXISTS discount_name_snapshot,
    DROP COLUMN IF EXISTS discount_code_snapshot,
    DROP COLUMN IF EXISTS discount_id;

-- 1) discounts
DROP INDEX IF EXISTS idx_discounts_active_channel;
DROP INDEX IF EXISTS idx_discounts_deleted_at;
DROP INDEX IF EXISTS idx_discounts_code_active;
DROP TABLE IF EXISTS discounts;
