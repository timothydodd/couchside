-- Encode progress (0..1) for long-running jobs such as "optimize".
ALTER TABLE jobs ADD COLUMN progress REAL;

-- Browser-friendly H.264/AAC MP4 copies made by background optimize jobs.
CREATE TABLE optimized (
  file_id    INTEGER PRIMARY KEY REFERENCES files(id) ON DELETE CASCADE,
  path       TEXT NOT NULL,
  size       INTEGER NOT NULL,
  height     INTEGER,
  created_at INTEGER NOT NULL DEFAULT (unixepoch())
);
