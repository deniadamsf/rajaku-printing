-- ============================================================================
-- 000017_optional_customer_phone — nomor WA jadi opsional saat registrasi
-- Google, OTP jadi kondisional (owner decision — lihat brief keamanan).
--
-- Kenapa: setiap registrasi Google sebelumnya mengirim 1 OTP WhatsApp lewat
-- Baileys (§13), bahkan untuk nomor yang belum pernah dikenal sistem sama
-- sekali. Baileys adalah WhatsApp tidak resmi — volume kirim tinggi ke orang
-- asing berisiko banned. OTP sekarang HANYA diterbitkan saat terjadi
-- tabrakan identitas (nomor sudah dipakai baris `users` lain, termasuk guest
-- hasil POS) — nomor kosong atau nomor yang benar-benar baru langsung
-- dipakai tanpa OTP.
--
-- Dua perubahan skema:
--   1. users.phone jadi nullable. Constraint UNIQUE inline lama (dari
--      000001_auth_schema, terwujud sebagai constraint `users_phone_key`)
--      diganti PARTIAL unique index `WHERE phone IS NOT NULL` — banyak akun
--      tanpa nomor boleh koeksis, tapi begitu nomor diisi, tetap harus unik
--      (§11: nomor WA = matching key tunggal untuk baris yang punya nomor).
--      Tambah phone_verified_at: NULL = nomor diklaim tapi belum dibuktikan
--      kepemilikannya; diisi HANYA oleh jalur OTP sukses.
--   2. phone_verifications (migration 000013) diperluas: oauth_login_code_id
--      jadi nullable, tambah user_id nullable — satu baris tantangan OTP kini
--      bisa terikat ke SALAH SATU dari dua pemilik: handoff registrasi Google
--      (alur lama, tidak berubah) ATAU user yang sudah login menambah/
--      mengganti nomornya sendiri (alur baru, POST /auth/phone/*). CHECK
--      constraint menjamin persis satu dari keduanya terisi per baris.
-- ============================================================================

-- ---------------------------------------------------------------------------
-- users.phone: NOT NULL UNIQUE -> nullable + partial unique index
-- ---------------------------------------------------------------------------
ALTER TABLE users DROP CONSTRAINT users_phone_key;
ALTER TABLE users ALTER COLUMN phone DROP NOT NULL;

CREATE UNIQUE INDEX users_phone_unique_idx ON users (phone) WHERE phone IS NOT NULL;

ALTER TABLE users ADD COLUMN phone_verified_at TIMESTAMPTZ;

-- ---------------------------------------------------------------------------
-- phone_verifications: handoff-only owner -> handoff OR authenticated user
-- ---------------------------------------------------------------------------
ALTER TABLE phone_verifications ALTER COLUMN oauth_login_code_id DROP NOT NULL;
ALTER TABLE phone_verifications ADD COLUMN user_id UUID REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE phone_verifications ADD CONSTRAINT phone_verifications_owner_check
    CHECK (num_nonnulls(oauth_login_code_id, user_id) = 1);

CREATE INDEX idx_phone_verifications_user_lookup ON phone_verifications (user_id, phone);
