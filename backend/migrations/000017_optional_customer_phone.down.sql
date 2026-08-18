-- ============================================================================
-- 000017_optional_customer_phone (down)
--
-- NOTE (jujur, bukan basa-basi): rollback ini akan GAGAL kalau pada saat
-- dijalankan ada baris `users` dengan phone IS NULL (ALTER COLUMN ... SET NOT
-- NULL akan ditolak Postgres), atau ada baris `phone_verifications` dengan
-- user_id IS NOT NULL (constraint CHECK lama tidak pernah mengizinkan
-- oauth_login_code_id NULL). Itu memang konsekuensi yang benar dari fitur ini
-- — begitu ada akun tanpa nomor atau tantangan OTP milik user, downgrade ke
-- skema lama berarti kehilangan data itu; operator harus membersihkan/
-- migrasikan data itu secara sadar sebelum rollback, bukan silently truncate
-- di sini.
-- ============================================================================

DROP INDEX IF EXISTS idx_phone_verifications_user_lookup;
ALTER TABLE phone_verifications DROP CONSTRAINT IF EXISTS phone_verifications_owner_check;
ALTER TABLE phone_verifications DROP COLUMN IF EXISTS user_id;
ALTER TABLE phone_verifications ALTER COLUMN oauth_login_code_id SET NOT NULL;

ALTER TABLE users DROP COLUMN IF EXISTS phone_verified_at;
DROP INDEX IF EXISTS users_phone_unique_idx;
ALTER TABLE users ALTER COLUMN phone SET NOT NULL;
ALTER TABLE users ADD CONSTRAINT users_phone_key UNIQUE (phone);
