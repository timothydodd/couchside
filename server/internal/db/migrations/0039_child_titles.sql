-- Child titles: a movie filed under another (a box set's bonus discs, the
-- shorts on a collection). A child is a full movie with its own match and
-- watch state, kept off the Movies grid and listed on its parent's page.
-- parent_pinned: set by hand (File under / Unfile), so scans leave it.
-- group_label: the subfolder a scanned child came from ("Bonus Materials").
-- sort_no: a leading track number in the child's file name ("01 - ...").
ALTER TABLE media_items ADD COLUMN parent_id INTEGER REFERENCES media_items(id) ON DELETE SET NULL;
ALTER TABLE media_items ADD COLUMN parent_pinned INTEGER NOT NULL DEFAULT 0;
ALTER TABLE media_items ADD COLUMN group_label TEXT NOT NULL DEFAULT '';
ALTER TABLE media_items ADD COLUMN sort_no INTEGER NOT NULL DEFAULT 0;
CREATE INDEX media_items_parent ON media_items(parent_id);
