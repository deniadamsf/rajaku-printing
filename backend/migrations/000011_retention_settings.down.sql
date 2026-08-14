-- 000011_retention_settings — reverse
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM menu_permissions WHERE code = 'settings.manage');
DELETE FROM menu_permissions WHERE code = 'settings.manage';

DROP INDEX IF EXISTS idx_design_files_retention_reminder;
ALTER TABLE design_files DROP COLUMN IF EXISTS retention_reminder_at;

DROP TABLE IF EXISTS app_settings;
