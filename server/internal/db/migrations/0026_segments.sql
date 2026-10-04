-- Where a file's intro and end credits are, so the player can offer to skip
-- them. source says how it's known: 'chapters' (the file's own chapter
-- names), 'detected' (the opening shared by a season's episodes), or 'manual'
-- (an admin marked it; never replaced by the other two).
CREATE TABLE file_segments (
  file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  kind    TEXT NOT NULL CHECK (kind IN ('intro', 'credits')),
  start   REAL NOT NULL,
  "end"   REAL NOT NULL,
  source  TEXT NOT NULL,
  PRIMARY KEY (file_id, kind)
);

-- Files whose chapters have been read, as they were then (size and mtime), so
-- each is read once and again only when it changes.
CREATE TABLE segment_checks (
  file_id INTEGER PRIMARY KEY REFERENCES files(id) ON DELETE CASCADE,
  size    INTEGER NOT NULL,
  mtime   INTEGER NOT NULL
);
