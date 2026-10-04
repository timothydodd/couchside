-- LatestJob (the newest job of a kind for one file or item) is asked for by
-- the player while it shows commercial detection's progress. Without this it
-- scans two weeks of finished jobs each time.
CREATE INDEX jobs_ref ON jobs(kind, ref_id, id);
