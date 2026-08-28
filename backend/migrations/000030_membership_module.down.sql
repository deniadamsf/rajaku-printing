-- 000030_membership_module — reverse

-- 4) permissions
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM menu_permissions
    WHERE code IN ('membership.manage', 'membership.view')
);
DELETE FROM menu_permissions
WHERE code IN ('membership.manage', 'membership.view');

-- 3) setting
DELETE FROM app_settings WHERE key = 'membership_enabled';

-- 2) membership_status_logs
DROP INDEX IF EXISTS idx_membership_status_logs_customer_id;
DROP TABLE IF EXISTS membership_status_logs;

-- 1) users — membership columns
DROP INDEX IF EXISTS idx_users_membership_status;
ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_users_membership_status;
ALTER TABLE users
    DROP COLUMN IF EXISTS membership_decision_note,
    DROP COLUMN IF EXISTS membership_decided_by,
    DROP COLUMN IF EXISTS membership_decided_at,
    DROP COLUMN IF EXISTS membership_requested_at,
    DROP COLUMN IF EXISTS membership_status;
