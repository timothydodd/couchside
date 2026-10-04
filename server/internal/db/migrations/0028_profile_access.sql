-- What a profile may see. A profile with no rows in profile_libraries sees
-- every library; with rows, only those. max_rating is the highest content
-- rating it's shown ('' = any): G, PG, PG-13 or R, with the TV ratings
-- mapped onto those. Admins are never limited.
CREATE TABLE profile_libraries (
  profile_id INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
  PRIMARY KEY (profile_id, library_id)
);
ALTER TABLE profiles ADD COLUMN max_rating TEXT NOT NULL DEFAULT '';
