CREATE TABLE libraries (
  id           INTEGER PRIMARY KEY,
  name         TEXT NOT NULL,
  path         TEXT NOT NULL UNIQUE,
  kind         TEXT NOT NULL CHECK (kind IN ('movies', 'tv')),
  last_scan_at INTEGER,
  created_at   INTEGER NOT NULL DEFAULT (unixepoch())
);

-- One row per movie or series. parsed_* is what the scanner read from the
-- path and is the grouping key; title/year/etc. are overwritten by metadata.
CREATE TABLE media_items (
  id             INTEGER PRIMARY KEY,
  library_id     INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
  kind           TEXT NOT NULL CHECK (kind IN ('movie', 'series')),
  parsed_title   TEXT NOT NULL,
  parsed_year    INTEGER NOT NULL DEFAULT 0,
  title          TEXT NOT NULL,
  sort_title     TEXT NOT NULL,
  year           INTEGER,
  plot           TEXT NOT NULL DEFAULT '',
  genres         TEXT NOT NULL DEFAULT '',
  rated          TEXT NOT NULL DEFAULT '',
  rating         REAL,
  runtime_min    INTEGER,
  imdb_id        TEXT NOT NULL DEFAULT '',
  total_seasons  INTEGER,
  poster_url     TEXT NOT NULL DEFAULT '',
  has_poster     INTEGER NOT NULL DEFAULT 0,
  has_backdrop   INTEGER NOT NULL DEFAULT 0,
  match_status   TEXT NOT NULL DEFAULT 'pending' CHECK (match_status IN ('pending', 'matched', 'unmatched')),
  match_provider TEXT NOT NULL DEFAULT '',
  added_at       INTEGER NOT NULL DEFAULT (unixepoch()),
  updated_at     INTEGER NOT NULL DEFAULT (unixepoch()),
  UNIQUE (library_id, kind, parsed_title, parsed_year)
);
CREATE INDEX media_items_kind ON media_items(kind);

CREATE TABLE episodes (
  id        INTEGER PRIMARY KEY,
  series_id INTEGER NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
  season    INTEGER NOT NULL,
  episode   INTEGER NOT NULL,
  title     TEXT NOT NULL DEFAULT '',
  released  TEXT NOT NULL DEFAULT '',
  rating    REAL,
  imdb_id   TEXT NOT NULL DEFAULT '',
  UNIQUE (series_id, season, episode)
);

CREATE TABLE files (
  id              INTEGER PRIMARY KEY,
  library_id      INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
  media_item_id   INTEGER NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
  episode_id      INTEGER REFERENCES episodes(id) ON DELETE SET NULL,
  path            TEXT NOT NULL UNIQUE,
  size            INTEGER NOT NULL,
  mtime           INTEGER NOT NULL,
  duration_sec    REAL,
  container       TEXT NOT NULL DEFAULT '',
  video_codec     TEXT NOT NULL DEFAULT '',
  audio_codec     TEXT NOT NULL DEFAULT '',
  width           INTEGER,
  height          INTEGER,
  audio_tracks    INTEGER NOT NULL DEFAULT 0,
  subtitle_tracks INTEGER NOT NULL DEFAULT 0,
  has_still       INTEGER NOT NULL DEFAULT 0,
  last_seen       INTEGER NOT NULL,
  added_at        INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX files_item ON files(media_item_id);
CREATE INDEX files_library ON files(library_id, last_seen);
CREATE INDEX files_episode ON files(episode_id);

CREATE TABLE watch_state (
  file_id      INTEGER PRIMARY KEY REFERENCES files(id) ON DELETE CASCADE,
  position_sec REAL NOT NULL DEFAULT 0,
  duration_sec REAL NOT NULL DEFAULT 0,
  watched      INTEGER NOT NULL DEFAULT 0,
  updated_at   INTEGER NOT NULL DEFAULT (unixepoch())
);

-- Background work queue. ref_id points at a library, item or file depending
-- on kind. The partial unique index stops the same work being queued twice.
CREATE TABLE jobs (
  id          INTEGER PRIMARY KEY,
  kind        TEXT NOT NULL,
  ref_id      INTEGER NOT NULL,
  label       TEXT NOT NULL DEFAULT '',
  status      TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'done', 'failed')),
  attempts    INTEGER NOT NULL DEFAULT 0,
  error       TEXT NOT NULL DEFAULT '',
  created_at  INTEGER NOT NULL DEFAULT (unixepoch()),
  started_at  INTEGER,
  finished_at INTEGER
);
CREATE UNIQUE INDEX jobs_one_active ON jobs(kind, ref_id) WHERE status IN ('queued', 'running');
CREATE INDEX jobs_status ON jobs(status, id);

-- Raw provider responses, so rematching and rescans don't burn API quota.
CREATE TABLE provider_cache (
  provider   TEXT NOT NULL,
  key        TEXT NOT NULL,
  body       BLOB NOT NULL,
  fetched_at INTEGER NOT NULL DEFAULT (unixepoch()),
  PRIMARY KEY (provider, key)
);
