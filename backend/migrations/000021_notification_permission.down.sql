-- 000021_notification_permission — reverse
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM menu_permissions WHERE code = 'notification.manage');
DELETE FROM menu_permissions WHERE code = 'notification.manage';
