-- 000032_customer_management — reverse

-- 3) customer_admin_logs
DROP INDEX IF EXISTS idx_customer_admin_logs_customer_id;
DROP TABLE IF EXISTS customer_admin_logs;

-- 2) index parsial listing pelanggan
DROP INDEX IF EXISTS idx_users_customer_list;

-- 1) permissions
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM menu_permissions
    WHERE code IN ('customer.view', 'customer.manage')
);
DELETE FROM menu_permissions
WHERE code IN ('customer.view', 'customer.manage');
