-- What a movie file is to its movie: another copy (the default), one part of
-- a movie split across files (played in part_no order as one timeline), or
-- an extra such as a featurette, titled extra_title. role_pinned marks a
-- choice made by hand, which scans no longer overwrite with their guess.
ALTER TABLE files ADD COLUMN role TEXT NOT NULL DEFAULT 'copy' CHECK (role IN ('copy', 'part', 'extra'));
ALTER TABLE files ADD COLUMN part_no INTEGER NOT NULL DEFAULT 0;
ALTER TABLE files ADD COLUMN extra_title TEXT NOT NULL DEFAULT '';
ALTER TABLE files ADD COLUMN role_pinned INTEGER NOT NULL DEFAULT 0;
