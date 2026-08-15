-- ============================================================================
-- 000013_phone_verifications — WhatsApp OTP challenge for the Google OAuth
-- registration flow.
--
-- Why: resolveUserForCompletion() used to trust the raw phone number posted
-- to POST /auth/google/complete at face value. WhatsApp numbers are not a
-- secret (they're printed on POS receipts), so anyone could type in a
-- stranger's number and, if it matched an existing guest customer, get a
-- full session issued for that guest's user_id — silently taking over their
-- order history. Every Google registration now must prove phone ownership by
-- receiving and echoing back a one-time code sent over WhatsApp.
--
-- One row per OTP challenge, tied to exactly one registration handoff code
-- (oauth_login_codes.kind='registration') via FK — a code verified for one
-- handoff can never be replayed against another handoff, even if the phone
-- number is identical.
-- ============================================================================

CREATE TABLE phone_verifications (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    oauth_login_code_id UUID NOT NULL REFERENCES oauth_login_codes(id) ON DELETE CASCADE,
    phone               VARCHAR(20)  NOT NULL,
    code_hash           CHAR(64)     NOT NULL,
    attempts            INT          NOT NULL DEFAULT 0,
    expires_at          TIMESTAMPTZ  NOT NULL,
    verified_at         TIMESTAMPTZ,
    consumed_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_phone_verifications_lookup ON phone_verifications(oauth_login_code_id, phone);
CREATE INDEX idx_phone_verifications_expires ON phone_verifications(expires_at);
