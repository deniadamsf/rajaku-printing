-- 000029_article_seo_fields — reverse
ALTER TABLE articles
    DROP COLUMN IF EXISTS focus_keyword,
    DROP COLUMN IF EXISTS secondary_keywords,
    DROP COLUMN IF EXISTS seo_score;
