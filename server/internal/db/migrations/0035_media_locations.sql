-- Where media lives: folders and drives on the server, and network shares
-- (signed in to with a saved user name and DPAPI-sealed password on
-- Windows). Libraries, the DVR folder and folder browsing stay inside them.
-- COUCHSIDE_MEDIA_ROOT adds more from the environment (containers).
CREATE TABLE media_locations (
  id         INTEGER PRIMARY KEY,
  path       TEXT NOT NULL UNIQUE,
  username   TEXT NOT NULL DEFAULT '',
  secret     BLOB,
  created_at INTEGER NOT NULL DEFAULT (unixepoch())
);

-- A server already in use skips the first-run setup (your name, your media).
INSERT INTO settings (key, value)
SELECT 'setup.complete', '1'
WHERE EXISTS (SELECT 1 FROM libraries)
   OR EXISTS (SELECT 1 FROM profiles WHERE password_hash <> '')
   OR EXISTS (SELECT 1 FROM recordings)
   OR EXISTS (SELECT 1 FROM virtual_channels);
