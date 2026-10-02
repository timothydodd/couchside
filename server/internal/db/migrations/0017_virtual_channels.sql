-- Couchside's own channels, built from the library (livetv/virtual.go). Each
-- also has a row in channels (virtual_id set) so the guide, channel list and
-- players treat it like a tuner channel.
CREATE TABLE virtual_channels (
  id         INTEGER PRIMARY KEY,
  number     TEXT NOT NULL UNIQUE,
  name       TEXT NOT NULL,
  config     TEXT NOT NULL,              -- JSON: what plays, in what order, and filler
  state      TEXT NOT NULL DEFAULT '{}', -- JSON: where the schedule builder left off
  created_at INTEGER NOT NULL DEFAULT (unixepoch()),
  updated_at INTEGER NOT NULL DEFAULT (unixepoch())
);

ALTER TABLE channels ADD COLUMN virtual_id INTEGER REFERENCES virtual_channels(id) ON DELETE CASCADE;

-- The schedule, built a couple of days ahead and extended as time passes.
-- A program is one or more pieces (it's split around mid-program breaks);
-- filler pieces are commercials. Times are in milliseconds.
CREATE TABLE virtual_playout (
  id           INTEGER PRIMARY KEY,
  channel_id   INTEGER NOT NULL REFERENCES virtual_channels(id) ON DELETE CASCADE,
  start_ms     INTEGER NOT NULL,
  end_ms       INTEGER NOT NULL,
  path         TEXT NOT NULL,              -- the file to play
  in_ms        INTEGER NOT NULL DEFAULT 0, -- where in the file this piece starts
  filler       INTEGER NOT NULL DEFAULT 0,
  has_audio    INTEGER NOT NULL DEFAULT 1,
  program_at   INTEGER NOT NULL            -- start (unix seconds) of the guide program it belongs to
);
CREATE INDEX virtual_playout_time ON virtual_playout(channel_id, end_ms);

-- Lengths of filler clips, so building a schedule doesn't probe every file.
CREATE TABLE filler_clips (
  path         TEXT PRIMARY KEY,
  mtime        INTEGER NOT NULL,
  duration_ms  INTEGER NOT NULL,
  has_audio    INTEGER NOT NULL DEFAULT 1
);
