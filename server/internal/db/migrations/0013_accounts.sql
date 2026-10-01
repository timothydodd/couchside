-- Accounts (only used when COUCHSIDE_AUTH is on): a profile becomes a user
-- with an Argon2id password hash, a role, and whether it may record.
-- must_change_password is set by an admin's reset, so a temporary password
-- only works until the user picks their own.
ALTER TABLE profiles ADD COLUMN password_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE profiles ADD COLUMN role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('admin', 'user'));
ALTER TABLE profiles ADD COLUMN can_record INTEGER NOT NULL DEFAULT 0;
ALTER TABLE profiles ADD COLUMN disabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE profiles ADD COLUMN must_change_password INTEGER NOT NULL DEFAULT 0;

-- A signed-in device. Only a SHA-256 of the current refresh token is kept;
-- it changes on every refresh, and the old hashes go to session_used_tokens
-- so a replayed (stolen) refresh token can be recognised and the session ended.
CREATE TABLE sessions (
  id           TEXT PRIMARY KEY, -- random; carried in access tokens
  profile_id   INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  refresh_hash TEXT NOT NULL UNIQUE,
  client       TEXT NOT NULL CHECK (client IN ('web', 'tv')),
  device       TEXT NOT NULL DEFAULT '',
  user_agent   TEXT NOT NULL DEFAULT '',
  ip           TEXT NOT NULL DEFAULT '',
  created_at   INTEGER NOT NULL DEFAULT (unixepoch()),
  last_used_at INTEGER NOT NULL DEFAULT (unixepoch()),
  expires_at   INTEGER NOT NULL
);
CREATE INDEX sessions_profile ON sessions(profile_id);

CREATE TABLE session_used_tokens (
  hash       TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  used_at    INTEGER NOT NULL
);
CREATE INDEX session_used_tokens_session ON session_used_tokens(session_id);

-- Who scheduled a recording or made a series rule, so users allowed to record
-- can manage their own. NULL (anything from before accounts) is admin-only.
ALTER TABLE recordings ADD COLUMN profile_id INTEGER REFERENCES profiles(id) ON DELETE SET NULL;
ALTER TABLE series_rules ADD COLUMN profile_id INTEGER REFERENCES profiles(id) ON DELETE SET NULL;
