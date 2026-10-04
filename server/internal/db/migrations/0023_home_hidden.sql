-- Titles a profile removed from Continue Watching. A title stays out of the
-- row until that profile watches some of it again (activity after hidden_at).
-- Its watch state isn't touched, so the resume point is still there.
CREATE TABLE home_hidden (
  profile_id INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  item_id    INTEGER NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
  hidden_at  INTEGER NOT NULL DEFAULT (unixepoch()),
  PRIMARY KEY (profile_id, item_id)
);
