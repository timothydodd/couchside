-- Artwork the user uploaded. The artwork job and re-matches leave it alone.
ALTER TABLE media_items ADD COLUMN custom_poster INTEGER NOT NULL DEFAULT 0;
ALTER TABLE media_items ADD COLUMN custom_backdrop INTEGER NOT NULL DEFAULT 0;
