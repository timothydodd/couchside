-- Commercial breaks found by comskip. size/mtime record which version of the
-- file was analysed, so a replaced file is detected again.
CREATE TABLE commercials (
  file_id    INTEGER PRIMARY KEY REFERENCES files(id) ON DELETE CASCADE,
  size       INTEGER NOT NULL,
  mtime      INTEGER NOT NULL,
  segments   TEXT NOT NULL DEFAULT '[]', -- JSON [{"start":s,"end":s}, ...] in seconds
  created_at INTEGER NOT NULL DEFAULT (unixepoch())
);
