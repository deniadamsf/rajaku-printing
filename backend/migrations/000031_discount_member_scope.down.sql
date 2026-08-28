-- 000031_discount_member_scope — reverse

DROP INDEX IF EXISTS idx_discount_customers_customer_id;
DROP TABLE IF EXISTS discount_customers;

ALTER TABLE discounts DROP CONSTRAINT IF EXISTS chk_discounts_member_scope_consistency;
ALTER TABLE discounts DROP CONSTRAINT IF EXISTS chk_discounts_member_scope;
ALTER TABLE discounts DROP CONSTRAINT IF EXISTS chk_discounts_audience_scope;
ALTER TABLE discounts DROP COLUMN IF EXISTS member_scope;
ALTER TABLE discounts DROP COLUMN IF EXISTS audience_scope;
