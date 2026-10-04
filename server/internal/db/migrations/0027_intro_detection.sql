-- Find the intro a season's episodes share, for TV libraries with this on.
ALTER TABLE libraries ADD COLUMN intros INTEGER NOT NULL DEFAULT 0;

-- Episode files that have been through intro detection, as they were then,
-- so a season is compared once and again only when files change or arrive.
CREATE TABLE intro_checks (
  file_id INTEGER PRIMARY KEY REFERENCES files(id) ON DELETE CASCADE,
  size    INTEGER NOT NULL,
  mtime   INTEGER NOT NULL
);
