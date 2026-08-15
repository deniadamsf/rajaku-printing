-- ============================================================================
-- 000012_oauth_login_codes — one-time handoff code for the Google OAuth login
-- flow (backend redirect handler → frontend SPA).
--
-- Why a handoff code instead of handing the JWT straight to the browser via
-- query string: GET /auth/google/callback is a top-level navigation the
-- browser makes directly to Google's redirect, so whatever the backend sends
-- back ends up in browser history / referrer headers / server access logs if
-- it's the access token itself. Instead the callback hands back a short-lived,
-- single-use, opaque code; the frontend immediately POSTs it to
-- /auth/google/exchange over XHR/fetch (never logged, never in history) to get
-- the real access token.
--
-- Two kinds:
--   - 'session'      — identity already resolved (existing linked account, or
--     email match against an existing customer). user_id set. TTL ~2 minutes.
--   - 'registration' — brand-new Google identity, no local phone number yet
--     (§13: phone is mandatory, WA-format 62xxx). subject/email carried so the
--     frontend can prompt for a phone number and POST it to
--     /auth/google/complete. TTL ~15 minutes (gives the user time to fill the
--     phone form).
-- ============================================================================

CREATE TABLE oauth_login_codes (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code_hash     CHAR(64)     NOT NULL UNIQUE,   -- sha256 hex dari code random
    kind          VARCHAR(20)  NOT NULL CHECK (kind IN ('session','registration')),
    user_id       UUID         REFERENCES users(id) ON DELETE CASCADE,
    provider      VARCHAR(20)  NOT NULL DEFAULT 'google',
    subject       VARCHAR(255),
    email         VARCHAR(255),
    name          VARCHAR(255),
    redirect_path TEXT,
    expires_at    TIMESTAMPTZ  NOT NULL,
    used_at       TIMESTAMPTZ,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT oauth_login_codes_kind_shape CHECK (
        (kind = 'session'      AND user_id IS NOT NULL) OR
        (kind = 'registration' AND subject IS NOT NULL AND email IS NOT NULL)
    )
);

CREATE INDEX idx_oauth_login_codes_expires ON oauth_login_codes(expires_at);
