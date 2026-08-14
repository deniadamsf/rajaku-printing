-- ============================================================================
-- 000009_cms_schema — modul CMS artikel (spec §14 + §15)
--
-- Author artikel SEO oleh role article_admin. Semua gambar diupload lewat
-- endpoint image-upload lalu auto-convert ke WebP di service (nativewebp)
-- sebelum masuk disk. Slug unik & manual-editable, auto-generate saat kosong.
--
-- Permissions (article.view/create/publish) sudah di-seed di 000001. Modul
-- ini tidak menambah permission baru — cukup pakai yang ada.
--
-- Public endpoint list & detail: hanya artikel status='published' + published_at
-- <= NOW(). Admin endpoint boleh lihat semua status.
-- ============================================================================

-- articles
CREATE TABLE articles (
    id                  UUID         PRIMARY KEY DEFAULT gen_random_uuid(),

    -- SEO-friendly slug, unik lintas artikel. Diedit admin bebas; service
    -- auto-generate dari title saat kosong.
    slug                VARCHAR(200) UNIQUE NOT NULL
                        CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),

    title               VARCHAR(200) NOT NULL,
    -- Ringkasan singkat; fallback untuk meta_description bila kosong,
    -- juga dipakai di card list artikel.
    excerpt             TEXT,
    -- Body markdown. Frontend render pakai marked/rehype; tidak render di sini.
    content_md          TEXT         NOT NULL,

    -- Cover image opsional. FK ke article_images ditambah via ALTER di akhir
    -- migration (forward reference tidak boleh dalam satu CREATE TABLE).
    -- ON DELETE SET NULL supaya penghapusan image tidak menghapus artikel.
    cover_image_id      UUID,

    -- SEO overrides (opsional). Fallback ke title & excerpt kalau kosong.
    meta_title          VARCHAR(200),
    meta_description    VARCHAR(320),

    -- Lifecycle: draft (belum tampil publik) → published (tampil, WAJIB published_at)
    --            → archived (disembunyikan tanpa hapus data).
    status              VARCHAR(20)  NOT NULL DEFAULT 'draft'
                        CHECK (status IN ('draft','published','archived')),
    -- Wajib ke-set saat status='published'. Bisa di masa depan (schedule publish).
    published_at        TIMESTAMPTZ,

    author_id           UUID         NOT NULL REFERENCES users(id) ON DELETE RESTRICT,

    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    -- Integrity: published → published_at wajib ada.
    CONSTRAINT articles_published_needs_ts CHECK (
        status <> 'published' OR published_at IS NOT NULL
    )
);

-- Listing publik (WHERE status='published' AND published_at <= NOW() ORDER BY published_at DESC).
CREATE INDEX idx_articles_public_list
    ON articles(published_at DESC)
    WHERE status = 'published';

-- Full-text-ish search di admin (belum dipakai, siapkan saja).
CREATE INDEX idx_articles_title_lower ON articles(lower(title));
CREATE INDEX idx_articles_author ON articles(author_id);


-- article_images
-- Semua gambar diupload lewat endpoint terpisah, auto-convert ke WebP
-- di service sebelum simpan. Row disimpan meski article_id masih NULL
-- (upload dulu, baru referenced di body markdown).
CREATE TABLE article_images (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    -- nullable karena user boleh upload dulu sebelum artikel dibuat. Kalau
    -- artikel dihapus (RESTRICT), gambar tetap ada (bisa dipakai artikel lain).
    article_id      UUID         REFERENCES articles(id) ON DELETE SET NULL,

    storage_path    TEXT         NOT NULL,      -- relative subpath di filestore root
    original_name   VARCHAR(255) NOT NULL,
    mime_type       VARCHAR(50)  NOT NULL,      -- selalu 'image/webp' setelah convert
    size_bytes      BIGINT       NOT NULL CHECK (size_bytes > 0),
    width_px        INTEGER      NOT NULL CHECK (width_px > 0),
    height_px       INTEGER      NOT NULL CHECK (height_px > 0),
    alt_text        VARCHAR(255),

    uploaded_by     UUID         REFERENCES users(id) ON DELETE SET NULL,
    uploaded_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_article_images_article ON article_images(article_id);


-- Deferred FK: articles.cover_image_id → article_images (dibuat di atas
-- tapi table article_images belum ada pada saat CREATE TABLE articles).
-- Postgres tidak allow forward reference dalam satu file, jadi tambahkan
-- FK constraint SETELAH kedua tabel dibuat.
ALTER TABLE articles
    ADD CONSTRAINT articles_cover_image_fk
    FOREIGN KEY (cover_image_id) REFERENCES article_images(id) ON DELETE SET NULL;
