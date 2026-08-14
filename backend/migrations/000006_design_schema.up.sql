-- ============================================================================
-- 000006_design_schema — modul design (spec §6 + §11 + §19)
--
-- 2 flow yg didukung:
--   1. Upload desain sendiri (order.design_source = 'upload')
--      customer upload file siap-cetak → staff verifikasi → advance ke
--      desain_diverifikasi → proses_cetak.
--
--   2. Request desain (order.design_source = 'request')
--      customer upload aset (logo/foto/brief) → staff kerjakan → upload draft
--      → customer approve / minta revisi → loop → final approved → ProsesCetak.
--      Walk-in POS (§11): staff bisa skip loop approval — mark instant di tempat.
--
-- Retention §19: file dihapus dari disk 30 hari setelah upload, row TETAP
-- disimpan (audit). Kolom is_purged + purged_at + file_url dikosongkan.
-- Job cron cleanup di-implement terpisah (mark TODO di service, blm ada di MVP).
-- ============================================================================
CREATE TABLE design_files (
    id                  UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id            UUID         NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,

    -- role — apa peran file ini dlm flow.
    --   customer_upload — file siap-cetak yg diupload customer (upload path)
    --   customer_asset  — aset/brief attachment untuk request desain
    --   staff_draft     — hasil kerja staff desain per iterasi (request path)
    role                VARCHAR(30)  NOT NULL CHECK (role IN (
        'customer_upload','customer_asset','staff_draft'
    )),

    file_path           TEXT         NOT NULL,
    file_original_name  VARCHAR(255) NOT NULL,
    file_size_bytes     BIGINT       NOT NULL CHECK (file_size_bytes > 0),
    file_mime_type      VARCHAR(100) NOT NULL,
    -- true untuk pdf/jpg/png/webp (bisa preview di browser).
    -- false untuk cdr/ai (staff harus download & buka manual, §6).
    is_previewable      BOOLEAN      NOT NULL DEFAULT false,

    notes               TEXT,        -- brief singkat / catatan staff / dll

    uploaded_by         UUID         REFERENCES users(id) ON DELETE SET NULL,
    uploaded_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    -- Retention (§19) — blob dihapus after 30d; row tetap untuk audit.
    is_purged           BOOLEAN      NOT NULL DEFAULT false,
    purged_at           TIMESTAMPTZ,

    -- Approval fields — HANYA berlaku untuk role='staff_draft'.
    --   pending             — baru diupload, menunggu response customer
    --   approved            — customer setuju; ini jadi final untuk order tsb
    --   revision_requested  — customer minta revisi, staff bikin draft baru
    approval_status     VARCHAR(20)  CHECK (
        approval_status IS NULL
        OR approval_status IN ('pending','approved','revision_requested')
    ),
    reviewed_by         UUID         REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at         TIMESTAMPTZ,
    revision_notes      TEXT,        -- alasan revisi dari customer

    -- Data-integrity constraints
    CONSTRAINT design_files_purge_ts CHECK (
        is_purged = false OR purged_at IS NOT NULL
    ),
    -- approval fields hanya untuk staff_draft; customer_* wajib NULL.
    CONSTRAINT design_files_approval_role CHECK (
        (role = 'staff_draft') OR (approval_status IS NULL AND reviewed_at IS NULL AND revision_notes IS NULL)
    ),
    -- staff_draft yg sudah direview wajib carry reviewed_at + reviewed_by.
    CONSTRAINT design_files_reviewed_fields CHECK (
        approval_status IS NULL
        OR approval_status = 'pending'
        OR (reviewed_at IS NOT NULL AND reviewed_by IS NOT NULL)
    ),
    -- revision_requested wajib punya revision_notes (alasan revisi).
    CONSTRAINT design_files_revision_needs_notes CHECK (
        approval_status <> 'revision_requested'
        OR (revision_notes IS NOT NULL AND length(trim(revision_notes)) > 0)
    )
);

CREATE INDEX idx_design_files_order    ON design_files(order_id);
CREATE INDEX idx_design_files_role     ON design_files(order_id, role);
CREATE INDEX idx_design_files_uploaded ON design_files(uploaded_at DESC);

-- Retention job filter — cari file belum ke-purge & sudah > N hari.
CREATE INDEX idx_design_files_retention
    ON design_files(uploaded_at)
    WHERE is_purged = false;

-- Enforce: max 1 pending staff_draft per order (staff tidak boleh upload 2 draft
-- sebelum customer respond yang pertama; hindari kebingungan customer).
CREATE UNIQUE INDEX ux_design_files_one_pending_draft_per_order
    ON design_files(order_id)
    WHERE role = 'staff_draft' AND approval_status = 'pending';
