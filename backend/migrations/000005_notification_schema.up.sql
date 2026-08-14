-- ============================================================================
-- 000005_notification_schema — modul notification (spec §13)
-- Postgres jadi job queue sederhana (bukan Redis). Backend Go insert row saat
-- event terjadi; worker Node.js/Baileys polling tabel ini + kirim + update.
--
-- Kind list (extensible via VARCHAR, tidak pakai enum supaya migration-friendly):
--   ongkir_ready       — admin isi ongkir; total fix, customer diminta bayar
--   payment_verified   — bukti bayar disetujui staff
--   payment_rejected   — bukti bayar ditolak; upload ulang
--   design_needs_revision, design_approved, ready_pickup, ready_ship,
--   shipped, invoice_ready, pos_order_created (menyusul saat modul terkait dibuat)
-- ============================================================================
CREATE TABLE notification_jobs (
    id               UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    kind             VARCHAR(50)  NOT NULL,
    recipient_phone  VARCHAR(20)  NOT NULL,       -- normalized 62xxx (§13)
    message          TEXT         NOT NULL,       -- rendered saat enqueue
    payload          JSONB        NOT NULL DEFAULT '{}'::jsonb,

    -- Idempotency: kalau enqueue trigger yang sama dua kali (mis. staff
    -- klik approve 2x karena lambat), dedup_key mencegah duplicate WA.
    -- NULL = no dedup (custom/broadcast). Bentuk konvensi: "<kind>:<order_id>".
    dedup_key        VARCHAR(150) UNIQUE,

    status           VARCHAR(20)  NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending','sending','sent','failed','dead')),
    attempts         INT          NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts     INT          NOT NULL DEFAULT 5 CHECK (max_attempts > 0),
    -- Kapan job berikutnya boleh di-claim worker. Untuk retry, worker
    -- update ke NOW() + backoff.
    next_attempt_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    last_error       TEXT,
    sent_at          TIMESTAMPTZ,

    -- Cross-ref opsional ke order (untuk audit / dashboard).
    order_id         UUID         REFERENCES orders(id) ON DELETE SET NULL,

    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    -- 'sent' butuh sent_at (konsistensi audit).
    CONSTRAINT notification_jobs_sent_needs_ts CHECK (
        status <> 'sent' OR sent_at IS NOT NULL
    )
);

-- Index utama untuk polling worker: cari yang eligible di-dispatch.
-- Partial index — hemat & tetap cepat karena worker hanya baca pending/sending.
CREATE INDEX idx_notif_jobs_dispatch
    ON notification_jobs(next_attempt_at)
    WHERE status IN ('pending','sending');

CREATE INDEX idx_notif_jobs_status_created
    ON notification_jobs(status, created_at DESC);

CREATE INDEX idx_notif_jobs_order
    ON notification_jobs(order_id)
    WHERE order_id IS NOT NULL;
