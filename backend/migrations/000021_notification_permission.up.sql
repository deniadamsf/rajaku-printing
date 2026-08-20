-- ============================================================================
-- 000021_notification_permission — permission baru `notification.manage`
-- untuk endpoint pairing WhatsApp (Baileys) admin panel
-- (GET/POST /admin/whatsapp/pairing, /admin/whatsapp/unlink).
--
-- Pairing dilakukan lewat QR code — memindai QR menautkan WhatsApp siapa pun
-- yang memindai ke nomor toko dan memberi kemampuan kirim pesan atas nama
-- toko. Karena setara kredensial, akses dibatasi permission tersendiri (BUKAN
-- digabung ke permission lain), mengikuti pola sitemedia.manage di migration
-- 000018.
-- ============================================================================

INSERT INTO menu_permissions (code, display_name, category, description) VALUES
    ('notification.manage', 'Kelola pairing WhatsApp', 'admin', 'Lihat status pairing WhatsApp (Baileys), tampilkan QR pairing, dan putus pairing dari admin panel');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, menu_permissions p
WHERE r.name = 'super_admin'
  AND p.code = 'notification.manage';
