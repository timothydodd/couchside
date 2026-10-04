-- "My list": titles a profile saved to watch later.
CREATE TABLE profile_items (
  profile_id INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  item_id    INTEGER NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
  added_at   INTEGER NOT NULL DEFAULT (unixepoch()),
  PRIMARY KEY (profile_id, item_id)
);
CREATE INDEX profile_items_recent ON profile_items(profile_id, added_at);

-- The Roku app kept its "My Shows" as favoriteShows in the profile's prefs.
-- They start the list.
INSERT OR IGNORE INTO profile_items (profile_id, item_id)
  SELECT p.id, CAST(j.value AS INTEGER)
  FROM profiles p, json_each(CASE WHEN json_valid(p.prefs) THEN p.prefs ELSE '{}' END, '$.favoriteShows') j
  WHERE CAST(j.value AS INTEGER) IN (SELECT id FROM media_items);
