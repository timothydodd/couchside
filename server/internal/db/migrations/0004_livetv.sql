-- Channel lineup from the HDHomeRun, refreshed hourly.
CREATE TABLE channels (
  number      TEXT PRIMARY KEY, -- guide number, e.g. "2.1"
  name        TEXT NOT NULL,
  affiliate   TEXT NOT NULL DEFAULT '',
  logo_url    TEXT NOT NULL DEFAULT '',
  url         TEXT NOT NULL,    -- tuner stream URL
  hd          INTEGER NOT NULL DEFAULT 0,
  drm         INTEGER NOT NULL DEFAULT 0,
  video_codec TEXT NOT NULL DEFAULT '',
  audio_codec TEXT NOT NULL DEFAULT '',
  sort_key    REAL NOT NULL DEFAULT 0,
  updated_at  INTEGER NOT NULL DEFAULT (unixepoch())
);

-- Program guide (SiliconDust's guide service), about a day ahead.
CREATE TABLE programs (
  id               INTEGER PRIMARY KEY,
  channel          TEXT NOT NULL,
  start_at         INTEGER NOT NULL,
  end_at           INTEGER NOT NULL,
  title            TEXT NOT NULL,
  episode_title    TEXT NOT NULL DEFAULT '',
  episode_num      TEXT NOT NULL DEFAULT '', -- "S25E12"
  synopsis         TEXT NOT NULL DEFAULT '',
  image_url        TEXT NOT NULL DEFAULT '',
  series_id        TEXT NOT NULL DEFAULT '',
  original_airdate INTEGER,
  is_new           INTEGER NOT NULL DEFAULT 0,
  categories       TEXT NOT NULL DEFAULT '',
  UNIQUE (channel, start_at)
);
CREATE INDEX programs_window ON programs(end_at, start_at);

-- DVR: scheduled, running and finished recordings. Program details are copied
-- in so a recording keeps its title after the guide rolls forward.
CREATE TABLE recordings (
  id            INTEGER PRIMARY KEY,
  channel       TEXT NOT NULL,
  channel_name  TEXT NOT NULL DEFAULT '',
  title         TEXT NOT NULL,
  episode_title TEXT NOT NULL DEFAULT '',
  episode_num   TEXT NOT NULL DEFAULT '',
  synopsis      TEXT NOT NULL DEFAULT '',
  image_url     TEXT NOT NULL DEFAULT '',
  series_id     TEXT NOT NULL DEFAULT '',
  categories    TEXT NOT NULL DEFAULT '',
  start_at      INTEGER NOT NULL,
  end_at        INTEGER NOT NULL,
  pad_before    INTEGER NOT NULL DEFAULT 60,  -- seconds
  pad_after     INTEGER NOT NULL DEFAULT 120,
  status        TEXT NOT NULL DEFAULT 'scheduled'
                CHECK (status IN ('scheduled', 'recording', 'completed', 'failed', 'cancelled')),
  path          TEXT NOT NULL DEFAULT '',
  size          INTEGER NOT NULL DEFAULT 0,
  error         TEXT NOT NULL DEFAULT '',
  started_at    INTEGER,
  finished_at   INTEGER,
  created_at    INTEGER NOT NULL DEFAULT (unixepoch()),
  UNIQUE (channel, start_at)
);
CREATE INDEX recordings_status ON recordings(status, start_at);
