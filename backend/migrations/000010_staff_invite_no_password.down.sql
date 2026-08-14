-- Revert: constraint lama (menolak staff pending invite).
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_password_or_oauth_or_guest;

ALTER TABLE users
    ADD CONSTRAINT users_password_or_oauth_or_guest CHECK (
        password_hash IS NOT NULL
        OR oauth_provider IS NOT NULL
        OR (user_type = 'customer' AND customer_type = 'guest')
    );
