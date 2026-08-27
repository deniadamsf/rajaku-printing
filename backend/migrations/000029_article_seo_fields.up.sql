-- ============================================================================
-- 000029_article_seo_fields — panel SEO ala Rank Math di editor artikel admin
--
-- focus_keyword / secondary_keywords ditulis manual oleh staff artikel dari
-- panel SEO editor. seo_score dihitung DI SISI KLIEN (TypeScript, lihat
-- frontend editor artikel) berdasarkan keyword tsb + judul/meta/konten;
-- backend hanya menyimpan hasil hitungnya (0-100), tidak menghitung ulang.
-- ============================================================================

ALTER TABLE articles
    ADD COLUMN focus_keyword      VARCHAR(100),
    ADD COLUMN secondary_keywords VARCHAR(300),
    ADD COLUMN seo_score          SMALLINT
        CONSTRAINT chk_articles_seo_score CHECK (seo_score BETWEEN 0 AND 100);
