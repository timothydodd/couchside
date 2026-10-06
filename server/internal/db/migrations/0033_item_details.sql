-- Details set by hand for a title (JSON: title, year, plot, genres, rated),
-- which win over what a provider says, match after match.
ALTER TABLE media_items ADD COLUMN overrides TEXT NOT NULL DEFAULT '{}';
