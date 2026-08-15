-- ============================================================================
-- 000015_notification_job_sensitive — marks notification_jobs that carry a
-- secret in `message` at rest (currently: WhatsApp OTP codes, review finding
-- #3) so the application can redact `message` once the job reaches a
-- terminal status (sent/dead), instead of leaving a 6-digit plaintext code
-- sitting in the database indefinitely — sha256 hashing the SAME code in
-- phone_verifications.code_hash is pointless if the raw value is one query
-- away in the table next door.
--
-- Redaction paths (both application-side, see internal/notification):
--   - repository.MarkSent / MarkFailure(->dead): inline, same statement.
--   - service.RedactStaleSensitiveMessages: periodic sweep safety net for
--     jobs that never reach a terminal status (mis. worker down).
-- ============================================================================

ALTER TABLE notification_jobs
    ADD COLUMN is_sensitive BOOLEAN NOT NULL DEFAULT FALSE;

-- Partial index — only sensitive rows are ever queried by the sweep, and
-- there should always be very few of them at any given time (OTP jobs are
-- short-lived by design).
CREATE INDEX idx_notification_jobs_sensitive_created
    ON notification_jobs (created_at)
    WHERE is_sensitive;
