-- 000028_discount_product_scope — reverse

DROP INDEX IF EXISTS idx_discount_products_product_id;
DROP TABLE IF EXISTS discount_products;

ALTER TABLE discounts DROP CONSTRAINT IF EXISTS chk_discounts_applies_to;
ALTER TABLE discounts DROP COLUMN IF EXISTS applies_to;
