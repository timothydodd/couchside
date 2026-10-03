-- Breaks someone marked "Not a commercial" in the player. Kept apart from the
-- detection result so running detection again doesn't bring them back; the
-- API hides a detected break that one of these covers.
CREATE TABLE commercial_dismissals (
  file_id    INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  start      REAL NOT NULL,
  "end"      REAL NOT NULL,
  created_at INTEGER NOT NULL DEFAULT (unixepoch()),
  PRIMARY KEY (file_id, start)
);
