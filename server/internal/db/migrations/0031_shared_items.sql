-- A title is one item however many libraries hold files for it (a show in
-- the TV library and the DVR's, say). Items that were split by library are
-- merged into the oldest: its episodes gain the others' missing ones, files
-- and the profiles' lists move over, and the rest are deleted.
CREATE TEMP TABLE dup AS
  SELECT m.id AS old_id,
         (SELECT MIN(k.id) FROM media_items k WHERE k.kind = m.kind AND k.parsed_title = m.parsed_title AND k.parsed_year = m.parsed_year) AS new_id
  FROM media_items m
  WHERE m.id <> (SELECT MIN(k.id) FROM media_items k WHERE k.kind = m.kind AND k.parsed_title = m.parsed_title AND k.parsed_year = m.parsed_year);

INSERT OR IGNORE INTO episodes (series_id, season, episode, title, released, rating, imdb_id)
  SELECT d.new_id, e.season, e.episode, e.title, e.released, e.rating, e.imdb_id
  FROM episodes e JOIN dup d ON d.old_id = e.series_id;

UPDATE files SET episode_id = (
    SELECT e2.id FROM episodes e1 JOIN dup d ON d.old_id = e1.series_id
    JOIN episodes e2 ON e2.series_id = d.new_id AND e2.season = e1.season AND e2.episode = e1.episode
    WHERE e1.id = files.episode_id)
  WHERE episode_id IN (SELECT e.id FROM episodes e JOIN dup d ON d.old_id = e.series_id);

UPDATE files SET media_item_id = (SELECT new_id FROM dup WHERE old_id = files.media_item_id)
  WHERE media_item_id IN (SELECT old_id FROM dup);

INSERT OR IGNORE INTO profile_items (profile_id, item_id, added_at)
  SELECT p.profile_id, d.new_id, p.added_at FROM profile_items p JOIN dup d ON d.old_id = p.item_id;
INSERT OR IGNORE INTO home_hidden (profile_id, item_id, hidden_at)
  SELECT h.profile_id, d.new_id, h.hidden_at FROM home_hidden h JOIN dup d ON d.old_id = h.item_id;
INSERT OR IGNORE INTO profile_versions (profile_id, item_id, file_id)
  SELECT v.profile_id, d.new_id, v.file_id FROM profile_versions v JOIN dup d ON d.old_id = v.item_id;

DELETE FROM media_items WHERE id IN (SELECT old_id FROM dup);
DROP TABLE dup;
