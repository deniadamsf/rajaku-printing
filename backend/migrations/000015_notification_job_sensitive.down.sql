DROP INDEX IF EXISTS idx_notification_jobs_sensitive_created;
ALTER TABLE notification_jobs DROP COLUMN IF EXISTS is_sensitive;
