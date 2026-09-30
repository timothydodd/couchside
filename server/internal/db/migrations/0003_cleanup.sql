-- Date-named DVR recordings: when it aired ("2006-01-02 15:04"), shown instead of an episode number.
ALTER TABLE episodes ADD COLUMN air_date TEXT NOT NULL DEFAULT '';

-- Why a file can't be played: '' (fine) | 'unreadable' (corrupt/truncated) | 'no-video' (no decodable video, e.g. DRM).
ALTER TABLE files ADD COLUMN problem TEXT NOT NULL DEFAULT '';
UPDATE files SET problem = CASE WHEN duration_sec IS NULL THEN 'unreadable' ELSE 'no-video' END
 WHERE video_codec = '';

-- Failures so far were dead poster links and unplayable files, both now
-- handled without failing. Start clean, retry matching with title variants,
-- and rescan so newly parseable TV files (DVR dates, S2024E04) are picked up.
DELETE FROM jobs WHERE status = 'failed';
INSERT OR IGNORE INTO jobs (kind, ref_id, label)
  SELECT 'match', id, 'Match ' || parsed_title FROM media_items WHERE match_status = 'unmatched';
INSERT OR IGNORE INTO jobs (kind, ref_id, label)
  SELECT 'scan', id, 'Scan ' || name FROM libraries;

-- "Fix match" pins an IMDb id; automatic matches just record one. Nobody has
-- pinned anything before this migration, so all existing ids are automatic.
ALTER TABLE media_items ADD COLUMN imdb_pinned INTEGER NOT NULL DEFAULT 0;

-- The first matcher trusted OMDb's fuzzy title lookup and picked up some
-- featurettes and podcast episodes. Redo matches whose title or year
-- disagrees with the filename; the new matcher scores candidates.
INSERT OR IGNORE INTO jobs (kind, ref_id, label)
  SELECT 'match', id, 'Match ' || parsed_title FROM media_items
   WHERE match_status = 'matched'
     AND (lower(title) <> lower(parsed_title) OR (parsed_year > 0 AND abs(year - parsed_year) > 1));
