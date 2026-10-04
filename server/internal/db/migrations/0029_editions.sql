-- Which cut of a film a file is ("Extended", "Director's Cut"; '' = the
-- ordinary one), from its name. Copies that are different cuts aren't
-- duplicates.
ALTER TABLE files ADD COLUMN edition TEXT NOT NULL DEFAULT '';

-- The copy of a title a profile chose to watch (4K or 1080p, which cut).
CREATE TABLE profile_versions (
  profile_id INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  item_id    INTEGER NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
  file_id    INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  PRIMARY KEY (profile_id, item_id)
);
