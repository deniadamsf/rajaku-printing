-- ============================================================================
-- 000008_staff_invites — modul auth extension: undangan staff (§10)
--
-- Super admin bikin staff → user row created inactive (is_active=false,
-- password_hash=NULL) → staff_invites row w/ token → email/WA link.
-- Staff klik link → set password → POST /invites/accept marks used_at & set
-- user active + password_hash.
--
-- Token disimpan sebagai SHA-256 hash — kalau DB bocor, raw token tidak leak
-- (attacker tidak bisa langsung pakai). Raw token cuma pernah ada di response
-- create (untuk admin forward manual atau kelak email/WA sender).
-- ============================================================================
CREATE TABLE staff_invites (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   VARCHAR(128) NOT NULL UNIQUE, -- hex sha256 of raw token
    expires_at   TIMESTAMPTZ  NOT NULL,
    used_at      TIMESTAMPTZ,
    created_by   UUID         NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_staff_invites_user ON staff_invites(user_id);
-- Partial: worker/service cari yg masih valid (belum used & belum expired).
CREATE INDEX idx_staff_invites_eligible
    ON staff_invites(expires_at)
    WHERE used_at IS NULL;
