-- Profiles: who's watching. No passwords; they separate watch history,
-- favourite channels and preferences. prefs is a JSON object owned by the UI.
CREATE TABLE profiles (
  id         INTEGER PRIMARY KEY,
  name       TEXT NOT NULL UNIQUE COLLATE NOCASE,
  color      TEXT NOT NULL DEFAULT 'accent',
  prefs      TEXT NOT NULL DEFAULT '{}',
  created_at INTEGER NOT NULL DEFAULT (unixepoch())
);
-- Everything watched so far belongs to the first profile.
INSERT INTO profiles (id, name) VALUES (1, 'Me');

-- Watch state becomes per profile.
CREATE TABLE watch_state_new (
  profile_id   INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  file_id      INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  position_sec REAL NOT NULL DEFAULT 0,
  duration_sec REAL NOT NULL DEFAULT 0,
  watched      INTEGER NOT NULL DEFAULT 0,
  updated_at   INTEGER NOT NULL DEFAULT (unixepoch()),
  PRIMARY KEY (profile_id, file_id)
);
INSERT INTO watch_state_new (profile_id, file_id, position_sec, duration_sec, watched, updated_at)
  SELECT 1, file_id, position_sec, duration_sec, watched, updated_at FROM watch_state;
DROP TABLE watch_state;
ALTER TABLE watch_state_new RENAME TO watch_state;
CREATE INDEX watch_state_file ON watch_state(file_id);
CREATE INDEX watch_state_recent ON watch_state(profile_id, updated_at);

-- Favourite (pinned) channels become per profile.
CREATE TABLE profile_channels (
  profile_id INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  number     TEXT NOT NULL,
  PRIMARY KEY (profile_id, number)
);
INSERT INTO profile_channels (profile_id, number) SELECT 1, number FROM channels WHERE pinned = 1;
ALTER TABLE channels DROP COLUMN pinned;
