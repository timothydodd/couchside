-- Which series a guide SeriesID turned out to be, looked up on OMDb when a
-- recording is scheduled. year = 0 means it couldn't be told apart from
-- same-titled series; checked_at decides when to ask again.
CREATE TABLE series_matches (
  series_id  TEXT PRIMARY KEY,
  imdb_id    TEXT NOT NULL DEFAULT '',
  year       INTEGER NOT NULL DEFAULT 0,
  checked_at INTEGER NOT NULL DEFAULT (unixepoch())
);
