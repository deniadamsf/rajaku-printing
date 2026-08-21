-- 000026_notification_job_cancelled_status — reverse

ALTER TABLE notification_jobs DROP CONSTRAINT notification_jobs_status_check;
ALTER TABLE notification_jobs ADD CONSTRAINT notification_jobs_status_check
    CHECK (status IN ('pending','sending','sent','failed','dead'));
