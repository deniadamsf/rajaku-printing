-- ============================================================================
-- 000004_payment_schema — modul payment (spec section 7)
-- Manual verification: customer upload bukti transfer/QRIS, staff approve/reject.
-- POS cash/qris di tempat tidak butuh proof (staff langsung mark dibayar).
--
-- Multi-attempt: 1 order boleh punya banyak baris payment_proof (per upload
-- attempt). Yang boleh `pending` cuma satu — enforced via partial unique index.
-- Baris rejected tetap disimpan buat audit.
-- ============================================================================
CREATE TABLE payment_proofs (
    id                  UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id            UUID         NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    metode_bayar        VARCHAR(20)  NOT NULL CHECK (metode_bayar IN ('transfer','qris')),

    -- File (disimpan di disk lokal, path relatif dari STORAGE_LOCAL_ROOT).
    file_path           TEXT         NOT NULL,
    file_original_name  VARCHAR(255) NOT NULL,
    file_size_bytes     BIGINT       NOT NULL CHECK (file_size_bytes > 0),
    file_mime_type      VARCHAR(100) NOT NULL,

    -- Optional — customer states amount they transferred (untuk bantu staff verifikasi).
    amount_claimed      BIGINT       CHECK (amount_claimed IS NULL OR amount_claimed >= 0),

    -- Upload meta.
    uploaded_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    uploaded_by         UUID         REFERENCES users(id) ON DELETE SET NULL,

    -- Status: pending → approved | rejected. Sekali approved/rejected, immutable.
    status              VARCHAR(20)  NOT NULL CHECK (status IN ('pending','approved','rejected')),
    reviewed_at         TIMESTAMPTZ,
    reviewed_by         UUID         REFERENCES users(id) ON DELETE SET NULL,
    reject_reason       TEXT,

    -- Rejected wajib punya reason (untuk feedback ke customer).
    CONSTRAINT payment_proofs_reject_needs_reason CHECK (
        status <> 'rejected' OR (reject_reason IS NOT NULL AND length(trim(reject_reason)) > 0)
    ),
    -- Approved/rejected wajib punya reviewed_at + reviewed_by (audit trail).
    CONSTRAINT payment_proofs_reviewed_fields CHECK (
        status = 'pending'
        OR (reviewed_at IS NOT NULL AND reviewed_by IS NOT NULL)
    )
);

CREATE INDEX idx_payment_proofs_order    ON payment_proofs(order_id);
CREATE INDEX idx_payment_proofs_status   ON payment_proofs(status);
CREATE INDEX idx_payment_proofs_uploaded ON payment_proofs(uploaded_at DESC);

-- Enforce: max 1 pending proof per order (customer tidak bisa upload double
-- sebelum staff verifikasi yg pertama). Rejected/approved rows tidak kena.
CREATE UNIQUE INDEX ux_payment_proofs_one_pending_per_order
    ON payment_proofs(order_id) WHERE status = 'pending';
