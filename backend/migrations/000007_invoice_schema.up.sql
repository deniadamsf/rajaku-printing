-- ============================================================================
-- 000007_invoice_schema — modul invoice (spec §12)
--
-- Satu invoice per order (unique). Nomor format INV/YYYY/NNNNN — sequential
-- per tahun, reset di 1 Januari (accounting-friendly). Counter atomic via
-- tabel invoice_counters (upsert).
--
-- Regenerasi didukung via kolom `version` (kalau ada perubahan data order
-- setelah invoice terbit, admin bisa regenerate — akan ganti pdf_path & bump
-- version). MVP: generate sekali saat payment approved.
--
-- File PDF disimpan di disk lokal (§19 storage). Retention TIDAK berlaku
-- untuk invoice (financial record, harus disimpan lama — bisa ditambah
-- kebijakan sendiri nanti).
-- ============================================================================
CREATE TABLE invoices (
    id                UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id          UUID         NOT NULL UNIQUE REFERENCES orders(id) ON DELETE RESTRICT,
    invoice_number    VARCHAR(30)  NOT NULL UNIQUE,   -- INV/2026/00042
    pdf_path          TEXT         NOT NULL,           -- relatif ke STORAGE_LOCAL_ROOT
    pdf_size_bytes    BIGINT       NOT NULL CHECK (pdf_size_bytes > 0),

    version           INT          NOT NULL DEFAULT 1 CHECK (version > 0),

    generated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    -- Untuk audit: siapa yg trigger regenerate (nil = auto/system).
    generated_by      UUID         REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX idx_invoices_generated ON invoices(generated_at DESC);

-- Atomic per-year sequence.
-- Increment via:
--   INSERT INTO invoice_counters (year, last_number) VALUES ($1, 1)
--   ON CONFLICT (year) DO UPDATE SET last_number = invoice_counters.last_number + 1
--   RETURNING last_number;
CREATE TABLE invoice_counters (
    year         INT PRIMARY KEY CHECK (year >= 2020),
    last_number  INT NOT NULL DEFAULT 0 CHECK (last_number >= 0)
);
