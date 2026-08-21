-- ============================================================================
-- 000019_customer_merges — audit trail for guest-identity absorption during
-- phone-claim (§11 satu pelanggan satu riwayat). Absorbing a guest moves
-- ownership of financial data (orders, and their totals) into a different
-- account — the only trace of that used to be a zerolog line in
-- internal/order/service/customer_merger.go, which is gone once logs rotate.
-- This table is the durable record: which guest row got absorbed into which
-- account, when, over which phone number, and exactly which orders moved.
--
-- order_ids is a plain UUID[], deliberately NOT a child table with FKs — this
-- is a point-in-time audit snapshot that must stay exactly as recorded, not
-- a relational set that gets joined/maintained over time, and absorption is
-- a rare event, so a child table isn't warranted.
--
-- No merged_into_user_id column is added to `users` — this table is the
-- single source of truth for "who got absorbed into whom"; a second pointer
-- on `users` could drift from it.
-- ============================================================================

CREATE TABLE customer_merges (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    from_user_id UUID         NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    to_user_id   UUID         NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    phone        VARCHAR(20)  NOT NULL,
    orders_moved INTEGER      NOT NULL,
    order_ids    UUID[]       NOT NULL DEFAULT '{}',
    source       VARCHAR(30)  NOT NULL,
    merged_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE customer_merges IS
    'Jejak audit absorpsi identitas guest ke akun terdaftar saat phone-claim (§11). from_user_id = baris guest yang diserap (jadi nisan, tidak pernah dihapus), to_user_id = akun tujuan. order_ids adalah snapshot, bukan relasi live.';

CREATE INDEX idx_customer_merges_from ON customer_merges(from_user_id);
CREATE INDEX idx_customer_merges_to   ON customer_merges(to_user_id);
