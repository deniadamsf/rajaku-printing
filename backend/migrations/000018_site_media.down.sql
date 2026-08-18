-- 000018_site_media — reverse
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM menu_permissions WHERE code = 'sitemedia.manage');
DELETE FROM menu_permissions WHERE code = 'sitemedia.manage';

DROP TABLE IF EXISTS site_media;
