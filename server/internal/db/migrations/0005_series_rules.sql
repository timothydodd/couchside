-- Series recording rules, keyed by the guide's stable SeriesID.
CREATE TABLE series_rules (
  id            INTEGER PRIMARY KEY,
  series_id     TEXT NOT NULL UNIQUE,
  title         TEXT NOT NULL,
  image_url     TEXT NOT NULL DEFAULT '',
  -- new: first airings only; missing: episodes not already in the library or
  -- recorded; all: every airing.
  mode          TEXT NOT NULL DEFAULT 'missing' CHECK (mode IN ('new', 'missing', 'all')),
  channel       TEXT NOT NULL DEFAULT '',  -- '' = any channel
  media_item_id INTEGER REFERENCES media_items(id) ON DELETE SET NULL, -- library show to compare against
  keep_last     INTEGER NOT NULL DEFAULT 0, -- 0 = keep everything
  enabled       INTEGER NOT NULL DEFAULT 1,
  last_run_at   INTEGER,
  last_summary  TEXT NOT NULL DEFAULT '',   -- JSON counts from the last evaluation
  created_at    INTEGER NOT NULL DEFAULT (unixepoch())
);

ALTER TABLE recordings ADD COLUMN rule_id INTEGER REFERENCES series_rules(id) ON DELETE SET NULL;
CREATE INDEX recordings_rule ON recordings(rule_id, status);
CREATE INDEX programs_series ON programs(series_id, start_at);
