-- What a finished job did, for Activity: a scan's added, changed and removed
-- counts, and which files it skipped and why.
ALTER TABLE jobs ADD COLUMN result TEXT NOT NULL DEFAULT '';
