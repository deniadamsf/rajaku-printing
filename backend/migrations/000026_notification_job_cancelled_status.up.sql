-- ============================================================================
-- 000026_notification_job_cancelled_status — tambah status 'cancelled' ke
-- notification_jobs (§ super admin order tools review finding #5): saat
-- super admin soft-delete sebuah order, WA yang masih pending/failed untuk
-- order itu wajib dibatalkan, bukan tetap terkirim membawa link
-- `/lacak/<resi>` yang sekarang 404.
--
-- 'dead' TIDAK dipakai untuk kasus ini — artinya "exhausted retries, needs
-- human", beda makna dari "sengaja dibatalkan karena order-nya dihapus".
-- ============================================================================
ALTER TABLE notification_jobs DROP CONSTRAINT notification_jobs_status_check;
ALTER TABLE notification_jobs ADD CONSTRAINT notification_jobs_status_check
    CHECK (status IN ('pending','sending','sent','failed','dead','cancelled'));
