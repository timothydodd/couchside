-- A TV app's refresh token from before its last refresh. It stays usable
-- until the newer one is used, so an app that never got the answer to a
-- refresh (a timeout) can ask again instead of losing its session. Browsers
-- don't need it: the new cookie is already in the jar.
ALTER TABLE sessions ADD COLUMN prev_refresh_hash TEXT NOT NULL DEFAULT '';
