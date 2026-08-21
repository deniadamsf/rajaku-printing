-- 000024_design_skip_upload_permission — reverse
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM menu_permissions WHERE code = 'design.skip_upload');
DELETE FROM menu_permissions WHERE code = 'design.skip_upload';
