-- ============================================================================
-- 000010_staff_invite_no_password
--
-- Konteks: modul auth (§10) menyediakan staff invite flow — super admin bikin
-- akun staff dulu (inactive), sistem kirim link invite, calon staff set
-- password sendiri via /invites/accept.
--
-- Constraint lama `users_password_or_oauth_or_guest` menolak insert ini karena
-- pending-invite staff belum punya password_hash. Kita longgarkan: staff yang
-- **belum aktif** boleh tanpa credential — begitu di-activate lewat accept
-- invite (yang otomatis set password_hash), constraint terpenuhi. Login tidak
-- terancam karena bcrypt tidak match NULL hash + middleware login juga cek
-- is_active.
-- ============================================================================

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_password_or_oauth_or_guest;

ALTER TABLE users
    ADD CONSTRAINT users_password_or_oauth_or_guest CHECK (
        -- Aktif → wajib punya credential atau guest customer:
        (is_active = TRUE AND (
            password_hash IS NOT NULL
            OR oauth_provider IS NOT NULL
            OR (user_type = 'customer' AND customer_type = 'guest')
        ))
        -- Nonaktif → boleh tanpa credential (mis. staff yg belum accept invite):
        OR is_active = FALSE
    );
