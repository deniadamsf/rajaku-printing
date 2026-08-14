-- 000009_cms_schema — reverse
ALTER TABLE IF EXISTS articles DROP CONSTRAINT IF EXISTS articles_cover_image_fk;
DROP TABLE IF EXISTS article_images;
DROP TABLE IF EXISTS articles;
