-- ============================================================================
-- 000022_pos_receipt_settings — lebar kertas struk thermal POS jadi
-- configurable (§11/§12 CLAUDE.md — thermal printer 58mm/80mm)
--
-- Sebelumnya lebar kertas struk tidak ada di settings sama sekali, sehingga
-- frontend kasir tidak punya cara tahu roll printer yang terpasang tanpa
-- hardcode. Migration ini seed 1 baris ke app_settings (tabel dibuat migration
-- 000011) supaya super admin bisa ubah dari admin panel tanpa deploy:
--   - pos.receipt_width_mm   lebar kertas struk thermal dalam mm, HANYA
--                            boleh 58 atau 80 (divalidasi sebagai enum di
--                            settings/service, bukan rentang bebas) — salah
--                            nilai bikin struk kepotong/ter-scale di printer.
--
-- Nilai default = 58mm (ukuran roll thermal paling umum untuk kasir kecil).
-- ============================================================================

INSERT INTO app_settings (key, value, display_name, description) VALUES
    (
        'pos.receipt_width_mm',
        '58',
        'Lebar kertas struk thermal',
        'Lebar kertas struk thermal POS dalam mm. Hanya boleh 58 atau 80 — sesuaikan dengan roll printer kasir yang terpasang. Salah nilai bikin struk kepotong atau ter-scale tidak proporsional saat dicetak.'
    )
-- ON CONFLICT: `app_settings.key` adalah PRIMARY KEY, jadi INSERT polos GAGAL
-- kalau baris ini sudah pernah dimasukkan manual (atau setelah `migrate force`
-- pasca-kegagalan sebagian) — golang-migrate lalu menandai skema `dirty` dan
-- deploy berhenti sampai ada intervensi manual. DO NOTHING bikin migration ini
-- aman diulang, dan sengaja TIDAK menimpa nilai yang mungkin sudah diubah
-- admin lewat PUT /admin/settings/:key.
ON CONFLICT (key) DO NOTHING;
