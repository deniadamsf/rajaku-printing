-- ============================================================================
-- 000014_phone_verification_rate_index — supports the per-phone (cross-
-- handoff) OTP rate limiting added after security review finding #2:
--   - RequestOTP: COUNT(*) WHERE phone = ? AND created_at > now() - 1h
--   - verifyOTP:  SUM(attempts) WHERE phone = ? AND created_at > now() - 1h
--
-- Both queries filter on `phone` and range-scan `created_at`, matching this
-- index's leading columns exactly.
-- ============================================================================

CREATE INDEX idx_phone_verifications_phone_created
    ON phone_verifications (phone, created_at DESC);
