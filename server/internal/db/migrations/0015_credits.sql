-- Cast and crew from TMDB. people.id is TMDB's person id, so the same person
-- links up across every title they're in. item_credits is replaced whenever
-- an item is matched; role is the character (cast) or the job (crew).
CREATE TABLE people (
  id           INTEGER PRIMARY KEY,
  name         TEXT NOT NULL,
  profile_path TEXT NOT NULL DEFAULT '',
  updated_at   INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE TABLE item_credits (
  item_id   INTEGER NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
  person_id INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
  kind      TEXT NOT NULL CHECK (kind IN ('cast', 'crew')),
  role      TEXT NOT NULL DEFAULT '',
  ord       INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (item_id, person_id, kind, role)
);
CREATE INDEX item_credits_person ON item_credits(person_id);
