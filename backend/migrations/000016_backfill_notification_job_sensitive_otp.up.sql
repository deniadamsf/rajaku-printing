-- ============================================================================
-- 000016_backfill_notification_job_sensitive_otp — redacts WhatsApp OTP
-- notification_jobs that were inserted BEFORE this deployment started
-- setting is_sensitive=TRUE on them (review finding #2, follow-up to
-- 000015).
--
-- Why a separate migration instead of editing 000015: 000015 has likely
-- already been applied in every environment that's run this codebase since
-- it landed — editing an already-applied migration file doesn't get
-- re-run and only creates drift between whatever state each environment's
-- migration history table thinks it's in and what the file on disk now says.
--
-- The bug being fixed: 000015 added `is_sensitive` with DEFAULT FALSE, so
-- every notification_jobs row that existed at the moment 000015 ran —
-- including any kind='otp_verification' rows already sitting in
-- 'sent'/'dead' with their WhatsApp OTP code still in plaintext `message` —
-- got is_sensitive=FALSE. Both application-side redaction paths
-- (repository.MarkSent / MarkFailure's inline CASE WHEN, and
-- service.RedactStaleSensitiveMessages' periodic sweep) filter on
-- is_sensitive=TRUE, so these rows would NEVER get redacted: MarkSent's
-- `WHERE status = 'sending'` also no longer matches an already-'sent' row,
-- so the plaintext code would sit in the database indefinitely.
--
-- Idempotent: the WHERE clause only ever matches rows that still carry a
-- non-empty message AND is_sensitive=FALSE, so re-running this migration (or
-- accidentally running it twice) is a safe no-op the second time.
-- ============================================================================

UPDATE notification_jobs
SET message = '', is_sensitive = TRUE
WHERE kind = 'otp_verification'
  AND is_sensitive = FALSE
  AND message <> '';
