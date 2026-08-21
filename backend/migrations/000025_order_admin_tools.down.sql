-- 000025_order_admin_tools — reverse

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM menu_permissions
    WHERE code IN ('order.edit', 'order.override_status', 'order.delete', 'audit.view')
);
DELETE FROM menu_permissions
WHERE code IN ('order.edit', 'order.override_status', 'order.delete', 'audit.view');

DROP TABLE IF EXISTS admin_audit_log;

DROP INDEX IF EXISTS idx_orders_deleted_at;
ALTER TABLE orders
    DROP COLUMN IF EXISTS delete_reason,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at;
