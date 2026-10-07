-- What a provider says about one episode: its synopsis, running time, still
-- and TMDB id, and its own cast (guest stars) and crew, for the episode page.
ALTER TABLE episodes ADD COLUMN plot TEXT NOT NULL DEFAULT '';
ALTER TABLE episodes ADD COLUMN runtime_min INTEGER;
ALTER TABLE episodes ADD COLUMN still_url TEXT NOT NULL DEFAULT '';
ALTER TABLE episodes ADD COLUMN tmdb_id INTEGER NOT NULL DEFAULT 0;

CREATE TABLE episode_credits (
  episode_id INTEGER NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
  person_id  INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
  kind       TEXT NOT NULL CHECK (kind IN ('cast', 'crew')),
  role       TEXT NOT NULL DEFAULT '',
  ord        INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (episode_id, person_id, kind, role)
);
CREATE INDEX episode_credits_person ON episode_credits(person_id);
