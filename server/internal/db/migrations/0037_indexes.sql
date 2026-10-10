-- Indexes for queries that scanned whole tables, which a large library
-- (tens of thousands of titles, 100k episodes) feels on every scan and poll.

-- A recording's part files: RecordingOwnsPath runs once per .partN.ts on
-- every scan, DeleteRecordingsAt on every file deleted from disk.
CREATE INDEX IF NOT EXISTS recordings_path ON recordings(path);

-- EnsureItem looks for a title across libraries for every new file; the
-- per-library UNIQUE starts with library_id, so it couldn't help.
CREATE INDEX IF NOT EXISTS media_items_group ON media_items(kind, parsed_title, parsed_year);

-- Titles sharing an IMDb id (Manage's duplicates, merges).
CREATE INDEX IF NOT EXISTS media_items_imdb ON media_items(imdb_id) WHERE imdb_id <> '';

-- Unmatched counts and the matches to retry after a scan.
CREATE INDEX IF NOT EXISTS media_items_match ON media_items(match_status);

-- A library's titles in order (the Movies and TV grids).
CREATE INDEX IF NOT EXISTS media_items_sort ON media_items(kind, sort_title);

-- The next queued job of a kind: ClaimJob asks kind by kind, in priority
-- order, instead of sorting every queued job.
CREATE INDEX IF NOT EXISTS jobs_queued ON jobs(kind, id) WHERE status = 'queued';
