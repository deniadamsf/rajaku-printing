-- ============================================================================
-- 000024_design_skip_upload_permission — permission baru `design.skip_upload`
-- untuk endpoint POST /admin/orders/:resi/design-skip-upload (§11 walk-in).
--
-- Endpoint itu melewati kewajiban adanya file desain di sistem sebelum order
-- POS boleh maju dari `dibayar` ke `desain_diverifikasi` — dipakai saat
-- pelanggan walk-in membawa desain siap cetak yang filenya cuma ada di
-- komputer desainer.
--
-- Dibuat sebagai permission TERSENDIRI, bukan menumpang `design.approve`,
-- supaya kasir bisa memakai tombol ini di momen transaksi tanpa sekalian
-- mendapat kuasa memverifikasi desain order ONLINE (mengikuti pola permission
-- granular §10 dan preseden notification.manage di migration 000021).
--
-- Catatan: super_admin di-grant eksplisit karena seed CROSS JOIN di migration
-- 000001 hanya berlaku untuk permission yang ada saat itu.
-- ============================================================================

INSERT INTO menu_permissions (code, display_name, category, description) VALUES
    ('design.skip_upload', 'Lewati upload desain (walk-in)', 'design', 'Loloskan order walk-in POS ke tahap cetak tanpa file desain di sistem, dengan catatan lokasi file sebagai jejak audit');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, menu_permissions p
WHERE r.name IN ('super_admin', 'designer', 'cashier')
  AND p.code = 'design.skip_upload';
